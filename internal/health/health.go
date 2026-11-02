// Package health 定时探活各厂商上游（模型列表接口，不发消息不花钱）并在状态翻转时通知。
package health

import (
	"context"
	"errors"
	"sync"
	"time"

	"llm-gateway-proxy/internal/i18n"
	"llm-gateway-proxy/internal/provider"
)

// Status 是一个厂商的健康状态。
type Status struct {
	ProviderID   string    `json:"providerId"`
	ProviderName string    `json:"providerName"`
	State        string    `json:"state"` // up | down | unknown（尚未检查）
	LatencyMS    int64     `json:"latencyMs"`
	LastCheck    time.Time `json:"lastCheck"`
	LastError    string    `json:"lastError,omitempty"`
}

// Monitor 是探活循环。
type Monitor struct {
	mu        sync.RWMutex
	states    map[string]*Status
	registry  func() *provider.Registry
	onChange  func(Status)
	interval  time.Duration
	stopCh    chan struct{} // 当前运行代的停止信号；nil = 未运行
}

// NewMonitor 创建（不启动）。
func NewMonitor(registryFn func() *provider.Registry, onChange func(Status)) *Monitor {
	return &Monitor{
		states:   map[string]*Status{},
		registry: registryFn,
		onChange: onChange,
		interval: 5 * time.Minute,
	}
}

// SetInterval 调整间隔（下限 60s）。
func (m *Monitor) SetInterval(d time.Duration) {
	if d < time.Minute {
		d = time.Minute
	}
	m.mu.Lock()
	m.interval = d
	m.mu.Unlock()
}

// Start 启动探活循环（立即执行一次，其后按间隔）。
// 可重复调用：已在运行时为幂等 no-op；Stop 之后可再次 Start（配置热更新）。
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.stopCh != nil {
		m.mu.Unlock()
		return // 已在运行
	}
	stop := make(chan struct{})
	m.stopCh = stop
	m.mu.Unlock()

	go func() {
		m.CheckAll(context.Background())
		for {
			m.mu.RLock()
			d := m.interval
			m.mu.RUnlock()
			select {
			case <-stop:
				return
			case <-time.After(d):
				m.CheckAll(context.Background())
			}
		}
	}()
}

// Stop 停止（幂等，且可再次 Start）。
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}
}

// Running 返回是否在探活循环中。
func (m *Monitor) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopCh != nil
}

// prober 探活接口适配（各 provider 类型的模型拉取能力）。
type prober interface {
	ProbeHealth(ctx context.Context) error
}

// errNoModels 表示探活未能取得模型列表（含网络失败/鉴权失败）。
// 函数式：每次求值按当前语言取词（包级变量会在 init 时固化中文）。
func errNoModels() error { return errors.New(i18n.T("health.noModels")) }

// CheckAll 探活全部启用中的 provider。
func (m *Monitor) CheckAll(ctx context.Context) {
	reg := m.registry()
	if reg == nil {
		return
	}
	for _, p := range reg.Providers() {
		if !p.Enabled() {
			continue
		}
		m.checkOne(ctx, p)
	}
}

func (m *Monitor) checkOne(ctx context.Context, p provider.Provider) {
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var probeErr error
	// 探活方式：全部 provider 走 Models()（gw 拉网关模型、openai/anthropic/gemini 拉厂商模型）
	// Models() 无错误返回，因此以"模型列表非空"判定存活（gw 未登录时 Enabled=false 不会进来）
	if models := p.Models(cctx); len(models) == 0 {
		probeErr = errNoModels()
	}
	latency := time.Since(start).Milliseconds()

	m.mu.Lock()
	st, ok := m.states[p.ID()]
	if !ok {
		st = &Status{ProviderID: p.ID(), ProviderName: p.Name(), State: "unknown"}
		m.states[p.ID()] = st
	}
	prev := st.State
	st.ProviderName = p.Name()
	st.LatencyMS = latency
	st.LastCheck = time.Now()
	if probeErr != nil {
		st.State = "down"
		st.LastError = probeErr.Error()
	} else {
		st.State = "up"
		st.LastError = ""
	}
	flipped := prev != st.State && prev != "unknown"
	snapshot := *st
	m.mu.Unlock()

	if flipped && m.onChange != nil {
		go m.onChange(snapshot)
	}
}

// Snapshot 返回全部状态。
func (m *Monitor) Snapshot() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.states))
	for _, st := range m.states {
		out = append(out, *st)
	}
	return out
}
