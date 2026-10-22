package metrics

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/round"
)

var (
	dictOnce sync.Once
	dict     *Dict
	dictErr  error
)

// testDict 加载仓库内的固定版本数据 / testDict loads the repository's pinned data
func testDict(t *testing.T) *Dict {
	t.Helper()
	dictOnce.Do(func() { dict, dictErr = LoadDict("../data/dict") })
	if dictErr != nil {
		t.Fatal(dictErr)
	}
	return dict
}

func TestGeneralMetrics(t *testing.T) {
	d := testDict(t)
	tests := []struct {
		name, content string
		check         func(m Metrics) bool
	}{
		{"粗体开头的行不是列表项", "**粗体**开头\n", func(m Metrics) bool { return m.ListItems == 0 && m.Bold == 1 }},
		{"分隔线不是列表项", "上文\n\n---\n\n下文\n", func(m Metrics) bool { return m.ListItems == 0 }},
		{"顿号编号是列表项", "1、苹果\n2、香蕉\n", func(m Metrics) bool { return m.ListItems == 2 }},
		{"各类列表标记", "- a\n* b\n+ c\n1. d\n2) e\n  - f\n", func(m Metrics) bool { return m.ListItems == 6 }},
		{"标题须带空格", "# 一\n###### 六\n#没有空格\n", func(m Metrics) bool { return m.Headings == 2 }},
		{"粗体伪标题可带冒号", "**结论**\n**原因：**\n**说明**：\n- **列表里**\n**粗体**后有字\n",
			func(m Metrics) bool { return m.BoldLine == 3 && m.Bold == 5 }},
		{"表格按分隔行计", "| a | b |\n|---|---|\n| 1 | 2 |\n\n| c |\n|:-:|\n", func(m Metrics) bool { return m.Tables == 2 }},
		{"502 Bad Gateway 计 3", "502 Bad Gateway", func(m Metrics) bool { return m.WordsCJK == 3 }},
		{"汉字逐字计数", "服务返回 HTTP 502。", func(m Metrics) bool { return m.WordsCJK == 6 }},
		{"chars 去除标记与空白", "## 标题\n\n**粗** `x` [链接](https://example.com)\n", func(m Metrics) bool { return m.Chars == 6 }},
		{"转义字符按原字计", `a\*b &amp; c`, func(m Metrics) bool { return m.Chars == 5 }},
		{"cjk_ratio", "中文ab", func(m Metrics) bool { return m.CJKRatio != nil && *m.CJKRatio == 0.5 }},
		{"无文字时 cjk_ratio 为空", "123 ！", func(m Metrics) bool { return m.CJKRatio == nil }},
		{"无代码块时 chars_before_code 为空", "正文", func(m Metrics) bool { return m.CharsBeforeCode == nil && m.CodeBlocks == 0 }},
		{"代码块内容不参与块外指标", "前言\n\n```go\n# 不是标题\n- 不是列表\n**不是粗体**\n文字\n```\n\n后记\n",
			func(m Metrics) bool {
				return m.Headings == 0 && m.ListItems == 0 && m.BoldLine == 0 && m.Bold == 0 &&
					m.Chars == 4 && m.CodeBlocks == 1 && m.CharsBeforeCode != nil && *m.CharsBeforeCode == 2
			}},
		{"列表内的代码块", "1. 步骤\n\n   ```sh\n   - rm x\n   ```\n", func(m Metrics) bool { return m.ListItems == 1 && m.CodeBlocks == 1 }},
		{"未闭合的代码块延续到末尾", "说明\n```\n# a\n", func(m Metrics) bool { return m.Headings == 0 && m.Chars == 2 }},
		{"emoji 序列", "好👍🏽 家👨‍👩‍👧 1️⃣ ✅ ✔️", func(m Metrics) bool { return m.Emoji == 5 && m.Chars == 7 }},
		{"文本默认的字符不是 emoji", "© ✔ ☺ # 1", func(m Metrics) bool { return m.Emoji == 0 }},
		{"代码块内的 emoji 不计", "```\n🎉\n```\n", func(m Metrics) bool { return m.Emoji == 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if m := d.analyze(tt.content).metrics; !tt.check(m) {
				t.Errorf("metrics = %+v", m)
			}
		})
	}
}

func TestScript(t *testing.T) {
	d := testDict(t)
	tests := []struct {
		content, want string
	}{
		{"這個說法們", "繁体"},
		{"这个说法们", "简体"},
		{"这個", "混用"},
		{"中文 OK", "无法判定"},
	}
	for _, tt := range tests {
		m := d.analyze(tt.content).metrics
		if got := script(m.TradChars, m.SimpChars); got != tt.want {
			t.Errorf("%s: script = %s (trad %d, simp %d), want %s", tt.content, got, m.TradChars, m.SimpChars, tt.want)
		}
	}
}

func TestSentenceCount(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"这是一句话。", 1},
		{"末句没有句末标点", 1},
		{"第一句。第二句", 2},
		{"版本 3.14 已发布。", 1},
		{"It works, e.g. on Linux, i.e. here, etc. and more.", 1},
		{"真的吗？！是的……", 2},
		{"他说：“好。”", 1},
		{"Done! Next step.", 2},
		{"", 0},
	}
	for _, tt := range tests {
		if got := sentenceCount(tt.text); got != tt.want {
			t.Errorf("sentenceCount(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestCodeChecks(t *testing.T) {
	a := testDict(t).analyze("说明\n\n```go\n// 计数 rune\nfunc f() {}\n\n  # x\n```\n")
	want := map[string]any{"code_lines": 3, "comment_lines": 2, "prose_chars": 2}
	for expr, w := range want {
		if v, _ := evaluators[expr](a); v != w {
			t.Errorf("%s = %v, want %v", expr, v, w)
		}
	}
	r, _ := evaluators["comment_cjk_ratio"](a)
	if got := r.(*float64); got == nil || *got != 2.0/7 {
		t.Errorf("comment_cjk_ratio = %v", got)
	}
}

// loadBank 读取仓库题库 / loadBank reads the repository bank
func loadBank(t *testing.T) []bank.Question {
	t.Helper()
	data, err := os.ReadFile("../data/questions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	qs, err := bank.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return qs
}

func TestEveryBankCheckHasAnEvaluator(t *testing.T) {
	plan, err := planChecks(loadBank(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan[35]) != 2 || plan[35][0].turn != 1 || plan[35][1].turn != 2 || plan[30][0].turn != 2 {
		t.Fatalf("turn routing: 35 = %+v, 30 = %+v", plan[35], plan[30])
	}
}

func TestScriptValuesAreBankOutcomes(t *testing.T) {
	for _, q := range loadBank(t) {
		if q.ID != 28 {
			continue
		}
		outcomes := map[string]bool{}
		for _, o := range q.Outcomes {
			outcomes[o.Value] = true
		}
		for _, counts := range [][2]int{{1, 0}, {0, 1}, {1, 1}, {0, 0}} {
			if v := script(counts[0], counts[1]); !outcomes[v] {
				t.Errorf("script %v = %q, not an outcome of question 28", counts, v)
			}
		}
	}
}

func TestComputeChecksEachTurn(t *testing.T) {
	recs := []round.Record{
		{Provider: "X", QuestionID: 35, Run: 1, Turn: 1, Content: "- 列表"},
		{Provider: "X", QuestionID: 35, Run: 1, Turn: 2, Content: "没有列表"},
		{Provider: "X", QuestionID: 2, Run: 1, Turn: 1, Content: "一句话。"},
	}
	lines, err := Compute(testDict(t), loadBank(t), recs)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		expr string
		pass bool
	}{
		{"list_items == 0@turn1", false},
		{"list_items == 0@turn2", true},
		{"sentence_count == 1", true},
	}
	for i, w := range want {
		c := lines[i].Checks
		if len(c) != 1 || c[0].Expr != w.expr || c[0].Pass == nil || *c[0].Pass != w.pass {
			t.Errorf("line %d checks = %+v, want %s pass %v", i, c, w.expr, w.pass)
		}
	}
	data, err := Encode(lines)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"expr":"sentence_count == 1","value":1,"pass":true`) {
		t.Errorf("encoded = %s", data)
	}

	if _, err := Compute(testDict(t), loadBank(t), []round.Record{{QuestionID: 99, Turn: 1}}); err == nil {
		t.Error("Compute accepted a question outside the bank")
	}
}

func TestPlanChecksRejects(t *testing.T) {
	for name, q := range map[string]bank.Question{
		"未实现":  {ID: 1, Turns: []string{"a"}, JudgedTurn: 1, ProgramChecks: []string{"words > 3"}},
		"轮次越界": {ID: 1, Turns: []string{"a"}, JudgedTurn: 1, ProgramChecks: []string{"chars@turn2"}},
		"轮次格式": {ID: 1, Turns: []string{"a"}, JudgedTurn: 1, ProgramChecks: []string{"chars@turnX"}},
	} {
		if _, err := planChecks([]bank.Question{q}); err == nil {
			t.Errorf("%s: planChecks accepted %q", name, q.ProgramChecks)
		}
	}
}

func TestLoadDictRejectsChangedFile(t *testing.T) {
	dir := t.TempDir()
	for _, f := range DictFiles {
		data, err := os.ReadFile("../data/dict/" + f.File)
		if err != nil {
			t.Fatal(err)
		}
		if f.File == "TSCharacters.txt" {
			data = append(data, "測\t测\n"...)
		}
		if err := os.WriteFile(dir+"/"+f.File, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadDict(dir); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v, want a sha256 mismatch", err)
	}
}

func TestParseOpenCC(t *testing.T) {
	got, err := parseOpenCC([]byte("# c\n# @tofu-risk: x\n㑮\t㑮 𫝈\n乾\t乾 干\n這\t这\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !got['㑮'] || got['乾'] || !got['這'] {
		t.Fatalf("changed = %v", got)
	}
	for _, bad := range []string{"ab\tc\n", "a\n", "# @tofu-risk\n這\t这\n"} {
		if _, err := parseOpenCC([]byte(bad)); err == nil {
			t.Errorf("parseOpenCC(%q) accepted", bad)
		}
	}
}

func TestMarkdownLibrary(t *testing.T) {
	if v, err := MarkdownLibrary(); err != nil || !strings.HasPrefix(v, goldmarkModule+"@v") {
		t.Fatalf("MarkdownLibrary() = %q, %v", v, err)
	}
}
