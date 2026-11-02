//go:build windows

// Package dpapi 封装 Windows DPAPI（当前用户域）加解密，用于敏感字段落盘。
package dpapi

import (
	"encoding/base64"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"llm-gateway-proxy/internal/i18n"
)

// Prefix 是密文值的标记前缀（"dpapi:"+base64(nonceless blob)）。
const Prefix = "dpapi:"

type dataBlob struct {
	cbData uint32
	pbData *byte
}

// Protect 加密（CryptProtectData，绑定当前用户）。
func Protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("dpapi.emptyData"))
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	r, _, _ := syscall.NewLazyDLL("crypt32.dll").NewProc("CryptProtectData").
		Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("%s", i18n.T("dpapi.protectFailed"))
	}
	return copyOutBlob(out)
}

// Unprotect 解密（CryptUnprotectData）。
func Unprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("dpapi.emptyData"))
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	r, _, _ := syscall.NewLazyDLL("crypt32.dll").NewProc("CryptUnprotectData").
		Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("%s", i18n.T("dpapi.unprotectFailed"))
	}
	return copyOutBlob(out)
}

// copyOutBlob 先拷贝 DPAPI 输出再 LocalFree——
// 直接 unsafe.Slice 引用后释放是 use-after-free，落盘数据会损坏。
func copyOutBlob(out dataBlob) ([]byte, error) {
	if out.pbData == nil || out.cbData == 0 {
		return nil, fmt.Errorf("%s", i18n.T("dpapi.emptyOutput"))
	}
	blob := make([]byte, out.cbData)
	copy(blob, unsafe.Slice(out.pbData, out.cbData))
	windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.pbData))))
	return blob, nil
}

// EncryptString 加密字符串并编码为 "dpapi:<base64>"。
func EncryptString(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	blob, err := Protect([]byte(s))
	if err != nil {
		return "", err
	}
	return Prefix + base64.StdEncoding.EncodeToString(blob), nil
}

// DecryptString 解密 "dpapi:<base64>"；无前缀视为明文原样返回（兼容旧配置）。
// 解密失败返回明文原文（保持可用，避免锁死配置）。
func DecryptString(s string) string {
	if !strings.HasPrefix(s, Prefix) {
		return s
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, Prefix))
	if err != nil {
		return s
	}
	plain, err := Unprotect(raw)
	if err != nil {
		return s
	}
	return string(plain)
}

// IsEncrypted 判断值是否已为密文形态。
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, Prefix)
}
