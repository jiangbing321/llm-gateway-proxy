package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/i18n"
)

// sharedHTTP 是公网厂商共用的客户端（标准 CA；不设整体 Timeout——SSE 不能截断）。
var sharedHTTP = &http.Client{
	Transport: &http.Transport{
		ForceAttemptHTTP2:   true,
		ReadBufferSize:      64 << 10,
		MaxIdleConns:        32,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	},
}

// OpenAIProvider 是 OpenAI 兼容上游（DeepSeek/Moonshot/智谱/OpenAI 官方等）。
type OpenAIProvider struct {
	cfg config.Provider
}

// NewOpenAIProvider 创建。
func NewOpenAIProvider(cfg config.Provider) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg}
}

func (p *OpenAIProvider) ID() string        { return p.cfg.ID }
func (p *OpenAIProvider) Name() string      { return p.cfg.Name }
func (p *OpenAIProvider) Type() string      { return "openai" }
func (p *OpenAIProvider) Enabled() bool     { return p.cfg.Enabled }
func (p *OpenAIProvider) Protocol() Protocol { return ProtocolOpenAI }

// StaticModels 返回配置的静态模型列表（含映射别名）。
func (p *OpenAIProvider) StaticModels() []ModelInfo {
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
func (p *OpenAIProvider) Models(ctx context.Context) []ModelInfo {
	return p.StaticModels()
}

// ModelMapping 返回别名映射。
func (p *OpenAIProvider) ModelMapping() map[string]string { return p.cfg.ModelMapping }

// Fallbacks 返回失败回退模型链。
func (p *OpenAIProvider) Fallbacks() []string { return p.cfg.Fallbacks }

// MaxTokensDefault 是 O→A 转换时 max_tokens 缺省值。
func (p *OpenAIProvider) MaxTokensDefault() int {
	if p.cfg.MaxTokens > 0 {
		return p.cfg.MaxTokens
	}
	return 8192
}

// APIKey 返回密钥（TestProvider 用）。
func (p *OpenAIProvider) APIKey() string { return p.cfg.APIKey }

// BaseURL 返回端点。
func (p *OpenAIProvider) BaseURL() string { return p.cfg.BaseURL }

// Do 转发 OpenAI 格式请求到 baseURL/v1/chat/completions。
func (p *OpenAIProvider) Do(ctx context.Context, model string, body []byte, stream bool) (*UpstreamResponse, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	req.Header.Del("Accept-Encoding")
	resp, err := sharedHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("provider.requestFailed"), err)
	}
	return &UpstreamResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}, nil
}

// FetchModels 拉取厂商 /v1/models（表单"从厂商拉取"按钮）。
func (p *OpenAIProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
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

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
