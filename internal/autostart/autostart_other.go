//go:build !windows

// Package autostart 非 Windows 降级实现（Xdg autostart .desktop 文件）。
package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopName = "llm-gateway-proxy.desktop"

func desktopPath() (string, error) {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfg = filepath.Join(home, ".config")
	}
	return filepath.Join(cfg, "autostart", desktopName), nil
}

// Enable 写入 .desktop 自启项。
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	p, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=LLM Gateway Proxy\nExec=%s\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", exe)
	return os.WriteFile(p, []byte(content), 0o644)
}

// Disable 删除 .desktop 自启项。
func Disable() error {
	p, err := desktopPath()
	if err != nil {
		return nil
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsEnabled 查询 .desktop 是否存在。
func IsEnabled() bool {
	p, err := desktopPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
