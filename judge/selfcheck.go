package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/runner"
)

// 自检用的固定文本：客套话最容易被中转层的上下文清理删掉 / Fixed self-check text; pleasantries are what relays strip first
const (
	echoInstruction = "把用户消息原样输出，一个字都不要改，不要加任何别的内容。"
	echoText        = "您好，非常感谢您的耐心！麻烦您帮个忙，好问题，请问还有什么可以帮您的吗？谢谢您，辛苦了。"
	// maxInjectedTokens 容许聊天模板自身的少量 token / maxInjectedTokens allows the few tokens a chat template adds
	maxInjectedTokens = 8
)

// SelfCheck 确认订阅席位的请求原样到达模型：模型须逐字复述 echoText；同一文本发一份与两份时，
// 输入 token 数之差即文本本身，2×一份 − 两份 即为请求之外被加入的 token 数。
//
// SelfCheck confirms a subscription seat reaches the model unaltered: the model must echo
// echoText verbatim, and sending the text once and twice isolates the text's own tokens,
// so 2×once − twice is the number of tokens added outside the request.
func SelfCheck(ctx context.Context, s Seat, sleep func(context.Context, time.Duration) error, now func() time.Time) (round.SelfCheck, error) {
	send := func(system, user string) (provider.Response, error) {
		at, err := runner.Send(ctx, s.Sender, sleep, func(maxTokens int) ([]byte, error) {
			return provider.JudgeBody(provider.JudgeRequest{Model: s.Judge.Model, System: system, User: user, MaxTokens: maxTokens})
		})
		return at.Response, err
	}

	echo, err := send(echoInstruction, echoText)
	if err != nil {
		return round.SelfCheck{}, fmt.Errorf("self-check echo: %w", err)
	}
	once, err := send("", echoText)
	if err != nil {
		return round.SelfCheck{}, fmt.Errorf("self-check tokens: %w", err)
	}
	twice, err := send("", echoText+echoText)
	if err != nil {
		return round.SelfCheck{}, fmt.Errorf("self-check tokens: %w", err)
	}
	a, errA := promptTokens(once.Usage)
	b, errB := promptTokens(twice.Usage)
	if err := errors.Join(errA, errB); err != nil {
		return round.SelfCheck{}, fmt.Errorf("self-check tokens: %w", err)
	}

	sc := round.SelfCheck{
		Echo:           strings.TrimSpace(echo.Content) == echoText,
		InjectedTokens: 2*a - b,
		CheckedAt:      now().Format(time.RFC3339),
	}
	switch {
	case !sc.Echo:
		return sc, fmt.Errorf("judge %s failed the self-check: the echo came back altered", s.Judge.Vendor)
	case sc.InjectedTokens > maxInjectedTokens:
		return sc, fmt.Errorf("judge %s failed the self-check: %d tokens were added to the request", s.Judge.Vendor, sc.InjectedTokens)
	}
	return sc, nil
}

// promptTokens 读取 usage 中的输入 token 数 / promptTokens reads the input token count from usage
func promptTokens(usage json.RawMessage) (int, error) {
	var u struct {
		PromptTokens *int `json:"prompt_tokens"`
	}
	if err := json.Unmarshal(usage, &u); err != nil || u.PromptTokens == nil {
		return 0, errors.New("usage has no prompt_tokens")
	}
	return *u.PromptTokens, nil
}
