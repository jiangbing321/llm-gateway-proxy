// Package stats 聚合代理请求用量（按天 × 厂商 × 模型）并支持计价估算。
package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/paths"
)

// Cell 是一个聚合单元（某天某厂商某模型）。
type Cell struct {
	Date             string `json:"date"` // YYYY-MM-DD
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Requests         int    `json:"requests"`
	Errors           int    `json:"errors"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
}

// PricingRule 是一条计价规则（pattern 支持 * 通配；命中即用，先命中优先）。
type PricingRule struct {
	ProviderPattern string  `json:"providerPattern"`
	ModelPattern    string  `json:"modelPattern"`
	InputPer1M      float64 `json:"inputPer1M"`  // 每 1M input tokens 单价
	OutputPer1M     float64 `json:"outputPer1M"` // 每 1M output tokens 单价
	Note            string  `json:"note"`
}

// Stats 是聚合器（线程安全；内存聚合 + 节流落盘）。
type Stats struct {
	mu      sync.Mutex
	path    string
	cells   map[string]*Cell // key: date|provider|model
	pricing []PricingRule

	dirty     bool
	lastFlush time.Time
}

// New 创建并加载历史数据。
func New() (*Stats, error) {
	dir, err := paths.DataDir()
	if err != nil {
		return nil, err
	}
	s := &Stats{path: filepath.Join(dir, "stats.json"), cells: map[string]*Cell{}}
	s.load()
	return s, nil
}

func (s *Stats) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var persisted struct {
		Cells   []Cell        `json:"cells"`
		Pricing []PricingRule `json:"pricing"`
	}
	if json.Unmarshal(raw, &persisted) != nil {
		return
	}
	for i := range persisted.Cells {
		c := persisted.Cells[i]
		s.cells[c.Date+"|"+c.Provider+"|"+c.Model] = &c
	}
	s.pricing = persisted.Pricing
	if s.pricing == nil {
		s.pricing = defaultPricing()
	}
}

func defaultPricing() []PricingRule {
	return []PricingRule{
		{ProviderPattern: "燎元*", ModelPattern: "*", Note: "企业内网，免费"},
	}
}

// flushLocked 节流落盘（≥5s 间隔或强制）。
func (s *Stats) flushLocked(force bool) {
	if !s.dirty && !force {
		return
	}
	if !force && time.Since(s.lastFlush) < 5*time.Second {
		return
	}
	var cells []Cell
	for _, c := range s.cells {
		cells = append(cells, *c)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Date != cells[j].Date {
			return cells[i].Date < cells[j].Date
		}
		return cells[i].Provider < cells[j].Provider
	})
	raw, err := json.MarshalIndent(map[string]any{"cells": cells, "pricing": s.pricing}, "", " ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, s.path)
	}
	s.dirty, s.lastFlush = false, time.Now()
}

// Record 摄入一条请求日志（由 logbuf 订阅回调）。
func (s *Stats) Record(e logbuf.Entry) {
	if e.Path == "/v1/models" || e.Path == "/health" {
		return // 非对话请求不计数
	}
	date := e.Time.Format("2006-01-02")
	provider := e.Provider
	if provider == "" {
		provider = "未路由"
	}
	model := e.Model
	// 回退/映射标记取首段（→ 之前）
	if i := strings.Index(model, "→"); i > 0 {
		model = model[:i]
	}
	if i := strings.Index(model, " "); i > 0 {
		model = model[:i]
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := date + "|" + provider + "|" + model
	c, ok := s.cells[key]
	if !ok {
		c = &Cell{Date: date, Provider: provider, Model: model}
		s.cells[key] = c
	}
	c.Requests++
	if e.Error != "" || (e.Status >= 400 && e.Status != 429) {
		c.Errors++
	}
	c.PromptTokens += e.PromptTokens
	c.CompletionTokens += e.CompletionTokens
	s.dirty = true
	s.flushLocked(false)
}

// Flush 强制落盘（退出时调用）。
func (s *Stats) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked(true)
}

// Row 是按厂商×模型汇总的一行。
type Row struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Requests         int     `json:"requests"`
	Errors           int     `json:"errors"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
}

// Daily 是按天汇总（含成本）。
type Daily struct {
	Date             string  `json:"date"`
	Requests         int     `json:"requests"`
	Errors           int     `json:"errors"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
}

// Summary 是 GetStats 的返回。
type Summary struct {
	Days  []Daily `json:"days"`
	Rows  []Row   `json:"rows"`
	Total Daily   `json:"total"`
}

// GetStats 返回近 N 天聚合。
func (s *Stats) GetStats(days int) Summary {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	s.mu.Lock()
	defer s.mu.Unlock()

	sum := Summary{Days: []Daily{}, Rows: []Row{}}
	dayMap := map[string]*Daily{}
	rowMap := map[string]*Row{}
	for _, c := range s.cells {
		if c.Date < cutoff {
			continue
		}
		d, ok := dayMap[c.Date]
		if !ok {
			d = &Daily{Date: c.Date}
			dayMap[c.Date] = d
		}
		k := c.Provider + "|" + c.Model
		r, ok := rowMap[k]
		if !ok {
			r = &Row{Provider: c.Provider, Model: c.Model}
			rowMap[k] = r
		}
		cost := s.costLocked(c.Provider, c.Model, c.PromptTokens, c.CompletionTokens)
		for _, tgt := range []struct {
			req, err, pt, ct int
			cost             float64
		}{
			{c.Requests, c.Errors, c.PromptTokens, c.CompletionTokens, cost},
		} {
			d.Requests += tgt.req; d.Errors += tgt.err
			d.PromptTokens += tgt.pt; d.CompletionTokens += tgt.ct
			d.Cost += tgt.cost
			r.Requests += tgt.req; r.Errors += tgt.err
			r.PromptTokens += tgt.pt; r.CompletionTokens += tgt.ct
			r.Cost += tgt.cost
		}
	}
	for _, d := range dayMap {
		sum.Days = append(sum.Days, *d)
	}
	for _, r := range rowMap {
		sum.Rows = append(sum.Rows, *r)
	}
	sort.Slice(sum.Days, func(i, j int) bool { return sum.Days[i].Date < sum.Days[j].Date })
	// 补齐连续日期轴：cutoff..今天 每天一行（无数据的天为零值）——
	// 图表按天连续展示，当天数据始终可见
	{
		var continuous []Daily
		dayIdx := map[string]Daily{}
		for _, d := range sum.Days {
			dayIdx[d.Date] = d
		}
		t, _ := time.ParseInLocation("2006-01-02", cutoff, time.Local)
		end, _ := time.ParseInLocation("2006-01-02", time.Now().Format("2006-01-02"), time.Local)
		for !t.After(end) {
			key := t.Format("2006-01-02")
			if d, ok := dayIdx[key]; ok {
				continuous = append(continuous, d)
			} else {
				continuous = append(continuous, Daily{Date: key})
			}
			t = t.AddDate(0, 0, 1)
		}
		sum.Days = continuous
	}
	sort.Slice(sum.Days, func(i, j int) bool { return sum.Days[i].Date < sum.Days[j].Date })
	sort.Slice(sum.Rows, func(i, j int) bool {
		if sum.Rows[i].Provider != sum.Rows[j].Provider {
			return sum.Rows[i].Provider < sum.Rows[j].Provider
		}
		return sum.Rows[i].Model < sum.Rows[j].Model
	})
	for _, d := range sum.Days {
		sum.Total.Requests += d.Requests
		sum.Total.Errors += d.Errors
		sum.Total.PromptTokens += d.PromptTokens
		sum.Total.CompletionTokens += d.CompletionTokens
		sum.Total.Cost += d.Cost
	}
	return sum
}

// GetPricing 返回计价规则。
func (s *Stats) GetPricing() []PricingRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pricing == nil {
		s.pricing = defaultPricing()
	}
	out := make([]PricingRule, len(s.pricing))
	copy(out, s.pricing)
	return out
}

// SetPricing 更新计价规则并落盘。
func (s *Stats) SetPricing(rules []PricingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pricing = rules
	s.dirty = true
	s.flushLocked(true)
}

// costLocked 按规则估算成本（需持锁调用）。先命中优先；未命中免费（0）。
func (s *Stats) costLocked(provider, model string, prompt, completion int) float64 {
	for _, r := range s.pricing {
		if matchPattern(r.ProviderPattern, provider) && matchPattern(r.ModelPattern, model) {
			return float64(prompt)/1e6*r.InputPer1M + float64(completion)/1e6*r.OutputPer1M
		}
	}
	return 0
}

// matchPattern 简单通配：* 匹配任意串；空 pattern 视为 *。
func matchPattern(pattern, s string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	pos := len(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		i := strings.Index(s[pos:], p)
		if i < 0 {
			return false
		}
		pos += i + len(p)
	}
	if last := parts[len(parts)-1]; last != "" && !strings.HasSuffix(s, last) {
		return false
	}
	return true
}
