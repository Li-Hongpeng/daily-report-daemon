package agent

import (
	"context"
	"fmt"

	"github.com/daily-report-daemon/internal/llm"
)

// MaxIterations limits total plan-investigate-reflect cycles.
const MaxIterations = 10 // Phase 3 M5: increased from 5 to 10 for free agent loop

// MaxToolCalls limits tool invocations per session.
const MaxToolCalls = 5

// Agent orchestrates the 3-step reasoning pipeline.
type Agent struct {
	Tools     *ToolRegistry
	Trace     *Trace
	Budget    *TokenBudget
	RepoRoot  string
	LLM       *llm.Client // DeepSeek client for tool calling
}

// NewAgent creates an agent for the given repo root.
func NewAgent(repoRoot string, llmClient *llm.Client) *Agent {
	return &Agent{
		Tools:    NewToolRegistry(repoRoot),
		Trace:    NewTrace(),
		Budget:   NewTokenBudget(50000),
		RepoRoot: repoRoot,
		LLM:      llmClient,
	}
}

// EngineResult holds the output of the agent engine.
type EngineResult struct {
	ReportJSON    string   `json:"report_json"`
	Confidence    string   `json:"confidence"` // high/medium/low
	ToolCallsUsed int      `json:"tool_calls_used"`
	TokensUsed    int      `json:"tokens_used"`
	Iterations    int      `json:"iterations"`
	FellBack      bool     `json:"fell_back"`
	Errors        []string `json:"errors,omitempty"`
}

// Run executes the full agent pipeline on evidence.
func (a *Agent) Run(ctx context.Context, evidenceJSON string) (*EngineResult, error) {
	result := &EngineResult{}

	// Step 1: Analyze
	a.Trace.Log("analyze", "开始分析 evidence...")
	if !a.Budget.Spend(2000) {
		a.Trace.Log("analyze", "Token 预算已耗尽，降级")
		return a.fallback(evidenceJSON), nil
	}
	plan, err := a.analyze(ctx, evidenceJSON)
	if err != nil {
		a.Trace.Log("analyze", fmt.Sprintf("分析失败: %v — 降级到 Phase 0 单次生成", err))
		return a.fallback(evidenceJSON), nil
	}
	a.Trace.Log("analyze", plan)

	// Step 2: Investigate (with tool calls)
	a.Trace.Log("investigate", "开始追问调查...")
	toolCount := 0
	for i := 0; i < MaxIterations && toolCount < MaxToolCalls; i++ {
		result.Iterations = i + 1

		calls := a.parseToolCalls(plan)
		if len(calls) == 0 {
			a.Trace.Log("investigate", "无更多工具调用，停止追问")
			break
		}

		for _, call := range calls {
			if toolCount >= MaxToolCalls {
				a.Trace.Log("investigate", fmt.Sprintf("已达最大工具调用次数 (%d)", MaxToolCalls))
				break
			}
			toolResult := a.Tools.Execute(call)
			a.Trace.LogToolCall(call.Tool, call.Args, toolResult.Output)
			toolCount++
		}

		// Revise plan based on findings
		if toolCount >= MaxToolCalls || i >= MaxIterations-1 {
			break
		}
		newPlan, err := a.revise(ctx, evidenceJSON)
		if err != nil {
			break
		}
		plan = newPlan
	}

	result.ToolCallsUsed = toolCount

	// Step 3: Synthesize
	a.Trace.Log("synthesize", "开始综合生成报告...")
	a.Budget.Spend(8000) // ~2000 tokens for synthesize prompt + output
	report, err := a.synthesize(ctx, evidenceJSON)
	if err != nil {
		a.Trace.Log("synthesize", fmt.Sprintf("综合失败: %v — 降级", err))
		return a.fallback(evidenceJSON), nil
	}

	result.ReportJSON = report
	result.Confidence = a.assessConfidence(toolCount)
	result.TokensUsed = a.Budget.Used()

	return result, nil
}

func (a *Agent) analyze(ctx context.Context, evidence string) (string, error) {
	if a.LLM == nil {
		return "分析完成：识别到多个文件变更", nil
	}

	sysPrompt := `你是一个开发活动分析 agent。请分析今日的 evidence，识别工作主题（功能开发/bugfix/重构/文档），
按模块聚类变更，标记需要追问的信息缺口。不要生成报告内容，只做分析规划。`
	userPrompt := fmt.Sprintf("今日 evidence:\n%s\n\n请分析并列出：1) 工作主题 2) 需要追问的 gap 3) 建议调用的工具", evidence)

	resp, err := a.LLM.Chat(sysPrompt, userPrompt, "agent-analyze")
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *Agent) synthesize(ctx context.Context, evidence string) (string, error) {
	if a.LLM == nil {
		return `{"date":"2026-05-29","summary":["test"],"completed":[],"changes":[],"risks":[],"blockers":[],"next_steps":[]}`, nil
	}

	sysPrompt := `你是一个技术报告生成器。基于 evidence 和 agent 追问结果，生成结构化日报 JSON。
每条结论引用 evidence_id，不确定内容标记 inferred:true。输出语言：zh-CN。`
	traceSummary := a.Trace.String()
	userPrompt := fmt.Sprintf("Evidence:\n%s\n\nAgent 推理过程:\n%s\n\n生成日报 JSON。", evidence, traceSummary)

	resp, err := a.LLM.Chat(sysPrompt, userPrompt, "agent-synthesize")
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *Agent) revise(ctx context.Context, evidence string) (string, error) {
	if a.LLM == nil {
		return "修订计划：继续调查", nil
	}

	sysPrompt := "基于已收集的信息，判断是否需要继续追问。如需要，列出下一步工具调用。"
	userPrompt := fmt.Sprintf("Evidence:\n%s\n\n已收集信息:\n%s", evidence, a.Trace.String())

	resp, err := a.LLM.Chat(sysPrompt, userPrompt, "agent-revise")
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (a *Agent) parseToolCalls(plan string) []ToolCall {
	// Simplified: look for tool references in plan text
	var calls []ToolCall
	for _, t := range a.Tools.AvailableTools() {
		if contains(plan, t.Name) {
			calls = append(calls, ToolCall{Tool: t.Name, Args: "."})
		}
	}
	return calls
}

func (a *Agent) fallback(evidence string) *EngineResult {
	return &EngineResult{
		ReportJSON:    evidence,
		Confidence:    "low",
		ToolCallsUsed: 0,
		TokensUsed:    0,
		Iterations:    0,
		FellBack:      true,
	}
}

func (a *Agent) assessConfidence(toolCalls int) string {
	if toolCalls >= 2 {
		return "high"
	}
	if toolCalls >= 1 {
		return "medium"
	}
	return "low"
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
