//go:build !windows

// Package singleinstance 非 Windows 降级实现：数据目录 lockfile（PID 存活检测）。
package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Instance 持有 lockfile 路径。
type Instance struct{ path string }

// Acquire 尝试成为唯一实例（lockfile PID 存活则拒绝）。
func Acquire() (*Instance, bool) {
	lockDir, err := lockDirPath()
	if err != nil {
		return &Instance{}, true // 目录不可用：放行
	}
	_ = os.MkdirAll(lockDir, 0o755)
	path := filepath.Join(lockDir, "instance.lock")
	if raw, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return nil, false // 存活实例存在（无唤醒通道，调用方直接退出）
				}
			}
		}
	}
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600)
	return &Instance{path: path}, true
}

func lockDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "LLMGatewayProxy"), nil
}

// WaitShowSignal 恒 false（无唤醒通道）。
func (i *Instance) WaitShowSignal() bool { return false }

// Release 删除 lockfile。
func (i *Instance) Release() {
	if i.path != "" {
		_ = os.Remove(i.path)
	}
}
