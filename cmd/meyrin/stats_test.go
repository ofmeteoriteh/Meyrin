package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/round"
)

func TestStatsRound(t *testing.T) {
	results := t.TempDir()
	dir := newRound(t, results, round.Manifest{Round: "r1", Run: &round.RunInfo{}})
	args := []string{"stats", "-round", "r1", "-results", results, "-bootstrap", "200"}
	if err := run(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no metrics") {
		t.Fatalf("err = %v, want no metrics", err)
	}

	w, err := round.OpenWriter[round.Record](dir, round.ResponsesFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append([]round.Record{{Provider: "A", QuestionID: 1, Run: 1, Turn: 1, Content: "DNS 是域名系统。"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"metrics", "-round", "r1", "-results", results, "-dict", repo(dictPath)}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	jw, err := round.OpenWriter[judge.Record](dir, judge.JudgmentsFile)
	if err != nil {
		t.Fatal(err)
	}
	vote := func(j string) judge.Record {
		return judge.Record{Judge: j, Provider: "A", QuestionID: 1, Run: 1, Valid: true, Judgment: &judge.Judgment{QuestionID: 1,
			Tags: []judge.TagVote{{Tag: "寒暄", Value: judge.TagAbsent}, {Tag: "过度发散", Value: judge.TagAbsent}, {Tag: "AI腔", Value: judge.TagPresent, Evidence: "域名系统"}}}}
	}
	if err := jw.Append([]judge.Record{vote("G"), vote("D")}); err != nil {
		t.Fatal(err)
	}
	if err := jw.Close(); err != nil {
		t.Fatal(err)
	}
	m, _, err := round.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Judge = &round.JudgeInfo{}
	if err := round.SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, round.SummaryFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), `"tag": "AI腔"`) || !strings.HasPrefix(stdout.String(), "wrote ") {
		t.Fatalf("summary = %s", first)
	}
	if err := run(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, round.SummaryFile))
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("rerunning stats changed summary.json")
	}
	m, _, err = round.LoadManifest(dir)
	if err != nil || m.Stats == nil || m.Stats.Bootstrap != 200 {
		t.Fatalf("manifest stats = %+v, %v", m.Stats, err)
	}
}
