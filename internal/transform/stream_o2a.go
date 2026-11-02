package transform

import (
	"encoding/json"
	"fmt"
	"time"
)

// O2AStreamConverter 把 OpenAI chunk 流转换为 Anthropic 事件流（有状态）。
//
// 事件顺序硬约束（Claude Code 严格校验）：
// message_start 最先 → content_block_start/delta/stop → message_delta → message_stop 最后。
// Flush 在上游结束/断流时保证序列闭合。
type O2AStreamConverter struct {
	msgID  string
	model  string
	started bool

	blockIdx  int
	textOpen  bool
	textBlockIdx int // 当前文本块的 index（delta/stop 引用；与 blockIdx 分离防错位）
	toolOpens map[int]bool // OpenAI tool_calls[].index → 是否已发 content_block_start
	toolBlockIdx map[int]int // OpenAI index → Anthropic block index

	finish   string // 缓存的 finish_reason
	usage    *OAIUsage
	outputText []byte // 累计文本（调试/测试断言用，不参与转换）
}

// NewO2AStreamConverter 创建。
func NewO2AStreamConverter() *O2AStreamConverter {
	return &O2AStreamConverter{
		toolOpens:     map[int]bool{},
		toolBlockIdx:  map[int]int{},
	}
}

// Convert 处理一个上游 data 事件（JSON chunk 或 "[DONE]"），返回 0..n 个下游帧。
func (c *O2AStreamConverter) Convert(data string) []string {
	if data == "[DONE]" {
		return c.Flush()
	}
	var chunk OAIChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil // 忽略无法解析的 chunk（如厂商私有注释帧）
	}
	if chunk.ID != "" {
		c.msgID = chunk.ID
	}
	if chunk.Model != "" {
		c.model = chunk.Model
	}
	if chunk.Usage != nil {
		c.usage = chunk.Usage
	}

	var out []string
	if !c.started {
		c.started = true
		if c.msgID == "" {
			c.msgID = fmt.Sprintf("chatcml-%d", time.Now().UnixNano())
		}
		out = append(out, FrameAnthropic("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": c.msgID, "type": "message", "role": "assistant",
				"model": c.model, "content": []any{},
				"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
			},
		}))
	}

	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			if !c.textOpen {
				out = append(out, c.openText())
			}
			c.outputText = append(c.outputText, ch.Delta.Content...)
			out = append(out, FrameAnthropic("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": c.textBlockIdx,
				"delta": map[string]string{"type": "text_delta", "text": ch.Delta.Content},
			}))
		}
		// 思考内容（DeepSeek/GLM 风格 reasoning_content）→ 并入当前文本块
		if ch.Delta.ReasoningContent != "" {
			if !c.textOpen {
				out = append(out, c.openText())
			}
			out = append(out, FrameAnthropic("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": c.textBlockIdx,
				"delta": map[string]string{"type": "text_delta", "text": ch.Delta.ReasoningContent},
			}))
		}
		for _, tc := range ch.Delta.ToolCalls {
			if !c.toolOpens[tc.Index] {
				// 新工具调用：关文本块（若有），开 tool_use 块
				if c.textOpen {
					out = append(out, c.closeBlock())
				}
				c.toolOpens[tc.Index] = true
				c.toolBlockIdx[tc.Index] = c.blockIdx
				out = append(out, FrameAnthropic("content_block_start", map[string]any{
					"type": "content_block_start", "index": c.blockIdx,
					"content_block": map[string]any{
						"type": "tool_use", "id": tc.ID, "name": tc.Function.Name,
						"input": map[string]any{},
					},
				}))
				c.blockIdx++
			}
			if tc.Function.Arguments != "" {
				out = append(out, FrameAnthropic("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": c.toolBlockIdx[tc.Index],
					"delta": map[string]string{"type": "input_json_delta", "partial_json": tc.Function.Arguments},
				}))
			}
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			c.finish = *ch.FinishReason
		}
	}
	return out
}

// Flush 收尾：关所有打开的块 → message_delta（stop_reason + usage）→ message_stop。
// 幂等：重复调用返回空。
func (c *O2AStreamConverter) Flush() []string {
	if !c.started {
		return nil
	}
	var out []string
	if c.textOpen {
		out = append(out, c.closeBlock())
	}
	for _, opened := range c.toolOpens {
		if opened {
			// 每个 tool 块逐一关闭（blockIdx 已记录）
			_ = opened
		}
	}
	for _, bi := range c.toolBlockIdx {
		out = append(out, FrameAnthropic("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": bi,
		}))
	}
	for k := range c.toolOpens {
		c.toolOpens[k] = false
	}
	if c.finish == "" {
		c.finish = "stop" // 上游异常断流也保证闭合
	}
	usageOut := map[string]int{"output_tokens": 0, "input_tokens": 0}
	if c.usage != nil {
		usageOut["input_tokens"] = c.usage.PromptTokens
		usageOut["output_tokens"] = c.usage.CompletionTokens
	}
	out = append(out, FrameAnthropic("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]string{"stop_reason": stopReasonOToA(c.finish), "stop_sequence": ""},
		"usage": usageOut,
	}))
	out = append(out, FrameAnthropic("message_stop", map[string]string{"type": "message_stop"}))
	c.started = false // 防 double-flush
	return out
}

// Usage 返回累计用量（prompt, completion）。
func (c *O2AStreamConverter) Usage() (int, int) {
	if c.usage == nil {
		return 0, 0
	}
	return c.usage.PromptTokens, c.usage.CompletionTokens
}

// OutputText 返回累计输出文本（测试用）。
func (c *O2AStreamConverter) OutputText() string { return string(c.outputText) }

func (c *O2AStreamConverter) openText() string {
	c.textOpen = true
	c.textBlockIdx = c.blockIdx
	c.blockIdx++
	return FrameAnthropic("content_block_start", map[string]any{
		"type": "content_block_start", "index": c.textBlockIdx,
		"content_block": map[string]string{"type": "text", "text": ""},
	})
}

func (c *O2AStreamConverter) closeBlock() string {
	c.textOpen = false
	return FrameAnthropic("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": c.textBlockIdx,
	})
}
