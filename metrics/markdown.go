package metrics

import (
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Markdown 按 CommonMark 与 GFM 解析回答；程序指标与静态站共用这一方言，默认不输出原始 HTML、
// 过滤危险链接。
//
// Markdown parses answers as CommonMark with GFM; program metrics and the site share this
// dialect, which by default omits raw HTML and filters dangerous links.
var Markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// goldmarkModule 是解析器的模块路径 / goldmarkModule is the parser's module path
const goldmarkModule = "github.com/yuin/goldmark"

// MarkdownLibrary 从构建信息读取解析器的模块版本 / MarkdownLibrary reads the parser's module version from the build info
func MarkdownLibrary() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range info.Deps {
			if dep.Path == goldmarkModule {
				return dep.Path + "@" + dep.Version, nil
			}
		}
	}
	return "", fmt.Errorf("%s is missing from the build info", goldmarkModule)
}

// document 是一条回答按 Markdown 结构拆出的各部分。代码块指 fenced 代码块；
// 缩进代码块与普通段落难以区分（列表内的续行常被误判），按正文处理。
//
// document is one answer split by Markdown structure. Code blocks are fenced blocks;
// indented code blocks are hard to tell from paragraphs (list continuations often parse
// as one), so they count as prose.
type document struct {
	prose       string   // 代码块外去除 Markdown 标记后的文字 / text outside code blocks, markup removed
	proseBefore *string  // 第一个代码块之前的 prose；无代码块时为 nil / prose before the first code block
	lines       []string // 代码块外的源码行 / source lines outside code blocks
	code        []string // 代码块内的行 / lines inside code blocks
	codeBlocks  int
	bold        int
	tables      int
}

// parseDocument 解析一条回答。原始 HTML 不计入文字。
// parseDocument parses one answer. Raw HTML contributes no text.
func parseDocument(content string) document {
	src := []byte(content)
	root := Markdown.Parser().Parse(text.NewReader(src))
	var d document
	var prose strings.Builder
	var codeStarts []int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if n.Type() == ast.TypeBlock {
				prose.WriteByte('\n')
			}
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.FencedCodeBlock:
			if d.codeBlocks == 0 {
				before := prose.String()
				d.proseBefore = &before
			}
			d.codeBlocks++
			for i := range n.Lines().Len() {
				seg := n.Lines().At(i)
				codeStarts = append(codeStarts, seg.Start)
				d.code = append(d.code, strings.TrimRight(string(seg.Value(src)), "\r\n"))
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			for i := range n.Lines().Len() {
				seg := n.Lines().At(i)
				prose.Write(seg.Value(src))
			}
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			v := n.Value(src)
			if n.Parent().Kind() != ast.KindCodeSpan {
				v = util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
			}
			prose.Write(v)
			if n.SoftLineBreak() || n.HardLineBreak() {
				prose.WriteByte('\n')
			}
		case *ast.String:
			prose.Write(n.Value)
		case *ast.AutoLink:
			prose.Write(n.Label(src))
			return ast.WalkSkipChildren, nil
		case *ast.Emphasis:
			if n.Level == 2 {
				d.bold++
			}
		case *east.Table:
			d.tables++
		}
		return ast.WalkContinue, nil
	})
	d.prose = prose.String()
	d.lines = linesOutside(content, codeStarts)
	return d
}

// linesOutside 返回不含代码块内容的源码行；围栏行本身不匹配任何行级指标，予以保留。
// linesOutside returns source lines without code block content; fence lines match no
// line-based metric and are kept.
func linesOutside(content string, codeStarts []int) []string {
	var starts []int
	lines := strings.Split(content, "\n")
	off := 0
	for _, l := range lines {
		starts = append(starts, off)
		off += len(l) + 1
	}
	inCode := map[int]bool{}
	for _, s := range codeStarts {
		inCode[sort.SearchInts(starts, s+1)-1] = true
	}
	var out []string
	for i, l := range lines {
		if !inCode[i] {
			out = append(out, strings.TrimRight(l, "\r"))
		}
	}
	return out
}
