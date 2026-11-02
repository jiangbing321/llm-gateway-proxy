package proxy

import (
	"strings"
	"testing"
)

// 直通流式的 usage 抓取：上游 usage 可能分布在流的头部（Anthropic message_start
// 带输入）与尾部（message_delta 带输出），必须逐行累积 per-field 非零值。

func TestUsageScannerAnthropicHeadTail(t *testing.T) {
	// dots3 实测格式：message_start 带 input_tokens，message_delta 只带 output_tokens
	var u usageScanner
	u.Feed([]byte("event: message_start\n" +
		`data: {"type":"message_start","message":{"usage":{"input_tokens":14,"output_tokens":0}}}` + "\n\n" +
		`data: {"type":"content_block_delta","delta":{"text":"hi","type":"text_delta"}}` + "\n\n"))
	u.Feed([]byte("event: message_delta\n" +
		`data: {"type":"message_delta","delta":{},"usage":{"input_tokens":0,"output_tokens":16}}` + "\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"))
	p, c := u.Result()
	if p != 14 || c != 16 {
		t.Fatalf("prompt=%d completion=%d, want 14/16", p, c)
	}
}

func TestUsageScannerUsageInTailOnly(t *testing.T) {
	// glm 格式：input/output 都在 message_delta；message_start 的 usage 全 0
	var u usageScanner
	u.Feed([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":25046,"output_tokens":109}}` + "\n\n"))
	p, c := u.Result()
	if p != 25046 || c != 109 {
		t.Fatalf("prompt=%d completion=%d, want 25046/109", p, c)
	}
}

func TestUsageScannerChunkSplitLine(t *testing.T) {
	// usage JSON 恰好被块边界截断
	var u usageScanner
	u.Feed([]byte(`data: {"type":"message_delta","usage":{"input_tokens":0,"out`))
	u.Feed([]byte(`put_tokens":42}}` + "\n\n"))
	p, c := u.Result()
	if p != 0 || c != 42 {
		t.Fatalf("prompt=%d completion=%d, want 0/42", p, c)
	}
}

func TestUsageScannerOpenAIFinalChunk(t *testing.T) {
	// OpenAI 直通：usage 在最后一个 chunk，prompt/completion 同帧
	var u usageScanner
	for i := 0; i < 100; i++ {
		u.Feed([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
	}
	u.Feed([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":123,\"completion_tokens\":45}}\n\ndata: [DONE]\n\n"))
	p, c := u.Result()
	if p != 123 || c != 45 {
		t.Fatalf("prompt=%d completion=%d, want 123/45", p, c)
	}
}

func TestUsageScannerNoUsage(t *testing.T) {
	var u usageScanner
	u.Feed([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	u.Feed([]byte("data: [DONE]\n\n"))
	p, c := u.Result()
	if p != 0 || c != 0 {
		t.Fatalf("prompt=%d completion=%d, want 0/0", p, c)
	}
}

// 缓存 token 必须计入输入：Anthropic 语义下 input_tokens 不含缓存部分，
// 实测 dots3 第二轮 in=291 + cache_read=5120，只加 input 会严重偏少。
func TestUsageScannerCacheTokens(t *testing.T) {
	var u usageScanner
	u.Feed([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":291,"cache_read_input_tokens":5120,"output_tokens":0}}}` + "\n\n"))
	u.Feed([]byte(`data: {"type":"message_delta","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":16}}` + "\n\n"))
	p, c := u.Result()
	if p != 5411 || c != 16 {
		t.Fatalf("prompt=%d completion=%d, want 5411/16", p, c)
	}
}

// 个别上游 message_delta 重复 input_tokens 却省略 cache 字段：
// 覆盖会丢缓存量，per-field 取 max 才能保住流头的完整值。
func TestUsageScannerDeltaOmitsCache(t *testing.T) {
	var u usageScanner
	u.Feed([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":291,"cache_read_input_tokens":5120,"output_tokens":0}}}` + "\n\n"))
	u.Feed([]byte(`data: {"type":"message_delta","usage":{"input_tokens":291,"output_tokens":16}}` + "\n\n"))
	p, c := u.Result()
	if p != 5411 || c != 16 {
		t.Fatalf("prompt=%d completion=%d, want 5411/16", p, c)
	}
}

// 非流式响应体同样要计入缓存 token。
func TestUsageFromBodyCacheTokens(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":291,"cache_read_input_tokens":5120,"cache_creation_input_tokens":100,"output_tokens":16}}`)
	p, c := usageFromBody(body)
	if p != 5511 || c != 16 {
		t.Fatalf("prompt=%d completion=%d, want 5511/16", p, c)
	}
	// OpenAI 形状不受影响（无 cache 字段）
	p, c = usageFromBody([]byte(`{"usage":{"prompt_tokens":123,"completion_tokens":45}}`))
	if p != 123 || c != 45 {
		t.Fatalf("prompt=%d completion=%d, want 123/45", p, c)
	}
}

func TestUsageScannerLongStreamKeepsHeadUsage(t *testing.T) {
	// 长流：message_start 之后输出远超旧的 16KB 尾窗，input_tokens 仍不丢
	var u usageScanner
	u.Feed([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":25046,"output_tokens":0}}}` + "\n\n"))
	u.Feed([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"" + strings.Repeat("字", 20000) + "\"}}\n\n"))
	u.Feed([]byte(`data: {"type":"message_delta","usage":{"output_tokens":777}}` + "\n\n"))
	p, c := u.Result()
	if p != 25046 || c != 777 {
		t.Fatalf("prompt=%d completion=%d, want 25046/777", p, c)
	}
}
