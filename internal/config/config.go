// Package config 管理代理应用的持久化配置。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"llm-gateway-proxy/internal/dpapi"
	"llm-gateway-proxy/internal/i18n"
	"llm-gateway-proxy/internal/paths"
)

// Config 是应用的全部可持久化配置。
type Config struct {
	// 代理
	ProxyEnabled bool   `json:"proxyEnabled"` // 启动时是否自动开启代理
	AutoStart    bool   `json:"autoStart"`    // 记录用；实际状态以注册表为准（SetAutoStart 同步写）
	Port         int    `json:"port"`         // 默认 8717
	Bind         string `json:"bind"`         // 监听地址：127.0.0.1（默认，仅本机）或 0.0.0.0（WSL/局域网可访问）
	LocalAPIKey  string `json:"localApiKey"`  // 非空时本地代理要求此 key（x-api-key 或 Bearer）
	// 消息录制
	RecordEnabled       bool   `json:"recordEnabled"`        // 开启后录制与网关来往的完整消息
	RecordDir           string `json:"recordDir"`            // 录制目录（空 = 数据目录下 recordings）
	RecordRetentionDays int    `json:"recordRetentionDays"`  // 录制文件保留天数（启动时自动清理更早的；默认 30，上限 3650≈永久）
	// 多厂商上游
	Providers         []Provider `json:"providers"`         // 用户配置的厂商
	DefaultProviderID string     `json:"defaultProviderId"` // 未命中模型名时的默认路由（空 = 无默认，未命中直接 404）
	// 健康监控
	HealthEnabled    bool `json:"healthEnabled"`    // 定时探活各厂商
	HealthIntervalSec int `json:"healthIntervalSec"` // 探活间隔秒（下限 60）
	// UI 偏好
	Language string `json:"language"` // "" = 跟随系统 | zh | en（托盘/通知/错误文案语言）
}

// Provider 是一个用户配置的厂商上游。
type Provider struct {
	ID           string            `json:"id"`                     // prov_ 前缀
	Name         string            `json:"name"`
	Type         string            `json:"type"`    // openai | anthropic
	BaseURL      string            `json:"baseUrl"`
	APIKey       string            `json:"apiKey"`
	Enabled      bool              `json:"enabled"`
	Models       []string          `json:"models"`                 // 可路由的模型名
	Note         string            `json:"note"`
	MaxTokens    int               `json:"maxTokens"`              // O→A 转换缺省 max_tokens（默认 8192）
	ModelMapping map[string]string `json:"modelMapping,omitempty"` // 下游别名 → 上游真名
	Fallbacks    []string          `json:"fallbacks,omitempty"`    // 本厂商失败时依次尝试的模型（经正常路由解析）
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		ProxyEnabled: true,
		Port:         8717,
		Bind:         "127.0.0.1",
		LocalAPIKey:  "",
	}
}

// Normalize 补齐空字段为默认值。
func (c *Config) Normalize() {
	d := Default()
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = d.Port
	}
	if c.Bind != "0.0.0.0" {
		c.Bind = "127.0.0.1" // 仅接受两个显式值
	}
	if c.RecordDir == "" {
		if dir, err := paths.DataDir(); err == nil {
			c.RecordDir = filepath.Join(dir, "recordings")
		}
	}
	if c.RecordRetentionDays < 1 || c.RecordRetentionDays > 3650 {
		c.RecordRetentionDays = 30
	}
	if c.HealthIntervalSec < 60 {
		c.HealthIntervalSec = 300
	}
	// 语言白名单：非法值归 ""（跟随系统，由启动时的系统检测接管）而非强回中文
	if c.Language != "zh" && c.Language != "en" {
		c.Language = ""
	}
	// 校验 provider：id 唯一、类型合法
	seen := map[string]bool{}
	valid := map[string]bool{"openai": true, "anthropic": true, "gemini": true}
	kept := c.Providers[:0]
	for _, p := range c.Providers {
		if p.ID == "" || seen[p.ID] || !valid[p.Type] {
			continue
		}
		seen[p.ID] = true
		kept = append(kept, p)
	}
	c.Providers = kept
	// 空为合法稳态（无默认路由）；旧版 "gw"（燎元内置厂商）或指向已删厂商时静默清除
	if c.DefaultProviderID == "gw" || !seen[c.DefaultProviderID] {
		c.DefaultProviderID = ""
	}
}

// Manager 提供线程安全的配置读写。
type Manager struct {
	mu   sync.RWMutex
	cfg  *Config
	path string
}

// NewInMemoryDefault 返回不落盘的默认配置管理器（数据目录不可写时的兜底）。
func NewInMemoryDefault() (*Manager, error) {
	cfg := Default()
	return &Manager{cfg: cfg}, nil
}

// NewManager 加载（或初始化默认）配置。
// apiKey 类字段：落盘为 DPAPI 密文（"dpapi:" 前缀），内存中始终为明文；
// 首次读到明文（旧版本配置）时立即回写完成迁移。
func NewManager() (*Manager, error) {
	dir, err := paths.DataDir()
	if err != nil {
		return nil, err
	}
	m := &Manager{path: filepath.Join(dir, "config.json")}
	cfg := Default()
	if raw, err := os.ReadFile(m.path); err == nil {
		if err := json.Unmarshal(raw, cfg); err != nil {
			cfg = Default() // 损坏则重置
		}
	}
	cfg.Normalize()

	// 解密敏感字段；发现明文则标记待回写迁移
	needMigrate := false
	if cfg.LocalAPIKey != "" {
		if dpapi.IsEncrypted(cfg.LocalAPIKey) {
			cfg.LocalAPIKey = dpapi.DecryptString(cfg.LocalAPIKey)
		} else {
			needMigrate = true
		}
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].APIKey != "" {
			if dpapi.IsEncrypted(cfg.Providers[i].APIKey) {
				cfg.Providers[i].APIKey = dpapi.DecryptString(cfg.Providers[i].APIKey)
			} else {
				needMigrate = true
			}
		}
	}
	m.cfg = cfg
	_ = m.saveLocked()
	_ = needMigrate
	return m, nil
}

// Get 返回配置副本。
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}

// Update 应用修改并落盘（原子写）。
func (m *Manager) Update(fn func(*Config)) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m.cfg)
	m.cfg.Normalize()
	if err := m.saveLocked(); err != nil {
		return *m.cfg, err
	}
	return *m.cfg, nil
}

// saveLocked 落盘（原子写）。敏感字段加密后写入，不影响内存副本。
// path 为空（内存态管理器）时跳过。
func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	out := *m.cfg
	out.LocalAPIKey = mustEncrypt(m.cfg.LocalAPIKey)
	out.Providers = make([]Provider, len(m.cfg.Providers))
	for i, p := range m.cfg.Providers {
		out.Providers[i] = p
		out.Providers[i].APIKey = mustEncrypt(p.APIKey)
	}
	raw, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("config.writeFailed"), err)
	}
	return os.Rename(tmp, m.path)
}

// mustEncrypt 加密失败时退回明文（保证配置仍可保存，不锁死应用）。
func mustEncrypt(s string) string {
	if s == "" || dpapi.IsEncrypted(s) {
		return s
	}
	enc, err := dpapi.EncryptString(s)
	if err != nil {
		return s
	}
	return enc
}
