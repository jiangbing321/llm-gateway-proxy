// Package paths 集中管理本应用的文件系统路径。
package paths

import (
	"os"
	"path/filepath"
)

// AppName 是本应用的数据目录名（%APPDATA%\<AppName>）。
const AppName = "LLMGatewayProxy"

// DataDir 返回本应用数据目录（不存在则创建）。
func DataDir() (string, error) {
	if dir, err := dataDirOS(); err == nil {
		p := filepath.Join(dir, AppName)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return "", err
		}
		return p, nil
	} else {
		return "", err
	}
}

func dataDirOS() (string, error) {
	if v := os.Getenv("APPDATA"); v != "" { // Windows
		return v, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		if h := os.Getenv("XDG_DATA_HOME"); h != "" {
			return h, nil
		}
		return filepath.Join(home, ".local", "share"), nil
	} else {
		return "", err
	}
}
