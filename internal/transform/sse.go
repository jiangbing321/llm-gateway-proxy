package transform

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"llm-gateway-proxy/internal/i18n"
)

// sseMaxLine 单行上限（大 tool-call 参数可能数 MB）。
const sseMaxLine = 16 << 20

// SSEEvent 是解析出的一帧（data 部分已拼接多行）。
type SSEEvent struct {
	Event string // event: 行（Anthropic 用；OpenAI 为空）
	Data  string // data: 行拼接
}

// EventReader 从 io.Reader 逐帧解析 SSE。
// 兼容两种流：OpenAI（纯 data: 行 + [DONE]）与 Anthropic（event:+data:）。
type EventReader struct {
	br     *bufio.Reader
	closed bool
}

// NewEventReader 创建。
func NewEventReader(r io.Reader) *EventReader {
	return &EventReader{br: bufio.NewReaderSize(r, 64<<10)}
}

// Next 返回下一帧；io.EOF 表示流结束。
func (er *EventReader) Next() (*SSEEvent, error) {
	var ev SSEEvent
	var dataLines []string
	sawAny := false

	for {
		line, err := er.readLine()
		if err == io.EOF {
			er.closed = true
			if sawAny && len(dataLines) > 0 {
				ev.Data = strings.Join(dataLines, "\n")
				return &ev, nil
			}
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		sawAny = true

		switch {
		case line == "": // 空行 = 帧边界
			if len(dataLines) > 0 || ev.Event != "" {
				ev.Data = strings.Join(dataLines, "\n")
				return &ev, nil
			}
		case strings.HasPrefix(line, ":"): // 注释/心跳（如 OpenAI 的 : openai 前缀注释、Anthropic ping）
			// 忽略
		case strings.HasPrefix(line, "event:"):
			ev.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			d := strings.TrimPrefix(line, "data:")
			d = strings.TrimPrefix(d, " ") // SSE 规范：去掉一个前导空格
			dataLines = append(dataLines, d)
		}
	}
}

// readLine 读一行（去掉 \r\n），带长度保护。
func (er *EventReader) readLine() (string, error) {
	var buf []byte
	for {
		chunk, err := er.br.ReadBytes('\n')
		buf = append(buf, chunk...)
		if len(buf) > sseMaxLine {
			return "", errors.New(i18n.T("transform.sseLineTooLong", sseMaxLine))
		}
		if err == nil {
			break
		}
		if err == io.EOF {
			if len(buf) == 0 {
				return "", io.EOF
			}
			break // 最后一行无换行
		}
		return "", err
	}
	return strings.TrimRight(string(buf), "\r\n"), nil
}

// FrameOAI 序列化一帧 OpenAI 流（data: 行 + 空行）。
func FrameOAI(data string) string {
	return "data: " + data + "\n\n"
}

// FrameDONE 是 OpenAI 流终止帧。
const FrameDONE = "data: [DONE]\n\n"

// FrameAnthropic 序列化一帧 Anthropic 流（event: + data: + 空行）。
func FrameAnthropic(event string, payload any) string {
	raw, _ := marshalJSON(payload)
	return "event: " + event + "\ndata: " + string(raw) + "\n\n"
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil { // Encode 自带换行
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
