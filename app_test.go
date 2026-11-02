package main

import (
	"encoding/json"
	"strings"
	"testing"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/provider"
)

// newTestApp 构造最小可测的 App。
func newTestApp(providers ...config.Provider) *App {
	m, _ := config.NewInMemoryDefault()
	_, _ = m.Update(func(c *config.Config) { c.Providers = providers })
	a := &App{cfgMgr: m, registry: provider.NewRegistry()}
	a.rebuildRegistry()
	return a
}

func providerJSON(id, name string, models ...string) string {
	b, _ := json.Marshal(config.Provider{
		ID: id, Name: name, Type: "openai",
		BaseURL: "https://api.example.com", Enabled: true, Models: models,
	})
	return string(b)
}

func TestSaveProviderUnlimited(t *testing.T) {
	a := newTestApp(
		config.Provider{ID: "p1", Name: "A", Type: "openai", BaseURL: "https://a.com", Enabled: true},
		config.Provider{ID: "p2", Name: "B", Type: "openai", BaseURL: "https://b.com", Enabled: true},
	)

	res := a.SaveProvider(providerJSON("", "C", "m1", "m2", "m3", "m4", "m5"))
	if !strings.Contains(res, `"success":true`) {
		t.Fatalf("新增厂商和多个模型应放行: %s", res)
	}

	var got struct {
		Data struct {
			Providers  []config.Provider `json:"providers"`
			Limits     json.RawMessage   `json:"limits"`
			Subscribed json.RawMessage   `json:"subscribed"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(a.ListProviders()), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data.Providers) != 3 || len(got.Data.Providers[2].Models) != 5 {
		t.Fatalf("厂商/模型未完整保存: %+v", got.Data.Providers)
	}
	if got.Data.Limits != nil || got.Data.Subscribed != nil {
		t.Fatalf("厂商列表仍包含付费限制字段")
	}
}

func TestSaveConfigAllowsRecording(t *testing.T) {
	a := newTestApp()
	cfg := *config.Default()
	cfg.RecordEnabled = true
	res := a.SaveConfig(mustJSON(cfg))
	if !strings.Contains(res, `"success":true`) {
		t.Fatalf("free 版应允许启用录制: %s", res)
	}
	var got struct {
		Data config.Config `json:"data"`
	}
	if err := json.Unmarshal([]byte(res), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Data.RecordEnabled {
		t.Fatal("录制配置未保存")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
