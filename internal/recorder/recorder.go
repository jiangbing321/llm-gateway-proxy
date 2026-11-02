// Package recorder 把与 LLM 网关来往的完整消息（请求与响应原文，含 SSE 流）录制为 JSONL 文件。
package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry 是一条录制记录（JSONL 中的一行）。
type Entry struct {
	Time   time.Time `json:"ts"`
	Dir    string    `json:"dir"`      // "req"（发往上游）| "resp"（上游返回）
	Model  string    `json:"model"`    // 请求体中的 model（映射改写后含 →）
	Status int       `json:"status"`   // resp 时的 HTTP 状态码
	Stream bool      `json:"stream"`   // resp 时是否 SSE 流式
	Path   string    `json:"path"`     // 代理端点（/v1/chat/completions、/v1/messages）
	Body   string    `json:"body"`     // 原文（请求 JSON / 响应全文或完整 SSE 流）
	Transform string `json:"transform,omitempty"` // 协议转换标记（openai2anthropic / anthropic2openai）
}

// Recorder 线程安全的 JSONL 录制器，按天滚动文件。
type Recorder struct {
	mu      sync.Mutex
	enabled bool
	dir     string
	day     string
	file    *os.File
}

// New 创建录制器（默认关闭）。
func New() *Recorder {
	return &Recorder{}
}

// Configure 运行时更新开关与目录；目录变化时关闭旧文件句柄。
func (r *Recorder) Configure(enabled bool, dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
	if dir != r.dir {
		r.closeLocked()
		r.dir = dir
	}
	if r.enabled && r.dir != "" {
		_ = os.MkdirAll(r.dir, 0o755)
	}
}

// Enabled 返回开关状态。
func (r *Recorder) Enabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

// CurrentFile 返回当天录制文件路径（无论是否开启）。
func (r *Recorder) CurrentFile() string {
	r.mu.Lock()
	dir := r.dir
	r.mu.Unlock()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, fileNameFor(time.Now()))
}

// Close 关闭文件句柄。
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeLocked()
}

// Write 追加一条记录；未开启时为空操作。
func (r *Recorder) Write(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled || r.dir == "" {
		return
	}
	today := fileNameFor(e.Time)
	if r.file == nil || today != r.day {
		r.closeLocked()
		if err := os.MkdirAll(r.dir, 0o755); err != nil {
			return
		}
		f, err := os.OpenFile(filepath.Join(r.dir, today),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		r.file, r.day = f, today
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = r.file.Write(append(raw, '\n'))
}

func (r *Recorder) closeLocked() {
	if r.file != nil {
		_ = r.file.Close()
		r.file, r.day = nil, ""
	}
}

// Delete 删除指定日期（YYYYMMDD）的录制文件。
// 若恰为当前打开的当天文件，先在锁内关闭句柄再删——Windows 上被句柄占用的
// 文件无法删除（sharing violation），且锁内执行可防并发 Write 在关闭后、删除前
// 重建文件。删除后继续录制时 Write 会自动重建同名文件，从头记录。
// 注意 r.day 存的是完整文件名（fileNameFor 的返回值），非纯日期。
func (r *Recorder) Delete(day string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.day == "llm-record-"+day+".jsonl" {
		r.closeLocked()
	}
	return os.Remove(filepath.Join(r.dir, "llm-record-"+day+".jsonl"))
}

// fileNameFor 生成按天滚动的文件名，如 llm-record-20260923.jsonl。
func fileNameFor(t time.Time) string {
	return fmt.Sprintf("llm-record-%s.jsonl", t.Format("20060102"))
}

// CleanOld 删除超过 keepDays 天的录制文件，返回被删除的文件名（按日期升序）。
// 文件名即录制日期（YYYYMMDD），直接字符串比较即可判断新旧。
func CleanOld(dir string, keepDays int) []string {
	if dir == "" || keepDays < 1 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("20060102")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var removed []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "llm-record-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(name, "llm-record-"), ".jsonl")
		if len(date) != 8 {
			continue // 非日期命名文件不动
		}
		if _, err := time.Parse("20060102", date); err != nil {
			continue
		}
		if date < cutoff {
			if os.Remove(filepath.Join(dir, name)) == nil {
				removed = append(removed, name)
			}
		}
	}
	return removed
}
