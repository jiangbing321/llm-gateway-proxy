package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/provider"
	"llm-gateway-proxy/internal/transform"
)

// streamPassthrough 同协议直通：逐块透传 + Flush（原 streamResponse 行为）。
func (s *Server) streamPassthrough(w http.ResponseWriter, body io.Reader, entry *logbuf.Entry, recordResp func(string)) {
	flusher, canFlush := w.(http.Flusher)
	w.WriteHeader(http.StatusOK)

	recording := s.rec != nil // 录制关闭时不累积流全文（否则长流白白驻留内存）
	var usage usageScanner    // 逐块解析完整 SSE 行抓 usage（流头尾都可能带，不能只看尾部）
	var fullBody strings.Builder // 录制全文（仅 recording 时累积）
	buf := make([]byte, 32<<10)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if recording {
				fullBody.Write(buf[:n])
			}
			usage.Feed(buf[:n])
			if _, werr := w.Write(buf[:n]); werr != nil {
				entry.PromptTokens, entry.CompletionTokens = usage.Result()
				recordResp(fullBody.String())
				return // 客户端断开
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				entry.Error = truncate(err.Error(), 200)
			}
			break
		}
	}
	entry.PromptTokens, entry.CompletionTokens = usage.Result()
	recordResp(fullBody.String())
}

// streamConverted 异构协议流式转换：EventReader → StreamConverter → 逐帧写出。
// Responses 客户端：上游非 openai 时级联（上游→chat chunk→Responses 事件）。
func (s *Server) streamConverted(w http.ResponseWriter, body io.Reader,
	clientProto, upstreamProto provider.Protocol, entry *logbuf.Entry, recordResp func(string)) {

	flusher, canFlush := w.(http.Flusher)
	w.WriteHeader(http.StatusOK)

	// Responses 客户端：统一级联到 O2R
	if clientProto == provider.ProtocolResponses {
		s.streamToResponses(w, body, upstreamProto, entry, recordResp)
		return
	}

	var conv interface {
		Convert(data string) []string
		Flush() []string
		Usage() (int, int)
	}
	// 方向：上游协议 → 客户端协议（转换器产出客户端协议的帧）
	switch {
	case upstreamProto == provider.ProtocolAnthropic && clientProto == provider.ProtocolOpenAI:
		conv = transform.NewA2OStreamConverter()
	case upstreamProto == provider.ProtocolGemini && clientProto == provider.ProtocolOpenAI:
		conv = transform.NewG2OStreamConverter()
	case upstreamProto == provider.ProtocolOpenAI && clientProto == provider.ProtocolAnthropic:
		conv = transform.NewO2AStreamConverter()
	case upstreamProto == provider.ProtocolOpenAI && clientProto == provider.ProtocolGemini:
		conv = transform.NewO2GStreamConverter()
	default:
		// 组合未实现（如 gemini↔anthropic 需两级转换）——直通兜底
		s.streamPassthrough(w, body, entry, recordResp)
		return
	}

	var fullBody strings.Builder // 录制：转换后的下游流（仅录制开启时累积）
	recording := s.rec != nil
	er := transform.NewEventReader(body)
	for {
		ev, err := er.Next()
		if ev != nil {
			for _, frame := range conv.Convert(ev.Data) {
				if recording {
					fullBody.WriteString(frame)
				}
				if _, werr := io.WriteString(w, frame); werr != nil {
					conv.Flush() // 保序收尾（不写客户端）
					recordResp(fullBody.String())
					finishUsage(entry, conv)
					return // 客户端断开
				}
				if canFlush {
					flusher.Flush()
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				entry.Error = truncate(err.Error(), 200)
			}
			break
		}
	}
	// 上游结束（含断流）：收尾帧保证序列闭合
	for _, frame := range conv.Flush() {
		fullBody.WriteString(frame)
		if _, werr := io.WriteString(w, frame); werr != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}
	}
	recordResp(fullBody.String())
	finishUsage(entry, conv)
}

func finishUsage(entry *logbuf.Entry, conv interface{ Usage() (int, int) }) {
	entry.PromptTokens, entry.CompletionTokens = conv.Usage()
}

// streamToResponses 把任意上游流转为 Responses 事件流。
// openai 上游：chat chunk 直接喂 O2R；anthropic/gemini 上游：先转 chat chunk 再喂 O2R。
func (s *Server) streamToResponses(w http.ResponseWriter, body io.Reader,
	upstreamProto provider.Protocol, entry *logbuf.Entry, recordResp func(string)) {

	flusher, canFlush := w.(http.Flusher)
	o2r := transform.NewO2RStreamConverter()

	// 一级转换器（上游→chat chunk）；openai 上游无需一级
	var first interface {
		Convert(data string) []string
		Flush() []string
	}
	switch upstreamProto {
	case provider.ProtocolAnthropic:
		first = transform.NewA2OStreamConverter()
	case provider.ProtocolGemini:
		first = transform.NewG2OStreamConverter()
	}

	var fullBody strings.Builder
	recording := s.rec != nil // 录制关闭时不累积
	writeFrames := func(frames []string) bool {
		for _, f := range frames {
			if recording {
				fullBody.WriteString(f)
			}
			if _, err := io.WriteString(w, f); err != nil {
				return false
			}
			if canFlush {
				flusher.Flush()
			}
		}
		return true
	}
	// 把 chat chunk 帧（"data: X\n\n"）解出 data 喂 O2R
	feed := func(chatFrames []string) bool {
		for _, cf := range chatFrames {
			if !strings.HasPrefix(cf, "data: ") {
				continue
			}
			data := strings.TrimSuffix(strings.TrimPrefix(cf, "data: "), "\n\n")
			if !writeFrames(o2r.Convert(data)) {
				return false
			}
		}
		return true
	}

	er := transform.NewEventReader(body)
	for {
		ev, err := er.Next()
		if ev != nil {
			if first != nil {
				if !feed(first.Convert(ev.Data)) {
					recordResp(fullBody.String()); finishUsage(entry, o2r); return
				}
			} else {
				if !writeFrames(o2r.Convert(ev.Data)) {
					recordResp(fullBody.String()); finishUsage(entry, o2r); return
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				entry.Error = truncate(err.Error(), 200)
			}
			break
		}
	}
	// 收尾：一级 Flush（产终 chunk+[DONE]）→ O2R 收尾
	if first != nil {
		feed(first.Flush())
	}
	writeFrames(o2r.Flush())
	recordResp(fullBody.String())
	finishUsage(entry, o2r)
}

// usageScanner 逐块解析 SSE 完整行提取用量。
// Anthropic 上游 input_tokens 在流头（message_start）、output_tokens 在流尾（message_delta），
// OpenAI 上游两个都在最后的 usage chunk——因此每个 data: 行都要看，per-field 取最后一个非零值。
type usageScanner struct {
	pending    string // 跨块的半行
	prompt     int
	completion int
}

// Feed 喂入一个透传块，解析其中完整行（块边界可能截断行，残段留到下一块）。
func (u *usageScanner) Feed(chunk []byte) {
	data := u.pending + string(chunk)
	lines := strings.SplitAfter(data, "\n")
	u.pending = lines[len(lines)-1] // 最后一段可能是半行，留待后续
	for _, line := range lines[:len(lines)-1] {
		u.scanLine(line)
	}
}

// Result 流结束时调用：处理以 EOF 结尾的最后一行（无换行的残段）。
func (u *usageScanner) Result() (prompt, completion int) {
	if u.pending != "" {
		u.scanLine(u.pending)
		u.pending = ""
	}
	return u.prompt, u.completion
}

// sseUsage 任一协议的 usage 字段形状（OpenAI prompt/completion_tokens、Anthropic input/output_tokens）。
// 缓存字段为 Anthropic 语义：input_tokens 不含缓存，缓存写入/读取需单独计入输入。
type sseUsage struct {
	PromptTokens             int `json:"prompt_tokens"`
	CompletionTokens         int `json:"completion_tokens"`
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// usageLine SSE data 行里 usage 的两种挂载位置：
// 顶层（OpenAI chunk、Anthropic message_delta）或嵌套在 message 下（Anthropic message_start）。
type usageLine struct {
	Usage   *sseUsage `json:"usage"`
	Message *struct {
		Usage *sseUsage `json:"usage"`
	} `json:"message"`
}

func (u *usageScanner) scanLine(line string) {
	idx := strings.Index(line, `"usage"`)
	if idx < 0 {
		return
	}
	if start := strings.Index(line, "data: "); start >= 0 && start < idx {
		line = line[start+6:]
	}
	var ul usageLine
	if json.Unmarshal([]byte(line), &ul) != nil {
		return
	}
	u.absorb(ul.Usage)
	if ul.Message != nil {
		u.absorb(ul.Message.Usage)
	}
}

// absorb 按字段取最大值（usage 事件为累积语义：流头给输入、流尾给更大的累计输出；
// 个别上游 message_delta 重复 input 却省略 cache 字段，覆盖会丢缓存量，取 max 可两全）。
func (u *usageScanner) absorb(us *sseUsage) {
	if us == nil {
		return
	}
	if p := us.PromptTokens + us.InputTokens + us.CacheCreationInputTokens + us.CacheReadInputTokens; p > u.prompt {
		u.prompt = p
	}
	if c := us.CompletionTokens + us.OutputTokens; c > u.completion {
		u.completion = c
	}
}
