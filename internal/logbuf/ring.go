// Package logbuf 提供固定容量的请求日志环形缓冲与订阅推送。
package logbuf

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Entry 是一条代理请求日志。
type Entry struct {
	ID               int64     `json:"id"`
	Time             time.Time `json:"time"`
	Path             string    `json:"path"`     // /v1/chat/completions、/v1/messages 等
	Provider         string    `json:"provider"` // 路由到的厂商名称
	Model            string    `json:"model"`    // 请求体里的 model（映射改写后含 →）
	Status           int       `json:"status"`   // 上游响应码
	DurationMS       int64     `json:"durationMs"`
	Stream           bool      `json:"stream"`
	PromptTokens     int       `json:"promptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	Error            string    `json:"error,omitempty"`
	ClientIP         string    `json:"clientIp"`
	Transform        string    `json:"transform,omitempty"` // 协议转换标记
}

// Ring 是容量固定的环形缓冲（默认 200 条），支持订阅与可选文件落盘。
type Ring struct {
	mu      sync.Mutex
	entries []Entry
	nextID  int64
	subs    []func(Entry)
	logFile *rotatingWriter
}

// NewRing 创建环形缓冲。
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		capacity = 200
	}
	return &Ring{entries: make([]Entry, 0, capacity)}
}

// RotateFile 在文件超过 maxBytes 时轮转：重命名为 path+".old"（覆盖旧 .old）。
// 供 app.log / proxy-access.log 在启动时清理上次运行的遗留增长；
// 运行中的轮转由 rotatingWriter 在写入路径上完成。
func RotateFile(path string, maxBytes int64) bool {
	if maxBytes <= 0 {
		return false
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() <= maxBytes {
		return false
	}
	_ = os.Remove(path + ".old")
	return os.Rename(path, path+".old") == nil
}

// rotatingWriter 是带大小上限的追加写文件：累计写入超过 maxBytes 时轮转
// （当前文件 → path.old，重新开新文件）。托盘常驻数周不重启也能保持有界。
type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	f        *os.File
	written  int64 // 当前文件累计写入
}

// OpenRotatingLog 打开带大小轮转的追加写日志文件，可直接交给 log.SetOutput。
func OpenRotatingLog(path string, maxBytes int64) (io.WriteCloser, error) {
	return openRotatingWriter(path, maxBytes)
}

func openRotatingWriter(path string, maxBytes int64) (*rotatingWriter, error) {
	w := &rotatingWriter{path: path, maxBytes: maxBytes}
	if err := w.reopen(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) reopen() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	w.f = f
	w.written = 0
	if st, err := f.Stat(); err == nil {
		w.written = st.Size()
	}
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil { // 上次轮转重开失败（如磁盘满），下次写入重试
		if err := w.reopen(); err != nil {
			return 0, err
		}
	}
	if w.maxBytes > 0 && w.written+int64(len(p)) > w.maxBytes {
		w.rotateLocked()
	}
	n, err := w.f.Write(p)
	w.written += int64(n)
	return n, err
}

// rotateLocked 关闭当前文件并轮转；重开失败时置 f=nil（Write 会重试）。
func (w *rotatingWriter) rotateLocked() {
	_ = w.f.Close()
	w.f = nil
	_ = os.Remove(w.path + ".old")
	if err := os.Rename(w.path, w.path+".old"); err != nil {
		return // 轮转失败（极少见）：继续写原句柄侧的新开文件
	}
	_ = w.reopen()
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// EnableFileLog 开启 JSON 行落盘（追加写，超 maxBytes 自动轮转，便于外部排查客户端行为）。
func (r *Ring) EnableFileLog(path string, maxBytes int64) error {
	w, err := openRotatingWriter(path, maxBytes)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logFile = w
	return nil
}

// CloseFileLog 关闭落盘文件。
func (r *Ring) CloseFileLog() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.logFile != nil {
		_ = r.logFile.Close()
		r.logFile = nil
	}
}

// Add 追加一条日志并通知订阅者。
func (r *Ring) Add(e Entry) {
	r.mu.Lock()
		r.nextID++
		e.ID = r.nextID
		if len(r.entries) < cap(r.entries) {
			r.entries = append(r.entries, e)
		} else {
			r.entries = append(r.entries[1:], e)
		}
		subs := make([]func(Entry), len(r.subs))
		copy(subs, r.subs)
		if w := r.logFile; w != nil {
			if raw, err := json.Marshal(e); err == nil {
				_, _ = w.Write(append(raw, '\n')) // 超限自动轮转
			}
		}
	r.mu.Unlock()

	for _, fn := range subs {
		go fn(e) // 订阅回调（EventsEmit）不阻塞请求路径
	}
}

// Snapshot 返回缓冲副本（旧→新）。
func (r *Ring) Snapshot() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Clear 清空。
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = r.entries[:0]
}

// Subscribe 注册订阅回调。
func (r *Ring) Subscribe(fn func(Entry)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs = append(r.subs, fn)
}
