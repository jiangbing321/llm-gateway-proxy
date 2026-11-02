// Package proxy 在本地暴露 OpenAI 兼容 API，按模型路由到各厂商上游。
package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/i18n"
	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/provider"
	"llm-gateway-proxy/internal/recorder"
)

// Server 是本地代理 HTTP 服务器。
type Server struct {
	mu        sync.Mutex
	cfg       config.Config
	registry  *provider.Registry
	logs      *logbuf.Ring
	rec       *recorder.Recorder
	server    *http.Server
	ln        net.Listener
	running   bool

	onStateChange []func(running bool, addr string, err error)
}

// NewServer 创建代理服务器。
func NewServer(cfg config.Config, logs *logbuf.Ring, rec *recorder.Recorder, registry *provider.Registry) *Server {
	return &Server{cfg: cfg, logs: logs, rec: rec, registry: registry}
}

// OnStateChange 注册运行状态回调（启动成功/停止/出错）。
func (s *Server) OnStateChange(fn func(running bool, addr string, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStateChange = append(s.onStateChange, fn)
}

func (s *Server) notify(running bool, addr string, err error) {
	fns := make([]func(bool, string, error), len(s.onStateChange))
	copy(fns, s.onStateChange)
	for _, fn := range fns {
		go fn(running, addr, err)
	}
}

// IsRunning 返回运行状态。
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Addr 返回监听地址（未运行为空）。
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Start 启动监听。已运行时返回错误。
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New(i18n.T("proxy.alreadyRunning"))
	}
	cfg := s.cfg
	s.mu.Unlock()

	// 注意：中间件/各 handler 均从 s.* 字段动态读取配置，
	// 保证 SaveConfig 热更新（如本地 API Key）即时生效。
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /v1/messages", s.handleMessages)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.handleCountTokens)
	mux.HandleFunc("POST /v1/responses", s.handleResponses)
	mux.HandleFunc("GET /health", s.handleHealth)
	// 路径兼容：Cline 系客户端（Cline/Roo/Zoo Code）配置 baseURL 不带 /v1 时，
	// 请求打到 /chat/completions、/models、/messages —— 一并接住
	mux.HandleFunc("POST /chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /messages", s.handleMessages)
	mux.HandleFunc("POST /messages/count_tokens", s.handleCountTokens)
	mux.HandleFunc("POST /responses", s.handleResponses)
	mux.HandleFunc("GET /models", s.handleModels)
	// 兜底：记录未知路径（排查客户端实际请求的端点，如 /v1/responses）
	mux.HandleFunc("/", s.handleUnknownPath)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port) // Bind: 127.0.0.1（默认）或 0.0.0.0（WSL/局域网）
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.notify(false, "", fmt.Errorf("%s: %w", i18n.T("proxy.listenFailed", cfg.Port), err))
		return err
	}

	srv := &http.Server{
		Handler:           s.withMiddleware(mux),
		ReadHeaderTimeout: 15 * time.Second,
		// 不设读写超时：SSE 长连接需要
		ErrorLog: log.New(log.Writer(), "[proxy] ", log.LstdFlags), // TLS 握手/协议错误留痕
	}

	s.mu.Lock()
	s.server = srv
	s.ln = ln
	s.running = true
	s.mu.Unlock()

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			s.notify(false, "", err)
		}
	}()

	httpAddr := "http://" + ln.Addr().String()
	s.notify(true, httpAddr, nil)
	return nil
}

// Stop 停止（不等待流式连接结束，交给 OS 收尾也可；这里给 2 秒优雅期）。
func (s *Server) Stop() {
	s.mu.Lock()
	srv, ln := s.server, s.ln
	s.server, s.ln = nil, nil
	s.running = false
	s.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
	}
	s.notify(false, "", nil)
}

// Restart 应用新配置重启。
func (s *Server) Restart(cfg config.Config) error {
	s.Stop()
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return s.Start()
}

// UpdateConfig 仅更新配置（不重启）。
func (s *Server) UpdateConfig(cfg config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// UpdateRegistry 更新路由注册表（provider 配置变更后调用）。
func (s *Server) UpdateRegistry(reg *provider.Registry) {
	s.mu.Lock()
	s.registry = reg
	s.mu.Unlock()
}

// withMiddleware 包装 CORS 与本地 API key 鉴权。
// API key 每请求动态读取（SaveConfig 热更新即时生效，无需重启代理）。
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mu.Lock()
		apiKey := s.cfg.LocalAPIKey
		s.mu.Unlock()
		// 本地 API key（可开关）
		if apiKey != "" {
			provided := r.Header.Get("x-api-key")
			if provided == "" {
				if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
					provided = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			if provided != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write(mustMarshalErr("invalid_api_key", i18n.T("proxy.apiKeyRequired")))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
