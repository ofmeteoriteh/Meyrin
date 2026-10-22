// Package provider 把一段对话发给某家厂商的 API，并把响应统一成相同的字段。
// 四类协议各有一个 adapter，只负责构造请求体与解析响应体；HTTP 传输在 Client 中，
// 重试策略由调用方决定。受测请求只含对话、模型 ID、输出上限与关闭服务端存储；
// 评审请求另带 system prompt 并要求输出 JSON 对象。采样与推理参数一律不传。
//
// Package provider sends a conversation to a vendor API and normalizes the response.
// Each protocol has an adapter that only builds request bodies and parses responses;
// HTTP transport lives in Client and the caller owns the retry policy. Requests carry
// the conversation, model ID, output limit and storage opt-out; judge requests add a
// system prompt and ask for a JSON object. Sampling and reasoning parameters are never sent.
package provider

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ofmeteoriteh/Meyrin/config"
)

// 对话角色 / Conversation roles
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message 是对话中的一轮发言，历史只保留正文 / Message is one utterance; history keeps only the visible content
type Message struct {
	Role    string
	Content string
}

// Request 是发给受测模型的一次请求 / Request is one call to a model under test
type Request struct {
	Model     string
	Messages  []Message
	MaxTokens int
}

// Response 是统一后的响应 / Response is a normalized response
type Response struct {
	Content       string
	Reasoning     string
	FinishReason  string
	ModelReturned string
	Usage         json.RawMessage
	// Truncated 表示因输出上限被截断 / Truncated marks output cut off by the token limit
	Truncated bool
	// Transient 表示厂商在响应体中报告的临时故障，可原样重试 / Transient marks a vendor-reported temporary failure
	Transient bool
}

// adapter 是一类协议的请求体构造与响应解析 / adapter builds request bodies and parses responses for one protocol
type adapter interface {
	body(Request) any
	parse([]byte) (Response, error)
	headers() map[string]string
}

var adapters = map[config.Protocol]adapter{
	config.OpenAIResponses:    responsesAdapter{},
	config.OpenAIChat:         chatAdapter{},
	config.AnthropicMessages:  anthropicAdapter{},
	config.GeminiInteractions: geminiAdapter{},
}

// Body 按协议编码请求体；返回的字节即记录在结果中的 request。
// Body encodes the request body for a protocol; the bytes are what results record as request.
func Body(p config.Protocol, req Request) ([]byte, error) {
	a, ok := adapters[p]
	if !ok {
		return nil, fmt.Errorf("unknown protocol %q", p)
	}
	return encode(a.body(req))
}

// encode 编码请求体，不转义 <>&，记录中的 request 与提示词原文一致。
// encode marshals a request body with <>& kept literal, so recorded requests read like the prompts.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// StoreDisabled 报告该协议的请求是否带 store: false；其余协议没有关闭服务端存储的参数。
// StoreDisabled reports whether the protocol's requests carry store: false; others have no such option.
func StoreDisabled(p config.Protocol) bool {
	return p == config.OpenAIResponses || p == config.GeminiInteractions
}

// Parse 按协议解析响应体 / Parse decodes a response body for a protocol
func Parse(p config.Protocol, data []byte) (Response, error) {
	a, ok := adapters[p]
	if !ok {
		return Response{}, fmt.Errorf("unknown protocol %q", p)
	}
	resp, err := a.parse(data)
	if err != nil {
		return Response{}, fmt.Errorf("parse %s response: %w", p, err)
	}
	return resp, nil
}

// rawOrNil 让缺失的 usage 记为 null / rawOrNil records a missing usage as null
func rawOrNil(r json.RawMessage) json.RawMessage {
	if len(r) == 0 || string(r) == "null" {
		return nil
	}
	return r
}
