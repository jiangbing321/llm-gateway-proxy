package provider

import (
	"strings"
	"testing"

	"llm-gateway-proxy/internal/config"
)

func mkCfg(id, name, typ string, models ...string) config.Provider {
	return config.Provider{ID: id, Name: name, Type: typ, Enabled: true, Models: models,
		BaseURL: "https://example.com", APIKey: "k"}
}

func TestResolveBasic(t *testing.T) {
	r := NewRegistry()
	r.Rebuild([]config.Provider{
		mkCfg("prov_a", "DeepSeek", "openai", "deepseek-chat", "deepseek-reasoner"),
		mkCfg("prov_b", "Moonshot", "openai", "kimi-k2"),
	}, "prov_a")

	// 常规命中
	p, m, ok := r.Resolve("deepseek-chat")
	if !ok || p.ID() != "prov_a" || m != "deepseek-chat" {
		t.Fatalf("常规路由错误: %v %s %v", p, m, ok)
	}
	// 未命中 → 默认
	p, m, ok = r.Resolve("unknown-model")
	if !ok || p.ID() != "prov_a" || m != "unknown-model" {
		t.Fatalf("默认路由错误: %v %s %v", p, m, ok)
	}
}

func TestResolveExplicitPrefix(t *testing.T) {
	r := NewRegistry()
	r.Rebuild([]config.Provider{
		mkCfg("prov_a", "DeepSeek", "openai", "gpt-x"),     // 故意与 b 同名
		mkCfg("prov_b", "Moonshot", "anthropic", "kimi-k2"),
	}, "prov_a")

	// 前缀按名称：Moonshot/kimi-k2 → prov_b，model 去前缀
	p, m, ok := r.Resolve("Moonshot/kimi-k2")
	if !ok || p.ID() != "prov_b" || m != "kimi-k2" {
		t.Fatalf("名称前缀路由错误: %v %s %v", p, m, ok)
	}
	// 前缀按 id
	p, m, ok = r.Resolve("prov_b/kimi-k2")
	if !ok || p.ID() != "prov_b" || m != "kimi-k2" {
		t.Fatalf("id 前缀路由错误: %v %s %v", p, m, ok)
	}
	// 冲突模型：无前缀 → 先注册的 prov_a；带前缀 → 指定厂商
	p, _, _ = r.Resolve("gpt-x")
	if p.ID() != "prov_a" {
		t.Fatalf("冲突默认应先注册者优先: %v", p.ID())
	}
	// 不匹配的前缀（openrouter 风格 id）→ 视为普通模型名
	p, m, ok = r.Resolve("anthropic/claude-3")
	if !ok || m != "anthropic/claude-3" || p.ID() != "prov_a" {
		t.Fatalf("未知前缀应按普通模型名走默认: %v %s %v", p, m, ok)
	}
}

func TestConflictDetection(t *testing.T) {
	r := NewRegistry()
	r.Rebuild([]config.Provider{
		mkCfg("prov_a", "A 厂", "openai", "same-model", "only-a"),
		mkCfg("prov_b", "B 厂", "anthropic", "same-model", "only-b"),
	}, "")
	cs := r.Conflicts()
	if len(cs) != 1 || !strings.Contains(cs[0], "same-model") {
		t.Fatalf("冲突未检测到: %v", cs)
	}
}
