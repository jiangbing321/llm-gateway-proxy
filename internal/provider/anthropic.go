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

// AnthropicVersion 是 Anthropic API 协议版本头。
const AnthropicVersion = "2023-06-01"

// AnthropicProvider 是 Anthropic 上游（官方或兼容端点）。
type AnthropicProvider struct {
	cfg config.Provider
}

// NewAnthropicProvider 创建。
func NewAnthropicProvider(cfg config.Provider) *AnthropicProvider {
	return &AnthropicProvider{cfg: cfg}
}

func (p *AnthropicProvider) ID() string         { return p.cfg.ID }
func (p *AnthropicProvider) Name() string       { return p.cfg.Name }
func (p *AnthropicProvider) Type() string       { return "anthropic" }
func (p *AnthropicProvider) Enabled() bool      { return p.cfg.Enabled }
func (p *AnthropicProvider) Protocol() Protocol { return ProtocolAnthropic }

// StaticModels 返回配置的静态模型列表（含映射别名）。
func (p *AnthropicProvider) StaticModels() []ModelInfo {
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
func (p *AnthropicProvider) Models(ctx context.Context) []ModelInfo {
	return p.StaticModels()
}

// ModelMapping 返回别名映射。
func (p *AnthropicProvider) ModelMapping() map[string]string { return p.cfg.ModelMapping }

// Fallbacks 返回失败回退模型链。
func (p *AnthropicProvider) Fallbacks() []string { return p.cfg.Fallbacks }

// APIKey 返回密钥。
func (p *AnthropicProvider) APIKey() string { return p.cfg.APIKey }

// BaseURL 返回端点。
func (p *AnthropicProvider) BaseURL() string { return p.cfg.BaseURL }

// Do 转发 Anthropic 格式请求到 baseURL/v1/messages。
func (p *AnthropicProvider) Do(ctx context.Context, model string, body []byte, stream bool) (*UpstreamResponse, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if p.cfg.APIKey != "" {
		req.Header.Set("x-api-key", p.cfg.APIKey)
		// 兼容部分中转站用 Bearer 的部署习惯：同时带 Authorization
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	req.Header.Set("anthropic-version", AnthropicVersion)
	req.Header.Del("Accept-Encoding")
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("provider.requestFailed"), err)
	}
	return &UpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}

// FetchModels 拉取模型列表（Anthropic 官方 /v1/models{data:[{id}]}；不支持的兼容站返回错误，用户手填即可）。
func (p *AnthropicProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("x-api-key", p.cfg.APIKey)
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	req.Header.Set("anthropic-version", AnthropicVersion)
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
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		out = append(out, ModelInfo{ID: m.ID})
	}
	return out, nil
}
