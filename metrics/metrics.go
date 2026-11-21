// Package metrics 实现规范「程序指标」：逐条回答计算通用指标，再按题库的 program_checks
// 求出特定题目的检查值。所有文字类指标都在 fenced 代码块外、去除 Markdown 标记后计算。
//
// Package metrics implements the spec's program metrics: general metrics for every answer,
// then the per-question values named by the bank's program_checks. Text metrics are taken
// outside fenced code blocks, after Markdown markup is removed.
package metrics

import (
	"regexp"
	"strings"
	"unicode"
)

// Metrics 是一条回答的通用指标，字段见规范「程序指标」 / Metrics holds one answer's general metrics
type Metrics struct {
	Chars           int      `json:"chars"`
	WordsCJK        int      `json:"words_cjk"`
	ListItems       int      `json:"list_items"`
	Headings        int      `json:"headings"`
	BoldLine        int      `json:"bold_line"`
	Tables          int      `json:"tables"`
	Emoji           int      `json:"emoji"`
	Bold            int      `json:"bold"`
	CodeBlocks      int      `json:"code_blocks"`
	CharsBeforeCode *int     `json:"chars_before_code"`
	CJKRatio        *float64 `json:"cjk_ratio"`
	TradChars       int      `json:"trad_chars"`
	SimpChars       int      `json:"simp_chars"`
}

// 行级指标的匹配规则，前两条照抄规范 / Line patterns; the first two are verbatim from the spec
var (
	listItemLine = regexp.MustCompile(`^\s*([-*+]\s|\d+[.)]\s|\d+、)`)
	headingLine  = regexp.MustCompile(`^#{1,6}\s`)
	boldLine     = regexp.MustCompile(`^\s*(\*\*[^*]+\*\*|__[^_]+__)\s*[:：]?\s*$`)
)

// answer 是一条回答的解析结果与通用指标 / answer is one parsed answer with its general metrics
type answer struct {
	doc     document
	metrics Metrics
}

// analyze 计算一条回答的通用指标 / analyze computes one answer's general metrics
func (d *Dict) analyze(content string) answer {
	doc := parseDocument(content)
	m := Metrics{
		Chars:      d.chars(doc.prose),
		WordsCJK:   wordsCJK(doc.prose),
		ListItems:  countLines(doc.lines, listItemLine),
		Headings:   countLines(doc.lines, headingLine),
		BoldLine:   countLines(doc.lines, boldLine),
		Tables:     doc.tables,
		Bold:       doc.bold,
		CodeBlocks: doc.codeBlocks,
		CJKRatio:   cjkRatio(doc.prose),
	}
	m.Emoji, _ = d.scanEmoji([]rune(doc.prose))
	if doc.proseBefore != nil {
		n := d.chars(*doc.proseBefore)
		m.CharsBeforeCode = &n
	}
	for _, r := range doc.prose {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		if d.trad[r] {
			m.TradChars++
		}
		if d.simp[r] {
			m.SimpChars++
		}
	}
	return answer{doc, m}
}

// chars 统计非空白字符数，一个 emoji 序列计 1 / chars counts non-space characters, one per emoji sequence
func (d *Dict) chars(s string) int {
	rs := []rune(s)
	n := 0
	for _, r := range rs {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	_, extra := d.scanEmoji(rs)
	return n - extra
}

// wordsCJK 每个汉字计 1，每段连续的拉丁字母或数字计 1 / wordsCJK counts each Han character and each run of Latin letters or digits
func wordsCJK(s string) int {
	n, inRun := 0, false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			n++
			inRun = false
		case unicode.Is(unicode.Latin, r) || unicode.IsDigit(r):
			if !inRun {
				n++
			}
			inRun = true
		default:
			inRun = false
		}
	}
	return n
}

// cjkRatio 是汉字数 ÷（汉字数 + 拉丁字母数），两者都为 0 时为 nil。
// cjkRatio is Han ÷ (Han + Latin letters), nil when both are zero.
func cjkRatio(s string) *float64 {
	han, latin := 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	if han+latin == 0 {
		return nil
	}
	v := float64(han) / float64(han+latin)
	return &v
}

// countLines 统计匹配的行数 / countLines counts matching lines
func countLines(lines []string, re *regexp.Regexp) int {
	n := 0
	for _, l := range lines {
		if re.MatchString(l) {
			n++
		}
	}
	return n
}

// abbreviation 匹配不切句的英文缩写 / abbreviation matches English abbreviations that end no sentence
var abbreviation = regexp.MustCompile(`(?i)\b(e\.g\.|i\.e\.|etc\.)`)

// sentenceCount 按 。！？.!? 切分，计含文字或数字的片段数；小数点与缩写中的点不切分。
// sentenceCount splits on 。！？.!? and counts fragments holding letters or digits; decimal
// points and abbreviation dots do not split.
func sentenceCount(s string) int {
	rs := []rune(abbreviation.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, ".", "")
	}))
	n, hasWord := 0, false
	for i, r := range rs {
		decimal := r == '.' && i > 0 && i+1 < len(rs) && unicode.IsDigit(rs[i-1]) && unicode.IsDigit(rs[i+1])
		end := strings.ContainsRune("。！？.!?", r) && !decimal
		switch {
		case end:
			if hasWord {
				n++
			}
			hasWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			hasWord = true
		}
	}
	if hasWord {
		n++
	}
	return n
}

// isComment 判断代码行是否以 // 或 # 开头，忽略前导空白 / isComment reports a code line starting with // or #
func isComment(line string) bool {
	l := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#")
}
