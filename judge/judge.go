package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/runner"
)

// JudgmentsFile 是评审输出的文件名 / JudgmentsFile is the file name of judge outputs
const JudgmentsFile = "judgments.jsonl"

// maxAttempts 是输出未通过校验时以同一请求重试的总次数 / maxAttempts is how often one request is sent when outputs fail validation
const maxAttempts = 3

// Record 是 judgments.jsonl 的一行。Provider 用于汇总，不发给评审；Errors 按次序记录每次未通过
// 校验的原因，用尽 maxAttempts 仍未通过时 Valid 为 false。
//
// Record is one judgments.jsonl line. Provider serves aggregation and is never sent to the
// judge; Errors lists why each failed attempt was rejected, and Valid is false once
// maxAttempts are used up.
type Record struct {
	Judge         string          `json:"judge"`
	Model         string          `json:"model"`
	ModelReturned string          `json:"model_returned"`
	Provider      string          `json:"provider"`
	QuestionID    int             `json:"question_id"`
	Run           int             `json:"run"`
	Valid         bool            `json:"valid"`
	Errors        []string        `json:"errors"`
	Judgment      *Judgment       `json:"judgment"`
	Content       string          `json:"content"`
	Reasoning     string          `json:"reasoning"`
	FinishReason  string          `json:"finish_reason"`
	Usage         json.RawMessage `json:"usage"`
	LatencyMS     int64           `json:"latency_ms"`
	Attempts      int             `json:"attempts"`
	Retries       int             `json:"retries"`
	Timestamp     string          `json:"timestamp"`
}

// Answer 返回该记录评审的对话 / Answer returns the conversation the record judges
func (r Record) Answer() round.Key {
	return round.Key{Provider: r.Provider, QuestionID: r.QuestionID, Run: r.Run}
}

// Done 标识一个评审对一段对话的已完成评审 / Done identifies one judge's finished judgment of one conversation
type Done struct {
	Judge  string
	Answer round.Key
}

// Seat 是一个评审席位及其客户端 / Seat is one judge seat with its client
type Seat struct {
	Judge  config.Judge
	Sender runner.Sender
}

// Judger 执行一轮评审；Sleep 与 Now 可在测试中替换 / Judger runs a round of judging; Sleep and Now are replaceable in tests
type Judger struct {
	Prompt    string
	Questions map[int]bank.Question
	// RuntimeReference 是运行时写入的参考值（第 7 题） / RuntimeReference is the reference filled at run time
	RuntimeReference string
	Seats            []Seat
	Seed             uint64
	Log              *log.Logger
	Sleep            func(context.Context, time.Duration) error
	Now              func() time.Time
}

// Run 让每个席位按各自的随机顺序评审全部回答，席位之间并行；done 中的跳过。返回调用失败、
// 未写入记录的评审数，重跑即可补齐。
//
// Run has every seat judge every answer in its own shuffled order, seats in parallel,
// skipping done. It returns how many judgments failed to call and were not written;
// rerunning fills them in.
func (j *Judger) Run(ctx context.Context, w *round.Writer[Record], answers []Answer, done map[Done]bool) (int, error) {
	var (
		mu       sync.Mutex
		failed   int
		writeErr error
		wg       sync.WaitGroup
	)
	for i, s := range j.Seats {
		order := rand.New(rand.NewPCG(j.Seed, uint64(i))).Perm(len(answers))
		wg.Go(func() {
			for _, k := range order {
				a := answers[k]
				if done[Done{s.Judge.Vendor, a.Key()}] || ctx.Err() != nil {
					continue
				}
				rec, err := j.judge(ctx, s, a)

				mu.Lock()
				switch {
				case err != nil:
					failed++
					if ctx.Err() == nil {
						j.Log.Printf("[%s] %s q%d run %d failed: %v", s.Judge.Vendor, a.Provider, a.QuestionID, a.Run, err)
					}
				case writeErr == nil:
					writeErr = w.Append([]Record{rec})
					j.Log.Printf("[%s] %s q%d run %d valid=%v", s.Judge.Vendor, a.Provider, a.QuestionID, a.Run, rec.Valid)
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if writeErr != nil {
		return failed, fmt.Errorf("write judgments: %w", writeErr)
	}
	return failed, ctx.Err()
}

// judge 评审一条回答：调用失败返回错误；输出未通过校验时以同一请求重试，用尽后记为无效。
// judge judges one answer: call failures return an error, and outputs failing validation
// resend the same request until maxAttempts, after which the record is invalid.
func (j *Judger) judge(ctx context.Context, s Seat, a Answer) (Record, error) {
	q := j.Questions[a.QuestionID]
	user, err := buildInput(q, a, j.RuntimeReference)
	if err != nil {
		return Record{}, err
	}
	rec := Record{Judge: s.Judge.Vendor, Model: s.Judge.Model, Provider: a.Provider, QuestionID: a.QuestionID, Run: a.Run}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		at, err := runner.Send(ctx, s.Sender, j.Sleep, func(maxTokens int) ([]byte, error) {
			return provider.JudgeBody(provider.JudgeRequest{
				Model: s.Judge.Model, System: j.Prompt, User: user, MaxTokens: maxTokens, JSON: true,
			})
		})
		if err != nil {
			return Record{}, err
		}
		resp := at.Response
		rec.ModelReturned, rec.Content, rec.Reasoning = resp.ModelReturned, resp.Content, resp.Reasoning
		rec.FinishReason, rec.Usage, rec.LatencyMS = resp.FinishReason, resp.Usage, at.Latency.Milliseconds()
		rec.Attempts, rec.Retries = attempt, rec.Retries+at.Retries
		rec.Timestamp = j.Now().Format(time.RFC3339)

		jd, err := validate(q, a, resp.Content)
		if err == nil {
			rec.Valid, rec.Judgment = true, &jd
			return rec, nil
		}
		rec.Errors = append(rec.Errors, err.Error())
	}
	return rec, nil
}

// plannedRequest 是 dry-run 输出的一行 / plannedRequest is one line of dry-run output
type plannedRequest struct {
	Judge      string          `json:"judge"`
	Provider   string          `json:"provider"`
	QuestionID int             `json:"question_id"`
	Run        int             `json:"run"`
	Request    json.RawMessage `json:"request"`
}

// DryRun 输出每个席位对每条回答的请求，不发送、不读取凭据 / DryRun prints every seat's request for every answer without sending
func (j *Judger) DryRun(out io.Writer, answers []Answer) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	for _, s := range j.Seats {
		for _, a := range answers {
			user, err := buildInput(j.Questions[a.QuestionID], a, j.RuntimeReference)
			if err != nil {
				return err
			}
			body, err := provider.JudgeBody(provider.JudgeRequest{
				Model: s.Judge.Model, System: j.Prompt, User: user, MaxTokens: runner.InitialMaxTokens, JSON: true,
			})
			if err != nil {
				return err
			}
			if err := enc.Encode(plannedRequest{s.Judge.Vendor, a.Provider, a.QuestionID, a.Run, body}); err != nil {
				return err
			}
		}
	}
	return nil
}
