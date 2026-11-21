package provider

import (
	"encoding/json"
	"strings"
)

// responsesAdapter 实现 OpenAI Responses 协议 / responsesAdapter implements OpenAI Responses
type responsesAdapter struct{}

type responsesInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesBody struct {
	Model           string           `json:"model"`
	Input           []responsesInput `json:"input"`
	MaxOutputTokens int              `json:"max_output_tokens"`
	Store           bool             `json:"store"`
}

func (responsesAdapter) body(req Request) any {
	input := make([]responsesInput, len(req.Messages))
	for i, m := range req.Messages {
		input[i] = responsesInput(m)
	}
	return responsesBody{Model: req.Model, Input: input, MaxOutputTokens: req.MaxTokens, Store: false}
}

func (responsesAdapter) headers() map[string]string { return nil }

func (responsesAdapter) parse(data []byte) (Response, error) {
	var r struct {
		Model             string `json:"model"`
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Output []struct {
			Type    string     `json:"type"`
			Content []textPart `json:"content"`
			Summary []textPart `json:"summary"`
		} `json:"output"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Response{}, err
	}

	// 正文取 message 项的 output_text，推理取 reasoning 项默认返回的明文 / Text from messages, reasoning from plain reasoning items
	var content, reasoning strings.Builder
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			appendText(&content, item.Content, "output_text")
		case "reasoning":
			appendText(&reasoning, item.Summary, "summary_text")
			appendText(&reasoning, item.Content, "reasoning_text")
		}
	}

	finish, truncated := r.Status, false
	if r.Status == "incomplete" && r.IncompleteDetails != nil {
		finish = r.Status + ":" + r.IncompleteDetails.Reason
		truncated = r.IncompleteDetails.Reason == "max_output_tokens"
	}
	return Response{
		Content:       content.String(),
		Reasoning:     reasoning.String(),
		FinishReason:  finish,
		ModelReturned: r.Model,
		Usage:         rawOrNil(r.Usage),
		Truncated:     truncated,
		Transient:     r.Status == "failed",
	}, nil
}

// textPart 是带类型的文本片段 / textPart is a typed text fragment
type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// appendText 拼接指定类型的文本片段 / appendText concatenates fragments of the given type
func appendText(b *strings.Builder, parts []textPart, typ string) {
	for _, p := range parts {
		if p.Type == typ {
			b.WriteString(p.Text)
		}
	}
}
