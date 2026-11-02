package proxy

import (
	"encoding/json"
	"testing"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/provider"
)

// convertRequest 对 OpenAI 上游的流式请求需补 stream_options.include_usage，
// 否则严格遵循 OpenAI 规范的上游（官方 API / 严格兼容网关）不发 usage，token 统计全 0。
func TestConvertRequestInjectsIncludeUsage(t *testing.T) {
	s := NewServer(config.Config{Bind: "127.0.0.1", Port: 0}, logbuf.NewRing(0), nil, nil)
	prov := provider.NewOpenAIProvider(config.Provider{Name: "o", Type: "openai", BaseURL: "https://x.example"})

	inject := func(t *testing.T, body string, stream bool) map[string]any {
		t.Helper()
		out, err := s.convertRequest(provider.ProtocolOpenAI, []byte(body), prov, stream)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	// Anthropic 客户端 → OpenAI 上游（A2O）：流式注入
	m := inject(t, `{"model":"m","stream":true,"max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, true)
	so, ok := m["stream_options"].(map[string]any)
	if !ok || so["include_usage"] != true {
		t.Fatalf("流式应注入 stream_options.include_usage: %v", m["stream_options"])
	}

	// 同路径非流式：不注入
	m = inject(t, `{"model":"m","stream":false,"max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, false)
	if _, has := m["stream_options"]; has {
		t.Fatalf("非流式不应注入 stream_options")
	}

	// 客户端已带 stream_options：不覆盖（保留原值）
	m = inject(t, `{"model":"m","stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}`, true)
	so, ok = m["stream_options"].(map[string]any)
	if !ok || so["include_usage"] != false {
		t.Fatalf("客户端已带 stream_options 应保留原值: %v", m["stream_options"])
	}

	// 未建模字段应原样保留（直传路径不经类型化结构体重序列化）
	m = inject(t, `{"model":"m","stream":true,"presence_penalty":0.5,"seed":42,"metadata":{"k":"v"},"messages":[{"role":"user","content":"hi"}]}`, true)
	if m["presence_penalty"] != 0.5 || m["seed"] != float64(42) {
		t.Fatalf("未建模字段丢失: %v", m)
	}
	if mm, ok := m["metadata"].(map[string]any); !ok || mm["k"] != "v" {
		t.Fatalf("metadata 丢失: %v", m["metadata"])
	}
}
