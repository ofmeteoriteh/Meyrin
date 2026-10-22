package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ofmeteoriteh/Meyrin/config"
)

var twoTurns = Request{
	Model:     "m1",
	MaxTokens: 8192,
	Messages: []Message{
		{RoleUser, "第一轮\t缩进"},
		{RoleAssistant, "回答"},
		{RoleUser, "第二轮"},
	},
}

func TestBody(t *testing.T) {
	tests := []struct {
		protocol config.Protocol
		want     string
	}{
		{config.OpenAIChat, `{"model":"m1","messages":[{"role":"user","content":"第一轮\t缩进"},` +
			`{"role":"assistant","content":"回答"},{"role":"user","content":"第二轮"}],"max_tokens":8192}`},
		{config.OpenAIResponses, `{"model":"m1","input":[{"role":"user","content":"第一轮\t缩进"},` +
			`{"role":"assistant","content":"回答"},{"role":"user","content":"第二轮"}],"max_output_tokens":8192,"store":false}`},
		{config.AnthropicMessages, `{"model":"m1","max_tokens":8192,"messages":[{"role":"user","content":"第一轮\t缩进"},` +
			`{"role":"assistant","content":"回答"},{"role":"user","content":"第二轮"}]}`},
		{config.GeminiInteractions, `{"model":"m1","input":[` +
			`{"type":"user_input","content":[{"type":"text","text":"第一轮\t缩进"}]},` +
			`{"type":"model_output","content":[{"type":"text","text":"回答"}]},` +
			`{"type":"user_input","content":[{"type":"text","text":"第二轮"}]}],` +
			`"generation_config":{"max_output_tokens":8192},"store":false}`},
	}
	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			got, err := Body(tt.protocol, twoTurns)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("body =\n%s\nwant\n%s", got, tt.want)
			}
			if strings.Contains(string(got), "temperature") || strings.Contains(string(got), "reasoning") {
				t.Fatal("body carries sampling or reasoning parameters")
			}
		})
	}
	html, err := Body(config.OpenAIChat, Request{Model: "m", Messages: []Message{{RoleUser, "<b> & </b>"}}})
	if err != nil || !strings.Contains(string(html), `"<b> & </b>"`) || strings.HasSuffix(string(html), "\n") {
		t.Fatalf("body = %q, %v; want literal <>& and no trailing newline", html, err)
	}
	if _, err := Body("SOAP", twoTurns); err == nil {
		t.Fatal("Body accepted an unknown protocol")
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		protocol config.Protocol
		body     string
		want     Response
	}{
		{
			"Chat 正文与推理", config.OpenAIChat,
			`{"model":"m1-0101","choices":[{"message":{"content":"答","reasoning_content":"想"},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`,
			Response{Content: "答", Reasoning: "想", FinishReason: "stop", ModelReturned: "m1-0101", Usage: json.RawMessage(`{"total_tokens":3}`)},
		},
		{
			"Chat 截断且正文为空", config.OpenAIChat,
			`{"model":"m1","choices":[{"message":{"content":null},"finish_reason":"length"}]}`,
			Response{FinishReason: "length", ModelReturned: "m1", Truncated: true},
		},
		{
			"Chat 临时故障", config.OpenAIChat,
			`{"model":"m1","choices":[{"message":{"content":""},"finish_reason":"insufficient_system_resource"}]}`,
			Response{FinishReason: "insufficient_system_resource", ModelReturned: "m1", Transient: true},
		},
		{
			"Chat 审核拦截照收", config.OpenAIChat,
			`{"model":"m1","choices":[{"message":{"content":"部分"},"finish_reason":"sensitive"}]}`,
			Response{Content: "部分", FinishReason: "sensitive", ModelReturned: "m1"},
		},
		{
			"Responses 正文与明文推理", config.OpenAIResponses,
			`{"model":"m2","status":"completed","incomplete_details":null,"output":[` +
				`{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"想"}],"encrypted_content":"x"},` +
				`{"type":"message","content":[{"type":"output_text","text":"答"},{"type":"output_text","text":"续"}]}],` +
				`"usage":{"output_tokens_details":{"reasoning_tokens":5}}}`,
			Response{Content: "答续", Reasoning: "想", FinishReason: "completed", ModelReturned: "m2",
				Usage: json.RawMessage(`{"output_tokens_details":{"reasoning_tokens":5}}`)},
		},
		{
			"Responses 只有加密推理", config.OpenAIResponses,
			`{"model":"m2","status":"completed","output":[{"type":"reasoning","summary":[],"encrypted_content":"x"},` +
				`{"type":"message","content":[{"type":"output_text","text":"答"}]}]}`,
			Response{Content: "答", FinishReason: "completed", ModelReturned: "m2"},
		},
		{
			"Responses 截断", config.OpenAIResponses,
			`{"model":"m2","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`,
			Response{FinishReason: "incomplete:max_output_tokens", ModelReturned: "m2", Truncated: true},
		},
		{
			"Responses 审核拦截不算截断", config.OpenAIResponses,
			`{"model":"m2","status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[]}`,
			Response{FinishReason: "incomplete:content_filter", ModelReturned: "m2"},
		},
		{
			"Responses 失败", config.OpenAIResponses,
			`{"model":"m2","status":"failed","output":[]}`,
			Response{FinishReason: "failed", ModelReturned: "m2", Transient: true},
		},
		{
			"Anthropic 文本与空推理", config.AnthropicMessages,
			`{"model":"claude-x","content":[{"type":"thinking","thinking":"","signature":"s"},{"type":"text","text":"答"}],` +
				`"stop_reason":"end_turn","usage":{"output_tokens":9}}`,
			Response{Content: "答", FinishReason: "end_turn", ModelReturned: "claude-x", Usage: json.RawMessage(`{"output_tokens":9}`)},
		},
		{
			"Anthropic 截断", config.AnthropicMessages,
			`{"model":"claude-x","content":[{"type":"thinking","thinking":"想"}],"stop_reason":"max_tokens"}`,
			Response{Reasoning: "想", FinishReason: "max_tokens", ModelReturned: "claude-x", Truncated: true},
		},
		{
			"Gemini 正文与摘要", config.GeminiInteractions,
			`{"model":"g1","status":"completed","steps":[{"type":"thought","signature":"s","summary":[{"type":"text","text":"想"}]},` +
				`{"type":"model_output","content":[{"type":"text","text":"答"}]}],"usage":{"total_thought_tokens":22}}`,
			Response{Content: "答", Reasoning: "想", FinishReason: "completed", ModelReturned: "g1",
				Usage: json.RawMessage(`{"total_thought_tokens":22}`)},
		},
		{
			"Gemini 截断", config.GeminiInteractions,
			`{"model":"g1","status":"incomplete","steps":[]}`,
			Response{FinishReason: "incomplete", ModelReturned: "g1", Truncated: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.protocol, []byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if string(got.Usage) != string(tt.want.Usage) {
				t.Fatalf("usage = %s, want %s", got.Usage, tt.want.Usage)
			}
			got.Usage, tt.want.Usage = nil, nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}

	if _, err := Parse(config.OpenAIChat, []byte(`{"choices":[]}`)); err == nil {
		t.Fatal("Parse accepted a response without choices")
	}
	if _, err := Parse(config.AnthropicMessages, []byte(`not json`)); err == nil {
		t.Fatal("Parse accepted malformed JSON")
	}
}

func TestClientSend(t *testing.T) {
	const key = "sk-secret-123"
	tests := []struct {
		name       string
		endpoint   config.Endpoint
		wantHeader [2]string
	}{
		{"Bearer", config.Endpoint{Auth: config.AuthBearer, Protocol: config.OpenAIChat}, [2]string{"Authorization", "Bearer " + key}},
		{"自定义头", config.Endpoint{Auth: "api-key", Protocol: config.OpenAIChat}, [2]string{"Api-Key", key}},
		{"Anthropic 版本头", config.Endpoint{Auth: "x-api-key", Protocol: config.AnthropicMessages}, [2]string{"Anthropic-Version", anthropicVersion}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				if tt.endpoint.Protocol == config.AnthropicMessages {
					w.Write([]byte(`{"model":"c","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
					return
				}
				w.Write([]byte(`{"model":"c","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
			}))
			defer srv.Close()

			tt.endpoint.URL = srv.URL
			resp, _, err := NewClient(tt.endpoint, key, nil).Send(context.Background(), []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if resp.Content != "ok" {
				t.Fatalf("content = %q", resp.Content)
			}
			if v := got.Get(tt.wantHeader[0]); v != tt.wantHeader[1] {
				t.Fatalf("header %s = %q, want %q", tt.wantHeader[0], v, tt.wantHeader[1])
			}
		})
	}
}

func TestClientErrors(t *testing.T) {
	const key = "sk-secret-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/limited":
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"bad key ` + key + `"}`))
		case "/bad":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.Write([]byte(`{"choices":[]}`))
		}
	}))
	defer srv.Close()
	send := func(path string) error {
		ep := config.Endpoint{URL: srv.URL + path, Auth: config.AuthBearer, Protocol: config.OpenAIChat}
		_, _, err := NewClient(ep, key, nil).Send(context.Background(), []byte(`{}`))
		return err
	}

	err := send("/limited")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != 429 || httpErr.RetryAfter != 7*time.Second || !IsRetryable(err) {
		t.Fatalf("429 err = %v", err)
	}
	if strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), redacted) {
		t.Fatalf("429 error leaks or misses redaction: %v", err)
	}
	if err := send("/bad"); err == nil || IsRetryable(err) {
		t.Fatalf("400 err = %v, want non-retryable", err)
	}
	if err := send("/empty"); err == nil || IsRetryable(err) {
		t.Fatalf("parse err = %v, want non-retryable", err)
	}

	srv.Close()
	if err := send("/"); err == nil || !IsRetryable(err) {
		t.Fatalf("network err = %v, want retryable", err)
	}
	if IsRetryable(context.Canceled) {
		t.Fatal("context cancellation must not be retried")
	}
}

func TestRedactTruncatesOnRuneBoundary(t *testing.T) {
	c := &Client{key: "k"}
	got := c.redact([]byte(strings.Repeat("中", errorBodyLimit)))
	if !strings.HasSuffix(got, "…") || !utf8Valid(got) {
		t.Fatalf("redact produced invalid output of length %d", len(got))
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "�") == s }
