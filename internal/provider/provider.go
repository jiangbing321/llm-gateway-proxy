// Package provider 抽象多厂商 LLM 上游：OpenAI 兼容、Anthropic、Gemini。
// 代理层按模型名路由到具体 Provider，协议转换在调用 Do 之前完成。
package provider

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/i18n"
)

// Protocol 是上游/客户端说的协议类型。
type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
	ProtocolGemini    Protocol = "gemini"
	// ProtocolResponses 是 OpenAI Responses API 客户端协议（仅作为下游端点协议，
	// 不存在 responses 类型的上游）。
	ProtocolResponses Protocol = "responses"
)

// ModelInfo 是一个可路由的模型。
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Type        string `json:"type,omitempty"` // general/vision
}

// UpstreamResponse 是 provider.Do 的统一返回（非流式与流式共用 Reader 形态）。
type UpstreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// Provider 是一个上游厂商。
type Provider interface {
	ID() string
	Name() string
	Type() string  // openai | anthropic | gemini
	Enabled() bool
	Protocol() Protocol // 上游原生协议

	// Models 返回模型列表（读配置）。
	Models(ctx context.Context) []ModelInfo

	// Do 执行一次对话请求。body 必须已是本 provider 原生协议格式，
	// model 为路由后（含映射改写）的目标模型名。
	Do(ctx context.Context, model string, body []byte, stream bool) (*UpstreamResponse, error)
}

// Registry 管理全部 provider 与模型路由表。
type Registry struct {
	mu          sync.RWMutex
	providers   []Provider
	route       map[string]Provider // 模型名 → provider（冲突时先注册者优先）
	defaultProv Provider
	conflicts   []string // 路由冲突记录（"模型名：A 优先，B 被忽略"）
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{route: map[string]Provider{}}
}

// Rebuild 按配置重建路由表。
func (r *Registry) Rebuild(cfgs []config.Provider, defaultID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers = nil
	r.route = map[string]Provider{}
	r.conflicts = nil
	for _, c := range cfgs {
		if !c.Enabled || c.ID == "" {
			continue
		}
		var p Provider
		switch c.Type {
		case "openai":
			p = NewOpenAIProvider(c)
		case "anthropic":
			p = NewAnthropicProvider(c)
		case "gemini":
			p = NewGeminiProvider(c)
		default:
			continue
		}
		r.providers = append(r.providers, p)
	}

	// 构建路由表：启用的 provider 的模型名（含映射别名）→ provider。
	// 冲突语义：先注册者优先（按配置顺序），后来者忽略并记录。
	for _, p := range r.providers {
		if !p.Enabled() {
			continue
		}
		sm, ok := p.(staticModelser)
		if !ok {
			continue
		}
		for _, m := range sm.StaticModels() {
			if owner, dup := r.route[m.ID]; dup {
				r.conflicts = append(r.conflicts,
					i18n.T("provider.conflict", m.ID, owner.Name(), p.Name(), owner.Name()))
				continue
			}
			r.route[m.ID] = p
		}
	}

	// 默认 provider（defaultID 为空 = 无默认，未命中的模型名直接 404）
	r.defaultProv = nil
	if defaultID != "" {
		for _, p := range r.providers {
			if p.ID() == defaultID {
				r.defaultProv = p
				break
			}
		}
	}
}

// staticModelser 是接口的可选扩展（配置型 provider 返回静态列表）。
type staticModelser interface {
	StaticModels() []ModelInfo
}

// Resolve 按模型名解析 provider，返回 (provider, 实际模型名, 是否命中)。
//
// 支持显式前缀语法消歧（不同厂商存在同名模型 id 时）：
//   "厂商ID/模型名" 或 "厂商名称/模型名"   例： "prov_123/deepseek-chat"、 "DeepSeek/deepseek-chat"
// 前缀不匹配任何厂商时视为普通模型名（兼容 openrouter 风格的 "org/model" id）。
// 未命中路由表时走默认 provider（model 原样传递）。
func (r *Registry) Resolve(model string) (Provider, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 显式前缀：provider id 或名称精确匹配
	if i := strings.Index(model, "/"); i > 0 {
		prefix, rest := model[:i], model[i+1:]
		if rest != "" {
			for _, p := range r.providers {
				if p.Enabled() && (p.ID() == prefix || p.Name() == prefix) {
					return p, rest, true
				}
			}
		}
	}

	if p, ok := r.route[model]; ok && p.Enabled() {
		return p, model, true
	}
	if r.defaultProv != nil && r.defaultProv.Enabled() {
		return r.defaultProv, model, true
	}
	return nil, "", false
}

// Conflicts 返回路由冲突记录（构建时检测）。
func (r *Registry) Conflicts() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.conflicts))
	copy(out, r.conflicts)
	return out
}

// Providers 返回全部 provider 副本。
func (r *Registry) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

// Find 按 id 查找。
func (r *Registry) Find(id string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

// KnownModels 返回路由表中全部模型名（提示用）。
func (r *Registry) KnownModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.route))
	for m := range r.route {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

