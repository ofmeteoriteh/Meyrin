package bank

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ofmeteoriteh/Meyrin/mdtable"
)

// 规范中题目部分的结构标记 / Structural markers of the question section in the spec
var (
	versionLine   = regexp.MustCompile(`(?m)^版本：(v\d+\.\d+)$`)
	sectionHead   = regexp.MustCompile(`(?m)^## [一二三四五六七八九]、(.+)$|^### (\d+)\. `)
	turnNote      = regexp.MustCompile(`（[^）]*轮[^）]*）$`)
	judgedNote    = regexp.MustCompile(`判定第 (\d+) 轮`)
	promptBlock   = regexp.MustCompile("(?s)```text\n(.*?)\n```")
	referenceLine = regexp.MustCompile(`(?m)^- \*\*参考\*\*：(.+)$`)
	outcomeLine   = regexp.MustCompile(`(?m)^- \*\*结果\*\*(（由程序判定）)?：(.+)$`)
	tagLine       = regexp.MustCompile(`(?m)^- \*\*标签\*\*：(.+)$`)
	backticked    = regexp.MustCompile("`([^`]+)`")
	optionHead    = regexp.MustCompile("`([^`]+)`：")
)

// 题目部分止于评审规范；风格标签表在评审规范之下 / Questions end at the judging section, which holds the tag table
const (
	judgingSection = "## 评审规范"
	tagSection     = "### 风格标签"
)

// Parse 从规范中解析全部题目，题号须从 1 起连续 / Parse extracts every question from the spec; IDs must run from 1
func Parse(spec []byte) ([]Question, error) {
	if !utf8.Valid(spec) {
		return nil, errors.New("spec is not valid UTF-8")
	}
	text := string(spec)

	version := versionLine.FindStringSubmatch(text)
	if version == nil {
		return nil, errors.New("spec has no 版本 line")
	}
	tags, err := styleTags(text)
	if err != nil {
		return nil, err
	}

	body, _, _ := strings.Cut(text, judgingSection)
	var qs []Question
	var dimension string
	heads := sectionHead.FindAllStringSubmatchIndex(body, -1)
	for i, m := range heads {
		if m[2] >= 0 {
			dimension = strings.TrimSpace(body[m[2]:m[3]])
			continue
		}
		end := len(body)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		id, _ := strconv.Atoi(body[m[4]:m[5]])
		if dimension == "" {
			return nil, fmt.Errorf("question %d precedes any dimension heading", id)
		}
		q, err := parseQuestion(id, body[m[1]:end], qs)
		if err != nil {
			return nil, err
		}
		if unknown := slices.DeleteFunc(slices.Clone(q.Flags), func(f string) bool {
			return slices.Contains(tags, f)
		}); len(unknown) > 0 {
			return nil, fmt.Errorf("question %d: unknown tags %q", id, unknown)
		}
		q.Version = version[1]
		q.Dimension = dimension
		qs = append(qs, q)
	}

	if len(qs) == 0 {
		return nil, errors.New("spec has no questions")
	}
	for i, q := range qs {
		if q.ID != i+1 {
			return nil, fmt.Errorf("question IDs must run from 1 without gaps: found %d at position %d", q.ID, i+1)
		}
	}
	return qs, nil
}

// parseQuestion 解析一题的正文；prev 为已解析的题目，供重放题取原文。
// parseQuestion parses one question body; prev holds the questions parsed so far.
func parseQuestion(id int, chunk string, prev []Question) (Question, error) {
	titleLine, _, _ := strings.Cut(chunk, "\n")
	q := Question{
		ID:            id,
		Title:         strings.TrimSpace(turnNote.ReplaceAllString(titleLine, "")),
		Turns:         []string{},
		Outcomes:      []Outcome{},
		Flags:         []string{},
		ProgramChecks: nonNil(programChecks[id]),
		HumanReview:   nonNil(humanReview[id]),
	}

	for _, m := range promptBlock.FindAllStringSubmatch(chunk, -1) {
		q.Turns = append(q.Turns, m[1])
	}
	if id == replayingQuestion {
		if len(prev) < replayedQuestion {
			return Question{}, fmt.Errorf("question %d replays question %d, which is missing", id, replayedQuestion)
		}
		q.Turns = append([]string{prev[replayedQuestion-1].Turns[0]}, q.Turns...)
	}
	if len(q.Turns) == 0 {
		return Question{}, fmt.Errorf("question %d has no prompt block", id)
	}

	q.JudgedTurn = len(q.Turns)
	if m := judgedNote.FindStringSubmatch(titleLine); m != nil {
		q.JudgedTurn, _ = strconv.Atoi(m[1])
	}
	q.Session = SessionIndependent
	if len(q.Turns) > 1 {
		q.Session = SessionSame
	}

	// 测试当天写入的参考值优先于正文参考 / A test-day reference overrides the written one
	if ref, ok := runtimeReference[id]; ok {
		q.Reference = &ref
		q.ReferenceRuntime = true
	} else if m := referenceLine.FindStringSubmatch(chunk); m != nil {
		ref := strings.TrimSpace(m[1])
		q.Reference = &ref
	}

	if m := outcomeLine.FindStringSubmatch(chunk); m != nil {
		by := OutcomeByJudge
		if m[1] != "" {
			by = OutcomeByProgram
		}
		q.OutcomeBy = &by
		if by == OutcomeByJudge {
			q.Outcomes = judgeOutcomes(m[2])
		} else {
			// 程序判定的选项列在第一个逗号之前 / Program-judged options precede the first comma
			values, _, _ := strings.Cut(m[2], "，")
			for _, v := range backticked.FindAllStringSubmatch(values, -1) {
				q.Outcomes = append(q.Outcomes, Outcome{Value: v[1]})
			}
		}
	}

	if m := tagLine.FindStringSubmatch(chunk); m != nil {
		for _, t := range backticked.FindAllStringSubmatch(m[1], -1) {
			q.Flags = append(q.Flags, t[1])
		}
	}
	return q, nil
}

// judgeOutcomes 解析「`选项`：定义；`选项`：定义。」形式的结果行。
// 定义取到下一个「；`」或行尾为止，行尾句号不计入定义。
//
// judgeOutcomes parses a "`option`：definition；`option`：definition。" line.
// A definition runs to the next "；`" or the end of line, excluding final full stops.
func judgeOutcomes(line string) []Outcome {
	var outcomes []Outcome
	for pos := 0; pos < len(line); {
		m := optionHead.FindStringSubmatchIndex(line[pos:])
		if m == nil {
			break
		}
		defStart := pos + m[1]
		defEnd := definitionEnd(line, defStart)
		if defEnd < 0 {
			// 选项后没有定义，从下一个字符继续找 / No definition follows; resume after this position
			_, size := utf8.DecodeRuneInString(line[pos+m[0]:])
			pos += m[0] + size
			continue
		}
		outcomes = append(outcomes, Outcome{
			Value:      line[pos+m[2] : pos+m[3]],
			Definition: strings.TrimRight(line[defStart:defEnd], "。"),
		})
		pos = defEnd
	}
	return outcomes
}

// definitionEnd 返回定义的结束位置：至少一个字符之后，首个「；`」处或行尾（可带一个句号）。
// definitionEnd returns where a definition stops, or -1 if it would be empty.
func definitionEnd(line string, start int) int {
	for i, r := range line[start:] {
		end := start + i + utf8.RuneLen(r)
		rest := line[end:]
		if rest == "" || rest == "。" || strings.HasPrefix(rest, "；`") {
			return end
		}
	}
	return -1
}

// styleTags 读取规范中风格标签表的标签列 / styleTags reads the tag column of the spec's style tag table
func styleTags(spec string) ([]string, error) {
	tbl, err := mdtable.After(spec, tagSection)
	if err != nil {
		return nil, fmt.Errorf("style tags: %w", err)
	}
	if err := tbl.Check("标签", "定义"); err != nil {
		return nil, fmt.Errorf("style tags: %w", err)
	}
	col, _ := tbl.Column("标签")
	tags := make([]string, len(col))
	for i, c := range col {
		tags[i] = mdtable.Unquote(c)
	}
	return tags, nil
}

// nonNil 让空列表编码为 [] 而非 null / nonNil makes empty lists encode as [] instead of null
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
