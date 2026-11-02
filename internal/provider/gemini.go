package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/i18n"
)

// GeminiProvider 是 Google AI Studio（Gemini）上游。
type GeminiProvider struct {
	cfg config.Provider
}

// NewGeminiProvider 创建。
func NewGeminiProvider(cfg config.Provider) *GeminiProvider {
	return &GeminiProvider{cfg: cfg}
}

func (p *GeminiProvider) ID() string         { return p.cfg.ID }
func (p *GeminiProvider) Name() string       { return p.cfg.Name }
func (p *GeminiProvider) Type() string       { return "gemini" }
func (p *GeminiProvider) Enabled() bool      { return p.cfg.Enabled }
func (p *GeminiProvider) Protocol() Protocol { return ProtocolGemini }

// StaticModels 返回配置的静态模型列表（含映射别名）。
func (p *GeminiProvider) StaticModels() []ModelInfo {
	out := make([]ModelInfo, 0, len(p.cfg.Models))
	for _, m := range p.cfg.Models {
		out = append(out, ModelInfo{ID: m})
	}
	for alias := range p.cfg.ModelMapping {
		out = append(out, ModelInfo{ID: alias})
	}
	return out
}

// Models 同 StaticModels。
func (p *GeminiProvider) Models(ctx context.Context) []ModelInfo {
	return p.StaticModels()
}

// ModelMapping 返回别名映射。
func (p *GeminiProvider) ModelMapping() map[string]string { return p.cfg.ModelMapping }

// Fallbacks 返回失败回退模型链。
func (p *GeminiProvider) Fallbacks() []string { return p.cfg.Fallbacks }

// APIKey 返回密钥。
func (p *GeminiProvider) APIKey() string { return p.cfg.APIKey }

// BaseURL 返回端点。
func (p *GeminiProvider) BaseURL() string { return p.cfg.BaseURL }

// Do 转发 Gemini 格式请求到 {base}/v1beta/models/{model}:generateContent（或 :streamGenerateContent?alt=sse）。
// 与 openai/anthropic 不同：端点路径内嵌模型名。
func (p *GeminiProvider) Do(ctx context.Context, model string, body []byte, stream bool) (*UpstreamResponse, error) {
	method := "generateContent"
	if stream {
		method = "streamGenerateContent?alt=sse"
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1beta/models/" + model + ":" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("x-goog-api-key", p.cfg.APIKey)
	}
	req.Header.Del("Accept-Encoding")
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("provider.requestFailed"), err)
	}
	return &UpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}

// FetchModels 拉取模型列表（{models:[{name:"models/gemini-..."}]}，去 models/ 前缀）。
func (p *GeminiProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1beta/models?pageSize=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("x-goog-api-key", p.cfg.APIKey)
	}
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id != "" {
			out = append(out, ModelInfo{ID: id})
		}
	}
	return out, nil
}
