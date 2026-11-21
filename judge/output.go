package judge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ofmeteoriteh/Meyrin/bank"
)

// 结果项与标签的取值，见规范「评审输出」与「题目说明」 / Outcome and tag values from the spec
const (
	OutcomeOther     = "其他"
	OutcomeUncertain = "uncertain"
	TagPresent       = "present"
	TagAbsent        = "absent"
	TagUncertain     = "uncertain"
)

// maxEvidence 是标签证据的字数上限 / maxEvidence caps tag evidence length in characters
const maxEvidence = 50

// Judgment 是一个评审对一条回答的输出，字段见规范「评审输出」 / Judgment is one judge's output for one answer
type Judgment struct {
	QuestionID int       `json:"question_id"`
	Outcome    *Outcome  `json:"outcome"`
	Tags       []TagVote `json:"tags"`
	Notes      string    `json:"notes"`
}

// Outcome 是结果项及其原文依据 / Outcome is the chosen outcome with its quoted evidence
type Outcome struct {
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

// TagVote 是一个标签的判断及其原文依据 / TagVote is one tag decision with its quoted evidence
type TagVote struct {
	Tag      string `json:"tag"`
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

// validate 解析并校验评审输出：字段与取值符合规范，证据逐字出自 assistant 回复。
// 错误一次列全，供重试时记录。
//
// validate decodes and checks a judge output: fields and values follow the spec and every
// piece of evidence is quoted from the assistant replies. All problems are reported at once.
func validate(q bank.Question, a Answer, content string) (Judgment, error) {
	var j Judgment
	dec := json.NewDecoder(strings.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return Judgment{}, fmt.Errorf("decode: %w", err)
	}
	if dec.More() {
		return Judgment{}, errors.New("decode: trailing data after the JSON object")
	}

	var replies strings.Builder
	for _, t := range a.Turns {
		replies.WriteString(normalize(t.Content))
	}
	quoted := func(evidence string) bool { return isQuoted(evidence, replies.String()) }

	var errs []error
	if j.QuestionID != q.ID {
		errs = append(errs, fmt.Errorf("question_id %d, want %d", j.QuestionID, q.ID))
	}

	judgedOutcome := judgesOutcome(q)
	switch {
	case !judgedOutcome && j.Outcome != nil:
		errs = append(errs, errors.New("outcome must be null for this question"))
	case judgedOutcome && j.Outcome == nil:
		errs = append(errs, errors.New("outcome is missing"))
	case judgedOutcome:
		v := j.Outcome.Value
		allowed := v == OutcomeOther || v == OutcomeUncertain ||
			slices.ContainsFunc(q.Outcomes, func(o bank.Outcome) bool { return o.Value == v })
		if !allowed {
			errs = append(errs, fmt.Errorf("outcome %q is not an option", v))
		}
		if (v == OutcomeOther || v == OutcomeUncertain) && strings.TrimSpace(j.Notes) == "" {
			errs = append(errs, fmt.Errorf("outcome %q needs notes", v))
		}
		if j.Outcome.Evidence != "" && !quoted(j.Outcome.Evidence) {
			errs = append(errs, errors.New("outcome evidence is not quoted from the replies"))
		}
	}

	seen := map[string]bool{}
	for _, t := range j.Tags {
		switch {
		case !slices.Contains(q.Flags, t.Tag):
			errs = append(errs, fmt.Errorf("tag %q does not apply to this question", t.Tag))
			continue
		case seen[t.Tag]:
			errs = append(errs, fmt.Errorf("tag %q is listed twice", t.Tag))
			continue
		}
		seen[t.Tag] = true
		switch t.Value {
		case TagPresent:
			switch {
			case strings.TrimSpace(t.Evidence) == "":
				errs = append(errs, fmt.Errorf("tag %q is present without evidence", t.Tag))
			case utf8.RuneCountInString(t.Evidence) > maxEvidence:
				errs = append(errs, fmt.Errorf("tag %q evidence exceeds %d characters", t.Tag, maxEvidence))
			case !quoted(t.Evidence):
				errs = append(errs, fmt.Errorf("tag %q evidence is not quoted from the replies", t.Tag))
			}
		case TagAbsent, TagUncertain:
		default:
			errs = append(errs, fmt.Errorf("tag %q has value %q", t.Tag, t.Value))
		}
	}
	for _, f := range q.Flags {
		if !seen[f] {
			errs = append(errs, fmt.Errorf("tag %q is missing", f))
		}
	}
	return j, errors.Join(errs...)
}

// ellipses 分隔证据中不相邻的片段 / ellipses separate non-adjacent pieces of evidence
var ellipses = strings.NewReplacer("……", "\x00", "…", "\x00", "...", "\x00")

// isQuoted 报告证据的每个片段都出现在已规范化的回复中 / isQuoted reports every piece of evidence occurs in the normalized replies
func isQuoted(evidence, replies string) bool {
	found := false
	for piece := range strings.SplitSeq(ellipses.Replace(evidence), "\x00") {
		p := normalize(piece)
		if p == "" {
			continue
		}
		if !strings.Contains(replies, p) {
			return false
		}
		found = true
	}
	return found
}

// markup 是比对证据时忽略的 Markdown 标记字符 / markup lists Markdown characters ignored when matching evidence
const markup = "*_`#>|~"

// normalize 去掉空白与 Markdown 标记字符，使引文不受排版差异影响。
// normalize drops whitespace and Markdown characters so quotes survive formatting differences.
func normalize(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		if unicode.IsSpace(r) || strings.ContainsRune(markup, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
