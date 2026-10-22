package judge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// Answer 是一段完成的对话：同一厂商、同一题、同一次运行的全部轮次。
// Answer is one finished conversation: every turn of one vendor, question and run.
type Answer struct {
	Provider   string
	QuestionID int
	Run        int
	Turns      []round.Record
}

// Key 标识这段对话 / Key identifies the conversation
func (a Answer) Key() round.Key {
	return round.Key{Provider: a.Provider, QuestionID: a.QuestionID, Run: a.Run}
}

// keyLess 按厂商、题号、运行次数排序 / keyLess orders by vendor, question and run
func keyLess(x, y round.Key) bool {
	if x.Provider != y.Provider {
		return x.Provider < y.Provider
	}
	if x.QuestionID != y.QuestionID {
		return x.QuestionID < y.QuestionID
	}
	return x.Run < y.Run
}

// Judged 报告该题是否需要评审：有评审判定的结果项或适用标签 / Judged reports whether a question has anything for judges
func Judged(q bank.Question) bool {
	return judgesOutcome(q) || len(q.Flags) > 0
}

// judgesOutcome 报告该题的结果项由评审判定 / judgesOutcome reports whether judges decide the question's outcome
func judgesOutcome(q bank.Question) bool {
	return q.OutcomeBy != nil && *q.OutcomeBy == bank.OutcomeByJudge
}

// Answers 把回答记录按对话分组，只保留需要评审的题；按厂商、题号、运行次数排序。
// Answers groups response records into conversations for judged questions, sorted by vendor, question and run.
func Answers(questions map[int]bank.Question, recs []round.Record) ([]Answer, error) {
	byKey := map[round.Key]*Answer{}
	for _, r := range recs {
		q, ok := questions[r.QuestionID]
		if !ok {
			return nil, fmt.Errorf("%s run %d: question %d is not in the bank", r.Provider, r.Run, r.QuestionID)
		}
		if !Judged(q) {
			continue
		}
		a := byKey[r.Key()]
		if a == nil {
			a = &Answer{Provider: r.Provider, QuestionID: r.QuestionID, Run: r.Run}
			byKey[r.Key()] = a
		}
		a.Turns = append(a.Turns, r)
	}
	answers := make([]Answer, 0, len(byKey))
	for _, a := range byKey {
		sort.Slice(a.Turns, func(i, j int) bool { return a.Turns[i].Turn < a.Turns[j].Turn })
		for i, t := range a.Turns {
			if t.Turn != i+1 || len(a.Turns) != len(questions[a.QuestionID].Turns) {
				return nil, fmt.Errorf("%s q%d run %d: turns are incomplete", a.Provider, a.QuestionID, a.Run)
			}
		}
		answers = append(answers, *a)
	}
	sort.Slice(answers, func(i, j int) bool { return keyLess(answers[i].Key(), answers[j].Key()) })
	return answers, nil
}

// input 是发给评审的用户消息，字段见规范「评审输入」；不含厂商、模型与推理内容。
// input is the user message sent to judges; it carries no vendor, model or reasoning.
type input struct {
	QuestionID   int            `json:"question_id"`
	Conversation []message      `json:"conversation"`
	JudgedTurn   int            `json:"judged_turn"`
	Reference    *string        `json:"reference"`
	Outcomes     []bank.Outcome `json:"outcomes"`
	Tags         []string       `json:"tags"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildInput 编码一条回答的评审输入。运行时写入的参考值附在题库说明之后。
// buildInput encodes an answer's judge input; a runtime reference follows the bank's description.
func buildInput(q bank.Question, a Answer, runtimeReference string) (string, error) {
	in := input{QuestionID: q.ID, JudgedTurn: q.JudgedTurn, Reference: q.Reference, Tags: q.Flags, Outcomes: []bank.Outcome{}}
	if judgesOutcome(q) {
		in.Outcomes = q.Outcomes
	}
	if q.ReferenceRuntime {
		if runtimeReference == "" {
			return "", fmt.Errorf("question %d needs the runtime reference", q.ID)
		}
		ref := fmt.Sprintf("%s（本轮为 %s）", *q.Reference, runtimeReference)
		in.Reference = &ref
	}
	for i, t := range a.Turns {
		in.Conversation = append(in.Conversation,
			message{Role: provider.RoleUser, Content: q.Turns[i]},
			message{Role: provider.RoleAssistant, Content: t.Content})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(in); err != nil {
		return "", err
	}
	return buf.String(), nil
}
