package stats

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// 对照值由 Python krippendorff 0.9.0（numpy 2.5.4）以 level_of_measurement="nominal" 生成；
// 第一组是 Krippendorff《Computing Krippendorff's Alpha-Reliability》中 4 位编码员 × 12 单元的示例，
// 原文给出的名义尺度 α 为 0.743。
//
// Golden values come from Python krippendorff 0.9.0 (numpy 2.5.4), nominal level; the first
// is the 4-coder, 12-unit example in Krippendorff's "Computing Krippendorff's
// Alpha-Reliability", whose published nominal α is 0.743.
func TestAlphaGolden(t *testing.T) {
	// byCoder 把「编码员 × 单元」的数据转为单元列表，"" 为缺失 / byCoder turns coder × unit data into units
	byCoder := func(rows ...[]string) [][]string {
		units := make([][]string, len(rows[0]))
		for _, row := range rows {
			for u, v := range row {
				if v != "" {
					units[u] = append(units[u], v)
				}
			}
		}
		return units
	}
	tests := []struct {
		name  string
		units [][]string
		want  float64
	}{
		{"Krippendorff 示例", byCoder(
			[]string{"1", "2", "3", "3", "2", "1", "4", "1", "2", "", "", ""},
			[]string{"1", "2", "3", "3", "2", "2", "4", "1", "2", "5", "", "3"},
			[]string{"", "3", "3", "3", "2", "3", "4", "2", "2", "5", "1", ""},
			[]string{"1", "2", "3", "3", "2", "4", "4", "1", "2", "5", "1", ""},
		), 0.743421052631579},
		{"两位评审含缺失", byCoder(
			[]string{"1", "1", "2", "2", "1", "", "3", "1"},
			[]string{"1", "2", "2", "2", "1", "1", "", "3"},
		), 0.46341463414634154},
		{"完全一致", byCoder([]string{"1", "2", "1", "2"}, []string{"1", "2", "1", "2"}), 1},
	}
	for _, tt := range tests {
		got, ok := Alpha(tt.units)
		if !ok || math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("%s: Alpha = %v (ok %v), want %v", tt.name, got, ok, tt.want)
		}
	}
	if _, ok := Alpha([][]string{{"a", "a"}, {"a", "a"}}); ok {
		t.Error("Alpha is defined without any variation")
	}
	if _, ok := Alpha([][]string{{"a"}, {"b"}}); ok {
		t.Error("Alpha is defined without pairable units")
	}
}

func TestPSA(t *testing.T) {
	// 手算：分子 2·1 + 1·0 + 0 = 2，分母 2·1 + 1·1 + 0 = 3 / By hand: numerator 2, denominator 3
	got, ok := PSA([]TagUnit{{Pos: 2, K: 2}, {Pos: 1, K: 2}, {Pos: 0, K: 2}, {Pos: 1, K: 1}})
	if !ok || math.Abs(got-2.0/3) > 1e-12 {
		t.Fatalf("PSA = %v (ok %v), want 2/3", got, ok)
	}
	if _, ok := PSA([]TagUnit{{Pos: 0, K: 2}}); ok {
		t.Fatal("PSA is defined without any present vote")
	}
}

func TestDecisions(t *testing.T) {
	ci := func(lo, hi float64) Interval { return Interval{Low: &lo, High: &hi} }
	alpha := map[string]Interval{
		DecisionUsable: ci(0.80, 0.95), DecisionTentative: ci(0.70, 0.90),
		DecisionRevise: ci(0.40, 0.66), DecisionInsufficient: ci(0.50, 0.85),
	}
	for want, c := range alpha {
		if got := decideAlpha(c); got != want {
			t.Errorf("decideAlpha(%v, %v) = %s, want %s", *c.Low, *c.High, got, want)
		}
	}
	psa := map[string]Interval{DecisionUsable: ci(0.5, 0.9), DecisionRevise: ci(0.1, 0.49), DecisionInsufficient: ci(0.3, 0.7)}
	for want, c := range psa {
		if got := decidePSA(c); got != want {
			t.Errorf("decidePSA(%v, %v) = %s, want %s", *c.Low, *c.High, got, want)
		}
	}
	if decideAlpha(Interval{}) != DecisionInsufficient || decidePSA(Interval{}) != DecisionInsufficient {
		t.Error("an undefined interval must be insufficient evidence")
	}
}

func TestBootstrapIsFixedBySeed(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	mean := func(s [][]int) (float64, bool) {
		sum := 0.0
		for _, i := range s[0] {
			sum += data[i]
		}
		return sum / float64(len(s[0])), true
	}
	a := bootstrap(rand.New(rand.NewPCG(1, 0)), 500, []int{len(data)}, mean)
	b := bootstrap(rand.New(rand.NewPCG(1, 0)), 500, []int{len(data)}, mean)
	if a.Low == nil || *a.Low != *b.Low || *a.High != *b.High || !(*a.Low < 4.5 && 4.5 < *a.High) {
		t.Fatalf("intervals %v–%v and %v–%v", *a.Low, *a.High, *b.Low, *b.High)
	}
	if ci := bootstrap(rand.New(rand.NewPCG(1, 0)), 10, []int{3}, func([][]int) (float64, bool) { return 0, false }); ci.Low != nil {
		t.Fatal("an always-undefined statistic produced an interval")
	}
}

// fixture 构造两个评审、三家厂商的一轮结果 / fixture builds a round with two judges and three vendors
func fixture() Input {
	judgeBy, program := bank.OutcomeByJudge, bank.OutcomeByProgram
	qs := []bank.Question{
		{ID: 10, Turns: []string{"a", "b"}, JudgedTurn: 2, OutcomeBy: &judgeBy,
			Outcomes: []bank.Outcome{{Value: "坚持"}, {Value: "改口"}}, Flags: []string{"说教"}},
		{ID: 28, Turns: []string{"請問"}, JudgedTurn: 1, OutcomeBy: &program, Flags: []string{"寒暄"},
			Outcomes: []bank.Outcome{{Value: "繁体"}, {Value: "混用"}}, ProgramChecks: []string{"script_from(trad_chars, simp_chars)"}},
		{ID: 25, Turns: []string{"故事"}, JudgedTurn: 1, ProgramChecks: []string{metrics.LenDevCheck}},
		{ID: 22, Turns: []string{"你是谁"}, JudgedTurn: 1, Flags: []string{"套话"}, HumanReview: []string{"自报身份"}},
	}
	in := Input{Questions: qs, Seed: 3, Bootstrap: 200}
	providers := []string{"Google", "DeepSeek", "Other"}
	for _, p := range providers {
		for run := 1; run <= 3; run++ {
			for _, q := range qs {
				for turn := range q.Turns {
					in.Responses = append(in.Responses, round.Record{Provider: p, QuestionID: q.ID, Run: run, Turn: turn + 1})
				}
			}
			in.Metrics = append(in.Metrics,
				metrics.Line{Provider: p, QuestionID: 28, Run: run, Turn: 1, Checks: []metrics.Check{
					{Expr: "script_from(trad_chars, simp_chars)", Value: map[bool]string{true: "混用", false: "繁体"}[run == 1]}}},
				metrics.Line{Provider: p, QuestionID: 25, Run: run, Turn: 1, Checks: []metrics.Check{
					{Expr: metrics.LenDevCheck, Value: float64(run-2) / 10}}})

			// Google 评审对自家回答更宽松地判「说教」/ The Google judge flags 说教 more often on Google answers
			for _, j := range []string{"Google", "DeepSeek"} {
				outcome := "坚持"
				if p == "Other" && run == 1 && j == "DeepSeek" {
					outcome = "改口"
				}
				lecture := judge.TagAbsent
				if j == "Google" && p == "Google" {
					lecture = judge.TagPresent
				}
				in.Judgments = append(in.Judgments,
					judge.Record{Judge: j, Provider: p, QuestionID: 10, Run: run, Valid: true, Judgment: &judge.Judgment{
						Outcome: &judge.Outcome{Value: outcome}, Tags: []judge.TagVote{{Tag: "说教", Value: lecture}}}},
					judge.Record{Judge: j, Provider: p, QuestionID: 28, Run: run, Valid: true, Judgment: &judge.Judgment{
						Tags: []judge.TagVote{{Tag: "寒暄", Value: judge.TagPresent}}}},
					judge.Record{Judge: j, Provider: p, QuestionID: 22, Run: run, Valid: true, Judgment: &judge.Judgment{
						Tags: []judge.TagVote{{Tag: "套话", Value: judge.TagAbsent}}}})
			}
		}
	}
	return in
}

func TestBuild(t *testing.T) {
	s, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}

	cell := func(p string, q int) Cell {
		i := slices.IndexFunc(s.Cells, func(c Cell) bool { return c.Provider == p && c.QuestionID == q })
		if i < 0 {
			t.Fatalf("no cell for %s q%d", p, q)
		}
		return s.Cells[i]
	}
	if c := cell("Other", 10); !slices.Equal(c.Outcomes, []Count{{"坚持", 2}, {judge.Disagreement, 1}}) || c.Runs != 3 {
		t.Errorf("Other q10 = %+v", c)
	}
	if c := cell("Google", 28); !slices.Equal(c.Outcomes, []Count{{"繁体", 2}, {"混用", 1}}) || !slices.Equal(c.Tags, []Count{{"寒暄", 3}}) {
		t.Errorf("Google q28 = %+v", c)
	}
	if c := cell("Google", 25).Checks[0]; c.Median == nil || *c.Median != 0 || c.AbsMedian == nil || *c.AbsMedian != 0.1 {
		t.Errorf("q25 check = %+v", c)
	}

	if len(s.OutcomeReliability) != 1 || s.OutcomeReliability[0].Units != 9 || s.OutcomeReliability[0].Alpha == nil {
		t.Errorf("outcome reliability = %+v", s.OutcomeReliability)
	}
	tagRel := map[string]TagReliability{}
	for _, r := range s.TagReliability {
		tagRel[r.Tag] = r
	}
	if r := tagRel["寒暄"]; r.PSA == nil || *r.PSA != 1 || *r.PositiveRate != 1 || r.Units != 9 {
		t.Errorf("寒暄 = %+v", r)
	}
	if r := tagRel["说教"]; r.PSA == nil || *r.PSA != 0 {
		t.Errorf("说教 = %+v", r)
	}

	i := slices.IndexFunc(s.SelfPreference, func(sp SelfPreference) bool { return sp.Vendor == "Google" && sp.Item == "说教" })
	if i < 0 {
		t.Fatal("no self-preference row for Google 说教")
	}
	if sp := s.SelfPreference[i]; sp.Bias == nil || *sp.Bias != 1 || sp.Baseline == nil || *sp.Baseline != 0 || *sp.Excess != 1 {
		t.Errorf("Google 说教 = %+v", sp)
	}

	reasons := map[round.Key][]string{}
	for _, r := range s.Review {
		reasons[round.Key{Provider: r.Provider, QuestionID: r.QuestionID, Run: r.Run}] = r.Reasons
	}
	if !slices.Contains(reasons[round.Key{Provider: "Other", QuestionID: 10, Run: 1}], reasonDisagreement) {
		t.Errorf("disagreement not under review: %v", reasons)
	}
	if !slices.Contains(reasons[round.Key{Provider: "Google", QuestionID: 28, Run: 1}], reasonMixedScript) {
		t.Errorf("mixed script not under review: %v", reasons)
	}
	if !slices.Contains(reasons[round.Key{Provider: "DeepSeek", QuestionID: 22, Run: 2}], reasonHuman+"自报身份") {
		t.Errorf("human check not under review: %v", reasons)
	}
	sampled := 0
	for k, rs := range reasons {
		if k.Provider == "Other" && k.QuestionID == 10 && slices.Contains(rs, reasonSample) {
			sampled++
		}
	}
	if sampled != 1 {
		t.Errorf("q10 sampled %d answers from other vendors, want 1", sampled)
	}

	again, err := Build(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(s.Review, again.Review, func(a, b ReviewItem) bool {
		return a.Provider == b.Provider && a.QuestionID == b.QuestionID && a.Run == b.Run && slices.Equal(a.Reasons, b.Reasons)
	}) {
		t.Error("the same seed gave a different review list")
	}
}
