package transform

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReqO2ABasic 验证 OpenAI→Anthropic 请求转换：system 抽取、max_tokens 必填。
func TestReqO2ABasic(t *testing.T) {
	in := &OAIChatRequest{
		Model: "deepseek-chat",
		Messages: []OAIMessage{
			{Role: "system", Content: json.RawMessage(`"你是助手"`)},
			{Role: "user", Content: json.RawMessage(`"你好"`)},
		},
	}
	out, err := ReqO2A(in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "deepseek-chat" || out.MaxTokens != 8192 {
		t.Fatalf("model/max_tokens 错误: %+v", out)
	}
	if sys := AnthropicContentToText(out.System); sys != "你是助手" {
		t.Fatalf("system 错误: %q", sys)
	}
	if len(out.Messages) != 1 || out.Messages[0].Role != "user" {
		t.Fatalf("messages 错误: %+v", out.Messages)
	}
	if txt := AnthropicContentToText(out.Messages[0].Content); txt != "你好" {
		t.Fatalf("user content 错误: %q", txt)
	}
}

// TestReqO2AToolRoundTrip 验证工具定义/调用/结果的消息级往返。
func TestReqO2AToolRoundTrip(t *testing.T) {
	oai := &OAIChatRequest{
		Model: "m1",
		Tools: []OAITool{{Type: "function", Function: OAIToolFunction{
			Name: "get_weather", Description: "查天气",
			Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}}},
		Messages: []OAIMessage{
			{Role: "user", Content: json.RawMessage(`"北京天气"`)},

			{Role: "assistant", ToolCalls: []OAIToolCall{{
				ID: "call_1", Type: "function",
				Function: OAIToolCallFunction{Name: "get_weather", Arguments: `{"city":"北京"}`},
			}}},
			{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"晴"`)},
		},
	}
	a, err := ReqO2A(oai, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Tools) != 1 || a.Tools[0].Name != "get_weather" || string(a.Tools[0].InputSchema) == "" {
		t.Fatalf("tools 转换错误: %+v", a.Tools)
	}
	// user / assistant(tool_use) / user(tool_result)
	if len(a.Messages) != 3 {
		t.Fatalf("messages 数量错误: %d %+v", len(a.Messages), a.Messages)
	}
	var asstBlocks []AnthropicContentBlock
	_ = json.Unmarshal(a.Messages[1].Content, &asstBlocks)
	if len(asstBlocks) != 1 || asstBlocks[0].Type != "tool_use" || asstBlocks[0].ID != "call_1" ||
		string(asstBlocks[0].Input) != `{"city":"北京"}` {
		t.Fatalf("tool_use 块错误: %+v", asstBlocks)
	}
	var resultBlocks []AnthropicContentBlock
	_ = json.Unmarshal(a.Messages[2].Content, &resultBlocks)
	if len(resultBlocks) != 1 || resultBlocks[0].Type != "tool_result" || resultBlocks[0].ToolUseID != "call_1" {
		t.Fatalf("tool_result 块错误: %+v", resultBlocks)
	}

	// 反向转回 OpenAI
	back, err := ReqA2O(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Tools) != 1 || back.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools 反向错误: %+v", back.Tools)
	}
	// system 无 → user / assistant(tool_calls) / tool
	var roles []string
	for _, m := range back.Messages {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool" {
		t.Fatalf("roles 错误: %v", roles)
	}
	if len(back.Messages[1].ToolCalls) != 1 || back.Messages[1].ToolCalls[0].ID != "call_1" ||
		back.Messages[1].ToolCalls[0].Function.Arguments != `{"city":"北京"}` {
		t.Fatalf("反向 tool_calls 错误: %+v", back.Messages[1].ToolCalls)
	}
	if back.Messages[2].ToolCallID != "call_1" || OAIContentToText(back.Messages[2].Content) != "晴" {
		t.Fatalf("反向 tool 消息错误: %+v", back.Messages[2])
	}
}

// TestRespBothWays 验证非流式响应互转。
func TestRespBothWays(t *testing.T) {
	// A→O
	ar := &AnthropicResponse{
		ID: "msg_1", Model: "claude-x", StopReason: "tool_use",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "我来查"},
			{Type: "tool_use", ID: "t1", Name: "get_weather", Input: json.RawMessage(`{"city":"BJ"}`)},
		},
		Usage: AnthropicUsage{InputTokens: 10, OutputTokens: 5},
	}
	oc := RespA2O(ar)
	if oc.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish 映射错误: %s", oc.Choices[0].FinishReason)
	}
	if OAIContentToText(oc.Choices[0].Message.Content) != "我来查" ||
		len(oc.Choices[0].Message.ToolCalls) != 1 || oc.Choices[0].Message.ToolCalls[0].ID != "t1" {
		t.Fatalf("A→O 内容错误: %+v", oc.Choices[0])
	}
	if oc.Usage.PromptTokens != 10 || oc.Usage.TotalTokens != 15 {
		t.Fatalf("A→O usage 错误: %+v", oc.Usage)
	}

	// O→A
	or := &OAIChatResponse{
		ID: "chatcmpl-1", Model: "gpt-x",
		Choices: []OAIChatChoice{{Index: 0, FinishReason: "length",
			Message: OAIMsgOut{Role: "assistant", Content: json.RawMessage(`"答案"`)}}},
		Usage: &OAIUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10},
	}
	ac := RespO2A(or)
	if ac.StopReason != "max_tokens" || len(ac.Content) != 1 || ac.Content[0].Text != "答案" {
		t.Fatalf("O→A 错误: %+v", ac)
	}
	if ac.Usage.InputTokens != 7 || ac.Usage.OutputTokens != 3 {
		t.Fatalf("O→A usage 错误: %+v", ac.Usage)
	}
}

// TestRespO2ANilUsage 部分上游非流式响应缺 usage 字段，转换不应 panic。
func TestRespO2ANilUsage(t *testing.T) {
	or := &OAIChatResponse{
		ID: "chatcmpl-2", Model: "gpt-x",
		Choices: []OAIChatChoice{{Index: 0, FinishReason: "stop",
			Message: OAIMsgOut{Role: "assistant", Content: json.RawMessage(`"ok"`)}}},
		// Usage: nil
	}
	ac := RespO2A(or)
	if ac.Usage.InputTokens != 0 || ac.Usage.OutputTokens != 0 {
		t.Fatalf("缺 usage 应得零值: %+v", ac.Usage)
	}
}

// TestO2AStream 验证 OpenAI→Anthropic 流转换：事件顺序与闭合。
func TestO2AStream(t *testing.T) {
	c := NewO2AStreamConverter()
	var frames []string
	send := func(s string) { frames = append(frames, c.Convert(s)...) }

	send(`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"你"}}]}`)
	send(`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"好"}}]}`)
	send(`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t1","type":"function","function":{"name":"f1","arguments":""}}]}}]}`)
	send(`{"id":"c1","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":1}"}}]}}]}`)
	send(`{"id":"c1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`)
	send(`[DONE]`)

	all := strings.Join(frames, "")
	// 顺序断言
	order := []string{"message_start", "content_block_start", "text_delta", "content_block_stop",
		"content_block_start", "tool_use", "input_json_delta", "message_delta", "message_stop"}
	pos := 0
	for _, key := range order {
		i := strings.Index(all[pos:], key)
		if i < 0 {
			t.Fatalf("缺少事件 %s（after %d）:\n%s", key, pos, all)
		}
		pos += i + len(key)
	}
	if !strings.Contains(all, `"stop_reason":"tool_use"`) {
		t.Fatalf("stop_reason 错误:\n%s", all)
	}
	if !strings.Contains(all, `"input_tokens":9`) || !strings.Contains(all, `"output_tokens":4`) {
		t.Fatalf("usage 错误:\n%s", all)
	}
	if p, c2 := c.Usage(); p != 9 || c2 != 4 {
		t.Fatalf("Usage() 错误: %d %d", p, c2)
	}
	// 幂等
	if extra := c.Flush(); len(extra) != 0 {
		t.Fatalf("Flush 应幂等，返回 %v", extra)
	}
}

// TestA2OStream 验证 Anthropic→OpenAI 流转换。
func TestA2OStream(t *testing.T) {
	c := NewA2OStreamConverter()
	var frames []string
	send := func(evt, data string) { frames = append(frames, c.Convert(data)...) }

	send("message_start", `{"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":8}}}`)
	send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`)
	send("content_block_stop", `{"type":"content_block_stop","index":0}`)
	send("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"f1","input":{}}}`)
	send("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":"}}`)
	send("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"1}"}}`)
	send("content_block_stop", `{"type":"content_block_stop","index":1}`)
	send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":6}}`)
	send("message_stop", `{"type":"message_stop"}`)

	all := strings.Join(frames, "")
	if !strings.Contains(all, `"role":"assistant"`) {
		t.Fatalf("缺首 chunk:\n%s", all)
	}
	if !strings.Contains(all, `"content":"Hi"`) {
		t.Fatalf("缺文本 delta:\n%s", all)
	}
	if !strings.Contains(all, `"id":"tu1"`) || !strings.Contains(all, `"name":"f1"`) {
		t.Fatalf("缺 tool_calls 首帧:\n%s", all)
	}
	if !strings.Contains(all, `"arguments":"{\"x\":"`) || !strings.Contains(all, `"arguments":"1}"`) {
		t.Fatalf("缺 arguments 增量:\n%s", all)
	}
	if !strings.Contains(all, `"finish_reason":"tool_calls"`) || !strings.Contains(all, `[DONE]`) {
		t.Fatalf("缺终帧:\n%s", all)
	}
	if !strings.Contains(all, `"prompt_tokens":8`) || !strings.Contains(all, `"completion_tokens":6`) {
		t.Fatalf("usage 错误:\n%s", all)
	}
}

// TestEventReader 验证 SSE 分帧（跨行/注释/event 行）。
func TestEventReader(t *testing.T) {
	src := `: keep-alive

event: message_start
data: {"type":"message_start"}

data: {"id":"c1"}

data: [DONE]

`
	r := NewEventReader(strings.NewReader(src))
	var got []string
	for {
		ev, err := r.Next()
		if err != nil {
			break
		}
		got = append(got, ev.Event+"|"+ev.Data)
	}
	want := []string{
		"message_start|{\"type\":\"message_start\"}",
		"|{\"id\":\"c1\"}",
		"|[DONE]",
	}
	if len(got) != len(want) {
		t.Fatalf("帧数错误: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("帧 %d 错误: got %s want %s", i, got[i], want[i])
		}
	}
}

// TestReqA2OSystemInMessages 验证 Anthropic→OpenAI 转换对 messages 内
// role=system/developer 的容错：按原位置保序转为 OpenAI system 消息
// （复现 Claude Code 将 agent 系统提示注入 messages 的场景，见 issue：
// "不支持的消息 role: system"）。
func TestReqA2OSystemInMessages(t *testing.T) {
	in := &AnthropicRequest{
		Model:     "deepseek-v4-1-flash-260910",
		MaxTokens: 1024,
		System:    json.RawMessage(`[{"type":"text","text":"顶层系统提示"}]`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"你好"`)},
			{Role: "system", Content: json.RawMessage(`"Available agent types for the Agent tool:\n- bug-analyzer: ..."`)},
			{Role: "developer", Content: json.RawMessage(`"额外指令"`)},
		},
	}
	out, err := ReqA2O(in)
	if err != nil {
		t.Fatalf("转换不应报错: %v", err)
	}
	// 期望顺序：顶层 system → user → messages 内 system → messages 内 developer
	// （developer 统一映射为 OpenAI system，以兼容不识别 developer 的端点）
	wantRoles := []string{"system", "user", "system", "system"}
	if len(out.Messages) != len(wantRoles) {
		t.Fatalf("消息数错误: got %d want %d (%+v)", len(out.Messages), len(wantRoles), out.Messages)
	}
	for i, want := range wantRoles {
		if out.Messages[i].Role != want {
			t.Fatalf("消息 %d role 错误: got %s want %s", i, out.Messages[i].Role, want)
		}
	}
	if txt := OAIContentToText(out.Messages[0].Content); txt != "顶层系统提示" {
		t.Fatalf("顶层 system 内容错误: %q", txt)
	}
	if txt := OAIContentToText(out.Messages[2].Content); !strings.Contains(txt, "bug-analyzer") {
		t.Fatalf("messages 内 system 内容错误: %q", txt)
	}
}
