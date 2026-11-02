package transform

import (
	"encoding/json"
	"time"
)

// A2OStreamConverter 把 Anthropic 事件流转换为 OpenAI chunk 流（有状态）。
type A2OStreamConverter struct {
	msgID   string
	model   string
	started bool

	toolIdx      int               // 已开的 tool_use 块计数（→ OpenAI tool_calls[].index）
	blockType    map[int]string    // Anthropic block index → 类型（text/tool_use/thinking）
	curToolIndex map[int]int       // Anthropic block index → OpenAI tool_calls index
	toolMeta     map[int]toolMeta  // OpenAI index → id/name

	finish    string
	usageIn   int
	usageOut  int
	sentFinal bool
}

type toolMeta struct {
	id, name string
}

// NewA2OStreamConverter 创建。
func NewA2OStreamConverter() *A2OStreamConverter {
	return &A2OStreamConverter{
		blockType:    map[int]string{},
		curToolIndex: map[int]int{},
		toolMeta:     map[int]toolMeta{},
	}
}

// Convert 处理一个上游事件 JSON，返回 0..n 个下游帧（data: ... 帧文本）。
func (c *A2OStreamConverter) Convert(data string) []string {
	var ev AnthropicEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil
	}

	switch ev.Type {
	case "message_start":
		var msg struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(ev.Message, &msg)
		c.msgID, c.model = msg.ID, msg.Model
		if msg.Usage != nil {
			c.usageIn = msg.Usage.InputTokens
		}
		c.started = true
		return []string{FrameOAI(mustJSON(map[string]any{
			"id": c.msgID, "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": c.model,
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]string{"role": "assistant"}, "finish_reason": nil,
			}},
		}))}

	case "content_block_start":
		var cb struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(ev.ContentBlock, &cb)
		c.blockType[ev.Index] = cb.Type
		if cb.Type == "tool_use" {
			idx := c.toolIdx
			c.toolIdx++
			c.curToolIndex[ev.Index] = idx
			c.toolMeta[idx] = toolMeta{id: cb.ID, name: cb.Name}
			return []string{FrameOAI(mustJSON(c.chunkWith(map[string]any{
				"tool_calls": []any{map[string]any{
					"index": idx, "id": cb.ID, "type": "function",
					"function": map[string]string{"name": cb.Name, "arguments": ""},
				}},
			})))}
		}
		return nil

	case "content_block_delta":
		var d struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			PartialJSON string `json:"partial_json"`
		}
		if json.Unmarshal(ev.Delta, &d) != nil {
			return nil
		}
		switch d.Type {
		case "text_delta":
			if d.Text == "" {
				return nil
			}
			return []string{FrameOAI(mustJSON(c.chunkWith(map[string]any{
				"content": d.Text,
			})))}
		case "thinking_delta":
			if d.Thinking == "" {
				return nil
			}
			// Claude 思考内容 → DeepSeek 风格 reasoning_content
			return []string{FrameOAI(mustJSON(c.chunkWith(map[string]any{
				"reasoning_content": d.Thinking,
			})))}
		case "input_json_delta":
			if d.PartialJSON == "" {
				return nil
			}
			return []string{FrameOAI(mustJSON(c.chunkWith(map[string]any{
				"tool_calls": []any{map[string]any{
					"index": c.curToolIndex[ev.Index],
					"function": map[string]string{"arguments": d.PartialJSON},
				}},
			})))}
		}
		return nil

	case "content_block_stop":
		return nil

	case "message_delta":
		// delta:{stop_reason} + 顶层 usage:{output_tokens}
		var d struct {
			StopReason string `json:"stop_reason"`
		}
		_ = json.Unmarshal(ev.Delta, &d)
		if d.StopReason != "" {
			c.finish = d.StopReason
		}
		if ev.Usage != nil {
			c.usageOut = ev.Usage.OutputTokens
			if ev.Usage.InputTokens > 0 {
				c.usageIn = ev.Usage.InputTokens
			}
		}
		return nil

	case "message_stop":
		return c.Flush()

	case "ping", "error":
		if ev.Type == "error" {
			// 上游流中错误：作为最后 chunk 的内容透出后结束
			return []string{FrameOAI(mustJSON(c.chunkWith(map[string]any{
				"content": "\n[上游流式错误] " + data,
			}))), FrameDONE}
		}
		return nil
	}
	return nil
}

// Flush 收尾：终 chunk（finish_reason + usage）+ [DONE]。幂等。
func (c *A2OStreamConverter) Flush() []string {
	if !c.started || c.sentFinal {
		return nil
	}
	c.sentFinal = true
	if c.finish == "" {
		c.finish = "end_turn"
	}
	usage := map[string]int{
		"prompt_tokens": c.usageIn, "completion_tokens": c.usageOut,
		"total_tokens": c.usageIn + c.usageOut,
	}
	final := map[string]any{
		"id": c.msgID, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": c.model,
		"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": stopReasonAToO(c.finish),
		}},
		"usage": usage,
	}
	return []string{FrameOAI(mustJSON(final)), FrameDONE}
}

// Usage 返回累计用量。
func (c *A2OStreamConverter) Usage() (int, int) { return c.usageIn, c.usageOut }

// chunkWith 构造一个增量 chunk（复用 id/model 元数据）。
func (c *A2OStreamConverter) chunkWith(delta map[string]any) map[string]any {
	return map[string]any{
		"id": c.msgID, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": c.model,
		"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": nil,
		}},
	}
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
