package proxy

import (
	"net/http"
	"testing"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/logbuf"
)

// TestMiddlewareAPIKeyHotReload 验证本地 API Key 热更新：
// 代理运行中 UpdateConfig 修改 key 后，新 key 立即生效、旧 key 立即失效
// （中间件每请求动态读取配置，而非固化启动时的值）。
func TestMiddlewareAPIKeyHotReload(t *testing.T) {
	cfg := config.Config{Bind: "127.0.0.1", Port: 0} // Port 0 = 随机端口
	s := NewServer(cfg, logbuf.NewRing(0), nil, nil)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	base := "http://" + s.Addr()

	get := func(key string) int {
		req, _ := http.NewRequest(http.MethodGet, base+"/health", nil)
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// 未设 key：任意请求放行
	if code := get(""); code != http.StatusOK {
		t.Fatalf("无 key 时应放行，实际 %d", code)
	}

	// 启用 key=k1（热更新，不重启代理）
	s.UpdateConfig(config.Config{Bind: cfg.Bind, Port: cfg.Port, LocalAPIKey: "k1"})
	if code := get("k1"); code != http.StatusOK {
		t.Fatalf("新 key 应放行，实际 %d", code)
	}
	if code := get(""); code != http.StatusUnauthorized {
		t.Fatalf("无 key 应 401，实际 %d", code)
	}

	// 再改为 k2：k1 应立即失效（旧版本此处会漏过——中间件固化了启动时的配置）
	s.UpdateConfig(config.Config{Bind: cfg.Bind, Port: cfg.Port, LocalAPIKey: "k2"})
	if code := get("k1"); code != http.StatusUnauthorized {
		t.Fatalf("旧 key 应立即失效，实际 %d", code)
	}
	if code := get("k2"); code != http.StatusOK {
		t.Fatalf("新 key 应放行，实际 %d", code)
	}

	// 撤销 key：恢复无鉴权
	s.UpdateConfig(config.Config{Bind: cfg.Bind, Port: cfg.Port})
	if code := get(""); code != http.StatusOK {
		t.Fatalf("撤销 key 后应放行，实际 %d", code)
	}
}
