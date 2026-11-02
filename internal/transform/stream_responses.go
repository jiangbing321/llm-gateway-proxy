package transform

import (
	"encoding/json"
	"strings"
)

// O2RStreamConverter 把 OpenAI chat chunk 流转换为 Responses 事件流。
//
// 事件序列（最小完整集，Codex/OpenAI SDK 兼容）：
//   response.created → [output_item.added(message) → response.output_text.delta* →
//   output_item.done]（每个 function_call：output_item.added →
//   response.function_call_arguments.delta* → output_item.done）→ response.completed
type O2RStreamConverter struct {
	respID   string
	model    string
	created  bool
	msgAdded bool
	msgID    string
	itemSeq  int

	textBuf strings.Builder // 消息项最终文本（completed 汇总）

	fcSeq     int                       // 已开启 function_call 计数
	fcOpen    map[int]*fcState          // OpenAI tool index → 状态
	fcEmitted []*fcState                // 完成顺序
	finish    string
	usage     *OAIUsage
	done      bool
}

type fcState struct {
	idx    int    // Responses output 数组序号
	callID string
	name   string
	args   strings.Builder
	started bool
}

// NewO2RStreamConverter 创建。
func NewO2RStreamConverter() *O2RStreamConverter {
	return &O2RStreamConverter{fcOpen: map[int]*fcState{}}
}

// Convert 处理一个 chat chunk（JSON 或 [DONE]），返回 0..n 个 Responses 事件帧
// （帧格式 "event: <type>\ndata: {...}\n\n"）。
func (c *O2RStreamConverter) Convert(data string) []string {
	if data == "[DONE]" {
		return c.Flush()
	}
	var chunk OAIChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil
	}
	var out []string
	if !c.created {
		c.created = true
		c.respID = "resp_" + chunk.ID
		c.model = chunk.Model
		out = append(out, rEvent("response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": c.respID, "object": "response", "status": "in_progress",
				"model": c.model, "output": []any{},
			},
		}))
	}

	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			if !c.msgAdded {
				c.msgAdded = true
				c.msgID = "msg_" + chunk.ID
				out = append(out, c.itemAdded("message"))
			}
			c.textBuf.WriteString(ch.Delta.Content)
			out = append(out, rEvent("response.output_text.delta", map[string]any{
				"type": "response.output_text.delta", "item_id": c.msgID,
				"output_index": 0, "content_index": 0, "delta": ch.Delta.Content,
			}))
		}
		for _, tc := range ch.Delta.ToolCalls {
			st, ok := c.fcOpen[tc.Index]
			if !ok {
				// 新 function_call：先关闭 message 项（若有）
				if c.msgAdded {
					out = append(out, c.itemDone("message"))
					c.msgAdded = false
				}
				st = &fcState{
					idx: c.nextItemIdx(), callID: tc.ID,
					name: tc.Function.Name,
				}
				c.fcOpen[tc.Index] = st
				c.fcSeq++
				out = append(out, c.fcItemAdded(st))
			}
			if tc.Function.Name != "" && st.name == "" {
				st.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				if !st.started {
					st.started = true
				}
				st.args.WriteString(tc.Function.Arguments)
				out = append(out, rEvent("response.function_call_arguments.delta", map[string]any{
					"type": "response.function_call_arguments.delta", "item_id": st.callID,
					"output_index": st.idx, "delta": tc.Function.Arguments,
				}))
			}
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			c.finish = *ch.FinishReason
		}
	}
	if chunk.Usage != nil {
		c.usage = chunk.Usage
	}
	return out
}

// nextItemIdx 分配 output 数组序号（message=0 之后递增）。
func (c *O2RStreamConverter) nextItemIdx() int {
	c.itemSeq++
	return c.itemSeq
}

func (c *O2RStreamConverter) itemAdded(kind string) string {
	return rEvent("response.output_item.added", map[string]any{
		"type": "response.output_item.added", "output_index": 0,
		"item": map[string]any{
			"type": "message", "id": c.msgID, "role": "assistant", "status": "in_progress",
			"content": []any{},
		},
	})
}

func (c *O2RStreamConverter) itemDone(kind string) string {
	return rEvent("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "output_index": 0,
		"item": map[string]any{
			"type": "message", "id": c.msgID, "role": "assistant", "status": "completed",
			"content": []any{map[string]any{
				"type": "output_text", "text": c.textBuf.String(),
			}},
		},
	})
}

func (c *O2RStreamConverter) fcItemAdded(st *fcState) string {
	return rEvent("response.output_item.added", map[string]any{
		"type": "response.output_item.added", "output_index": st.idx,
		"item": map[string]any{
			"type": "function_call", "id": st.callID, "call_id": st.callID,
			"name": st.name, "arguments": "", "status": "in_progress",
		},
	})
}

func (c *O2RStreamConverter) fcItemDone(st *fcState) string {
	return rEvent("response.output_item.done", map[string]any{
		"type": "response.output_item.done", "output_index": st.idx,
		"item": map[string]any{
			"type": "function_call", "id": st.callID, "call_id": st.callID,
			"name": st.name, "arguments": st.args.String(), "status": "completed",
		},
	})
}

// Flush 收尾：关闭未关项 → response.completed（含 output 汇总与 usage）。幂等。
func (c *O2RStreamConverter) Flush() []string {
	if !c.created || c.done {
		return nil
	}
	c.done = true
	var out []string

	// 汇总 output
	var output []any
	if c.textBuf.Len() > 0 || c.msgAdded {
		if c.msgAdded {
			out = append(out, c.itemDone("message"))
			c.msgAdded = false
		}
		output = append(output, map[string]any{
			"type": "message", "id": c.msgID, "role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": c.textBuf.String()}},
		})
	}
	for _, st := range c.fcOpen {
		out = append(out, c.fcItemDone(st))
		c.fcEmitted = append(c.fcEmitted, st)
		output = append(output, map[string]any{
			"type": "function_call", "id": st.callID, "call_id": st.callID,
			"name": st.name, "arguments": st.args.String(), "status": "completed",
		})
	}
	if output == nil {
		output = []any{}
	}

	usage := map[string]int{}
	if c.usage != nil {
		usage = map[string]int{
			"input_tokens": c.usage.PromptTokens, "output_tokens": c.usage.CompletionTokens,
			"total_tokens": c.usage.TotalTokens,
		}
	}
	out = append(out, rEvent("response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": c.respID, "object": "response", "status": "completed",
			"model": c.model, "output": output, "usage": usage,
		},
	}))
	return out
}

// Usage 返回累计用量。
func (c *O2RStreamConverter) Usage() (int, int) {
	if c.usage == nil {
		return 0, 0
	}
	return c.usage.PromptTokens, c.usage.CompletionTokens
}

// rEvent 序列化一帧 Responses SSE。
func rEvent(event string, payload any) string {
	raw, _ := json.Marshal(payload)
	return "event: " + event + "\ndata: " + string(raw) + "\n\n"
}
