package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// Check 是一项特定题目检查的结果；只记录数值的检查没有 pass。
// Check is one per-question check result; checks that only record a value have no pass.
type Check struct {
	Expr  string `json:"expr"`
	Value any    `json:"value"`
	Pass  *bool  `json:"pass,omitempty"`
}

// Line 是 metrics.jsonl 的一行，对应 responses.jsonl 的一条记录 / Line is one metrics.jsonl line for one response record
type Line struct {
	Provider   string  `json:"provider"`
	QuestionID int     `json:"question_id"`
	Run        int     `json:"run"`
	Turn       int     `json:"turn"`
	Metrics    Metrics `json:"metrics"`
	Checks     []Check `json:"checks,omitempty"`
}

// LenDevCheck 是第 25 题的字数偏差率检查，汇总时另报绝对值的中位数 / LenDevCheck is question 25's length deviation check
const LenDevCheck = "len_dev = (words_cjk - 100) / 100"

// evaluator 求一项检查的值与是否通过 / evaluator computes a check's value and pass
type evaluator func(a answer) (any, *bool)

// evaluators 以题库 program_checks 去掉 @turnN 后的表达式为键，判定规则见规范「特定题目的检查：」。
// evaluators is keyed by the bank's program_checks expressions without @turnN.
var evaluators = map[string]evaluator{
	"sentence_count == 1": func(a answer) (any, *bool) {
		n := sentenceCount(a.doc.prose)
		return n, new(n == 1)
	},
	"list_items == 0 && headings == 0 && bold_line == 0": func(a answer) (any, *bool) {
		m := a.metrics
		return nil, new(m.ListItems == 0 && m.Headings == 0 && m.BoldLine == 0)
	},
	"list_items == 0": func(a answer) (any, *bool) {
		return a.metrics.ListItems, new(a.metrics.ListItems == 0)
	},
	LenDevCheck: func(a answer) (any, *bool) {
		return float64(a.metrics.WordsCJK-100) / 100, nil
	},
	"script_from(trad_chars, simp_chars)": func(a answer) (any, *bool) {
		return script(a.metrics.TradChars, a.metrics.SimpChars), nil
	},
	"cjk_ratio":         func(a answer) (any, *bool) { return a.metrics.CJKRatio, nil },
	"chars":             func(a answer) (any, *bool) { return a.metrics.Chars, nil },
	"chars_before_code": func(a answer) (any, *bool) { return a.metrics.CharsBeforeCode, nil },
	"prose_chars":       func(a answer) (any, *bool) { return a.metrics.Chars, nil },
	"code_lines": func(a answer) (any, *bool) {
		n := 0
		for _, l := range a.doc.code {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		return n, nil
	},
	"comment_lines": func(a answer) (any, *bool) {
		return len(comments(a.doc.code)), nil
	},
	"comment_cjk_ratio": func(a answer) (any, *bool) {
		return cjkRatio(strings.Join(comments(a.doc.code), "\n")), nil
	},
}

// script 按规范第 28 题判定简繁 / script classifies the script as in question 28 of the spec
func script(trad, simp int) string {
	switch {
	case trad > 0 && simp > 0:
		return "混用"
	case trad > 0:
		return "繁体"
	case simp > 0:
		return "简体"
	}
	return "无法判定"
}

// comments 返回以 // 或 # 开头的代码行 / comments returns code lines starting with // or #
func comments(code []string) []string {
	var out []string
	for _, l := range code {
		if isComment(l) {
			out = append(out, l)
		}
	}
	return out
}

// plannedCheck 是一项检查及其作用的轮次 / plannedCheck is a check and the turn it applies to
type plannedCheck struct {
	expr string
	turn int
	eval evaluator
}

// planChecks 把各题的 program_checks 解析为按轮次执行的检查；未实现的表达式报错。
// planChecks resolves each question's program_checks into per-turn checks, rejecting unknown expressions.
func planChecks(questions []bank.Question) (map[int][]plannedCheck, error) {
	plan := map[int][]plannedCheck{}
	for _, q := range questions {
		for _, expr := range q.ProgramChecks {
			base, turn := expr, q.JudgedTurn
			if b, t, ok := strings.Cut(expr, "@turn"); ok {
				n, err := strconv.Atoi(t)
				if err != nil {
					return nil, fmt.Errorf("question %d: check %q: bad turn", q.ID, expr)
				}
				base, turn = b, n
			}
			if turn < 1 || turn > len(q.Turns) {
				return nil, fmt.Errorf("question %d: check %q: turn %d out of range", q.ID, expr, turn)
			}
			eval, ok := evaluators[base]
			if !ok {
				return nil, fmt.Errorf("question %d: no evaluator for check %q", q.ID, expr)
			}
			plan[q.ID] = append(plan[q.ID], plannedCheck{expr, turn, eval})
		}
	}
	return plan, nil
}

// Compute 按记录顺序计算一轮全部回答的指标 / Compute computes metrics for every record, in record order
func Compute(d *Dict, questions []bank.Question, recs []round.Record) ([]Line, error) {
	plan, err := planChecks(questions)
	if err != nil {
		return nil, err
	}
	known := map[int]bool{}
	for _, q := range questions {
		known[q.ID] = true
	}
	lines := make([]Line, 0, len(recs))
	for _, r := range recs {
		if !known[r.QuestionID] {
			return nil, fmt.Errorf("%s run %d: question %d is not in the bank", r.Provider, r.Run, r.QuestionID)
		}
		a := d.analyze(r.Content)
		l := Line{Provider: r.Provider, QuestionID: r.QuestionID, Run: r.Run, Turn: r.Turn, Metrics: a.metrics}
		for _, c := range plan[r.QuestionID] {
			if c.turn == r.Turn {
				v, pass := c.eval(a)
				l.Checks = append(l.Checks, Check{Expr: c.expr, Value: v, Pass: pass})
			}
		}
		lines = append(lines, l)
	}
	return lines, nil
}

// Encode 把指标编码为 JSONL，非 ASCII 字符原样输出 / Encode writes lines as JSONL with non-ASCII kept raw
func Encode(lines []Line) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
