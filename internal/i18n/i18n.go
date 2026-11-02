// Package i18n 提供后端用户可见文案的中英文支持。
// 不在翻译范围：日志文件 log.Printf、代码注释、上游厂商返回的错误（原样透传）。
//
// 语言模型：进程内单一当前语言；lang 初值恒 "zh"，不依赖 SetLang 被调用，
// 保证单测与 startup 之前的输出确定（测试永远拿到中文）。
package i18n

import (
	"fmt"
	"sync"
)

var (
	mu   sync.RWMutex
	lang = "zh"
)

// SetLang 设置当前语言。"" 或非法值 = 跟随系统（Windows 用户 UI 语言）。并发安全。
func SetLang(l string) {
	if l != "zh" && l != "en" {
		l = detectSystemLang()
	}
	mu.Lock()
	lang = l
	mu.Unlock()
}

// Lang 返回当前生效语言（"zh" | "en"）。
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// T 取当前语言消息；args 非空时按 fmt.Sprintf 语义填充。
// 回退链：当前语言 → zh → key 本身（开发期漏键可见）。
// 注意：调用方禁止 fmt.Errorf(T(...))（go vet 报 non-constant format string），
// 应写 errors.New(T(...)) 或 fmt.Errorf("%s: %w", T(...), err)。
func T(key string, args ...any) string {
	mu.RLock()
	l := lang
	mu.RUnlock()
	msg, ok := messages[l][key]
	if !ok {
		msg, ok = messages["zh"][key]
	}
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}
