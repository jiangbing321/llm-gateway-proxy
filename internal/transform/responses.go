package transform

import (
	"encoding/json"
	"errors"
	"fmt"

	"llm-gateway-proxy/internal/i18n"
)

// ---- OpenAI Responses API 结构（仅作为客户端协议的解析/输出） ----

// ResponsesRequest 是 /v1/responses 请求。
type ResponsesRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`      // 字符串 或 items 数组
	Instructions    string          `json:"instructions,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Tools           []ResponsesTool `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"`
	// v1 不支持的有状态/内置工具特性（明确拒绝）
	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

// ResponsesTool 是工具定义（扁平格式 {type:"function",name,...}）。
type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ResponsesResponse 是 /v1/responses 非流式响应。
type ResponsesResponse struct {
	ID     string               `json:"id"`
	Object string               `json:"object"` // response
	Status string               `json:"status"` // completed
	Model  string               `json:"model"`
	Output []ResponsesOutputItem `json:"output"`
	Usage  *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// ResponsesOutputItem 是 output 数组项（message / function_call）。
type ResponsesOutputItem struct {
	Type    string `json:"type"` // message | function_call
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"` // assistant
	Status  string `json:"status,omitempty"`
	Content []struct {
		Type string `json:"type"` // output_text
		Text string `json:"text"`
	} `json:"content,omitempty"`
	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ReqR2O 把 Responses 请求转为 OpenAI Chat 请求。
func ReqR2O(in *ResponsesRequest) (*OAIChatRequest, error) {
	if in.PreviousResponseID != "" {
		return nil, errors.New(i18n.T("transform.prevRespUnsupported"))
	}
	out := &OAIChatRequest{
		Model:       in.Model,
		MaxTokens:   in.MaxOutputTokens,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stream:      in.Stream,
	}
	// instructions → 首条 system
	if in.Instructions != "" {
		raw, _ := json.Marshal(in.Instructions)
		out.Messages = append(out.Messages, OAIMessage{Role: "system", Content: raw})
	}
	// tools（扁平 → chat function 格式）
	for _, t := range in.Tools {
		if t.Type != "function" {
			return nil, errors.New(i18n.T("transform.toolTypeUnsupported", t.Type))
		}
		out.Tools = append(out.Tools, OAITool{
			Type: "function",
			Function: OAIToolFunction{
				Name: t.Name, Description: t.Description, Parameters: t.Parameters,
			},
		})
	}
	out.ToolChoice = in.ToolChoice // 字符串语义一致（auto/none/required）

	// input：字符串 → 单 user 消息；数组 → 逐项映射
	var s string
	if json.Unmarshal(in.Input, &s) == nil {
		raw, _ := json.Marshal(s)
		out.Messages = append(out.Messages, OAIMessage{Role: "user", Content: raw})
		return out, nil
	}
	var items []json.RawMessage
	if len(in.Input) > 0 && json.Unmarshal(in.Input, &items) != nil {
		return nil, errors.New(i18n.T("transform.inputInvalid"))
	}
	for _, raw := range items {
		var probe struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		_ = json.Unmarshal(raw, &probe)
		switch {
		case probe.Type == "message" || (probe.Type == "" && probe.Role != ""):
			var m struct {
				Role    string `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(raw, &m) != nil {
				continue
			}
			text := responsesContentToText(m.Content)
			rawText, _ := json.Marshal(text)
			role := m.Role
			if role == "developer" {
				role = "system"
			}
			out.Messages = append(out.Messages, OAIMessage{Role: role, Content: rawText})
		case probe.Type == "function_call":
			var fc struct {
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(raw, &fc) != nil {
				continue
			}
			// 并入相邻 assistant 消息（Responses 的连续 items 对应一条 assistant turn）
			if n := len(out.Messages); n > 0 && out.Messages[n-1].Role == "assistant" {
				out.Messages[n-1].ToolCalls = append(out.Messages[n-1].ToolCalls, OAIToolCall{
					ID: fc.CallID, Type: "function",
					Function: OAIToolCallFunction{Name: fc.Name, Arguments: fc.Arguments},
				})
			} else {
				args := fc.Arguments
				if args == "" {
					args = "{}"
				}
				out.Messages = append(out.Messages, OAIMessage{
					Role: "assistant",
					ToolCalls: []OAIToolCall{{
						ID: fc.CallID, Type: "function",
						Function: OAIToolCallFunction{Name: fc.Name, Arguments: args},
					}},
				})
			}
		case probe.Type == "function_call_output":
			var fo struct {
				CallID string `json:"call_id"`
				Output string `json:"output"`
			}
			if json.Unmarshal(raw, &fo) != nil {
				continue
			}
			rawOut, _ := json.Marshal(fo.Output)
			out.Messages = append(out.Messages, OAIMessage{
				Role: "tool", ToolCallID: fo.CallID, Content: rawOut,
			})
		case probe.Type == "reasoning":
			// 推理项不透传（chat 协议无对应概念）
		default:
			// 未知 item 跳过（保持宽容）
		}
	}
	return out, nil
}

// responsesContentToText 提取 Responses message content（字符串或 [{type,text}]）。
func responsesContentToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb []byte
		for _, p := range parts {
			if p.Type == "output_text" || p.Type == "input_text" || p.Text != "" {
				sb = append(sb, p.Text...)
			}
		}
		return string(sb)
	}
	return ""
}

// RespO2R 把 OpenAI Chat 响应转为 Responses 格式。
func RespO2R(in *OAIChatResponse) *ResponsesResponse {
	out := &ResponsesResponse{
		ID: "resp_" + in.ID, Object: "response", Status: "completed", Model: in.Model,
	}
	if len(in.Choices) > 0 {
		ch := in.Choices[0]
		if text := OAIContentToText(ch.Message.Content); text != "" {
			item := ResponsesOutputItem{Type: "message", ID: "msg_" + in.ID, Role: "assistant", Status: "completed"}
			item.Content = []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "output_text", Text: text}}
			out.Output = append(out.Output, item)
		}
		for i, tc := range ch.Message.ToolCalls {
			out.Output = append(out.Output, ResponsesOutputItem{
				Type: "function_call", ID: "fc_" + in.ID + "_" + fmt.Sprint(i),
				CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
				Status: "completed",
			})
		}
	}
	if out.Output == nil {
		out.Output = []ResponsesOutputItem{}
	}
	if in.Usage != nil {
		out.Usage = &struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		}{InputTokens: in.Usage.PromptTokens, OutputTokens: in.Usage.CompletionTokens,
			TotalTokens: in.Usage.TotalTokens}
	}
	return out
}
