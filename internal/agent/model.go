package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/daily-report-daemon/internal/llm"
)

// ChatModelAdapter adapts the project's OpenAI-compatible client to Eino's model interface.
type ChatModelAdapter struct {
	client     *llm.Client
	label      string
	callNumber uint64
}

// NewChatModelAdapter creates an Eino chat model backed by the existing LLM client.
func NewChatModelAdapter(client *llm.Client, label string) *ChatModelAdapter {
	return &ChatModelAdapter{client: client, label: label}
}

func (m *ChatModelAdapter) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m == nil || m.client == nil {
		return schema.AssistantMessage("", nil), nil
	}
	options := model.GetCommonOptions(nil, opts...)
	req := llm.ChatRequest{
		Model:       m.client.Model,
		Messages:    toLLMMessages(input),
		Temperature: 0.2,
		MaxTokens:   4096,
	}
	if options.Model != nil {
		req.Model = *options.Model
	}
	if options.Temperature != nil {
		req.Temperature = float64(*options.Temperature)
	}
	if options.MaxTokens != nil {
		req.MaxTokens = *options.MaxTokens
	}
	if len(options.Tools) > 0 {
		req.Tools = toLLMTools(options.Tools)
		req.ToolChoice = "auto"
	}
	if options.ToolChoice != nil {
		switch *options.ToolChoice {
		case schema.ToolChoiceForbidden:
			req.ToolChoice = "none"
		case schema.ToolChoiceForced:
			req.ToolChoice = "required"
		default:
			req.ToolChoice = "auto"
		}
	}

	label := m.nextLabel()
	resp, err := m.client.Complete(ctx, req, label)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("model returned no choices")
	}
	msg := resp.Choices[0].Message
	return schema.AssistantMessage(msg.Content, toSchemaToolCalls(msg.ToolCalls)), nil
}

func (m *ChatModelAdapter) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *ChatModelAdapter) nextLabel() string {
	base := m.label
	if base == "" {
		base = "eino-agent"
	}
	n := atomic.AddUint64(&m.callNumber, 1)
	if n == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, n)
}

func toLLMMessages(input []*schema.Message) []llm.Message {
	out := make([]llm.Message, 0, len(input))
	for _, msg := range input {
		if msg == nil {
			continue
		}
		out = append(out, llm.Message{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCalls:  toLLMToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		})
	}
	return out
}

func toLLMToolCalls(calls []schema.ToolCall) []llm.ToolCall {
	out := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		out = append(out, llm.ToolCall{
			ID:   call.ID,
			Type: callType,
			Function: llm.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func toSchemaToolCalls(calls []llm.ToolCall) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(calls))
	for i, call := range calls {
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		idx := i
		out = append(out, schema.ToolCall{
			Index: &idx,
			ID:    call.ID,
			Type:  callType,
			Function: schema.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func toLLMTools(tools []*schema.ToolInfo) []llm.ToolDef {
	out := make([]llm.ToolDef, 0, len(tools))
	for _, toolInfo := range tools {
		if toolInfo == nil {
			continue
		}
		out = append(out, llm.ToolDef{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        toolInfo.Name,
				Description: toolInfo.Desc,
				Parameters:  toolParameters(toolInfo),
			},
		})
	}
	return out
}

func toolParameters(toolInfo *schema.ToolInfo) any {
	if toolInfo == nil || toolInfo.ParamsOneOf == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	js, err := toolInfo.ParamsOneOf.ToJSONSchema()
	if err != nil || js == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	data, err := json.Marshal(js)
	if err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return v
}
