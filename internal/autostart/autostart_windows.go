//go:build windows

// Package autostart 管理开机自启（HKCU Run 注册表项，无需管理员权限）。
package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"

	"llm-gateway-proxy/internal/i18n"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "LLMGatewayProxy"

// Enable 写入当前用户 Run 项。
func Enable() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("autostart.registryOpenFailed"), err)
	}
	defer k.Close()
	return k.SetStringValue(valueName, `"`+exe+`"`)
}

// Disable 删除 Run 项（不存在视为成功）。
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("autostart.registryOpenFailed"), err)
	}
	defer k.Close()
	if err := k.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// IsEnabled 查询 Run 项是否存在。
func IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}

func exePath() (string, error) {
	return os.Executable()
}
