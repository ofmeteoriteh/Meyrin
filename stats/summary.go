package stats

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// Input 是一轮的全部结果 / Input is everything a round produced
type Input struct {
	Questions []bank.Question
	Responses []round.Record
	Metrics   []metrics.Line
	Judgments []judge.Record
	Seed      uint64
	Bootstrap int
}

// Summary 是 summary.json 的内容 / Summary is the content of summary.json
type Summary struct {
	Cells              []Cell               `json:"cells"`
	OutcomeReliability []OutcomeReliability `json:"outcome_reliability"`
	TagReliability     []TagReliability     `json:"tag_reliability"`
	SelfPreference     []SelfPreference     `json:"self_preference"`
	Review             []ReviewItem         `json:"review"`
}

// Cell 是记录表的一格：一家厂商在一道题上各次运行的汇总 / Cell is one record-table cell: a vendor's runs on a question
type Cell struct {
	Provider   string         `json:"provider"`
	QuestionID int            `json:"question_id"`
	Runs       int            `json:"runs"`
	Outcomes   []Count        `json:"outcomes"`
	Tags       []Count        `json:"tags"`
	Checks     []CheckSummary `json:"checks"`
}

// Count 是一个取值出现的运行次数 / Count is how many runs had a value
type Count struct {
	Value string `json:"value"`
	N     int    `json:"n"`
}

// CheckSummary 是一项程序检查在各次运行中的结果；数值检查附中位数。
// CheckSummary is one program check across runs; numeric checks carry medians.
type CheckSummary struct {
	Expr      string   `json:"expr"`
	Values    []any    `json:"values"`
	Passes    *int     `json:"passes,omitempty"`
	Median    *float64 `json:"median,omitempty"`
	AbsMedian *float64 `json:"abs_median,omitempty"`
}

// OutcomeReliability 是一道题结果项的 α 与判定；Units 为可配对的回答数。
// OutcomeReliability is a question's outcome α and decision; Units counts pairable answers.
type OutcomeReliability struct {
	QuestionID int      `json:"question_id"`
	Alpha      *float64 `json:"alpha"`
	CI         Interval `json:"ci"`
	Units      int      `json:"units"`
	Decision   string   `json:"decision"`
}

// TagReliability 是一个标签的 PSA、阳性率与判定；Units 为至少 2 票有效的回答数。
// TagReliability is a tag's PSA, positive rate and decision; Units counts answers with two or more valid votes.
type TagReliability struct {
	Tag          string   `json:"tag"`
	PSA          *float64 `json:"psa"`
	CI           Interval `json:"ci"`
	PositiveRate *float64 `json:"positive_rate"`
	Units        int      `json:"units"`
	Decision     string   `json:"decision"`
}

// SelfPreference 是一个评审在自家回答上的偏差估计。Bias 为自家评审与其余评审的阳性率（或选项比例）
// 之差；Baseline 为同样两方在其余厂商回答上的差，即整体松紧；Excess = Bias − Baseline。
//
// SelfPreference estimates a judge's bias on its own vendor's answers. Bias is the gap
// between the own judge and the other judges; Baseline is the same gap on other vendors'
// answers; Excess is Bias minus Baseline.
type SelfPreference struct {
	Vendor     string   `json:"vendor"`
	QuestionID int      `json:"question_id,omitempty"`
	Item       string   `json:"item"`
	Bias       *float64 `json:"bias"`
	BiasCI     Interval `json:"bias_ci"`
	Baseline   *float64 `json:"baseline"`
	BaselineCI Interval `json:"baseline_ci"`
	Excess     *float64 `json:"excess"`
	ExcessCI   Interval `json:"excess_ci"`
	Units      int      `json:"units"`
}

// ReviewItem 是须人工复核的一条回答及原因 / ReviewItem is an answer due for human review, with reasons
type ReviewItem struct {
	Provider   string   `json:"provider"`
	QuestionID int      `json:"question_id"`
	Run        int      `json:"run"`
	Reasons    []string `json:"reasons"`
}

// 复核原因 / Review reasons
const (
	reasonDisagreement   = "分歧"
	reasonOutcomeUnclear = "结果项证据不足"
	reasonTagUnclear     = "标签证据不足："
	reasonHuman          = "人工核查："
	reasonMixedScript    = "简繁混用"
	reasonSample         = "抽检"
	scriptMixed          = "混用"
)

// Build 计算一轮的汇总 / Build computes a round's summary
func Build(in Input) (Summary, error) {
	questions := map[int]bank.Question{}
	for _, q := range in.Questions {
		questions[q.ID] = q
	}
	answers, err := answerKeys(questions, in.Responses)
	if err != nil {
		return Summary{}, err
	}
	votes := map[round.Key][]judge.Record{}
	for _, r := range in.Judgments {
		if r.Valid {
			votes[r.Answer()] = append(votes[r.Answer()], r)
		}
	}
	checks := map[round.Key]map[string]metrics.Check{}
	for _, l := range in.Metrics {
		k := round.Key{Provider: l.Provider, QuestionID: l.QuestionID, Run: l.Run}
		if checks[k] == nil {
			checks[k] = map[string]metrics.Check{}
		}
		for _, c := range l.Checks {
			checks[k][c.Expr] = c
		}
	}
	verdicts := map[round.Key]judge.Verdict{}
	for _, v := range judge.Tally(questions, in.Judgments) {
		verdicts[round.Key{Provider: v.Provider, QuestionID: v.QuestionID, Run: v.Run}] = v
	}

	rng := rand.New(rand.NewPCG(in.Seed, 0))
	s := Summary{
		Cells:              cells(in.Questions, answers, verdicts, checks),
		OutcomeReliability: outcomeReliability(rng, in.Bootstrap, in.Questions, answers, votes),
		TagReliability:     tagReliability(rng, in.Bootstrap, in.Questions, answers, votes),
		SelfPreference:     selfPreference(rng, in.Bootstrap, in.Questions, answers, votes, in.Judgments),
	}
	s.Review = review(rng, s, in.Questions, answers, verdicts, checks, votes)
	return s, nil
}

// answerKeys 按题号列出每题的全部回答，回答按厂商与运行次数排序。
// answerKeys lists each question's answers, sorted by vendor and run.
func answerKeys(questions map[int]bank.Question, recs []round.Record) (map[int][]round.Key, error) {
	seen := map[round.Key]bool{}
	byQuestion := map[int][]round.Key{}
	for _, r := range recs {
		if _, ok := questions[r.QuestionID]; !ok {
			return nil, fmt.Errorf("%s run %d: question %d is not in the bank", r.Provider, r.Run, r.QuestionID)
		}
		if k := r.Key(); !seen[k] {
			seen[k] = true
			byQuestion[k.QuestionID] = append(byQuestion[k.QuestionID], k)
		}
	}
	for _, keys := range byQuestion {
		slices.SortFunc(keys, func(a, b round.Key) int {
			return cmp.Or(cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Run, b.Run))
		})
	}
	return byQuestion, nil
}

// cells 按厂商与题号汇总各次运行 / cells aggregates runs per vendor and question
func cells(qs []bank.Question, answers map[int][]round.Key, verdicts map[round.Key]judge.Verdict, checks map[round.Key]map[string]metrics.Check) []Cell {
	var out []Cell
	for _, q := range qs {
		byProvider := map[string][]round.Key{}
		var providers []string
		for _, k := range answers[q.ID] {
			if byProvider[k.Provider] == nil {
				providers = append(providers, k.Provider)
			}
			byProvider[k.Provider] = append(byProvider[k.Provider], k)
		}
		for _, p := range providers {
			keys := byProvider[p]
			c := Cell{Provider: p, QuestionID: q.ID, Runs: len(keys), Checks: []CheckSummary{}}
			outcomes, tags := map[string]int{}, map[string]int{}
			for _, k := range keys {
				if o := runOutcome(q, verdicts[k], checks[k]); o != "" {
					outcomes[o]++
				}
				for _, t := range verdicts[k].Tags {
					tags[t]++
				}
			}
			c.Outcomes, c.Tags = sortedCounts(outcomes), sortedCounts(tags)
			for _, expr := range q.ProgramChecks {
				c.Checks = append(c.Checks, summarizeCheck(expr, keys, checks))
			}
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b Cell) int { return cmp.Compare(a.Provider, b.Provider) })
	return out
}

// runOutcome 返回一次运行成立的结果项：评审判定的取计票结果，程序判定的取该题检查的取值。
// runOutcome returns a run's outcome: the tally for judged outcomes, the check value for program outcomes.
func runOutcome(q bank.Question, v judge.Verdict, checks map[string]metrics.Check) string {
	if q.OutcomeBy == nil {
		return ""
	}
	if *q.OutcomeBy != bank.OutcomeByProgram {
		return v.Outcome
	}
	for _, expr := range q.ProgramChecks {
		if s, ok := checks[expr].Value.(string); ok {
			return s
		}
	}
	return ""
}

// sortedCounts 按次数降序、取值升序排列 / sortedCounts orders by count descending, then value
func sortedCounts(m map[string]int) []Count {
	out := []Count{}
	for v, n := range m {
		out = append(out, Count{v, n})
	}
	slices.SortFunc(out, func(a, b Count) int { return cmp.Or(cmp.Compare(b.N, a.N), cmp.Compare(a.Value, b.Value)) })
	return out
}

// summarizeCheck 汇总一项检查在各次运行中的取值 / summarizeCheck gathers one check across runs
func summarizeCheck(expr string, keys []round.Key, checks map[round.Key]map[string]metrics.Check) CheckSummary {
	cs := CheckSummary{Expr: expr, Values: []any{}}
	var numbers []float64
	passes, judged := 0, false
	for _, k := range keys {
		c, ok := checks[k][expr]
		if !ok {
			cs.Values = append(cs.Values, nil)
			continue
		}
		cs.Values = append(cs.Values, c.Value)
		if c.Pass != nil {
			judged = true
			if *c.Pass {
				passes++
			}
		}
		if f, ok := number(c.Value); ok {
			numbers = append(numbers, f)
		}
	}
	if judged {
		cs.Passes = &passes
	}
	if len(numbers) > 0 {
		cs.Median = new(median(numbers))
		if expr == metrics.LenDevCheck {
			abs := make([]float64, len(numbers))
			for i, f := range numbers {
				abs[i] = math.Abs(f)
			}
			cs.AbsMedian = new(median(abs))
		}
	}
	return cs
}

// number 把 JSON 解码后的数值统一为 float64 / number normalizes a decoded JSON number to float64
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// median 返回中位数 / median returns the median
func median(xs []float64) float64 {
	s := slices.Clone(xs)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// outcomeValues 返回一条回答上各评审的有效结果项，uncertain 按缺失处理 / outcomeValues returns valid outcome votes, uncertain as missing
func outcomeValues(recs []judge.Record) []string {
	var vs []string
	for _, r := range recs {
		if o := r.Judgment.Outcome; o != nil && o.Value != judge.OutcomeUncertain {
			vs = append(vs, o.Value)
		}
	}
	return vs
}

// outcomeReliability 对评审判定结果项的每道题计算 α 及其区间 / outcomeReliability computes α per judged question
func outcomeReliability(rng *rand.Rand, b int, qs []bank.Question, answers map[int][]round.Key, votes map[round.Key][]judge.Record) []OutcomeReliability {
	out := []OutcomeReliability{}
	for _, q := range qs {
		if q.OutcomeBy == nil || *q.OutcomeBy != bank.OutcomeByJudge || len(answers[q.ID]) == 0 {
			continue
		}
		units := make([][]string, len(answers[q.ID]))
		pairable := 0
		for i, k := range answers[q.ID] {
			units[i] = outcomeValues(votes[k])
			if len(units[i]) >= 2 {
				pairable++
			}
		}
		r := OutcomeReliability{QuestionID: q.ID, Units: pairable}
		if a, ok := Alpha(units); ok {
			r.Alpha = &a
		}
		r.CI = bootstrap(rng, b, []int{len(units)}, func(s [][]int) (float64, bool) {
			pick := make([][]string, len(s[0]))
			for i, j := range s[0] {
				pick[i] = units[j]
			}
			return Alpha(pick)
		})
		r.Decision = decideAlpha(r.CI)
		out = append(out, r)
	}
	return out
}

// tagUnit 统计一条回答上某标签的有效票，uncertain 按缺失处理 / tagUnit counts valid votes on a tag, uncertain as missing
func tagUnit(recs []judge.Record, tag string, judgeOK func(string) bool) TagUnit {
	var u TagUnit
	for _, r := range recs {
		if !judgeOK(r.Judge) {
			continue
		}
		for _, t := range r.Judgment.Tags {
			if t.Tag != tag {
				continue
			}
			switch t.Value {
			case judge.TagPresent:
				u.Pos++
				u.K++
			case judge.TagAbsent:
				u.K++
			}
		}
	}
	return u
}

// anyJudge 接受所有评审 / anyJudge accepts every judge
func anyJudge(string) bool { return true }

// tagList 按题库首次出现的顺序列出全部标签及其适用的回答 / tagList lists every tag in bank order with the answers it applies to
func tagList(qs []bank.Question, answers map[int][]round.Key) ([]string, map[string][]round.Key) {
	var tags []string
	applies := map[string][]round.Key{}
	for _, q := range qs {
		for _, f := range q.Flags {
			if _, ok := applies[f]; !ok {
				tags = append(tags, f)
				applies[f] = []round.Key{}
			}
			applies[f] = append(applies[f], answers[q.ID]...)
		}
	}
	return tags, applies
}

// tagReliability 对每个标签计算 PSA、阳性率及区间 / tagReliability computes PSA and positive rate per tag
func tagReliability(rng *rand.Rand, b int, qs []bank.Question, answers map[int][]round.Key, votes map[round.Key][]judge.Record) []TagReliability {
	out := []TagReliability{}
	tags, applies := tagList(qs, answers)
	for _, tag := range tags {
		keys := applies[tag]
		if len(keys) == 0 {
			continue
		}
		units := make([]TagUnit, len(keys))
		r := TagReliability{Tag: tag}
		pos, k := 0, 0
		for i, key := range keys {
			units[i] = tagUnit(votes[key], tag, anyJudge)
			pos, k = pos+units[i].Pos, k+units[i].K
			if units[i].K >= 2 {
				r.Units++
			}
		}
		if v, ok := PSA(units); ok {
			r.PSA = &v
		}
		if k > 0 {
			r.PositiveRate = new(float64(pos) / float64(k))
		}
		r.CI = bootstrap(rng, b, []int{len(units)}, func(s [][]int) (float64, bool) {
			pick := make([]TagUnit, len(s[0]))
			for i, j := range s[0] {
				pick[i] = units[j]
			}
			return PSA(pick)
		})
		r.Decision = decidePSA(r.CI)
		out = append(out, r)
	}
	return out
}

// share 是一组回答上被选中的票数与有效票数 / share holds selected and valid votes over a set of answers
type share struct{ hit, valid int }

// gap 计算两组票的比例之差，任一组无有效票时无定义 / gap is the difference of two vote shares, undefined without valid votes
func gap(a, b share) (float64, bool) {
	if a.valid == 0 || b.valid == 0 {
		return 0, false
	}
	return float64(a.hit)/float64(a.valid) - float64(b.hit)/float64(b.valid), true
}

// selfPreference 估计每个评审在自家厂商回答上的偏差：对每个标签与评审判定的结果选项，比较自家评审与
// 其余评审在自家回答上的比例差，并以两方在其余厂商回答上的比例差为基线。
//
// selfPreference estimates each judge's bias on its own vendor's answers, for every tag and
// judged outcome option, against the same two parties' gap on other vendors' answers.
func selfPreference(rng *rand.Rand, b int, qs []bank.Question, answers map[int][]round.Key, votes map[round.Key][]judge.Record, records []judge.Record) []SelfPreference {
	judges := map[string]bool{}
	for _, r := range records {
		judges[r.Judge] = true
	}
	var vendors []string
	for _, q := range qs {
		for _, k := range answers[q.ID] {
			if judges[k.Provider] && !slices.Contains(vendors, k.Provider) {
				vendors = append(vendors, k.Provider)
			}
		}
	}
	sort.Strings(vendors)
	selfVendor := func(p string) bool { return judges[p] }

	// item 描述一项比较：适用的回答与一条回答上某个评审的票 / item is one comparison: its answers and how one judge's votes count
	type item struct {
		question int
		name     string
		keys     []round.Key
		count    func(recs []judge.Record, judgeOK func(string) bool) share
	}
	var items []item
	tags, applies := tagList(qs, answers)
	for _, tag := range tags {
		items = append(items, item{name: tag, keys: applies[tag], count: func(recs []judge.Record, ok func(string) bool) share {
			u := tagUnit(recs, tag, ok)
			return share{u.Pos, u.K}
		}})
	}
	for _, q := range qs {
		if q.OutcomeBy == nil || *q.OutcomeBy != bank.OutcomeByJudge {
			continue
		}
		var options []string
		for _, o := range q.Outcomes {
			options = append(options, o.Value)
		}
		for _, value := range append(options, judge.OutcomeOther) {
			items = append(items, item{question: q.ID, name: value, keys: answers[q.ID], count: func(recs []judge.Record, ok func(string) bool) share {
				var s share
				for _, r := range recs {
					if o := r.Judgment.Outcome; ok(r.Judge) && o != nil && o.Value != judge.OutcomeUncertain {
						s.valid++
						if o.Value == value {
							s.hit++
						}
					}
				}
				return s
			}})
		}
	}

	out := []SelfPreference{}
	for _, v := range vendors {
		own := func(j string) bool { return j == v }
		other := func(j string) bool { return j != v }
		for _, it := range items {
			var mine, base []round.Key
			for _, k := range it.keys {
				switch {
				case k.Provider == v:
					mine = append(mine, k)
				case !selfVendor(k.Provider):
					base = append(base, k)
				}
			}
			if len(mine) == 0 {
				continue
			}
			sum := func(keys []round.Key, idx []int, ok func(string) bool) share {
				var s share
				for _, i := range idx {
					c := it.count(votes[keys[i]], ok)
					s.hit, s.valid = s.hit+c.hit, s.valid+c.valid
				}
				return s
			}
			bias := func(idx []int) (float64, bool) { return gap(sum(mine, idx, own), sum(mine, idx, other)) }
			baseline := func(idx []int) (float64, bool) { return gap(sum(base, idx, own), sum(base, idx, other)) }
			sp := SelfPreference{Vendor: v, QuestionID: it.question, Item: it.name, Units: len(mine)}
			if x, ok := bias(identity(len(mine))); ok {
				sp.Bias = &x
			}
			if x, ok := baseline(identity(len(base))); ok {
				sp.Baseline = &x
			}
			if sp.Bias != nil && sp.Baseline != nil {
				sp.Excess = new(*sp.Bias - *sp.Baseline)
			}
			sp.BiasCI = bootstrap(rng, b, []int{len(mine)}, func(s [][]int) (float64, bool) { return bias(s[0]) })
			sp.BaselineCI = bootstrap(rng, b, []int{len(base)}, func(s [][]int) (float64, bool) { return baseline(s[0]) })
			sp.ExcessCI = bootstrap(rng, b, []int{len(mine), len(base)}, func(s [][]int) (float64, bool) {
				x, ok1 := bias(s[0])
				y, ok2 := baseline(s[1])
				return x - y, ok1 && ok2
			})
			out = append(out, sp)
		}
	}
	return out
}

// identity 返回 0..n-1，即不重采样的全部回答 / identity returns 0..n-1, every answer without resampling
func identity(n int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return idx
}

// review 列出须人工复核的回答：全部 分歧、证据不足涉及的回答、人工核查项、简繁混用，以及按题分层的抽检。
// 抽检每题从其余厂商取 1 条；评审自家厂商的回答含自评，按两倍比例逐条抽取。
//
// review lists answers due for human review: every disagreement, answers behind an
// insufficient-evidence decision, human checks, mixed scripts, and a stratified sample:
// one answer per question from the other vendors, with judges' own vendors sampled at
// twice the per-answer rate.
func review(rng *rand.Rand, s Summary, qs []bank.Question, answers map[int][]round.Key, verdicts map[round.Key]judge.Verdict, checks map[round.Key]map[string]metrics.Check, votes map[round.Key][]judge.Record) []ReviewItem {
	reasons := map[round.Key][]string{}
	add := func(k round.Key, reason string) {
		if !slices.Contains(reasons[k], reason) {
			reasons[k] = append(reasons[k], reason)
		}
	}
	unclearQuestion := map[int]bool{}
	for _, r := range s.OutcomeReliability {
		if r.Decision == DecisionInsufficient {
			unclearQuestion[r.QuestionID] = true
		}
	}
	var unclearTags []string
	for _, r := range s.TagReliability {
		if r.Decision == DecisionInsufficient {
			unclearTags = append(unclearTags, r.Tag)
		}
	}
	judges := map[string]bool{}
	for _, recs := range votes {
		for _, r := range recs {
			judges[r.Judge] = true
		}
	}

	for _, q := range qs {
		for _, k := range answers[q.ID] {
			if verdicts[k].Outcome == judge.Disagreement {
				add(k, reasonDisagreement)
			}
			if unclearQuestion[q.ID] {
				add(k, reasonOutcomeUnclear)
			}
			for _, tag := range unclearTags {
				if slices.Contains(q.Flags, tag) && tagUnit(votes[k], tag, anyJudge).Pos > 0 {
					add(k, reasonTagUnclear+tag)
				}
			}
			for _, h := range q.HumanReview {
				add(k, reasonHuman+h)
			}
			if runOutcome(q, verdicts[k], checks[k]) == scriptMixed {
				add(k, reasonMixedScript)
			}
		}

		if !judge.Judged(q) {
			continue
		}
		var others, own []round.Key
		for _, k := range answers[q.ID] {
			if judges[k.Provider] {
				own = append(own, k)
			} else {
				others = append(others, k)
			}
		}
		if len(others) > 0 {
			add(others[rng.IntN(len(others))], reasonSample)
			rate := 2 / float64(len(others))
			for _, k := range own {
				if rng.Float64() < rate {
					add(k, reasonSample)
				}
			}
		}
	}

	out := []ReviewItem{}
	for _, q := range qs {
		for _, k := range answers[q.ID] {
			if rs := reasons[k]; len(rs) > 0 {
				out = append(out, ReviewItem{Provider: k.Provider, QuestionID: k.QuestionID, Run: k.Run, Reasons: rs})
			}
		}
	}
	return out
}
