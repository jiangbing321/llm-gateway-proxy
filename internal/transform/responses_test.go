package transform

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReqR2OBasic 验证 Responses→Chat：instructions、字符串 input。
func TestReqR2OBasic(t *testing.T) {
	in := &ResponsesRequest{
		Model: "gpt-x", Instructions: "你是助手",
		Input: json.RawMessage(`"你好"`), MaxOutputTokens: 100,
	}
	out, err := ReqR2O(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.MaxTokens != 100 {
		t.Fatalf("max_tokens 错误: %d", out.MaxTokens)
	}
	if len(out.Messages) != 2 || out.Messages[0].Role != "system" ||
		OAIContentToText(out.Messages[0].Content) != "你是助手" ||
		out.Messages[1].Role != "user" || OAIContentToText(out.Messages[1].Content) != "你好" {
		t.Fatalf("messages 错误: %+v", out.Messages)
	}
}

// TestReqR2OItems 验证 items 数组：message/function_call/function_call_output 往返。
func TestReqR2OItems(t *testing.T) {
	in := &ResponsesRequest{
		Model: "gpt-x",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"天气"}]},
			{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"BJ\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"晴"}
		]`),
		Tools: []ResponsesTool{{Type: "function", Name: "get_weather",
			Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	out, err := ReqR2O(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools 错误: %+v", out.Tools)
	}
	// user → assistant(tool_calls) → tool
	var roles []string
	for _, m := range out.Messages {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool" {
		t.Fatalf("roles 错误: %v", roles)
	}
	if len(out.Messages[1].ToolCalls) != 1 || out.Messages[1].ToolCalls[0].ID != "call_1" ||
		out.Messages[1].ToolCalls[0].Function.Arguments != `{"city":"BJ"}` {
		t.Fatalf("tool_calls 错误: %+v", out.Messages[1].ToolCalls)
	}
	if out.Messages[2].ToolCallID != "call_1" || OAIContentToText(out.Messages[2].Content) != "晴" {
		t.Fatalf("tool 消息错误: %+v", out.Messages[2])
	}
}

// TestReqR2ORejectsStateful 验证 previous_response_id 明确拒绝。
func TestReqR2ORejectsStateful(t *testing.T) {
	in := &ResponsesRequest{Model: "m", Input: json.RawMessage(`"x"`),
		PreviousResponseID: "resp_123"}
	if _, err := ReqR2O(in); err == nil {
		t.Fatal("previous_response_id 应报错")
	}
}

// TestRespO2R 验证 Chat→Responses 非流式。
func TestRespO2R(t *testing.T) {
	in := &OAIChatResponse{
		ID: "chatcmpl-1", Model: "gpt-x",
		Choices: []OAIChatChoice{{
			FinishReason: "tool_calls",
			Message: OAIMsgOut{
				Role:      "assistant",
				Content:   json.RawMessage(`"我来查"`),
				ToolCalls: []OAIToolCall{{ID: "tc1", Type: "function",
					Function: OAIToolCallFunction{Name: "f1", Arguments: `{"a":1}`}}},
			},
		}},
		Usage: &OAIUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}
	out := RespO2R(in)
	if out.Status != "completed" || len(out.Output) != 2 {
		t.Fatalf("输出结构错误: %+v", out)
	}
	if out.Output[0].Type != "message" || len(out.Output[0].Content) != 1 ||
		out.Output[0].Content[0].Text != "我来查" {
		t.Fatalf("message item 错误: %+v", out.Output[0])
	}
	if out.Output[1].Type != "function_call" || out.Output[1].CallID != "tc1" ||
		out.Output[1].Arguments != `{"a":1}` {
		t.Fatalf("function_call item 错误: %+v", out.Output[1])
	}
	if out.Usage.InputTokens != 5 || out.Usage.TotalTokens != 8 {
		t.Fatalf("usage 错误: %+v", out.Usage)
	}
}

// TestO2RStream 验证 chat chunk→Responses 事件流：文本 + 工具调用 + 收尾。
func TestO2RStream(t *testing.T) {
	c := NewO2RStreamConverter()
	var frames []string
	send := func(s string) { frames = append(frames, c.Convert(s)...) }

	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":"我来"}}]}`)
	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"content":"查"}}]}`)
	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"tc1","type":"function","function":{"name":"f1","arguments":""}}]}}]}`)
	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":"}}]}}]}`)
	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`)
	send(`{"id":"c1","model":"gpt-x","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`)
	send(`[DONE]`)

	all := strings.Join(frames, "")
	// 事件序列断言
	order := []string{"response.created", "response.output_item.added",
		"response.output_text.delta", "response.output_item.done",
		"response.output_item.added", "function_call", "response.function_call_arguments.delta",
		"response.output_item.done", "response.completed"}
	pos := 0
	for _, key := range order {
		i := strings.Index(all[pos:], key)
		if i < 0 {
			t.Fatalf("缺少事件 %s（after %d）:\n%s", key, pos, all)
		}
		pos += i + len(key)
	}
	if !strings.Contains(all, `"delta":"我来"`) || !strings.Contains(all, `"delta":"查"`) {
		t.Fatalf("文本 delta 错误:\n%s", all)
	}
	if !strings.Contains(all, `"call_id":"tc1"`) || !strings.Contains(all, `"arguments":"{\"a\":1}"`) {
		t.Fatalf("function_call 错误:\n%s", all)
	}
	if !strings.Contains(all, `"status":"completed"`) {
		t.Fatalf("completed 缺失:\n%s", all)
	}
	if !strings.Contains(all, `"input_tokens":9`) || !strings.Contains(all, `"output_tokens":4`) {
		t.Fatalf("usage 错误:\n%s", all)
	}
	if p, c2 := c.Usage(); p != 9 || c2 != 4 {
		t.Fatalf("Usage() 错误: %d %d", p, c2)
	}
	// 幂等
	if extra := c.Flush(); len(extra) != 0 {
		t.Fatalf("Flush 应幂等: %v", extra)
	}
}
