package bank

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestBuildMatchesCommittedBank(t *testing.T) {
	spec, err := os.ReadFile("../docs/SPEC.md")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../data/questions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	generated, n, err := Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := Compare(generated, committed); err != nil {
		t.Fatalf("%d questions: %v", n, err)
	}
}

// miniSpec 是覆盖各类题目结构的最小规范 / miniSpec is a minimal spec covering each question shape
const miniSpec = "# 测试集\n\n版本：v0.9\n\n" +
	"## 一、格式\n\n" +
	"### 1. 单轮\n\n```text\n第一题\n\t缩进\n```\n\n" +
	"- **参考**：`bookworm`。\n" +
	"- **结果**：`甲`：定义一；`乙`：含；分号与 `代码` 的定义。\n" +
	"- **标签**：`寒暄`、`AI腔`\n\n" +
	"## 二、交互\n\n" +
	"### 2. 多轮（2 轮，判定第 1 轮）\n\n```text\n第一轮\n```\n\n```text\n第二轮\n```\n\n" +
	"- **结果**（由程序判定）：`繁体` / `简体`，判定规则见程序指标。\n\n" +
	"## 评审规范\n\n### 风格标签\n\n| 标签 | 定义 |\n|---|---|\n| `寒暄` | 客套 |\n| `AI腔` | 套路 |\n"

func TestParse(t *testing.T) {
	qs, err := Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 2 {
		t.Fatalf("got %d questions, want 2", len(qs))
	}

	q1 := qs[0]
	if q1.Version != "v0.9" || q1.Dimension != "格式" || q1.Title != "单轮" {
		t.Errorf("q1 header = %q %q %q", q1.Version, q1.Dimension, q1.Title)
	}
	if !slices.Equal(q1.Turns, []string{"第一题\n\t缩进"}) {
		t.Errorf("q1 turns = %q", q1.Turns)
	}
	if q1.Reference == nil || *q1.Reference != "`bookworm`。" {
		t.Errorf("q1 reference = %v", q1.Reference)
	}
	wantOutcomes := []Outcome{{"甲", "定义一"}, {"乙", "含；分号与 `代码` 的定义"}}
	if q1.OutcomeBy == nil || *q1.OutcomeBy != OutcomeByJudge || !slices.Equal(q1.Outcomes, wantOutcomes) {
		t.Errorf("q1 outcomes = %v %+v", q1.OutcomeBy, q1.Outcomes)
	}
	if !slices.Equal(q1.Flags, []string{"寒暄", "AI腔"}) || q1.Session != SessionIndependent || q1.JudgedTurn != 1 {
		t.Errorf("q1 flags/session/judged = %q %q %d", q1.Flags, q1.Session, q1.JudgedTurn)
	}

	q2 := qs[1]
	if q2.Title != "多轮" || q2.JudgedTurn != 1 || q2.Session != SessionSame || len(q2.Turns) != 2 {
		t.Errorf("q2 = %q judged %d session %q turns %d", q2.Title, q2.JudgedTurn, q2.Session, len(q2.Turns))
	}
	wantProgram := []Outcome{{Value: "繁体"}, {Value: "简体"}}
	if q2.OutcomeBy == nil || *q2.OutcomeBy != OutcomeByProgram || !slices.Equal(q2.Outcomes, wantProgram) {
		t.Errorf("q2 outcomes = %v %+v", q2.OutcomeBy, q2.Outcomes)
	}
	if q2.Reference != nil || len(q2.Flags) != 0 {
		t.Errorf("q2 reference/flags = %v %q", q2.Reference, q2.Flags)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, spec, want string
	}{
		{"缺版本行", strings.Replace(miniSpec, "版本：v0.9", "", 1), "版本"},
		{"未知标签", strings.Replace(miniSpec, "`寒暄`、", "`说教`、", 1), "unknown tags"},
		{"题号不连续", strings.Replace(miniSpec, "### 2. ", "### 3. ", 1), "without gaps"},
		{"缺提示词", strings.Replace(miniSpec, "```text\n第一题\n\t缩进\n```", "", 1), "no prompt block"},
		{"缺风格标签表", strings.Replace(miniSpec, "### 风格标签", "### 标签", 1), "style tags"},
		{"无效 UTF-8", miniSpec + "\xff", "UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.spec))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestJudgeOutcomes(t *testing.T) {
	tests := []struct {
		name, line string
		want       []Outcome
	}{
		{"两个选项", "`甲`：定义一；`乙`：定义二。", []Outcome{{"甲", "定义一"}, {"乙", "定义二"}}},
		{"无句号结尾", "`甲`：定义一", []Outcome{{"甲", "定义一"}}},
		{"多个句号全部去掉", "`甲`：定义。。", []Outcome{{"甲", "定义"}}},
		{"定义内的分号不切分", "`甲`：a；b；`乙`：c", []Outcome{{"甲", "a；b"}, {"乙", "c"}}},
		{"定义内的反引号不切分", "`甲`：用 `x` 表示", []Outcome{{"甲", "用 `x` 表示"}}},
		{"末尾选项无定义则跳过", "`甲`：a；`乙`：", []Outcome{{"甲", "a"}}},
		{"无选项", "没有选项", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := judgeOutcomes(tt.line); !slices.Equal(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSpaceSeparators(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"字符串外补空格", `{"a":1,"b":[1,2]}`, `{"a": 1, "b": [1, 2]}`},
		{"字符串内不动", `{"a":"x, y: z"}`, `{"a": "x, y: z"}`},
		{"转义引号不结束字符串", `{"a":"q\",:"}`, `{"a": "q\",:"}`},
		{"转义反斜杠", `{"a":"\\","b":2}`, `{"a": "\\", "b": 2}`},
		{"控制字符保持转义", `{"a":"\t\n"}`, `{"a": "\t\n"}`},
		{"还原 U+2028 与 U+2029", `{"a":"  "}`, "{\"a\": \"  \"}"},
		{"字面量反斜杠 u2028 不还原", `{"a":"\\u2028"}`, `{"a": "\\u2028"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			spaceSeparators(&out, []byte(tt.in))
			if got := out.String(); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEncodeKeepsHTMLAndNonASCII(t *testing.T) {
	ref := "<a> & 中文"
	data, err := Encode([]Question{{ID: 1, Reference: &ref, Turns: []string{}, Outcomes: []Outcome{}}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"reference": "<a> & 中文"`)) || !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("encoded = %s", data)
	}
	if !bytes.Contains(data, []byte(`"outcome_by": null, "outcomes": [], "flags": null`)) {
		t.Fatalf("encoded = %s", data)
	}
}

func TestCompare(t *testing.T) {
	committed := []byte("a\nb\nc\n")
	tests := []struct {
		name, generated, want string
	}{
		{"一致", "a\nb\nc\n", ""},
		{"中间行不同", "a\nx\nc\n", "line 2"},
		{"入库题库多一行", "a\nb\n", "line 3"},
		{"生成题库多一行", "a\nb\nc\nd\n", "line 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Compare([]byte(tt.generated), committed)
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

func TestLoad(t *testing.T) {
	committed, err := os.ReadFile("../data/questions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	qs, err := Load(committed)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 37 || qs[23].ID != 24 || qs[23].Turns[0] != qs[0].Turns[0] {
		t.Fatalf("loaded %d questions; q24 must replay q1", len(qs))
	}

	for name, data := range map[string]string{
		"非 JSON": "not json\n",
		"题号跳号":   `{"question_id":2,"turns":["a"]}` + "\n",
		"缺提示词":   `{"question_id":1,"turns":[]}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]byte(data)); err == nil {
				t.Fatal("Load accepted an invalid bank")
			}
		})
	}
}
