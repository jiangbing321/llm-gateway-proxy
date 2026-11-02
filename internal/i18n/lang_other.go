//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// detectSystemLang 非 Windows 平台的可编译兜底：读 LANG 环境变量前缀。
func detectSystemLang() string {
	if strings.HasPrefix(strings.ToLower(os.Getenv("LANG")), "zh") {
		return "zh"
	}
	return "en"
}
