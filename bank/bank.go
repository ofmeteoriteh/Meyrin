// Package bank 从测试规范生成机读题库，校验已入库的题库与规范一致，并读取题库供运行使用。
// 题目、结果选项与适用标签取自 docs/SPEC.md，程序检查项与人工核查项见 checks.go。
//
// Package bank builds the machine-readable question bank from the test spec,
// verifies that the committed bank matches the spec, and loads it for runs.
// Prompts, outcome options and tags come from docs/SPEC.md; program checks
// and human review items live in checks.go.
package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Question 是题库中的一题，字段顺序即 JSONL 中的键顺序 / Question is one bank entry; field order is the JSONL key order
type Question struct {
	ID               int       `json:"question_id"`
	Version          string    `json:"version"`
	Title            string    `json:"title"`
	Dimension        string    `json:"dimension"`
	Turns            []string  `json:"turns"`
	JudgedTurn       int       `json:"judged_turn"`
	Reference        *string   `json:"reference"`
	ReferenceRuntime bool      `json:"reference_runtime"`
	OutcomeBy        *string   `json:"outcome_by"`
	Outcomes         []Outcome `json:"outcomes"`
	Flags            []string  `json:"flags"`
	ProgramChecks    []string  `json:"program_checks"`
	HumanReview      []string  `json:"human_review"`
	Session          string    `json:"session"`
}

// Outcome 是一个结果选项；程序判定的选项没有定义 / Outcome is one result option; program-judged options have no definition
type Outcome struct {
	Value      string `json:"value"`
	Definition string `json:"definition"`
}

// 结果项的判定方 / Who decides the outcome
const (
	OutcomeByJudge   = "judge"
	OutcomeByProgram = "program"
)

// 会话方式 / Session modes
const (
	SessionIndependent = "independent"
	SessionSame        = "same_session"
)

// Build 解析规范并编码为题库字节，返回题目数 / Build parses the spec and encodes the bank, returning the question count
func Build(spec []byte) ([]byte, int, error) {
	qs, err := Parse(spec)
	if err != nil {
		return nil, 0, err
	}
	data, err := Encode(qs)
	if err != nil {
		return nil, 0, err
	}
	return data, len(qs), nil
}

// Load 读取 JSONL 题库，题号须从 1 起连续 / Load reads a JSONL bank whose IDs run from 1
func Load(data []byte) ([]Question, error) {
	var qs []Question
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		var q Question
		if err := json.Unmarshal(line, &q); err != nil {
			return nil, fmt.Errorf("bank line %d: %w", i+1, err)
		}
		if q.ID != i+1 {
			return nil, fmt.Errorf("bank line %d: question %d, want %d", i+1, q.ID, i+1)
		}
		if len(q.Turns) == 0 {
			return nil, fmt.Errorf("question %d has no turns", q.ID)
		}
		qs = append(qs, q)
	}
	return qs, nil
}

// Compare 逐行比较生成的题库与已入库的题库，报告第一处差异。
// Compare reports the first line where the generated bank differs from the committed one.
func Compare(generated, committed []byte) error {
	if bytes.Equal(generated, committed) {
		return nil
	}
	want := bytes.SplitAfter(generated, []byte("\n"))
	got := bytes.SplitAfter(committed, []byte("\n"))
	for i := range max(len(want), len(got)) {
		if i >= len(want) || i >= len(got) || !bytes.Equal(want[i], got[i]) {
			return fmt.Errorf("line %d differs from the spec; regenerate with `meyrin bank build`", i+1)
		}
	}
	return fmt.Errorf("bank differs from the spec; regenerate with `meyrin bank build`")
}
