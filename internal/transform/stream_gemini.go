package transform

import (
	"encoding/json"
	"time"
)

// G2OStreamConverter 把 Gemini SSE 流（每帧一个完整 GenerateContentResponse）
// 转换为 OpenAI chunk 流。Gemini 的 functionCall 是全量的（无增量语义），
// 出现在哪帧就在哪帧发完整 tool_calls。
type G2OStreamConverter struct {
	msgID    string
	model    string
	started  bool
	toolIdx  int
	finish   string
	prompt   int
	output   int
	sentDone bool
}

// NewG2OStreamConverter 创建。
func NewG2OStreamConverter() *G2OStreamConverter { return &G2OStreamConverter{} }

// Convert 处理一帧 Gemini data（JSON），返回 0..n 个 OpenAI 帧。
func (c *G2OStreamConverter) Convert(data string) []string {
	var gr GeminiResponse
	if err := json.Unmarshal([]byte(data), &gr); err != nil {
		return nil
	}
	var out []string
	if !c.started {
		c.started = true
		c.msgID = "gemini-stream"
		if gr.UsageMetadata != nil {
			c.prompt = gr.UsageMetadata.PromptTokenCount
		}
		out = append(out, FrameOAI(mustJSON(map[string]any{
			"id": c.msgID, "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": c.model,
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]string{"role": "assistant"}, "finish_reason": nil,
			}},
		})))
	}
	if len(gr.Candidates) == 0 {
		return out
	}
	cand := gr.Candidates[0]
	for _, p := range cand.Content.Parts {
		if p.Text != "" {
			out = append(out, FrameOAI(mustJSON(chunkG2O(c, map[string]any{"content": p.Text}))))
		}
		if p.FunctionCall != nil {
			args := "{}"
			if len(p.FunctionCall.Args) > 0 {
				args = string(p.FunctionCall.Args)
			}
			idx := c.toolIdx
			c.toolIdx++
			out = append(out, FrameOAI(mustJSON(chunkG2O(c, map[string]any{
				"tool_calls": []any{map[string]any{
					"index": idx, "id": "gemini-call-" + mustJSON(idx), "type": "function",
					"function": map[string]string{"name": p.FunctionCall.Name, "arguments": args},
				}},
			}))))
			if c.finish == "" {
				c.finish = "tool_calls"
			}
		}
	}
	if cand.FinishReason != "" && c.finish == "" {
		c.finish = geminiFinishToOAI(cand.FinishReason)
	}
	if gr.UsageMetadata != nil {
		c.prompt = gr.UsageMetadata.PromptTokenCount
		c.output += gr.UsageMetadata.CandidatesTokenCount
	}
	return out
}

// Flush 收尾：终 chunk + [DONE]。幂等。
func (c *G2OStreamConverter) Flush() []string {
	if !c.started || c.sentDone {
		return nil
	}
	c.sentDone = true
	if c.finish == "" {
		c.finish = "stop"
	}
	final := map[string]any{
		"id": c.msgID, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": c.model,
		"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": c.finish,
		}},
		"usage": map[string]int{
			"prompt_tokens": c.prompt, "completion_tokens": c.output,
			"total_tokens": c.prompt + c.output,
		},
	}
	return []string{FrameOAI(mustJSON(final)), FrameDONE}
}

// Usage 返回累计用量。
func (c *G2OStreamConverter) Usage() (int, int) { return c.prompt, c.output }

func chunkG2O(c *G2OStreamConverter, delta map[string]any) map[string]any {
	return map[string]any{
		"id": c.msgID, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": c.model,
		"choices": []any{map[string]any{
			"index": 0, "delta": delta, "finish_reason": nil,
		}},
	}
}

// O2GStreamConverter 把 OpenAI chunk 流转换为 Gemini SSE 流
// （openai 上游 × anthropic 客户端场景不存在本方向；此转换器用于
// openai 上游 × gemini 直连客户端——当前代理无 gemini 下游端点，预留）。
type O2GStreamConverter struct {
	started bool
	finish  string
	usage   *OAIUsage
}

// NewO2GStreamConverter 创建。
func NewO2GStreamConverter() *O2GStreamConverter { return &O2GStreamConverter{} }

// Convert 处理 OpenAI chunk，返回 Gemini SSE 帧。
func (c *O2GStreamConverter) Convert(data string) []string {
	if data == "[DONE]" {
		return c.Flush()
	}
	var chunk OAIChunk
	if json.Unmarshal([]byte(data), &chunk) != nil {
		return nil
	}
	c.started = true
	if chunk.Usage != nil {
		c.usage = chunk.Usage
	}
	var out []string
	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			out = append(out, geminiFrame(GeminiResponse{
				Candidates: []struct {
					Content      GeminiContent `json:"content"`
					FinishReason string        `json:"finishReason,omitempty"`
				}{{Content: GeminiContent{Parts: []GeminiPart{{Text: ch.Delta.Content}}}}},
			}))
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			c.finish = *ch.FinishReason
		}
	}
	return out
}

// Flush 收尾帧（含 finishReason 与 usageMetadata）。幂等。
func (c *O2GStreamConverter) Flush() []string {
	if !c.started {
		return nil
	}
	c.started = false
	gr := GeminiResponse{
		Candidates: []struct {
			Content      GeminiContent `json:"content"`
			FinishReason string        `json:"finishReason,omitempty"`
		}{{Content: GeminiContent{Parts: []GeminiPart{{Text: ""}}}, FinishReason: geminiFinishFromOAI(c.finish)}},
	}
	if c.usage != nil {
		gr.UsageMetadata = &struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		}{PromptTokenCount: c.usage.PromptTokens, CandidatesTokenCount: c.usage.CompletionTokens}
	}
	return []string{geminiFrame(gr)}
}

// Usage 返回累计用量。
func (c *O2GStreamConverter) Usage() (int, int) {
	if c.usage == nil {
		return 0, 0
	}
	return c.usage.PromptTokens, c.usage.CompletionTokens
}

// geminiFrame 序列化一帧 Gemini SSE。
func geminiFrame(gr GeminiResponse) string {
	return FrameOAI(mustJSON(gr))
}
