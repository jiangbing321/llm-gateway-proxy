package transform

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReqO2GBasic 验证 OpenAI→Gemini 请求转换：system 抽取、role 映射、generationConfig。
func TestReqO2GBasic(t *testing.T) {
	in := &OAIChatRequest{
		Model:     "gemini-2.0-flash",
		MaxTokens: 100,
		Messages: []OAIMessage{
			{Role: "system", Content: json.RawMessage(`"你是助手"`)},
			{Role: "user", Content: json.RawMessage(`"你好"`)},
			{Role: "assistant", Content: json.RawMessage(`"你好呀"`)},
			{Role: "user", Content: json.RawMessage(`"再见"`)},
		},
	}
	out, err := ReqO2G(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.SystemInstruction == nil || len(out.SystemInstruction.Parts) != 1 ||
		out.SystemInstruction.Parts[0].Text != "你是助手" {
		t.Fatalf("systemInstruction 错误: %+v", out.SystemInstruction)
	}
	if len(out.Contents) != 3 {
		t.Fatalf("contents 数量错误: %+v", out.Contents)
	}
	roles := []string{out.Contents[0].Role, out.Contents[1].Role, out.Contents[2].Role}
	if strings.Join(roles, ",") != "user,model,user" {
		t.Fatalf("role 映射错误: %v", roles)
	}
	if out.GenerationConfig == nil || out.GenerationConfig.MaxOutputTokens != 100 {
		t.Fatalf("generationConfig 错误: %+v", out.GenerationConfig)
	}
	if out.Contents[2].Parts[0].Text != "再见" {
		t.Fatalf("内容错误: %+v", out.Contents[2])
	}
}

// TestReqO2GToolRoundTrip 验证工具定义/调用/结果转换。
func TestReqO2GToolRoundTrip(t *testing.T) {
	in := &OAIChatRequest{
		Model: "gemini-x",
		Tools: []OAITool{{Type: "function", Function: OAIToolFunction{
			Name: "get_weather", Description: "查天气",
			Parameters: json.RawMessage(`{"type":"object"}`),
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
	out, err := ReqO2G(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 1 || len(out.Tools[0].FunctionDeclarations) != 1 ||
		out.Tools[0].FunctionDeclarations[0].Name != "get_weather" {
		t.Fatalf("tools 转换错误: %+v", out.Tools)
	}
	// user / model(functionCall) / user(functionResponse)
	if len(out.Contents) != 3 {
		t.Fatalf("contents 数量错误: %+v", out.Contents)
	}
	modelPart := out.Contents[1].Parts[0]
	if modelPart.FunctionCall == nil || modelPart.FunctionCall.Name != "get_weather" ||
		string(modelPart.FunctionCall.Args) != `{"city":"北京"}` {
		t.Fatalf("functionCall 错误: %+v", modelPart)
	}
	respPart := out.Contents[2].Parts[0]
	if respPart.FunctionResponse == nil || respPart.FunctionResponse.Name != "get_weather" {
		t.Fatalf("functionResponse 错误: %+v", respPart)
	}
	// tool 消息 content 纯文本 → {result: "..."} 包装
	var probe map[string]any
	if json.Unmarshal(respPart.FunctionResponse.Response, &probe) != nil || probe["result"] != "晴" {
		t.Fatalf("functionResponse.response 错误: %s", string(respPart.FunctionResponse.Response))
	}
}

// TestRespG2O 验证 Gemini 响应→OpenAI。
func TestRespG2O(t *testing.T) {
	raw := `{"candidates":[{"content":{"parts":[{"text":"我来查"},{"functionCall":{"name":"f1","args":{"a":1}}},{"text":"完成"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
	var gr GeminiResponse
	if err := json.Unmarshal([]byte(raw), &gr); err != nil {
		t.Fatal(err)
	}
	oc := RespG2O(&gr)
	if OAIContentToText(oc.Choices[0].Message.Content) != "我来查\n完成" {
		t.Fatalf("文本合并错误: %s", OAIContentToText(oc.Choices[0].Message.Content))
	}
	if len(oc.Choices[0].Message.ToolCalls) != 1 ||
		oc.Choices[0].Message.ToolCalls[0].Function.Arguments != `{"a":1}` {
		t.Fatalf("tool_calls 错误: %+v", oc.Choices[0].Message.ToolCalls)
	}
	if oc.Usage.PromptTokens != 10 || oc.Usage.TotalTokens != 15 {
		t.Fatalf("usage 错误: %+v", oc.Usage)
	}
}

// TestG2OStream 验证 Gemini SSE→OpenAI chunk 流。
func TestG2OStream(t *testing.T) {
	c := NewG2OStreamConverter()
	var frames []string
	send := func(s string) { frames = append(frames, c.Convert(s)...) }

	send(`{"candidates":[{"content":{"parts":[{"text":"你"}]}}]}`)
	send(`{"candidates":[{"content":{"parts":[{"text":"好"}]}}]}`)
	send(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"f1","args":{"x":1}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":6}}`)

	all := strings.Join(frames, "")
	if !strings.Contains(all, `"role":"assistant"`) {
		t.Fatalf("缺首 chunk:\n%s", all)
	}
	if !strings.Contains(all, `"content":"你"`) || !strings.Contains(all, `"content":"好"`) {
		t.Fatalf("缺文本 delta:\n%s", all)
	}
	if !strings.Contains(all, `"name":"f1"`) || !strings.Contains(all, `"arguments":"{\"x\":1}"`) {
		t.Fatalf("缺 tool_calls:\n%s", all)
	}
	// 收尾
	tail := c.Flush()
	all += strings.Join(tail, "")
	if !strings.Contains(all, `"finish_reason":"tool_calls"`) || !strings.Contains(all, `[DONE]`) {
		t.Fatalf("缺终帧:\n%s", all)
	}
	if !strings.Contains(all, `"prompt_tokens":8`) || !strings.Contains(all, `"completion_tokens":6`) {
		t.Fatalf("usage 错误:\n%s", all)
	}
	// 幂等
	if extra := c.Flush(); len(extra) != 0 {
		t.Fatalf("Flush 应幂等: %v", extra)
	}
}

// TestO2GStream 验证 OpenAI chunk→Gemini SSE。
func TestO2GStream(t *testing.T) {
	c := NewO2GStreamConverter()
	var frames []string
	send := func(s string) { frames = append(frames, c.Convert(s)...) }

	send(`{"id":"c1","choices":[{"index":0,"delta":{"content":"Hi"}}]}`)
	send(`{"id":"c1","choices":[{"index":0,"delta":{"content":"!"}}]}`)
	send(`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)
	send(`[DONE]`)

	all := strings.Join(frames, "")
	if !strings.Contains(all, `"text":"Hi"`) || !strings.Contains(all, `"text":"!"`) {
		t.Fatalf("缺文本帧:\n%s", all)
	}
	if !strings.Contains(all, `"finishReason":"STOP"`) {
		t.Fatalf("缺收尾帧:\n%s", all)
	}
	if !strings.Contains(all, `"promptTokenCount":5`) || !strings.Contains(all, `"candidatesTokenCount":2`) {
		t.Fatalf("usageMetadata 错误:\n%s", all)
	}
}
