package transform

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"llm-gateway-proxy/internal/i18n"
)

// OAIContentToText 把 OpenAI content（string 或 parts）提取为纯文本（tool_result/判断用）。
func OAIContentToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []OAIContentPart
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}
	return ""
}

// oaiContentToBlocks 把 OpenAI content（string 或 parts）转为 Anthropic blocks。
// 图片 data URL → base64 source；不支持的 part 降级为文本。
func oaiContentToBlocks(raw json.RawMessage) []AnthropicContentBlock {
	if len(raw) == 0 {
		return []AnthropicContentBlock{{Type: "text", Text: ""}}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []AnthropicContentBlock{{Type: "text", Text: s}}
	}
	var parts []OAIContentPart
	if json.Unmarshal(raw, &parts) != nil {
		return []AnthropicContentBlock{{Type: "text", Text: ""}}
	}
	var out []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, AnthropicContentBlock{Type: "text", Text: p.Text})
		case "image_url":
			if p.ImageURL != nil {
				if media, data, ok := splitDataURL(p.ImageURL.URL); ok {
					out = append(out, AnthropicContentBlock{
						Type:   "image",
						Source: &AnthropicImageSource{Type: "base64", MediaType: media, Data: data},
					})
				} else if strings.HasPrefix(p.ImageURL.URL, "http") {
					// Anthropic 只支持 base64；URL 图降级为占位文本
					out = append(out, AnthropicContentBlock{
						Type: "text", Text: "[image: " + p.ImageURL.URL + "]",
					})
				}
			}
		}
	}
	if len(out) == 0 {
		out = append(out, AnthropicContentBlock{Type: "text", Text: ""})
	}
	return out
}

// splitDataURL 解析 data:<media>;base64,<data>。
func splitDataURL(u string) (media, data string, ok bool) {
	const prefix = "data:"
	const marker = ";base64,"
	if !strings.HasPrefix(u, prefix) {
		return "", "", false
	}
	rest := u[len(prefix):]
	i := strings.Index(rest, marker)
	if i < 0 {
		return "", "", false
	}
	media = rest[:i]
	if _, err := base64.StdEncoding.DecodeString(rest[i+len(marker):]); err != nil {
		return "", "", false
	}
	return media, rest[i+len(marker):], true
}

// ReqO2A 把 OpenAI Chat 请求转为 Anthropic Messages 请求。
// defaultMaxTokens 在请求缺省 max_tokens 时使用（Anthropic 必填）。
func ReqO2A(in *OAIChatRequest, defaultMaxTokens int) (*AnthropicRequest, error) {
	if defaultMaxTokens <= 0 {
		defaultMaxTokens = 8192
	}
	out := &AnthropicRequest{
		Model:       in.Model,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stream:      in.Stream,
	}
	if in.MaxTokens > 0 {
		out.MaxTokens = in.MaxTokens
	} else {
		out.MaxTokens = defaultMaxTokens
	}
	if len(in.Stop) > 0 {
		var s string
		if json.Unmarshal(in.Stop, &s) == nil {
			out.StopSequences = []string{s}
		} else {
			_ = json.Unmarshal(in.Stop, &out.StopSequences)
		}
	}

	// tools
	for _, t := range in.Tools {
		if t.Type != "function" && t.Type != "" {
			continue
		}
		out.Tools = append(out.Tools, AnthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	// tool_choice
	if len(in.ToolChoice) > 0 {
		var s string
		if json.Unmarshal(in.ToolChoice, &s) == nil {
			switch s {
			case "auto":
				out.ToolChoice = json.RawMessage(`{"type":"auto"}`)
			case "none":
				out.ToolChoice = json.RawMessage(`{"type":"none"}`)
			case "required":
				out.ToolChoice = json.RawMessage(`{"type":"any"}`)
			}
		} else {
			var tc struct {
				Type     string `json:"type"`
				Function *struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if json.Unmarshal(in.ToolChoice, &tc) == nil && tc.Type == "function" && tc.Function != nil {
				out.ToolChoice, _ = json.Marshal(map[string]string{"type": "tool", "name": tc.Function.Name})
			}
		}
	}

	// messages：system 抽取；tool 消息并入 user turn
	var systemTexts []string
	type turn struct {
		role    string // user/assistant
		blocks  []AnthropicContentBlock
	}
	var turns []turn
	flushToolResults := func() {} // 占位（逻辑在主循环内）

	for _, m := range in.Messages {
		switch m.Role {
		case "system", "developer":
			if t := OAIContentToText(m.Content); t != "" {
				systemTexts = append(systemTexts, t)
			}
		case "user":
			turns = append(turns, turn{role: "user", blocks: oaiContentToBlocks(m.Content)})
		case "assistant":
			var blocks []AnthropicContentBlock
			if txt := OAIContentToText(m.Content); txt != "" {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: txt})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(strings.TrimSpace(tc.Function.Arguments)) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, AnthropicContentBlock{
					Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: input,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
			}
			turns = append(turns, turn{role: "assistant", blocks: blocks})
		case "tool":
			// 相邻 tool 消息合并进同一 user turn
			if len(turns) > 0 && turns[len(turns)-1].role == "user" && len(turns[len(turns)-1].blocks) > 0 &&
				turns[len(turns)-1].blocks[0].Type == "tool_result" {
				turns[len(turns)-1].blocks = append(turns[len(turns)-1].blocks, toolResultBlock(m))
			} else {
				turns = append(turns, turn{role: "user", blocks: []AnthropicContentBlock{toolResultBlock(m)}})
			}
		default:
			return nil, errors.New(i18n.T("transform.roleUnsupported", m.Role))
		}
	}
	flushToolResults()

	if len(systemTexts) > 0 {
		out.System, _ = json.Marshal(strings.Join(systemTexts, "\n\n"))
	}
	for _, t := range turns {
		raw, _ := json.Marshal(t.blocks)
		out.Messages = append(out.Messages, AnthropicMessage{Role: t.role, Content: raw})
	}
	// Anthropic 要求首条消息为 user；若首条是 assistant，前置空 user
	if len(out.Messages) > 0 && out.Messages[0].Role != "user" {
		out.Messages = append([]AnthropicMessage{{Role: "user", Content: json.RawMessage(`""`)}}, out.Messages...)
	}
	// 不支持多选项
	if in.N > 1 {
		// Anthropic 无 n 概念，忽略
	}
	return out, nil
}

// toolResultBlock 把 OpenAI tool 消息转为 tool_result 块。
func toolResultBlock(m OAIMessage) AnthropicContentBlock {
	return AnthropicContentBlock{
		Type:      "tool_result",
		ToolUseID: m.ToolCallID,
		Content:   m.Content,
	}
}
