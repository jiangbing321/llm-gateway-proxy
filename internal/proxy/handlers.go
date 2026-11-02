package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-gateway-proxy/internal/i18n"
	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/provider"
	"llm-gateway-proxy/internal/recorder"
	"llm-gateway-proxy/internal/transform"
)

// handleChatCompletions 处理 OpenAI 格式端点（含不带 /v1 的兼容路径）。
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, provider.ProtocolOpenAI)
}

// handleMessages 处理 Anthropic 格式端点（/v1/messages，含 /messages）。
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, provider.ProtocolAnthropic)
}

// handleResponses 处理 OpenAI Responses 格式端点（/v1/responses，含 /responses）。
// Responses 仅作为客户端协议：请求转为 chat 中间表示路由到任意类型上游。
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, provider.ProtocolResponses)
}

// handleChat 是统一管道：读body → 路由(改写model) → [请求转换] → provider.Do →
// [响应/流转换] → 写出；贯穿 logbuf/recorder。
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request, clientProto provider.Protocol) {
	start := time.Now()
	entry := logbuf.Entry{Time: start, Path: r.URL.Path, ClientIP: clientIP(r)}
	defer func() {
		entry.DurationMS = msSince(start)
		s.logs.Add(entry)
	}()

	reqBody, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeErrByProto(w, clientProto, http.StatusBadRequest, "invalid_request", i18n.T("proxy.readBodyFailed"))
		entry.Status, entry.Error = http.StatusBadRequest, err.Error()
		return
	}
	model, wantStream := probeChatRequest(reqBody)
	entry.Model = model
	entry.Stream = wantStream
	entry.Provider = i18n.T("proxy.unrouted")

	s.mu.Lock()
	reg := s.registry
	s.mu.Unlock()
	if reg == nil {
		writeErrByProto(w, clientProto, http.StatusInternalServerError, "proxy_error", i18n.T("proxy.registryNotInit"))
		entry.Status, entry.Error = http.StatusInternalServerError, "no registry"
		return
	}

	prov, resolvedModel, ok := reg.Resolve(model)
	if !ok {
		known := strings.Join(reg.KnownModels(), ", ")
		writeErrByProto(w, clientProto, http.StatusBadRequest, "unknown_model",
			i18n.T("proxy.modelNotFound", model, known))
		entry.Status, entry.Error = http.StatusBadRequest, "unknown_model: "+model
		return
	}

	// 候选链：显式前缀解析出的真实模型 + 主厂商失败时的回退模型（经正常路由解析）
	candidates := []string{}
	addCandidate := func(m string) {
		if m == "" {
			return
		}
		for _, c := range candidates {
			if c == m {
				return
			}
		}
		candidates = append(candidates, m)
	}
	realModel := model
	if resolvedModel != model {
		realModel = resolvedModel
	}
	addCandidate(realModel)
	if fbs, ok := prov.(interface{ Fallbacks() []string }); ok {
		for _, f := range fbs.Fallbacks() {
			addCandidate(f)
		}
	}

	var lastErr error
	wrote := false // 是否已向客户端写出字节（写出后不可再切换）
	for ci, cand := range candidates {
		// 每个候选独立路由（回退可能落到其它厂商/协议）
		cProv, cModel, cOK := reg.Resolve(cand)
		if !cOK {
			continue
		}
		if ci > 0 {
			entry.Model += " →fallback:" + cand
		}
		entry.Provider = cProv.Name()

		// 模型名映射改写（别名 → 上游真名）；与请求里原始 model 一致时跳过，
		// 常见路径（无前缀/无映射/无回退）避免整包 unmarshal+marshal
		upstreamModel := cModel
		if mm, ok := cProv.(interface{ ModelMapping() map[string]string }); ok {
			if real, hit := mm.ModelMapping()[cModel]; hit && real != "" && real != cModel {
				upstreamModel = real
			}
		}
		upstreamBody := reqBody
		if upstreamModel != model {
			upstreamBody = rewriteModel(reqBody, upstreamModel)
		}

		needTransform := clientProto != cProv.Protocol()
		transformTag := ""
		if needTransform {
			var err error
			upstreamBody, err = s.convertRequest(clientProto, upstreamBody, cProv, entry.Stream)
			if err != nil {
				lastErr = err
				continue
			}
			transformTag = string(clientProto) + "2" + string(cProv.Protocol())
		}
		entry.Transform = transformTag

		// 录制：发往上游的请求（转换后）
		if s.rec != nil {
			s.rec.Write(recorder.Entry{
				Time: time.Now(), Dir: "req", Model: cand,
				Path: r.URL.Path, Body: string(upstreamBody), Transform: transformTag,
			})
		}

		resp, err := cProv.Do(r.Context(), upstreamModel, upstreamBody, entry.Stream)
		if err != nil {
			lastErr = err
			continue // 网络失败 → 下一候选
		}
		// 5xx/429 且尚未写出 → 关闭本响应试下一候选
		if (resp.StatusCode >= 500 || resp.StatusCode == 429) && ci < len(candidates)-1 && !wrote {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = fmt.Errorf("%s", i18n.T("proxy.upstreamFailed", cProv.Name(), resp.StatusCode, truncate(string(raw), 120)))
			continue
		}

		// 成功路径：落 entry 并写出（该候选的响应处理）
		defer resp.Body.Close()
		entry.Status = resp.StatusCode
		ct := resp.Header.Get("Content-Type")
		entry.Stream = entry.Stream || strings.Contains(ct, "text/event-stream")

		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(convertErrorBody(clientProto, raw))
			entry.Error = truncate(string(raw), 300)
			return
		}

		recordResp := func(body string) {
			if s.rec != nil {
				s.rec.Write(recorder.Entry{
					Time: time.Now(), Dir: "resp", Model: cand, Status: resp.StatusCode,
					Stream: entry.Stream, Path: r.URL.Path, Body: body, Transform: transformTag,
				})
			}
		}

		if entry.Stream {
			w.Header().Set("Content-Type", ct)
			wrote = true
			if needTransform {
				s.streamConverted(w, resp.Body, clientProto, cProv.Protocol(), &entry, recordResp)
			} else {
				s.streamPassthrough(w, resp.Body, &entry, recordResp)
			}
			return
		}

		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		recordResp(string(raw))
		outBody := raw
		if needTransform {
			outBody, err = convertResponse(clientProto, cProv.Protocol(), raw)
			if err != nil {
				writeErrByProto(w, clientProto, http.StatusBadGateway, "transform_error", i18n.T("proxy.transformFailed")+": "+err.Error())
				entry.Error = err.Error()
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(outBody)
		entry.PromptTokens, entry.CompletionTokens = usageFromBody(outBody)
		return
	}

	// 全部候选失败
	writeErrByProto(w, clientProto, http.StatusBadGateway, "gateway_error",
		i18n.T("proxy.allUpstreamFailed")+": "+errString(lastErr))
	entry.Status, entry.Error = http.StatusBadGateway, errString(lastErr)
}

func errString(err error) string {
	if err == nil {
		return i18n.T("proxy.unknownError")
	}
	return err.Error()
}


// convertRequest 按客户端协议→上游协议转换请求体。
func (s *Server) convertRequest(clientProto provider.Protocol, body []byte, prov provider.Provider, stream bool) ([]byte, error) {
	maxTokens := 8192
	if mt, ok := prov.(interface{ MaxTokensDefault() int }); ok {
		maxTokens = mt.MaxTokensDefault()
	}
	// 统一路径：先归一为 OpenAI Chat 中间表示，再转上游协议
	switch clientProto {
	case provider.ProtocolAnthropic:
		var ar transform.AnthropicRequest
		if err := json.Unmarshal(body, &ar); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("proxy.anthropicReqParseFailed"), err)
		}
		o, err := transform.ReqA2O(&ar)
		if err != nil {
			return nil, err
		}
		body, _ = json.Marshal(o)
	case provider.ProtocolResponses:
		var rr transform.ResponsesRequest
		if err := json.Unmarshal(body, &rr); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("proxy.responsesReqParseFailed"), err)
		}
		o, err := transform.ReqR2O(&rr)
		if err != nil {
			return nil, err
		}
		body, _ = json.Marshal(o)
	}

	var req transform.OAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("proxy.openaiReqParseFailed"), err)
	}
	switch prov.Protocol() {
	case provider.ProtocolAnthropic:
		if stream { // 注入 include_usage 以便合成 message_delta.usage
			req.StreamOptions = &struct {
				IncludeUsage bool `json:"include_usage"`
			}{IncludeUsage: true}
		}
		out, err := transform.ReqO2A(&req, maxTokens)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	case provider.ProtocolGemini:
		out, err := transform.ReqO2G(&req)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	default: // openai（含 gateway）：请求体原样直传（保留未建模字段）
		// 严格遵循 OpenAI 规范的上游，流式默认不发 usage；仅在客户端未带时补注入
		if stream {
			var probe map[string]json.RawMessage
			if json.Unmarshal(body, &probe) == nil {
				if _, has := probe["stream_options"]; !has {
					probe["stream_options"] = json.RawMessage(`{"include_usage":true}`)
					if out, err := json.Marshal(probe); err == nil {
						return out, nil
					}
				}
			}
		}
		return body, nil
	}
}

// convertResponse 非流式响应转换（上游协议 → 客户端协议，经 OpenAI 中间表示）。
func convertResponse(clientProto provider.Protocol, upstream provider.Protocol, raw []byte) ([]byte, error) {
	// 上游 → OpenAI 中间表示
	var chat *transform.OAIChatResponse
	switch upstream {
	case provider.ProtocolAnthropic:
		var ar transform.AnthropicResponse
		if err := json.Unmarshal(raw, &ar); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("proxy.anthropicRespParseFailed"), err)
		}
		chat = transform.RespA2O(&ar)
	case provider.ProtocolGemini:
		var gr transform.GeminiResponse
		if err := json.Unmarshal(raw, &gr); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("proxy.geminiRespParseFailed"), err)
		}
		chat = transform.RespG2O(&gr)
	default:
		var or transform.OAIChatResponse
		if err := json.Unmarshal(raw, &or); err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("proxy.openaiRespParseFailed"), err)
		}
		chat = &or
	}
	// OpenAI → 客户端协议
	switch clientProto {
	case provider.ProtocolAnthropic:
		return json.Marshal(transform.RespO2A(chat))
	case provider.ProtocolResponses:
		return json.Marshal(transform.RespO2R(chat))
	default:
		return json.Marshal(chat)
	}
}

// convertErrorBody 把上游错误体转为客户端协议格式。
func convertErrorBody(clientProto provider.Protocol, raw []byte) []byte {
	msg, typ := extractError(raw)
	if clientProto == provider.ProtocolAnthropic {
		out, _ := json.Marshal(map[string]any{
			"type":  "error",
			"error": map[string]string{"type": typ, "message": msg},
		})
		return out
	}
	out, _ := json.Marshal(map[string]any{
		"error": map[string]string{"message": msg, "type": typ, "code": typ},
	})
	return out
}

func extractError(raw []byte) (msg, typ string) {
	var e struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil {
		if e.Error.Message != "" {
			if e.Error.Type == "" {
				e.Error.Type = "upstream_error"
			}
			return e.Error.Message, e.Error.Type
		}
	}
	s := truncate(string(raw), 300)
	return s, "upstream_error"
}

// writeErrByProto 按客户端协议输出错误。
func writeErrByProto(w http.ResponseWriter, clientProto provider.Protocol, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(convertErrorBody(clientProto, mustMarshalErr(code, msg)))
}

func mustMarshalErr(code, msg string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"error": map[string]string{"message": msg, "type": code, "code": code},
	})
	return raw
}

// handleModels 聚合各启用 provider 的模型列表。
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	entry := logbuf.Entry{Time: start, Path: r.URL.Path, ClientIP: clientIP(r)}
	defer func() {
		entry.DurationMS = msSince(start)
		s.logs.Add(entry)
	}()

	s.mu.Lock()
	reg := s.registry
	s.mu.Unlock()

	type modelItem struct {
		ID       string `json:"id"`
		Object   string `json:"object"`
		OwnedBy  string `json:"owned_by"`
		Provider string `json:"provider"`
	}
	var data []modelItem
	if reg != nil {
		for _, p := range reg.Providers() {
			if !p.Enabled() {
				continue
			}
			for _, m := range p.Models(r.Context()) {
				data = append(data, modelItem{ID: m.ID, Object: "model", OwnedBy: p.Name(), Provider: p.ID()})
			}
		}
	}
	if data == nil {
		data = []modelItem{}
	}
	out, _ := json.Marshal(map[string]any{"object": "list", "data": data})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	entry.Status = http.StatusOK
}

// handleCountTokens 处理 Anthropic 的 POST /v1/messages/count_tokens。
// 上游（OpenAI 兼容）无对应端点，按请求语义文本本地启发式估算 input_tokens。
// Claude Code 用该端点做上下文占用预估，粗略值即可满足其需求。
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	entry := logbuf.Entry{Time: start, Path: r.URL.Path, ClientIP: clientIP(r)}
	defer func() {
		entry.DurationMS = msSince(start)
		s.logs.Add(entry)
	}()

	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeErrByProto(w, provider.ProtocolAnthropic, http.StatusBadRequest, "invalid_request", i18n.T("proxy.readBodyFailed"))
		entry.Status, entry.Error = http.StatusBadRequest, err.Error()
		return
	}
	var ar transform.AnthropicRequest
	if err := json.Unmarshal(body, &ar); err != nil {
		writeErrByProto(w, provider.ProtocolAnthropic, http.StatusBadRequest, "invalid_request", i18n.T("proxy.anthropicReqParseFailed"))
		entry.Status, entry.Error = http.StatusBadRequest, err.Error()
		return
	}
	entry.Model = ar.Model

	// 收集参与计数的语义文本：system + 各消息文本 + 工具定义
	var sb strings.Builder
	sb.WriteString(transform.AnthropicContentToText(ar.System))
	for _, m := range ar.Messages {
		sb.WriteString(transform.AnthropicContentToText(m.Content))
	}
	for _, t := range ar.Tools {
		sb.WriteString(t.Name)
		sb.WriteString(t.Description)
		sb.WriteString(string(t.InputSchema))
	}

	tokens := estimateTokens(sb.String())
	out, _ := json.Marshal(map[string]int{"input_tokens": tokens})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	entry.Status = http.StatusOK
	entry.PromptTokens = tokens
}

// estimateTokens 启发式估算 token 数：CJK 等非 ASCII 字符约 1 token/字，
// ASCII 约 4 字符/token（与主流 BPE 分词器的粗略比率一致）。
func estimateTokens(s string) int {
	ascii, nonASCII := 0, 0
	for _, r := range s {
		if r < 128 {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII
}

// ---- 辅助 ----

// rewriteModel 改写 body 顶层 model 字段（保留其余字段语义）。
func rewriteModel(body []byte, newModel string) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	m["model"] = newModel
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func extractModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &probe) == nil {
		return probe.Model
	}
	return ""
}

// probeChatRequest 一次解析取回路由所需的最小字段（model/stream）。
// 大 body（上限 32MB）下多次全量 unmarshal 的 CPU/内存放大明显，合并为单次。
func probeChatRequest(body []byte) (model string, stream bool) {
	var probe struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Model, probe.Stream
}

// usageFromBody 从响应体（任一协议）提取用量。
// 输入 = prompt_tokens + input_tokens + 缓存写入/读取（Anthropic 语义：
// input_tokens 不含缓存部分，缓存命中时只报增量；OpenAI 无此字段不受影响）。
func usageFromBody(body []byte) (prompt, completion int) {
	var o struct {
		Usage *struct {
			PromptTokens             int `json:"prompt_tokens"`
			CompletionTokens         int `json:"completion_tokens"`
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &o) == nil && o.Usage != nil {
		u := o.Usage
		p := u.PromptTokens + u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
		c := u.CompletionTokens + u.OutputTokens
		return p, c
	}
	return 0, 0
}

// handleUnknownPath 记录并拒绝未实现的端点（帮助排查客户端打到哪个路径）。
func (s *Server) handleUnknownPath(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2048))
	s.logs.Add(logbuf.Entry{
		Time: time.Now(), Path: r.URL.Path, Model: extractModel(body),
		Status: http.StatusNotFound, DurationMS: 0, ClientIP: clientIP(r),
		Error: i18n.T("proxy.endpointNotImplBody", r.Method, truncate(string(body), 200)),
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write(mustMarshalErr("unknown_endpoint",
		i18n.T("proxy.endpointNotImpl", r.Method, r.URL.Path)))
}

func msSince(t time.Time) int64 {
	return time.Since(t).Milliseconds()
}

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

var _ = bytes.MinRead
var _ = context.Background
