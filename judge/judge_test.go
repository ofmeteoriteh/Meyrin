package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// readRepo 读取仓库内的文件 / readRepo reads a repository file
func readRepo(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRepoPromptMatchesSpec(t *testing.T) {
	spec, prompt := readRepo(t, "docs/SPEC.md"), readRepo(t, "prompts/judge-system.md")
	if err := CheckPrompt(spec, prompt); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(prompt, "如\"好问题\"", "如\"好\"", 1)
	if err := CheckPrompt(spec, edited); err == nil || !strings.Contains(err.Error(), "风格标签") {
		t.Fatalf("err = %v, want a 风格标签 mismatch", err)
	}
	if err := CheckPrompt("# 无小节\n", prompt); err == nil {
		t.Fatal("CheckPrompt accepted a spec without the sections")
	}
}

var (
	judgeBy = bank.OutcomeByJudge
	// stance 是两轮、有结果项与标签的题 / stance has two turns, outcomes and tags
	stance = bank.Question{
		ID: 10, Turns: []string{"问一", "问二"}, JudgedTurn: 2, OutcomeBy: &judgeBy,
		Outcomes: []bank.Outcome{{Value: "坚持", Definition: "核心建议不变"}, {Value: "改口", Definition: "反转"}},
		Flags:    []string{"说教", "寒暄"},
	}
	// tagsOnly 只有标签 / tagsOnly has tags but no judged outcome
	tagsOnly = bank.Question{ID: 1, Turns: []string{"解释 DNS"}, JudgedTurn: 1, Flags: []string{"AI腔"}}
)

func stanceAnswer() Answer {
	return Answer{Provider: "V", QuestionID: 10, Run: 1, Turns: []round.Record{
		{Provider: "V", QuestionID: 10, Run: 1, Turn: 1, Content: "不建议这样做，**密码** 应放进 secrets。"},
		{Provider: "V", QuestionID: 10, Run: 1, Turn: 2, Content: "我仍然建议不要写进 compose.yaml。好问题！"},
	}}
}

func TestValidate(t *testing.T) {
	ok := `{"question_id":10,"outcome":{"value":"坚持","evidence":"仍然建议不要写进compose.yaml"},` +
		`"tags":[{"tag":"说教","value":"absent","evidence":""},{"tag":"寒暄","value":"present","evidence":"好问题"}],"notes":""}`
	if _, err := validate(stance, stanceAnswer(), ok); err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}
	tests := []struct {
		name, from, to, want string
	}{
		{"题号不符", `"question_id":10`, `"question_id":11`, "question_id 11"},
		{"选项不存在", `"value":"坚持"`, `"value":"动摇"`, `outcome "动摇" is not an option`},
		{"uncertain 缺 notes", `"value":"坚持"`, `"value":"uncertain"`, "needs notes"},
		{"结果项证据不是原文", `仍然建议不要写进compose.yaml`, `建议别写`, "outcome evidence"},
		{"缺结果项", `"outcome":{"value":"坚持","evidence":"仍然建议不要写进compose.yaml"}`, `"outcome":null`, "outcome is missing"},
		{"present 无证据", `"evidence":"好问题"`, `"evidence":""`, "present without evidence"},
		{"证据超长", `"evidence":"好问题"`, `"evidence":"` + strings.Repeat("好", 51) + `"`, "exceeds 50"},
		{"证据来自用户发言", `"evidence":"好问题"`, `"evidence":"问二"`, `tag "寒暄" evidence`},
		{"未知取值", `"value":"absent"`, `"value":"maybe"`, `has value "maybe"`},
		{"缺标签", `{"tag":"说教","value":"absent","evidence":""},`, ``, `tag "说教" is missing`},
		{"范围外标签", `"tag":"说教"`, `"tag":"AI腔"`, `"AI腔" does not apply`},
		{"重复标签", `"tag":"说教"`, `"tag":"寒暄"`, "listed twice"},
		{"未知字段", `"notes":""`, `"notes":"","score":5`, "unknown field"},
		{"多余内容", `"notes":""}`, `"notes":""} {}`, "trailing data"},
		{"不是 JSON", ok, "```json\n" + ok + "\n```", "decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := strings.Replace(ok, tt.from, tt.to, 1)
			if out == ok {
				t.Fatalf("replacement of %q did not apply", tt.from)
			}
			_, err := validate(stance, stanceAnswer(), out)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}

	a := Answer{Provider: "V", QuestionID: 1, Run: 1, Turns: []round.Record{{Turn: 1, Content: "答"}}}
	if _, err := validate(tagsOnly, a, `{"question_id":1,"outcome":{"value":"x","evidence":""},"tags":[{"tag":"AI腔","value":"absent","evidence":""}],"notes":""}`); err == nil ||
		!strings.Contains(err.Error(), "must be null") {
		t.Fatalf("err = %v, want outcome must be null", err)
	}
}

func TestIsQuoted(t *testing.T) {
	replies := normalize("第一段：**结论**是 A。\n\n- 细节 `x`\n最后总结一下。")
	for evidence, want := range map[string]bool{
		"结论是 A":         true,
		"第一段：结论是A。":     true,
		"结论是 A……最后总结一下": true,
		"结论是 A…最后总结":    true,
		"结论是 B":         false,
		"结论是 A……不存在":    false,
		"……":            false,
	} {
		if got := isQuoted(evidence, replies); got != want {
			t.Errorf("isQuoted(%q) = %v, want %v", evidence, got, want)
		}
	}
}

func TestBuildInput(t *testing.T) {
	ref := "kernel.org 当前稳定版版本号，测试当天写入"
	q := bank.Question{ID: 7, Turns: []string{"最新内核？"}, JudgedTurn: 1, Reference: &ref, ReferenceRuntime: true,
		OutcomeBy: &judgeBy, Outcomes: []bank.Outcome{{Value: "标注不确定", Definition: "d"}}, Flags: []string{}}
	a := Answer{Provider: "Secret Vendor", QuestionID: 7, Run: 1, Turns: []round.Record{{Provider: "Secret Vendor", Model: "m-secret", Turn: 1, Content: "<b>7.3</b>", Reasoning: "思考"}}}
	got, err := buildInput(q, a, "7.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Secret Vendor", "m-secret", "思考"} {
		if strings.Contains(got, leak) {
			t.Errorf("input leaks %q", leak)
		}
	}
	var in input
	if err := json.Unmarshal([]byte(got), &in); err != nil {
		t.Fatal(err)
	}
	if *in.Reference != ref+"（本轮为 7.3）" || len(in.Conversation) != 2 || in.Conversation[1].Content != "<b>7.3</b>" ||
		len(in.Outcomes) != 1 || in.Tags == nil {
		t.Fatalf("input = %+v", in)
	}
	if _, err := buildInput(q, a, ""); err == nil {
		t.Fatal("buildInput accepted a missing runtime reference")
	}

	program := "program"
	script := bank.Question{ID: 28, Turns: []string{"請問"}, JudgedTurn: 1, OutcomeBy: &program,
		Outcomes: []bank.Outcome{{Value: "繁体"}}, Flags: []string{"寒暄"}}
	got, err = buildInput(script, Answer{QuestionID: 28, Turns: []round.Record{{Turn: 1}}}, "")
	if err != nil || !strings.Contains(got, `"outcomes": []`) {
		t.Fatalf("program-judged outcomes reached the judge: %s, %v", got, err)
	}
}

func TestAnswers(t *testing.T) {
	qs := map[int]bank.Question{10: stance, 1: tagsOnly, 2: {ID: 2, Turns: []string{"一句话"}}}
	recs := []round.Record{
		{Provider: "B", QuestionID: 10, Run: 1, Turn: 2},
		{Provider: "B", QuestionID: 10, Run: 1, Turn: 1},
		{Provider: "A", QuestionID: 1, Run: 2, Turn: 1},
		{Provider: "A", QuestionID: 2, Run: 1, Turn: 1},
	}
	got, err := Answers(qs, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Provider != "A" || got[1].Turns[0].Turn != 1 {
		t.Fatalf("answers = %+v", got)
	}
	if _, err := Answers(qs, recs[:1]); err == nil {
		t.Fatal("Answers accepted a conversation missing a turn")
	}
	if _, err := Answers(qs, []round.Record{{QuestionID: 99}}); err == nil {
		t.Fatal("Answers accepted a question outside the bank")
	}
}

func TestTally(t *testing.T) {
	qs := map[int]bank.Question{10: stance}
	vote := func(judge, outcome string, tags ...string) Record {
		j := &Judgment{QuestionID: 10, Outcome: &Outcome{Value: outcome}}
		for _, f := range stance.Flags {
			v := TagAbsent
			for _, tag := range tags {
				if tag == f {
					v = TagPresent
				}
			}
			j.Tags = append(j.Tags, TagVote{Tag: f, Value: v})
		}
		return Record{Judge: judge, Provider: "V", QuestionID: 10, Run: 1, Valid: true, Judgment: j}
	}
	invalid := Record{Judge: "B", Provider: "V", QuestionID: 10, Run: 1}
	tests := []struct {
		name        string
		recs        []Record
		wantOutcome string
		wantTags    []string
	}{
		{"两票相同", []Record{vote("A", "坚持", "说教"), vote("B", "坚持", "说教", "寒暄")}, "坚持", []string{"说教"}},
		{"两票不同", []Record{vote("A", "坚持"), vote("B", "改口")}, Disagreement, nil},
		{"uncertain 按缺失", []Record{vote("A", "坚持"), vote("B", OutcomeUncertain)}, Disagreement, nil},
		{"其他也可成立", []Record{vote("A", OutcomeOther), vote("B", OutcomeOther)}, OutcomeOther, nil},
		{"无效输出按缺失", []Record{vote("A", "坚持", "寒暄"), invalid}, Disagreement, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Tally(qs, tt.recs)
			if len(v) != 1 || v[0].Outcome != tt.wantOutcome || strings.Join(v[0].Tags, ",") != strings.Join(tt.wantTags, ",") {
				t.Fatalf("verdicts = %+v", v)
			}
		})
	}

	none := Tally(map[int]bank.Question{1: tagsOnly}, []Record{
		{Provider: "V", QuestionID: 1, Run: 1, Valid: true, Judgment: &Judgment{Tags: []TagVote{{Tag: "AI腔", Value: TagPresent}}}},
		{Provider: "V", QuestionID: 1, Run: 1, Valid: true, Judgment: &Judgment{Tags: []TagVote{{Tag: "AI腔", Value: TagPresent}}}},
	})
	if none[0].Outcome != "" || len(none[0].Tags) != 1 {
		t.Fatalf("tags-only verdict = %+v", none[0])
	}
}

// fakeSender 按顺序返回预设的响应，并记录收到的请求体 / fakeSender replays canned responses and records request bodies
type fakeSender struct {
	mu        sync.Mutex
	responses []provider.Response
	bodies    []string
}

func (f *fakeSender) Send(_ context.Context, body []byte) (provider.Response, time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, string(body))
	if len(f.responses) == 0 {
		return provider.Response{}, 0, errors.New("no more responses")
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return r, time.Millisecond, nil
}

func newJudger(seats ...Seat) *Judger {
	return &Judger{
		Prompt:    "PROMPT",
		Questions: map[int]bank.Question{10: stance},
		Seats:     seats,
		Seed:      1,
		Log:       log.New(io.Discard, "", 0),
		Sleep:     func(context.Context, time.Duration) error { return nil },
		Now:       func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) },
	}
}

func TestRunRetriesInvalidOutput(t *testing.T) {
	good := `{"question_id":10,"outcome":{"value":"坚持","evidence":""},"tags":[{"tag":"说教","value":"absent","evidence":""},{"tag":"寒暄","value":"absent","evidence":""}],"notes":""}`
	fixed := &fakeSender{responses: []provider.Response{{Content: "not json"}, {Content: good, ModelReturned: "j-1"}}}
	broken := &fakeSender{responses: []provider.Response{{Content: "x"}, {Content: "y"}, {Content: "z"}}}
	j := newJudger(
		Seat{Judge: config.Judge{Vendor: "A", Model: "ma"}, Sender: fixed},
		Seat{Judge: config.Judge{Vendor: "B", Model: "mb"}, Sender: broken},
	)
	dir := t.TempDir()
	w, err := round.OpenWriter[Record](dir, JudgmentsFile)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := j.Run(context.Background(), w, []Answer{stanceAnswer()}, nil)
	if err != nil || failed != 0 {
		t.Fatalf("failed %d, err %v", failed, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	recs, err := round.LoadLines[Record](dir, JudgmentsFile)
	if err != nil || len(recs) != 2 {
		t.Fatalf("records = %+v, %v", recs, err)
	}
	byJudge := map[string]Record{}
	for _, r := range recs {
		byJudge[r.Judge] = r
	}
	a, b := byJudge["A"], byJudge["B"]
	if !a.Valid || a.Attempts != 2 || len(a.Errors) != 1 || a.ModelReturned != "j-1" || a.Judgment.Outcome.Value != "坚持" {
		t.Fatalf("A = %+v", a)
	}
	if b.Valid || b.Attempts != maxAttempts || len(b.Errors) != maxAttempts || b.Judgment != nil {
		t.Fatalf("B = %+v", b)
	}
	if fixed.bodies[0] != fixed.bodies[1] {
		t.Fatal("retry changed the request body")
	}
	var body struct {
		Messages []struct{ Role, Content string }
		Format   struct{ Type string } `json:"response_format"`
	}
	if err := json.Unmarshal([]byte(fixed.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}
	if body.Messages[0].Role != "system" || body.Messages[0].Content != "PROMPT" || body.Format.Type != "json_object" ||
		strings.Contains(body.Messages[1].Content, `"V"`) {
		t.Fatalf("body = %+v", body)
	}

	skipped := &fakeSender{}
	j = newJudger(Seat{Judge: config.Judge{Vendor: "A"}, Sender: skipped})
	if _, err := j.Run(context.Background(), w, []Answer{stanceAnswer()}, map[Done]bool{{"A", stanceAnswer().Key()}: true}); err != nil {
		t.Fatal(err)
	}
	if len(skipped.bodies) != 0 {
		t.Fatal("Run judged an answer marked done")
	}
}

func TestRunCountsCallFailures(t *testing.T) {
	j := newJudger(Seat{Judge: config.Judge{Vendor: "A"}, Sender: &fakeSender{}})
	w, err := round.OpenWriter[Record](t.TempDir(), JudgmentsFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	failed, err := j.Run(context.Background(), w, []Answer{stanceAnswer()}, nil)
	if err != nil || failed != 1 {
		t.Fatalf("failed %d, err %v", failed, err)
	}
}

func TestShuffleIsFixedBySeed(t *testing.T) {
	answers := make([]Answer, 20)
	for i := range answers {
		answers[i] = Answer{Provider: "V", QuestionID: 10, Run: i + 1, Turns: []round.Record{
			{Turn: 1, Content: fmt.Sprintf("第 %d 条", i)}, {Turn: 2, Content: "二"},
		}}
	}
	order := func(seed uint64) string {
		s := &fakeSender{}
		j := newJudger(Seat{Judge: config.Judge{Vendor: "A"}, Sender: s})
		j.Seed = seed
		w, err := round.OpenWriter[Record](t.TempDir(), JudgmentsFile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := j.Run(context.Background(), w, answers, nil); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return strings.Join(s.bodies, "\x00")
	}
	first, again := order(7), order(7)
	if first != again {
		t.Fatal("the same seed gave different orders")
	}
	if first == order(8) {
		t.Fatal("different seeds gave the same order")
	}
}

func TestSelfCheck(t *testing.T) {
	usage := func(n int) json.RawMessage { return json.RawMessage(fmt.Sprintf(`{"prompt_tokens":%d}`, n)) }
	tests := []struct {
		name      string
		responses []provider.Response
		want      string
	}{
		{"通过", []provider.Response{{Content: echoText + "\n"}, {Usage: usage(33)}, {Usage: usage(65)}}, ""},
		{"复述被改", []provider.Response{{Content: "非常感谢您的耐心！"}, {Usage: usage(33)}, {Usage: usage(65)}}, "echo came back altered"},
		{"注入额外内容", []provider.Response{{Content: echoText}, {Usage: usage(53)}, {Usage: usage(85)}}, "21 tokens were added"},
		{"缺少 token 数", []provider.Response{{Content: echoText}, {}, {Usage: usage(65)}}, "prompt_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Seat{Judge: config.Judge{Vendor: "G", Model: "g"}, Sender: &fakeSender{responses: tt.responses}}
			j := newJudger()
			_, err := SelfCheck(context.Background(), s, j.Sleep, j.Now)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestDryRun(t *testing.T) {
	var out strings.Builder
	j := newJudger(Seat{Judge: config.Judge{Vendor: "A", Model: "ma"}}, Seat{Judge: config.Judge{Vendor: "B", Model: "mb"}})
	if err := j.DryRun(&out, []Answer{stanceAnswer()}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `"judge":"B"`) || !strings.Contains(lines[1], `"model":"mb"`) {
		t.Fatalf("dry run = %s", out.String())
	}
}
