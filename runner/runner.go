// Package runner 调度一轮测试对受测模型的调用：每家厂商一个串行工作者，各家之间并行；
// 已完成的对话跳过，限流与服务端故障以同一请求体退避重试，截断则提高输出上限重跑该轮。
//
// Package runner schedules a round's calls to the models under test: one serial worker
// per vendor, vendors in parallel. Finished conversations are skipped, rate limits and
// server faults are retried with the same body after backoff, and truncated turns are
// rerun with a higher output limit.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// 输出上限与重试策略 / Output limits and retry policy
const (
	// InitialMaxTokens 取规范「足够大（如 8192）」/ InitialMaxTokens follows the spec's "large enough (e.g. 8192)"
	InitialMaxTokens = 8192
	// MaxTokensCap 是截断重跑时输出上限的天花板 / MaxTokensCap bounds the limit raised by truncation reruns
	MaxTokensCap = 65536
	maxRetries   = 6
	backoffBase  = 2 * time.Second
	backoffCap   = time.Minute
)

// HistoryPolicy 写入 manifest，说明多轮历史的回传方式 / historyPolicy records in the manifest how history is resent
const HistoryPolicy = "content only; reasoning is never resent"

// Sender 发送一个已编码的请求体，由 provider.Client 实现 / Sender posts an encoded body; provider.Client implements it
type Sender interface {
	Send(ctx context.Context, body []byte) (provider.Response, time.Duration, error)
}

// Target 是一家参与本轮的厂商 / Target is one vendor taking part in the round
type Target struct {
	Vendor   config.Vendor
	Endpoint config.Endpoint
	Sender   Sender
}

// Runner 执行一轮调用；Sleep 与 Now 可在测试中替换 / Runner executes a round; Sleep and Now are replaceable in tests
type Runner struct {
	Questions []bank.Question
	Targets   []Target
	Runs      int
	Log       io.Writer
	Sleep     func(context.Context, time.Duration) error
	Now       func() time.Time
}

// Run 调用全部目标厂商，记录追加到 w；done 中的对话跳过。返回失败的对话数。
// Run calls every target vendor and appends records to w, skipping conversations in done.
// It returns the number of failed conversations.
func (r *Runner) Run(ctx context.Context, w *round.Writer, done map[round.Key]bool) (int, error) {
	var (
		mu       sync.Mutex
		failed   int
		writeErr error
		wg       sync.WaitGroup
	)
	for _, t := range r.Targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, q := range r.Questions {
				for run := 1; run <= r.Runs; run++ {
					key := round.Key{Provider: t.Vendor.Name, QuestionID: q.ID, Run: run}
					if done[key] || ctx.Err() != nil {
						continue
					}
					recs, err := r.conversation(ctx, t, q, run)

					mu.Lock()
					switch {
					case err != nil:
						failed++
						if ctx.Err() == nil {
							fmt.Fprintf(r.Log, "[%s] q%d run %d failed: %v\n", t.Vendor.Name, q.ID, run, err)
						}
					case writeErr == nil:
						writeErr = w.Append(recs)
						fmt.Fprintf(r.Log, "[%s] q%d run %d ok\n", t.Vendor.Name, q.ID, run)
					}
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if writeErr != nil {
		return failed, fmt.Errorf("write records: %w", writeErr)
	}
	return failed, ctx.Err()
}

// conversation 依次发送一题的各轮，历史只回传正文 / conversation sends each turn in order, resending only content
func (r *Runner) conversation(ctx context.Context, t Target, q bank.Question, run int) ([]round.Record, error) {
	var history []provider.Message
	recs := make([]round.Record, 0, len(q.Turns))
	for i, prompt := range q.Turns {
		history = append(history, provider.Message{Role: provider.RoleUser, Content: prompt})
		rec, err := r.turn(ctx, t, history)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", i+1, err)
		}
		rec.QuestionID, rec.Run, rec.Turn = q.ID, run, i+1
		recs = append(recs, rec)
		history = append(history, provider.Message{Role: provider.RoleAssistant, Content: rec.Content})
	}
	return recs, nil
}

// turn 发送一轮并处理重试与截断 / turn sends one turn, handling retries and truncation
func (r *Runner) turn(ctx context.Context, t Target, history []provider.Message) (round.Record, error) {
	maxTokens, retries, failures := InitialMaxTokens, 0, 0
	for {
		body, err := provider.Body(t.Endpoint.Protocol, provider.Request{
			Model: t.Vendor.Model, Messages: history, MaxTokens: maxTokens,
		})
		if err != nil {
			return round.Record{}, err
		}
		resp, latency, err := t.Sender.Send(ctx, body)

		// 临时故障按同一请求体退避重试 / Temporary faults resend the same body after backoff
		if err == nil && resp.Transient {
			err = fmt.Errorf("vendor reported %q", resp.FinishReason)
		}
		if err != nil {
			if (!provider.IsRetryable(err) && !resp.Transient) || failures >= maxRetries {
				return round.Record{}, err
			}
			if err := r.Sleep(ctx, backoff(failures, err)); err != nil {
				return round.Record{}, err
			}
			failures++
			retries++
			continue
		}

		// 截断的结果作废，提高上限重跑 / A truncated result is discarded and rerun with a higher limit
		if resp.Truncated {
			if maxTokens*2 > MaxTokensCap {
				return round.Record{}, fmt.Errorf("still truncated at max tokens %d", maxTokens)
			}
			maxTokens *= 2
			retries++
			continue
		}

		return round.Record{
			Provider:      t.Vendor.Name,
			Model:         t.Vendor.Model,
			ModelReturned: resp.ModelReturned,
			Request:       body,
			Content:       resp.Content,
			Reasoning:     resp.Reasoning,
			FinishReason:  resp.FinishReason,
			Usage:         resp.Usage,
			LatencyMS:     latency.Milliseconds(),
			Retries:       retries,
			Timestamp:     r.Now().Format(time.RFC3339),
		}, nil
	}
}

// backoff 按指数退避，厂商给出 Retry-After 时取较大者 / backoff grows exponentially, honoring a longer Retry-After
func backoff(failures int, err error) time.Duration {
	d := min(backoffBase<<failures, backoffCap)
	var httpErr *provider.HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > d {
		d = httpErr.RetryAfter
	}
	return d
}

// SleepContext 等待 d，ctx 取消时提前返回 / SleepContext waits for d, returning early when ctx is canceled
func SleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
