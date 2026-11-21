package judge

import (
	"sort"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// 计票规则，见规范「汇总」 / Tally rules from the spec's aggregation section
const (
	// minVotes 是结果项或标签成立所需的最少同票数 / minVotes is the agreeing votes needed for an outcome or tag
	minVotes = 2
	// Disagreement 表示有效票中没有任何结果选项达到 minVotes / Disagreement means no option reached minVotes
	Disagreement = "分歧"
)

// Verdict 是一条回答汇总后成立的结果项与标签 / Verdict is the aggregated outcome and tags of one answer
type Verdict struct {
	Provider   string
	QuestionID int
	Run        int
	// Outcome 为空表示该题没有评审判定的结果项 / An empty Outcome means judges decide no outcome for the question
	Outcome string
	Tags    []string
}

func (v Verdict) key() round.Key {
	return round.Key{Provider: v.Provider, QuestionID: v.QuestionID, Run: v.Run}
}

// Tally 按规范「汇总」计票：uncertain 与无效输出按缺失处理，结果项至少 minVotes 票相同才成立，
// 否则记为 Disagreement；标签至少 minVotes 票 present 才成立。
//
// Tally aggregates votes: uncertain and invalid outputs count as missing, an outcome holds
// with at least minVotes identical votes and is otherwise Disagreement, and a tag holds
// with at least minVotes present votes.
func Tally(questions map[int]bank.Question, recs []Record) []Verdict {
	type votes struct {
		outcomes map[string]int
		tags     map[string]int
	}
	byKey := map[round.Key]*votes{}
	for _, r := range recs {
		k := r.Answer()
		v := byKey[k]
		if v == nil {
			v = &votes{outcomes: map[string]int{}, tags: map[string]int{}}
			byKey[k] = v
		}
		if !r.Valid {
			continue
		}
		if o := r.Judgment.Outcome; o != nil && o.Value != OutcomeUncertain {
			v.outcomes[o.Value]++
		}
		for _, t := range r.Judgment.Tags {
			if t.Value == TagPresent {
				v.tags[t.Tag]++
			}
		}
	}

	verdicts := make([]Verdict, 0, len(byKey))
	for k, v := range byKey {
		q := questions[k.QuestionID]
		vd := Verdict{Provider: k.Provider, QuestionID: k.QuestionID, Run: k.Run}
		if judgesOutcome(q) {
			var held []string
			for value, n := range v.outcomes {
				if n >= minVotes {
					held = append(held, value)
				}
			}
			vd.Outcome = Disagreement
			if len(held) == 1 {
				vd.Outcome = held[0]
			}
		}
		for _, f := range q.Flags {
			if v.tags[f] >= minVotes {
				vd.Tags = append(vd.Tags, f)
			}
		}
		verdicts = append(verdicts, vd)
	}
	sort.Slice(verdicts, func(i, j int) bool { return keyLess(verdicts[i].key(), verdicts[j].key()) })
	return verdicts
}
