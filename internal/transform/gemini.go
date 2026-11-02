package transform

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"llm-gateway-proxy/internal/i18n"
)

// ---- Gemini GenerateContent 结构 ----

// GeminiRequest 是 generateContent 请求体。
type GeminiRequest struct {
	// contents：仅 user/model 两种 role
	Contents        []GeminiContent `json:"contents"`
	SystemInstruction *GeminiContent `json:"systemInstruction,omitempty"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools           []GeminiToolBlock `json:"tools,omitempty"`
	ToolConfig      *GeminiToolConfig `json:"toolConfig,omitempty"`
}

// GeminiContent 是一条消息（role: user/model）。
type GeminiContent struct {
	Role  string       `json:"role,omitempty"` // user | model（systemInstruction 中省略
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart 是内容片段。
type GeminiPart struct {
	Text string `json:"text,omitempty"`
	// functionCall（model 发起）
	FunctionCall *GeminiFunctionCall `json:"functionCall,omitempty"`
	// functionResponse（user 回传）
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiFunctionCall 是模型发起的工具调用。
type GeminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// GeminiFunctionResponse 是工具结果。
type GeminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

// GeminiGenerationConfig 是生成参数。
type GeminiGenerationConfig struct {
	MaxOutputTokens int       `json:"maxOutputTokens,omitempty"`
	Temperature     *float64  `json:"temperature,omitempty"`
	TopP            *float64  `json:"topP,omitempty"`
	StopSequences   []string  `json:"stopSequences,omitempty"`
}

// GeminiToolBlock 是工具定义块。
type GeminiToolBlock struct {
	FunctionDeclarations []GeminiFunctionDecl `json:"functionDeclarations"`
}

// GeminiFunctionDecl 是函数声明。
type GeminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// GeminiToolConfig 是工具选择配置。
type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiFunctionCallingConfig 是函数调用模式。
type GeminiFunctionCallingConfig struct {
	Mode         string   `json:"mode,omitempty"` // AUTO/ANY/NONE
	AllowedNames []string `json:"allowedFunctionNames,omitempty"`
}

// GeminiResponse 是 generateContent 响应（流式时每帧同构）。
type GeminiResponse struct {
	Candidates []struct {
		Content      GeminiContent `json:"content"`
		FinishReason string        `json:"finishReason,omitempty"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata,omitempty"`
}

// ---- 请求转换：OAIChat → Gemini ----

// ReqO2G 把 OpenAI Chat 请求转为 Gemini generateContent 请求。
func ReqO2G(in *OAIChatRequest) (*GeminiRequest, error) {
	out := &GeminiRequest{}
	if in.MaxTokens > 0 || in.Temperature != nil || in.TopP != nil || len(in.Stop) > 0 {
		gc := &GeminiGenerationConfig{Temperature: in.Temperature, TopP: in.TopP}
		if in.MaxTokens > 0 {
			gc.MaxOutputTokens = in.MaxTokens
		}
		if len(in.Stop) > 0 {
			var s string
			if json.Unmarshal(in.Stop, &s) == nil {
				gc.StopSequences = []string{s}
			} else {
				_ = json.Unmarshal(in.Stop, &gc.StopSequences)
			}
		}
		out.GenerationConfig = gc
	}

	// tools
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, GeminiToolBlock{
			FunctionDeclarations: []GeminiFunctionDecl{{
				Name: t.Function.Name, Description: t.Function.Description,
				Parameters: t.Function.Parameters,
			}},
		})
	}
	// tool_choice
	if len(in.ToolChoice) > 0 {
		var s string
		if json.Unmarshal(in.ToolChoice, &s) == nil {
			switch s {
			case "auto":
				out.ToolConfig = &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "AUTO"}}
			case "none":
				out.ToolConfig = &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "NONE"}}
			case "required":
				out.ToolConfig = &GeminiToolConfig{FunctionCallingConfig: &GeminiFunctionCallingConfig{Mode: "ANY"}}
			}
		}
	}

	// messages
	var systemTexts []string
	toolName := map[string]string{} // tool_call_id → 函数名（functionResponse 需要名字）
	for _, m := range in.Messages {
		switch m.Role {
		case "system", "developer":
			if t := OAIContentToText(m.Content); t != "" {
				systemTexts = append(systemTexts, t)
			}
		case "user":
			out.Contents = append(out.Contents, GeminiContent{
				Role:  "user",
				Parts: []GeminiPart{{Text: OAIContentToText(m.Content)}},
			})
		case "assistant":
			var parts []GeminiPart
			if txt := OAIContentToText(m.Content); txt != "" {
				parts = append(parts, GeminiPart{Text: txt})
			}
			for _, tc := range m.ToolCalls {
				args := json.RawMessage(tc.Function.Arguments)
				if len(strings.TrimSpace(tc.Function.Arguments)) == 0 {
					args = json.RawMessage("{}")
				}
				toolName[tc.ID] = tc.Function.Name
				parts = append(parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{Name: tc.Function.Name, Args: args},
				})
			}
			if len(parts) == 0 {
				parts = []GeminiPart{{Text: ""}}
			}
			out.Contents = append(out.Contents, GeminiContent{Role: "model", Parts: parts})
		case "tool":
			// tool 消息 → user turn 的 functionResponse part
			resp := m.Content
			if len(resp) == 0 {
				resp = json.RawMessage(`{}`)
			}
			// 兼容：content 为纯文本字符串时包成 {result: "..."}
			var probe string
			if json.Unmarshal(resp, &probe) == nil {
				wrapped, _ := json.Marshal(map[string]string{"result": probe})
				resp = wrapped
			}
			part := GeminiPart{FunctionResponse: &GeminiFunctionResponse{
				Name: toolName[m.ToolCallID], Response: resp,
			}}
			// 相邻 tool 消息合并进同一 user turn
			if n := len(out.Contents); n > 0 && out.Contents[n-1].Role == "user" &&
				len(out.Contents[n-1].Parts) > 0 && out.Contents[n-1].Parts[0].FunctionResponse != nil {
				out.Contents[n-1].Parts = append(out.Contents[n-1].Parts, part)
			} else {
				out.Contents = append(out.Contents, GeminiContent{Role: "user", Parts: []GeminiPart{part}})
			}
		default:
			return nil, errors.New(i18n.T("transform.roleUnsupported", m.Role))
		}
	}
	if len(systemTexts) > 0 {
		out.SystemInstruction = &GeminiContent{Parts: []GeminiPart{{Text: strings.Join(systemTexts, "\n\n")}}}
	}
	// Gemini 要求首条为 user
	if len(out.Contents) > 0 && out.Contents[0].Role != "user" {
		out.Contents = append([]GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: ""}}}}, out.Contents...)
	}
	return out, nil
}

// ---- 响应转换：Gemini → OAIChat ----

// RespG2O 把 Gemini 响应转为 OpenAI 格式。
func RespG2O(in *GeminiResponse) *OAIChatResponse {
	out := &OAIChatResponse{
		ID: "gemini-response", Object: "chat.completion", Model: "gemini",
	}
	var texts []string
	var toolCalls []OAIToolCall
	finish := ""
	if len(in.Candidates) > 0 {
		c := in.Candidates[0]
		finish = geminiFinishToOAI(c.FinishReason)
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
			if p.FunctionCall != nil {
				args := "{}"
				if len(p.FunctionCall.Args) > 0 {
					args = string(p.FunctionCall.Args)
				}
				toolCalls = append(toolCalls, OAIToolCall{
					ID: "gemini-call-" + fmt.Sprint(len(toolCalls)), Type: "function",
					Function: OAIToolCallFunction{Name: p.FunctionCall.Name, Arguments: args},
				})
			}
		}
	}
	if finish == "" {
		finish = "stop"
	}
	content := "null"
	if len(texts) > 0 {
		raw, _ := json.Marshal(joinStrings(texts))
		content = string(raw)
	}
	msg := OAIMsgOut{Role: "assistant", Content: json.RawMessage(content)}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	out.Choices = []OAIChatChoice{{Index: 0, Message: msg, FinishReason: finish}}
	if in.UsageMetadata != nil {
		p, c := in.UsageMetadata.PromptTokenCount, in.UsageMetadata.CandidatesTokenCount
		out.Usage = &OAIUsage{PromptTokens: p, CompletionTokens: c, TotalTokens: p + c}
	}
	return out
}

func geminiFinishToOAI(r string) string {
	switch r {
	case "STOP", "stop":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		if r == "" {
			return "stop"
		}
		return "stop"
	}
}

// geminiFinishFromOAI：OpenAI finish_reason → Gemini finishReason（O2G 流收尾用）。
func geminiFinishFromOAI(r string) string {
	switch r {
	case "length":
		return "MAX_TOKENS"
	default:
		return "STOP"
	}
}
