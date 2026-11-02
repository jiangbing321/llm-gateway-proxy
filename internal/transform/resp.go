package transform

import (
	"encoding/json"
	"time"
)

// RespA2O 把 Anthropic 非流式响应转为 OpenAI 格式。
func RespA2O(in *AnthropicResponse) *OAIChatResponse {
	out := &OAIChatResponse{
		ID:      in.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   in.Model,
		Usage: &OAIUsage{
			PromptTokens:     in.Usage.InputTokens,
			CompletionTokens: in.Usage.OutputTokens,
			TotalTokens:      in.Usage.InputTokens + in.Usage.OutputTokens,
		},
	}
	var texts []string
	var toolCalls []OAIToolCall
	for _, b := range in.Content {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			toolCalls = append(toolCalls, OAIToolCall{
				ID: b.ID, Type: "function",
				Function: OAIToolCallFunction{Name: b.Name, Arguments: args},
			})
		}
	}
	content := "null"
	if len(texts) > 0 {
		raw, _ := json.Marshal(joinStrings(texts))
		content = string(raw)
	}
	msg := OAIMsgOut{Role: "assistant", Content: json.RawMessage(content)}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	out.Choices = []OAIChatChoice{{
		Index:        0,
		Message:      msg,
		FinishReason: stopReasonAToO(in.StopReason),
	}}
	return out
}

// RespO2A 把 OpenAI 非流式响应转为 Anthropic 格式。
func RespO2A(in *OAIChatResponse) *AnthropicResponse {
	var inTokens, outTokens int
	if in.Usage != nil { // 部分上游非流式响应缺 usage，防 nil panic
		inTokens, outTokens = in.Usage.PromptTokens, in.Usage.CompletionTokens
	}
	out := &AnthropicResponse{
		ID:    in.ID,
		Type:  "message",
		Role:  "assistant",
		Model: in.Model,
		Usage: AnthropicUsage{
			InputTokens:  inTokens,
			OutputTokens: outTokens,
		},
	}
	if len(in.Choices) == 0 {
		out.Content = []AnthropicContentBlock{{Type: "text", Text: ""}}
		out.StopReason = "end_turn"
		return out
	}
	ch := in.Choices[0]
	var reasoning string
	_ = json.Unmarshal(ch.Message.Content, &reasoning)
	if ch.Message.Reasoning != "" {
		reasoning = ch.Message.Reasoning
	}
	if reasoning != "" {
		out.Content = append(out.Content, AnthropicContentBlock{Type: "text", Text: reasoning})
	}
	if len(ch.Message.ToolCalls) > 0 {
		for _, tc := range ch.Message.ToolCalls {
			input := json.RawMessage(tc.Function.Arguments)
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			out.Content = append(out.Content, AnthropicContentBlock{
				Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: input,
			})
		}
	} else if reasoning == "" {
		out.Content = append(out.Content, AnthropicContentBlock{Type: "text", Text: ""})
	}
	out.StopReason = stopReasonOToA(ch.FinishReason)
	return out
}

// stopReasonAToO：Anthropic stop_reason → OpenAI finish_reason。
func stopReasonAToO(s string) string {
	switch s {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// stopReasonOToA：OpenAI finish_reason → Anthropic stop_reason。
func stopReasonOToA(s string) string {
	switch s {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}
