package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// step 是假厂商对一次请求的回应 / step is the fake vendor's reply to one request
type step struct {
	resp provider.Response
	err  error
}

// fakeSender 按顺序回放预设回应并记录收到的请求体 / fakeSender replays scripted replies and records bodies
type fakeSender struct {
	mu     sync.Mutex
	steps  []step
	bodies []string
}

func (f *fakeSender) Send(_ context.Context, body []byte) (provider.Response, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, string(body))
	if len(f.steps) == 0 {
		return provider.Response{Content: "默认回答", FinishReason: "stop", ModelReturned: "m-0101"}, time.Millisecond, nil
	}
	s := f.steps[0]
	f.steps = f.steps[1:]
	return s.resp, 5 * time.Millisecond, s.err
}

var (
	single   = bank.Question{ID: 1, Turns: []string{"问题"}}
	twoTurns = bank.Question{ID: 10, Turns: []string{"第一轮", "第二轮"}}
	ok       = func(content string) step {
		return step{resp: provider.Response{Content: content, Reasoning: "推理", FinishReason: "stop", ModelReturned: "m-0101"}}
	}
)

// newRunner 构造单厂商 runner，记录全部退避时长 / newRunner builds a one-vendor runner that records backoff waits
func newRunner(qs []bank.Question, runs int, s Sender) (*Runner, *[]time.Duration) {
	var waits []time.Duration
	return &Runner{
		Questions: qs,
		Targets: []Target{{
			Vendor:   config.Vendor{Name: "V", Model: "m"},
			Endpoint: config.Endpoint{Protocol: config.OpenAIChat},
			Sender:   s,
		}},
		Runs: runs,
		Log:  log.New(io.Discard, "", 0),
		Sleep: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.FixedZone("SGT", 8*3600)) },
	}, &waits
}

// runOnce 运行并读回写入的记录 / runOnce runs and reads back the written records
func runOnce(t *testing.T, r *Runner, done map[round.Key]bool) ([]round.Record, int) {
	t.Helper()
	dir := t.TempDir()
	w, err := round.OpenWriter[round.Record](dir, round.ResponsesFile)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := r.Run(context.Background(), w, done)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	recs, err := round.LoadRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	return recs, failed
}

func TestMultiTurnResendsContentOnly(t *testing.T) {
	s := &fakeSender{steps: []step{ok("回答一"), ok("回答二")}}
	r, _ := newRunner([]bank.Question{twoTurns}, 1, s)
	recs, failed := runOnce(t, r, nil)
	if failed != 0 || len(recs) != 2 {
		t.Fatalf("failed %d, records %d", failed, len(recs))
	}
	if recs[0].Turn != 1 || recs[1].Turn != 2 || recs[1].Content != "回答二" || recs[1].Reasoning != "推理" {
		t.Fatalf("records = %+v", recs)
	}
	want := `"messages":[{"role":"user","content":"第一轮"},{"role":"assistant","content":"回答一"},{"role":"user","content":"第二轮"}]`
	if !strings.Contains(s.bodies[1], want) || strings.Contains(s.bodies[1], "推理") {
		t.Fatalf("second request = %s", s.bodies[1])
	}
	if recs[0].Timestamp != "2026-10-22T12:00:00+08:00" || string(recs[1].Request) != s.bodies[1] {
		t.Fatalf("timestamp %q / request %s", recs[0].Timestamp, recs[1].Request)
	}
}

func TestRetryKeepsBodyAndBacksOff(t *testing.T) {
	limited := &provider.HTTPError{Status: 429, RetryAfter: 30 * time.Second}
	s := &fakeSender{steps: []step{
		{err: limited},
		{err: &provider.HTTPError{Status: 503}},
		{resp: provider.Response{FinishReason: "network_error", Transient: true}},
		ok("终于"),
	}}
	r, waits := newRunner([]bank.Question{single}, 1, s)
	recs, failed := runOnce(t, r, nil)
	if failed != 0 || len(recs) != 1 || recs[0].Retries != 3 || recs[0].Content != "终于" {
		t.Fatalf("failed %d, records %+v", failed, recs)
	}
	for _, b := range s.bodies[1:] {
		if b != s.bodies[0] {
			t.Fatalf("retried body changed: %s vs %s", b, s.bodies[0])
		}
	}
	if want := []time.Duration{30 * time.Second, 4 * time.Second, 8 * time.Second}; !equalDurations(*waits, want) {
		t.Fatalf("waits = %v, want %v", *waits, want)
	}
}

func TestTruncationRaisesMaxTokens(t *testing.T) {
	s := &fakeSender{steps: []step{{resp: provider.Response{FinishReason: "length", Truncated: true}}, ok("完整")}}
	r, _ := newRunner([]bank.Question{single}, 1, s)
	recs, _ := runOnce(t, r, nil)
	if len(recs) != 1 || recs[0].Retries != 1 {
		t.Fatalf("records = %+v", recs)
	}
	if !strings.Contains(s.bodies[0], `"max_tokens":8192`) || !strings.Contains(string(recs[0].Request), `"max_tokens":16384`) {
		t.Fatalf("bodies = %q, recorded %s", s.bodies, recs[0].Request)
	}
}

func TestFailuresAreNotWritten(t *testing.T) {
	truncated := step{resp: provider.Response{Truncated: true}}
	tests := []struct {
		name  string
		steps []step
	}{
		{"不可重试的错误", []step{{err: &provider.HTTPError{Status: 400}}}},
		{"重试耗尽", []step{
			{err: &provider.HTTPError{Status: 500}}, {err: &provider.HTTPError{Status: 500}},
			{err: &provider.HTTPError{Status: 500}}, {err: &provider.HTTPError{Status: 500}},
			{err: &provider.HTTPError{Status: 500}}, {err: &provider.HTTPError{Status: 500}},
			{err: &provider.HTTPError{Status: 500}},
		}},
		{"上限内始终截断", []step{truncated, truncated, truncated, truncated}},
		{"多轮题第二轮失败", []step{ok("一"), {err: errors.New("boom")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := single
			if strings.HasPrefix(tt.name, "多轮") {
				q = twoTurns
			}
			r, _ := newRunner([]bank.Question{q}, 1, &fakeSender{steps: tt.steps})
			recs, failed := runOnce(t, r, nil)
			if failed != 1 || len(recs) != 0 {
				t.Fatalf("failed %d, records %d", failed, len(recs))
			}
		})
	}
}

func TestResumeSkipsDoneConversations(t *testing.T) {
	s := &fakeSender{}
	r, _ := newRunner([]bank.Question{single, twoTurns}, 2, s)
	done := map[round.Key]bool{{Provider: "V", QuestionID: 1, Run: 1}: true, {Provider: "V", QuestionID: 10, Run: 2}: true}
	recs, _ := runOnce(t, r, done)
	var got []round.Key
	for _, rec := range recs {
		if rec.Turn == 1 {
			got = append(got, rec.Key())
		}
	}
	want := []round.Key{{Provider: "V", QuestionID: 1, Run: 2}, {Provider: "V", QuestionID: 10, Run: 1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ran %v, want %v", got, want)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := newRunner([]bank.Question{single}, 3, &fakeSender{})
	w, err := round.OpenWriter[round.Record](t.TempDir(), round.ResponsesFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := r.Run(ctx, w, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestBackoff(t *testing.T) {
	tests := []struct {
		failures int
		err      error
		want     time.Duration
	}{
		{0, &provider.HTTPError{Status: 500}, 2 * time.Second},
		{3, &provider.HTTPError{Status: 500}, 16 * time.Second},
		{10, &provider.HTTPError{Status: 500}, time.Minute},
		{0, &provider.HTTPError{Status: 429, RetryAfter: 90 * time.Second}, 90 * time.Second},
		{4, &provider.HTTPError{Status: 429, RetryAfter: time.Second}, 32 * time.Second},
	}
	for _, tt := range tests {
		if got := backoff(tt.failures, tt.err); got != tt.want {
			t.Errorf("backoff(%d, %v) = %v, want %v", tt.failures, tt.err, got, tt.want)
		}
	}
}

func TestDryRun(t *testing.T) {
	var out bytes.Buffer
	targets := []Target{{
		Vendor:   config.Vendor{Name: "V", Model: "m"},
		Endpoint: config.Endpoint{URL: "https://example.com/v1/messages", Protocol: config.AnthropicMessages},
	}}
	if err := DryRun(&out, []bank.Question{single, twoTurns}, targets); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	var last plannedRequest
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatal(err)
	}
	if last.QuestionID != 10 || last.Turn != 2 || last.Endpoint != targets[0].Endpoint.URL ||
		!strings.Contains(string(last.Request), placeholderReply) {
		t.Fatalf("last line = %+v", last)
	}
}

func TestFetchKernelStable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			respond(t, w, `{"latest_stable":{"version":"7.3"},"releases":[]}`)
		case "/empty":
			respond(t, w, `{"releases":[]}`)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	if v, err := FetchKernelStable(context.Background(), srv.URL+"/ok"); err != nil || v != "7.3" {
		t.Fatalf("got %q, %v", v, err)
	}
	for _, path := range []string{"/empty", "/down"} {
		if _, err := FetchKernelStable(context.Background(), srv.URL+path); err == nil {
			t.Errorf("%s: want an error", path)
		}
	}
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// respond 在测试服务器中写出响应体；处理函数运行在其他 goroutine，只能用 t.Error。
// respond writes a response body in a test server; handlers run on another goroutine, so it uses t.Error.
func respond(t *testing.T, w http.ResponseWriter, body string) {
	if _, err := io.WriteString(w, body); err != nil {
		t.Error(err)
	}
}
