// Package site 把已入库的各轮结果与测试规范生成纯静态站点：首页、记录表、题目页、模型页、
// 回答详情、可靠性页与规范页。模型回答是不可信内容，按与程序指标相同的 Markdown 方言渲染，
// 不输出原始 HTML、过滤危险链接，其余文字一律经 html/template 转义。
//
// Package site renders committed rounds and the test spec into a static site: home,
// record table, question, model, answer, reliability and spec pages. Model answers are
// untrusted: they are rendered with the same Markdown dialect as program metrics, which
// omits raw HTML and filters dangerous links, and all other text is escaped by html/template.
package site

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/stats"
)

//go:embed templates/*.html static/*
var assets embed.FS

// Round 是一轮的全部结果 / Round is everything one round produced
type Round struct {
	Name      string
	Manifest  round.Manifest
	Questions []bank.Question
	Responses []round.Record
	Metrics   []metrics.Line
	Judgments []judge.Record
	Summary   stats.Summary
}

// Providers 按 manifest 中的厂商顺序列出本轮有回答的厂商 / Providers lists vendors with answers, in manifest order
func (r Round) Providers() []string {
	has := map[string]bool{}
	for _, rec := range r.Responses {
		has[rec.Provider] = true
	}
	var out []string
	if r.Manifest.Run != nil {
		for _, v := range r.Manifest.Run.Vendors {
			if has[v.Name] {
				out = append(out, v.Name)
			}
		}
	}
	return out
}

// marker 标记由 Build 生成的目录；只有空目录或带此标记的目录会被清空重建。
// marker marks a directory written by Build; only an empty or marked directory is emptied and rebuilt.
const marker = ".meyrin-site"

// Build 在 out 下生成站点 / Build writes the site under out
func Build(out string, spec, icon []byte, rounds []Round) error {
	tmpl, err := template.New("").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if err != nil {
		return err
	}
	if err := clear(out); err != nil {
		return err
	}
	w := writer{out: out, tmpl: tmpl}
	if err := w.raw(marker, nil); err != nil {
		return err
	}

	static, err := fs.Sub(assets, "static")
	if err != nil {
		return err
	}
	if err := fs.WalkDir(static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(static, path)
		if err != nil {
			return err
		}
		return w.raw(path, data)
	}); err != nil {
		return err
	}
	if err := w.raw("favicon.svg", icon); err != nil {
		return err
	}

	specHTML, err := render(string(spec))
	if err != nil {
		return err
	}
	if err := w.page("index.html", "home", "", map[string]any{"Rounds": rounds}); err != nil {
		return err
	}
	if err := w.page("spec.html", "spec", "", map[string]any{"Spec": specHTML}); err != nil {
		return err
	}
	for _, r := range rounds {
		if err := buildRound(w, r); err != nil {
			return fmt.Errorf("round %s: %w", r.Name, err)
		}
	}
	return nil
}

// clear 清空上次生成的站点；out 非空且没有 marker 时拒绝，以免误删其他目录。
// clear removes a previous build, refusing a non-empty out without marker so no other directory is wiped.
func clear(out string) error {
	entries, err := os.ReadDir(out)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(entries) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(out, marker)); err != nil {
		return fmt.Errorf("%s is not empty and was not built by meyrin site; choose another output directory", out)
	}
	return os.RemoveAll(out)
}

// writer 写出页面与静态文件 / writer writes pages and static files
type writer struct {
	out  string
	tmpl *template.Template
}

// raw 写出原样文件 / raw writes a file as is
func (w writer) raw(path string, data []byte) error {
	full := filepath.Join(w.out, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// page 以模板 name 渲染页面；Root 是从页面回到站点根目录的相对前缀。
// page renders template name; Root is the relative prefix from the page back to the site root.
func (w writer) page(path, name, title string, data map[string]any) error {
	data["Root"] = strings.Repeat("../", strings.Count(path, "/"))
	data["Title"] = title
	var buf bytes.Buffer
	if err := w.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return w.raw(path, buf.Bytes())
}

// render 把 Markdown 渲染为 HTML；原始 HTML 被省略，危险链接被过滤。
// render turns Markdown into HTML with raw HTML omitted and dangerous links filtered.
func render(md string) (template.HTML, error) {
	var buf bytes.Buffer
	if err := metrics.Markdown.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil //nolint:gosec // goldmark 默认不输出原始 HTML / goldmark omits raw HTML by default
}

// answerView 是回答详情页的一段对话 / answerView is one conversation on an answer page
type answerView struct {
	Key       round.Key
	Turns     []turnView
	Judgments []judge.Record
	Verdict   *judge.Verdict
}

type turnView struct {
	Prompt  string
	Record  round.Record
	Content template.HTML
	Metrics *metrics.Line
}

// buildRound 生成一轮的全部页面 / buildRound writes every page of a round
func buildRound(w writer, r Round) error {
	questions := map[int]bank.Question{}
	for _, q := range r.Questions {
		questions[q.ID] = q
	}
	cells := map[round.Key]stats.Cell{}
	for _, c := range r.Summary.Cells {
		cells[round.Key{Provider: c.Provider, QuestionID: c.QuestionID}] = c
	}
	providers := r.Providers()
	returned := map[string][]string{}
	for _, rec := range r.Responses {
		if rec.ModelReturned != "" && !slices.Contains(returned[rec.Provider], rec.ModelReturned) {
			returned[rec.Provider] = append(returned[rec.Provider], rec.ModelReturned)
		}
	}
	base := "r/" + r.Name + "/"
	common := func(extra map[string]any) map[string]any {
		extra["Round"] = r
		extra["Providers"] = providers
		extra["Cells"] = cells
		extra["Returned"] = returned
		return extra
	}

	if err := w.page(base+"index.html", "round", r.Name, common(map[string]any{})); err != nil {
		return err
	}
	if err := w.page(base+"reliability.html", "reliability", r.Name, common(map[string]any{"Summary": r.Summary})); err != nil {
		return err
	}
	for _, q := range r.Questions {
		if err := w.page(fmt.Sprintf("%sq/%d.html", base, q.ID), "question", r.Name, common(map[string]any{"Q": q})); err != nil {
			return err
		}
	}
	for _, p := range providers {
		if err := w.page(base+"m/"+p+".html", "model", r.Name, common(map[string]any{"Provider": p})); err != nil {
			return err
		}
	}

	views, err := answers(r, questions)
	if err != nil {
		return err
	}
	for _, v := range views {
		path := fmt.Sprintf("%sa/%s/%d-%d.html", base, v.Key.Provider, v.Key.QuestionID, v.Key.Run)
		if err := w.page(path, "answer", r.Name, common(map[string]any{"A": v, "Q": questions[v.Key.QuestionID]})); err != nil {
			return err
		}
	}
	return nil
}

// answers 组装每段对话的详情 / answers assembles each conversation's detail
func answers(r Round, questions map[int]bank.Question) ([]answerView, error) {
	byKey := map[round.Key]*answerView{}
	var order []round.Key
	for _, rec := range r.Responses {
		k := rec.Key()
		v := byKey[k]
		if v == nil {
			v = &answerView{Key: k}
			byKey[k] = v
			order = append(order, k)
		}
		html, err := render(rec.Content)
		if err != nil {
			return nil, err
		}
		prompt := ""
		if q, ok := questions[k.QuestionID]; ok && rec.Turn >= 1 && rec.Turn <= len(q.Turns) {
			prompt = q.Turns[rec.Turn-1]
		}
		v.Turns = append(v.Turns, turnView{Prompt: prompt, Record: rec, Content: html})
	}
	for i := range r.Metrics {
		l := &r.Metrics[i]
		if v := byKey[round.Key{Provider: l.Provider, QuestionID: l.QuestionID, Run: l.Run}]; v != nil {
			for j := range v.Turns {
				if v.Turns[j].Record.Turn == l.Turn {
					v.Turns[j].Metrics = l
				}
			}
		}
	}
	for _, j := range r.Judgments {
		if v := byKey[j.Answer()]; v != nil {
			v.Judgments = append(v.Judgments, j)
		}
	}
	for _, vd := range judge.Tally(questions, r.Judgments) {
		if v := byKey[round.Key{Provider: vd.Provider, QuestionID: vd.QuestionID, Run: vd.Run}]; v != nil {
			v.Verdict = &vd
		}
	}
	out := make([]answerView, 0, len(order))
	for _, k := range order {
		v := byKey[k]
		slices.SortFunc(v.Turns, func(a, b turnView) int { return a.Record.Turn - b.Record.Turn })
		out = append(out, *v)
	}
	return out, nil
}

// funcs 是模板中可用的函数 / funcs are the functions available to templates
var funcs = template.FuncMap{
	"cell": func(cells map[round.Key]stats.Cell, provider string, q int) stats.Cell {
		return cells[round.Key{Provider: provider, QuestionID: q}]
	},
	"runs": func(c stats.Cell) []int {
		out := make([]int, c.Runs)
		for i := range out {
			out[i] = i + 1
		}
		return out
	},
	"join": strings.Join,
	"inc":  func(i int) int { return i + 1 },
	"num": func(v any) string {
		switch x := v.(type) {
		case nil:
			return "—"
		case *float64:
			if x == nil {
				return "—"
			}
			return fmt.Sprintf("%.3f", *x)
		case float64:
			return fmt.Sprintf("%.3g", x)
		case *int:
			if x == nil {
				return "—"
			}
			return fmt.Sprint(*x)
		}
		return fmt.Sprint(v)
	},
}
