//go:build !windows

// Package dpapi 非 Windows 平台的恒等实现（不加密，明文存取）。
package dpapi

// Prefix 恒等实现下无密文形态。
const Prefix = ""

// Protect 原样返回。
func Protect(data []byte) ([]byte, error) { return data, nil }

// Unprotect 原样返回。
func Unprotect(data []byte) ([]byte, error) { return data, nil }

// EncryptString 原样返回。
func EncryptString(s string) (string, error) { return s, nil }

// DecryptString 原样返回。
func DecryptString(s string) string { return s }

// IsEncrypted 恒 false。
func IsEncrypted(s string) bool { return false }
