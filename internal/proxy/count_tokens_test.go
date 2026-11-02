package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-gateway-proxy/internal/logbuf"
)

// TestEstimateTokens 验证启发式估算：CJK 约 1 token/字，ASCII 约 4 字符/token。
func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("空串应为 0，got %d", got)
	}
	// 8 个 ASCII 字符 → ceil(8/4) = 2
	if got := estimateTokens("abcdefgh"); got != 2 {
		t.Fatalf("8 ASCII 应为 2，got %d", got)
	}
	// 4 个 CJK 字符 → 4
	if got := estimateTokens("你好世界"); got != 4 {
		t.Fatalf("4 CJK 应为 4，got %d", got)
	}
}

// TestHandleCountTokens 验证端点返回 Anthropic 格式 {"input_tokens": N}。
func TestHandleCountTokens(t *testing.T) {
	srv := &Server{logs: logbuf.NewRing(10)}
	body := `{
		"model": "（公网）deepseek-v4-1-flash-260910",
		"system": [{"type":"text","text":"你是助手"}],
		"messages": [{"role":"user","content":"你好世界"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body))
	rec := httptest.NewRecorder()

	srv.handleCountTokens(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码错误: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type 错误: %q", ct)
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应解析失败: %v (%s)", err, rec.Body.String())
	}
	// system "你是助手"(4) + user "你好世界"(4) = 8
	if out.InputTokens != 8 {
		t.Fatalf("input_tokens 应为 8，got %d", out.InputTokens)
	}
}
