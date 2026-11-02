package transform

import (
	"encoding/json"
	"errors"

	"llm-gateway-proxy/internal/i18n"
)

// AnthropicContentToText 把 Anthropic content（string 或 blocks）提取为纯文本。
func AnthropicContentToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []AnthropicContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var sb []byte
		for _, b := range blocks {
			if b.Type == "text" {
				sb = append(sb, b.Text...)
			}
		}
		return string(sb)
	}
	return ""
}

// anthropicContentToOAI 把 Anthropic content 转为 OpenAI content：
// 纯文本 → string；含图/工具结果 → parts 数组。
func anthropicContentToOAI(raw json.RawMessage, isToolResult bool) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`""`), nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return json.RawMessage(raw), nil
	}
	var blocks []AnthropicContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return json.RawMessage(`""`), nil
	}

	if isToolResult {
		// tool_result 的 content：提取文本（tool 结果不支持图片回传给 OpenAI tool 消息）
		var sb []byte
		for _, b := range blocks {
			if b.Type == "text" {
				sb = append(sb, b.Text...)
			}
		}
		// 注：嵌套 content 为 blocks 时提取其文本
		for _, b := range blocks {
			if len(b.Content) > 0 {
				sb = append(sb, AnthropicContentToText(b.Content)...)
			}
		}
		out, _ := json.Marshal(string(sb))
		return out, nil
	}

	onlyText := true
	for _, b := range blocks {
		if b.Type != "text" {
			onlyText = false
			break
		}
	}
	if onlyText {
		var sb []byte
		for _, b := range blocks {
			if len(sb) > 0 {
				sb = append(sb, '\n')
			}
			sb = append(sb, b.Text...)
		}
		out, _ := json.Marshal(string(sb))
		return out, nil
	}

	// 多模态 parts
	type part struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	var parts []part
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, part{Type: "text", Text: b.Text})
		case "image":
			if b.Source != nil && b.Source.Type == "base64" {
				parts = append(parts, part{
					Type:     "image_url",
					ImageURL: &struct {
						URL string `json:"url"`
					}{URL: "data:" + b.Source.MediaType + ";base64," + b.Source.Data},
				})
			}
		}
	}
	out, err := json.Marshal(parts)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReqA2O 把 Anthropic Messages 请求转为 OpenAI Chat 请求。
func ReqA2O(in *AnthropicRequest) (*OAIChatRequest, error) {
	out := &OAIChatRequest{
		Model:       in.Model,
		MaxTokens:   in.MaxTokens,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stream:      in.Stream,
	}
	if len(in.StopSequences) > 0 {
		out.Stop, _ = json.Marshal(in.StopSequences)
	}

	// tools
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, OAITool{
			Type: "function",
			Function: OAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	// tool_choice
	if len(in.ToolChoice) > 0 {
		var tc struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(in.ToolChoice, &tc) == nil {
			switch tc.Type {
			case "auto":
				out.ToolChoice = json.RawMessage(`"auto"`)
			case "none":
				out.ToolChoice = json.RawMessage(`"none"`)
			case "any":
				out.ToolChoice = json.RawMessage(`"required"`)
			case "tool":
				if tc.Name != "" {
					out.ToolChoice, _ = json.Marshal(map[string]any{
						"type": "function", "function": map[string]string{"name": tc.Name},
					})
				}
			}
		}
	}

	// system
	if sys := AnthropicContentToText(in.System); sys != "" {
		raw, _ := json.Marshal(sys)
		out.Messages = append(out.Messages, OAIMessage{Role: "system", Content: raw})
	}

	// messages：tool_result 块拆为独立 tool 消息；tool_use 块并入 assistant.tool_calls
	for _, m := range in.Messages {
		var blocks []AnthropicContentBlock
		isString := false
		var s string
		if json.Unmarshal(m.Content, &s) == nil {
			isString = true
		} else if json.Unmarshal(m.Content, &blocks) != nil {
			blocks = nil
		}

		switch m.Role {
		case "system", "developer":
			// Anthropic 规范中 system 属顶层字段，但 Claude Code 会把 agent 系统提示
			// 作为 role=system 的消息注入 messages（且可能位于 user 之后）。
			// 此处按原位置保序转成 OpenAI system 消息（OpenAI 允许多条 system）。
			if t := AnthropicContentToText(m.Content); t != "" {
				raw, _ := json.Marshal(t)
				out.Messages = append(out.Messages, OAIMessage{Role: "system", Content: raw})
			}
		case "user":
			if isString {
				out.Messages = append(out.Messages, OAIMessage{Role: "user", Content: m.Content})
				continue
			}
			// 分离 tool_result 与普通内容
			var plainBlocks []AnthropicContentBlock
			for _, b := range blocks {
				if b.Type == "tool_result" {
					content, err := anthropicContentToOAI(b.Content, true)
					if err != nil {
						return nil, err
					}
					out.Messages = append(out.Messages, OAIMessage{
						Role: "tool", ToolCallID: b.ToolUseID, Content: content,
					})
				} else {
					plainBlocks = append(plainBlocks, b)
				}
			}
			if len(plainBlocks) > 0 {
				raw, _ := json.Marshal(plainBlocks)
				content, err := anthropicContentToOAI(raw, false)
				if err != nil {
					return nil, err
				}
				out.Messages = append(out.Messages, OAIMessage{Role: "user", Content: content})
			}
		case "assistant":
			var texts []string
			var toolCalls []OAIToolCall
			for _, b := range blocks {
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
			var content json.RawMessage
			if len(texts) > 0 {
				content, _ = json.Marshal(joinStrings(texts))
			}
			msg := OAIMessage{Role: "assistant", Content: content}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			out.Messages = append(out.Messages, msg)
		default:
			return nil, errors.New(i18n.T("transform.roleUnsupported", m.Role))
		}
	}
	return out, nil
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "\n"
		}
		out += s
	}
	return out
}
