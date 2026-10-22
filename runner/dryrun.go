package runner

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/provider"
)

// placeholderReply 代替多轮题中尚未产生的上一轮回答 / placeholderReply stands in for a previous answer not yet produced
const placeholderReply = "<previous turn's reply>"

// plannedRequest 是 dry-run 输出的一行 / plannedRequest is one line of dry-run output
type plannedRequest struct {
	Provider   string          `json:"provider"`
	QuestionID int             `json:"question_id"`
	Turn       int             `json:"turn"`
	Endpoint   string          `json:"endpoint"`
	Request    json.RawMessage `json:"request"`
}

// DryRun 输出每家厂商每题每轮将要发送的请求，不发请求、不读凭据；各次运行的请求相同，只输出一份。
// DryRun prints the request for every vendor, question and turn without sending anything
// or reading credentials; runs share identical requests, so each is printed once.
func DryRun(out io.Writer, questions []bank.Question, targets []Target) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	for _, t := range targets {
		for _, q := range questions {
			var history []provider.Message
			for i, prompt := range q.Turns {
				if i > 0 {
					history = append(history, provider.Message{Role: provider.RoleAssistant, Content: placeholderReply})
				}
				history = append(history, provider.Message{Role: provider.RoleUser, Content: prompt})
				body, err := provider.Body(t.Endpoint.Protocol, provider.Request{
					Model: t.Vendor.Model, Messages: history, MaxTokens: InitialMaxTokens,
				})
				if err != nil {
					return fmt.Errorf("%s q%d: %w", t.Vendor.Name, q.ID, err)
				}
				if err := enc.Encode(plannedRequest{t.Vendor.Name, q.ID, i + 1, t.Endpoint.URL, body}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
