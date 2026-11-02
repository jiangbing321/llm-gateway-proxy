//go:build windows

package i18n

import "syscall"

var procGetUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// detectSystemLang 返回系统 UI 语言（中文→zh，其余→en）。
// LANGID 的主语言位 0x04（LANG_CHINESE）覆盖 zh-CN/TW/HK/SG 全部变体。
func detectSystemLang() string {
	r1, _, _ := procGetUserDefaultUILanguage.Call()
	if r1&0x3FF == 0x04 {
		return "zh"
	}
	return "en"
}
