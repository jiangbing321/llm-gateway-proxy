// Package transform 实现 OpenAI Chat 与 Anthropic Messages 两种协议的双向转换：
// 请求、非流式响应、SSE 流事件。转换覆盖主流客户端（Claude Code / Cline / OpenAI SDK）
// 依赖的语义字段；双方各自的非语义扩展字段（如 metadata、response_format）不透传。
package transform

import "encoding/json"

// ---- OpenAI Chat 请求/响应 ----

// OAIChatRequest 是 OpenAI chat/completions 请求。
type OAIChatRequest struct {
	Model         string          `json:"model"`
	Messages      []OAIMessage    `json:"messages"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	Stop          json.RawMessage `json:"stop,omitempty"` // string 或 []string
	Stream        bool            `json:"stream,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
	Tools      []OAITool       `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"` // "auto"/"none"/"required"/{type:"function",function:{name}}
	N          int             `json:"n,omitempty"`
}

// OAIMessage 是一条 OpenAI 消息。
type OAIMessage struct {
	Role       string          `json:"role"` // system/user/assistant/tool
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []OAIToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// OAIToolCall 是 assistant 消息里的工具调用。
type OAIToolCall struct {
	Index    int                  `json:"index,omitempty"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function OAIToolCallFunction  `json:"function"`
}

// OAIToolCallFunction 是工具调用函数体。
type OAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// OAITool 是工具定义。
type OAITool struct {
	Type     string         `json:"type"` // function
	Function OAIToolFunction `json:"function"`
}

// OAIToolFunction 是工具定义的函数描述。
type OAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
}

// OAIContentPart 是多模态 content 的一个片段。
type OAIContentPart struct {
	Type     string `json:"type"` // text | image_url
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// OAIChatResponse 是非流式响应。
type OAIChatResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OAIChatChoice  `json:"choices"`
	Usage   *OAIUsage        `json:"usage,omitempty"`
}

// OAIChatChoice 是非流式响应的一个选项。
type OAIChatChoice struct {
	Index        int          `json:"index"`
	Message      OAIMsgOut    `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

// OAIMsgOut 是输出消息。
type OAIMsgOut struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"` // string 或 null
	Reasoning    string          `json:"reasoning_content,omitempty"`
	ToolCalls    []OAIToolCall   `json:"tool_calls,omitempty"`
}

// OAIUsage 是用量。
type OAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OAIChunk 是流式响应的一个 chunk。
type OAIChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []OAIChunkChoice `json:"choices"`
	Usage   *OAIUsage       `json:"usage,omitempty"`
}

// OAIChunkChoice 是流式 chunk 的一个选项。
type OAIChunkChoice struct {
	Index        int      `json:"index"`
	Delta        OAIDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
}

// OAIDelta 是流式增量。
type OAIDelta struct {
	Role             string        `json:"role,omitempty"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCalls        []OAIToolCall `json:"tool_calls,omitempty"`
}

// ---- Anthropic Messages 请求/响应 ----

// AnthropicRequest 是 /v1/messages 请求。
type AnthropicRequest struct {
	Model         string              `json:"model"`
	MaxTokens     int                 `json:"max_tokens"`
	System        json.RawMessage     `json:"system,omitempty"` // string 或 blocks
	Messages      []AnthropicMessage  `json:"messages"`
	Temperature   *float64            `json:"temperature,omitempty"`
	TopP          *float64            `json:"top_p,omitempty"`
	StopSequences []string            `json:"stop_sequences,omitempty"`
	Stream        bool                `json:"stream,omitempty"`
	Tools         []AnthropicTool     `json:"tools,omitempty"`
	ToolChoice    json.RawMessage     `json:"tool_choice,omitempty"`
}

// AnthropicMessage 是一条 Anthropic 消息（仅 user/assistant）。
type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string 或 blocks 数组
}

// AnthropicTool 是工具定义。
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// AnthropicContentBlock 是 content 数组的一个块。
type AnthropicContentBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result | image
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string 或 blocks
	IsError   bool            `json:"is_error,omitempty"`
	// image
	Source *AnthropicImageSource `json:"source,omitempty"`
}

// AnthropicImageSource 是图片源（base64）。
type AnthropicImageSource struct {
	Type      string `json:"type"` // base64
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// AnthropicResponse 是非流式响应。
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"` // message
	Role         string                  `json:"role"` // assistant
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason,omitempty"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicUsage 是用量。
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ---- Anthropic SSE 事件 ----

// AnthropicEvent 是 /v1/messages 流式事件（宽松解析）。
type AnthropicEvent struct {
	Type string `json:"type"`
	// message_start: {message:{id,model,role,usage:{input_tokens}}}
	Message json.RawMessage `json:"message,omitempty"`
	// content_block_start: {index, content_block:{type,name/id}}
	Index        int             `json:"index,omitempty"`
	ContentBlock json.RawMessage `json:"content_block,omitempty"`
	// content_block_delta: {index, delta:{type, text | partial_json}}
	Delta json.RawMessage `json:"delta,omitempty"`
	// message_delta: {delta:{stop_reason}, usage:{output_tokens}}
	StopReason string          `json:"stop_reason,omitempty"`
	Usage      *AnthropicUsage `json:"usage,omitempty"`
}
