package provider

import (
	"encoding/json"
	"errors"
	"slices"
)

// chatTransient 是 Chat Completions 厂商在响应体中报告的临时故障。
// chatTransient lists finish reasons that report a temporary vendor-side failure.
var chatTransient = []string{"network_error", "insufficient_system_resource"}

// chatAdapter 实现 OpenAI Chat Completions 协议 / chatAdapter implements OpenAI Chat Completions
type chatAdapter struct{}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatBody struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

func (chatAdapter) body(req Request) any {
	msgs := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMessage(m)
	}
	return chatBody{Model: req.Model, Messages: msgs, MaxTokens: req.MaxTokens}
}

func (chatAdapter) headers() map[string]string { return nil }

// JudgeRequest 是发给评审模型的请求：可选的 system prompt 与一条用户消息。
// JudgeRequest is a call to a judge: an optional system prompt and one user message.
type JudgeRequest struct {
	Model     string
	System    string
	User      string
	MaxTokens int
	// JSON 要求输出 JSON 对象 / JSON asks for a JSON object as output
	JSON bool
}

type responseFormat struct {
	Type string `json:"type"`
}

type judgeBody struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// JudgeBody 按 Chat Completions 编码评审请求；两个评审席位都走该协议。
// JudgeBody encodes a judge request as Chat Completions, the protocol both judge seats use.
func JudgeBody(req JudgeRequest) ([]byte, error) {
	var msgs []chatMessage
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, chatMessage{Role: RoleUser, Content: req.User})
	b := judgeBody{Model: req.Model, Messages: msgs, MaxTokens: req.MaxTokens}
	if req.JSON {
		b.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	return encode(b)
}

func (chatAdapter) parse(data []byte) (Response, error) {
	var r struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content          *string `json:"content"`
				ReasoningContent *string `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Response{}, err
	}
	if len(r.Choices) == 0 {
		return Response{}, errors.New("no choices")
	}
	c := r.Choices[0]
	return Response{
		Content:       deref(c.Message.Content),
		Reasoning:     deref(c.Message.ReasoningContent),
		FinishReason:  c.FinishReason,
		ModelReturned: r.Model,
		Usage:         rawOrNil(r.Usage),
		Truncated:     c.FinishReason == "length",
		Transient:     slices.Contains(chatTransient, c.FinishReason),
	}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
