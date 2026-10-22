package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/round"
)

func TestJudgeDryRun(t *testing.T) {
	results := t.TempDir()
	dir := newRound(t, results, round.Manifest{Round: "r1", Run: &round.RunInfo{KernelStable: "7.3"}})
	w, err := round.OpenWriter[round.Record](dir, round.ResponsesFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append([]round.Record{
		{Provider: "A", QuestionID: 7, Run: 1, Turn: 1, Content: "7.3"},
		{Provider: "A", QuestionID: 30, Run: 1, Turn: 1, Content: "x"},
		{Provider: "A", QuestionID: 30, Run: 1, Turn: 2, Content: "y"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	args := []string{"judge", "-round", "r1", "-results", results, "-dry-run", "-judges", "DeepSeek",
		"-spec", repo(specPath), "-prompt", repo(promptPath), "-models", repo(modelsPath),
		"-env", filepath.Join(results, "missing.env")}
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"judge":"DeepSeek"`) || !strings.Contains(lines[0], `本轮为 7.3`) {
		t.Fatalf("dry run = %s", stdout.String())
	}

	args[7] = "Nobody"
	if err := run(args, &bytes.Buffer{}); !errors.Is(err, errUsage) {
		t.Fatalf("err = %v, want a usage error for an unknown judge", err)
	}
}

func TestOpenJudgeInfo(t *testing.T) {
	judges := []config.Judge{
		{Vendor: "G", Access: config.AccessSubscription, Model: "g"},
		{Vendor: "D", Access: config.AccessAPI, Model: "d"},
	}
	eps := map[string]config.Endpoint{"D": {URL: "https://d/chat/completions"}}
	info, err := openJudgeInfo(nil, judges, eps, "sha")
	if err != nil || info.Seats[0].Endpoint != "" || info.Seats[1].Endpoint != eps["D"].URL || info.StartedAt == "" {
		t.Fatalf("info = %+v, %v", info, err)
	}
	if again, err := openJudgeInfo(info, judges, eps, "sha"); err != nil || again.Seed != info.Seed {
		t.Fatalf("resume = %+v, %v", again, err)
	}
	if _, err := openJudgeInfo(info, judges, eps, "other"); err == nil {
		t.Fatal("openJudgeInfo accepted a changed prompt")
	}
	judges[1].Model = "d2"
	if _, err := openJudgeInfo(info, judges, eps, "sha"); err == nil {
		t.Fatal("openJudgeInfo accepted a changed seat")
	}
}
