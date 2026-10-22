package provider

import (
	"encoding/json"
	"strings"
)

// anthropicVersion 是 Anthropic Messages 要求的 API 版本头 / anthropicVersion is the required API version header
const anthropicVersion = "2023-06-01"

// anthropicAdapter 实现 Anthropic Messages 协议 / anthropicAdapter implements Anthropic Messages
type anthropicAdapter struct{}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicBody struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

func (anthropicAdapter) body(req Request) any {
	msgs := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMessage(m)
	}
	return anthropicBody{Model: req.Model, MaxTokens: req.MaxTokens, Messages: msgs}
}

func (anthropicAdapter) headers() map[string]string {
	return map[string]string{"anthropic-version": anthropicVersion}
}

func (anthropicAdapter) parse(data []byte) (Response, error) {
	var r struct {
		Model   string `json:"model"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
		StopReason string          `json:"stop_reason"`
		Usage      json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Response{}, err
	}

	// thinking 块的明文默认可能为空，照收不补 / Thinking text may be empty by default; take it as returned
	var content, reasoning strings.Builder
	for _, block := range r.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		}
	}
	return Response{
		Content:       content.String(),
		Reasoning:     reasoning.String(),
		FinishReason:  r.StopReason,
		ModelReturned: r.Model,
		Usage:         rawOrNil(r.Usage),
		Truncated:     r.StopReason == "max_tokens",
	}, nil
}
