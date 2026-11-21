package provider

import (
	"encoding/json"
	"strings"
)

// Gemini Interactions 的 step 类型 / Step types of Gemini Interactions
const (
	stepUserInput   = "user_input"
	stepModelOutput = "model_output"
	stepThought     = "thought"
)

// geminiAdapter 实现 Gemini Interactions 协议；历史以 step 数组无状态地完整发送。
// geminiAdapter implements Gemini Interactions; history is sent statelessly as a step array.
type geminiAdapter struct{}

type geminiStep struct {
	Type    string     `json:"type"`
	Content []textPart `json:"content"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"max_output_tokens"`
}

type geminiBody struct {
	Model            string                 `json:"model"`
	Input            []geminiStep           `json:"input"`
	GenerationConfig geminiGenerationConfig `json:"generation_config"`
	Store            bool                   `json:"store"`
}

func (geminiAdapter) body(req Request) any {
	steps := make([]geminiStep, len(req.Messages))
	for i, m := range req.Messages {
		typ := stepUserInput
		if m.Role == RoleAssistant {
			typ = stepModelOutput
		}
		steps[i] = geminiStep{Type: typ, Content: []textPart{{Type: "text", Text: m.Content}}}
	}
	return geminiBody{
		Model:            req.Model,
		Input:            steps,
		GenerationConfig: geminiGenerationConfig{MaxOutputTokens: req.MaxTokens},
		Store:            false,
	}
}

func (geminiAdapter) headers() map[string]string { return nil }

func (geminiAdapter) parse(data []byte) (Response, error) {
	var r struct {
		Model  string `json:"model"`
		Status string `json:"status"`
		Steps  []struct {
			Type    string     `json:"type"`
			Content []textPart `json:"content"`
			Summary []textPart `json:"summary"`
		} `json:"steps"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Response{}, err
	}

	var content, reasoning strings.Builder
	for _, step := range r.Steps {
		switch step.Type {
		case stepModelOutput:
			appendText(&content, step.Content, "text")
		case stepThought:
			appendText(&reasoning, step.Summary, "text")
		}
	}
	return Response{
		Content:       content.String(),
		Reasoning:     reasoning.String(),
		FinishReason:  r.Status,
		ModelReturned: r.Model,
		Usage:         rawOrNil(r.Usage),
		Truncated:     r.Status == "incomplete",
		Transient:     r.Status == "failed",
	}, nil
}
