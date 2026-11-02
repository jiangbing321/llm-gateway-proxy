package dpapi

import (
	"strings"
	"testing"
)

// TestEncryptDecryptString 验证字符串加解密 roundtrip 与前缀形态（Windows DPAPI 实测）。
func TestEncryptDecryptString(t *testing.T) {
	in := "sk-test-12345-中文密钥"
	enc, err := EncryptString(in)
	if err != nil {
		t.Skipf("DPAPI 不可用: %v", err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("密文应带 %q 前缀: %s", Prefix, enc)
	}
	if strings.Contains(enc, in) {
		t.Fatal("密文包含明文！")
	}
	if got := DecryptString(enc); got != in {
		t.Fatalf("roundtrip 错误: %q", got)
	}
}

// TestDecryptPlaintextPassthrough 无前缀明文应原样返回（旧配置兼容）。
func TestDecryptPlaintextPassthrough(t *testing.T) {
	if got := DecryptString("sk-plain"); got != "sk-plain" {
		t.Fatalf("明文应透传: %q", got)
	}
	if enc, _ := EncryptString(""); enc != "" {
		t.Fatal("空串应返回空")
	}
}
