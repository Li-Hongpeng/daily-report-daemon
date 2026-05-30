package agent

import (
	"fmt"
	"strings"
	"time"
)

// TraceStep is one step in the agent's reasoning trace.
type TraceStep struct {
	Step      string    `json:"step"`      // "analyze", "investigate", "synthesize"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ToolTrace records a tool call in the trace log.
type ToolTrace struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Output string `json:"output"`
}

// Trace records the agent's reasoning process for debugging.
type Trace struct {
	Steps     []TraceStep `json:"steps"`
	ToolCalls []ToolTrace `json:"tool_calls"`
	Tokens    int         `json:"tokens_used"`
}

// NewTrace creates a new trace recorder.
func NewTrace() *Trace {
	return &Trace{}
}

// Log records a reasoning step.
func (tr *Trace) Log(step, message string) {
	tr.Steps = append(tr.Steps, TraceStep{
		Step:      step,
		Message:   message,
		Timestamp: time.Now(),
	})
}

// LogToolCall records a tool invocation.
func (tr *Trace) LogToolCall(tool, args, output string) {
	tr.ToolCalls = append(tr.ToolCalls, ToolTrace{
		Tool:   tool,
		Args:   args,
		Output: truncateToolOutput(output, 500),
	})
}

// String renders the trace as a readable log.
func (tr *Trace) String() string {
	var b strings.Builder
	b.WriteString("=== Agent Reasoning Trace ===\n\n")
	for _, s := range tr.Steps {
		b.WriteString(fmt.Sprintf("[%s] %s\n", s.Step, s.Message))
	}
	if len(tr.ToolCalls) > 0 {
		b.WriteString(fmt.Sprintf("\n--- Tool Calls (%d) ---\n", len(tr.ToolCalls)))
		for i, tc := range tr.ToolCalls {
			b.WriteString(fmt.Sprintf("%d. %s(%s)\n   → %s\n", i+1, tc.Tool, tc.Args, tc.Output))
		}
	}
	b.WriteString(fmt.Sprintf("\nTokens used: %d\n", tr.Tokens))
	b.WriteString("=============================\n")
	return b.String()
}

func truncateToolOutput(s string, maxLen int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 5 {
		lines = lines[:5]
		s = strings.Join(lines, "\n") + "\n..."
	}
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

// TokenBudget tracks and limits token consumption.
type TokenBudget struct {
	Max   int `json:"max"`
	used  int
}

// NewTokenBudget creates a budget with a hard limit.
func NewTokenBudget(max int) *TokenBudget {
	return &TokenBudget{Max: max}
}

// Spend deducts tokens. Returns false if budget exceeded.
func (tb *TokenBudget) Spend(n int) bool {
	if tb.used+n > tb.Max {
		return false
	}
	tb.used += n
	return true
}

// Used returns total tokens consumed.
func (tb *TokenBudget) Used() int {
	return tb.used
}

// Remaining returns unused tokens.
func (tb *TokenBudget) Remaining() int {
	return tb.Max - tb.used
}
