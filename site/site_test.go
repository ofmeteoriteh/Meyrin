package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/stats"
)

// fixture 是一轮最小的结果，回答中带脚本与危险链接 / fixture is a minimal round whose answer carries a script and a dangerous link
func fixture() Round {
	judgeBy := bank.OutcomeByJudge
	return Round{
		Name: "r1",
		Manifest: round.Manifest{Round: "r1", SpecVersion: "v0.1", Run: &round.RunInfo{
			StartedAt: "2026-10-22T12:00:00+08:00", Vendors: []round.VendorInfo{{Name: "智谱", Model: "glm"}},
		}},
		Questions: []bank.Question{{ID: 10, Title: "坚持立场", Turns: []string{"<b>问</b>"}, JudgedTurn: 1, OutcomeBy: &judgeBy,
			Outcomes: []bank.Outcome{{Value: "坚持", Definition: "不变"}}, Flags: []string{"说教"}}},
		Responses: []round.Record{{Provider: "智谱", QuestionID: 10, Run: 1, Turn: 1, ModelReturned: "glm-x",
			Content: "答：**坚持**\n\n<script>alert(1)</script>\n\n[点我](javascript:alert(2)) <img src=x onerror=alert(3)>"}},
		Judgments: []judge.Record{{Judge: "G", Provider: "智谱", QuestionID: 10, Run: 1, Valid: true,
			Judgment: &judge.Judgment{Outcome: &judge.Outcome{Value: "坚持", Evidence: "<i>坚持</i>"}}}},
		Summary: stats.Summary{Cells: []stats.Cell{{Provider: "智谱", QuestionID: 10, Runs: 1, Outcomes: []stats.Count{{Value: "坚持", N: 1}}}}},
	}
}

func TestBuild(t *testing.T) {
	out := filepath.Join(t.TempDir(), "_site")
	if err := Build(out, []byte("# 规范\n\n正文"), []byte("<svg/>"), []Round{fixture()}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.html", "spec.html", "style.css", "site.js", "favicon.svg", "r/r1/index.html",
		"r/r1/reliability.html", "r/r1/q/10.html", "r/r1/m/智谱.html", "r/r1/a/智谱/10-1.html"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(path))); err != nil {
			t.Errorf("missing %s", path)
		}
	}
	answer, err := os.ReadFile(filepath.Join(out, "r", "r1", "a", "智谱", "10-1.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(answer)
	for _, bad := range []string{"<script>alert", "javascript:alert", "onerror=", "<b>问</b>", "<i>坚持</i>"} {
		if strings.Contains(page, bad) {
			t.Errorf("answer page contains %q", bad)
		}
	}
	if !strings.Contains(page, "<strong>坚持</strong>") || !strings.Contains(page, `href="../../../../style.css"`) {
		t.Errorf("answer page lacks rendered Markdown or root-relative assets:\n%s", page)
	}
	record, err := os.ReadFile(filepath.Join(out, "r", "r1", "index.html"))
	if err != nil || !strings.Contains(string(record), "坚持 1/1") || !strings.Contains(string(record), "glm-x") {
		t.Fatalf("record table = %s, %v", record, err)
	}

	if err := Build(out, nil, nil, nil); err != nil {
		t.Fatalf("rebuilding a marked directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "r")); !os.IsNotExist(err) {
		t.Fatal("rebuild kept pages from the previous build")
	}
}

func TestBuildRefusesForeignDirectory(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Build(dir, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "not built by meyrin site") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("Build deleted a file in a foreign directory")
	}
}
