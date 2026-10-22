package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/round"
)

func TestMetricsRound(t *testing.T) {
	results := t.TempDir()
	dir := newRound(t, results, round.Manifest{Round: "r1"})
	args := []string{"metrics", "-round", "r1", "-results", results, "-dict", repo(dictPath)}

	if err := run(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no responses") {
		t.Fatalf("err = %v, want no responses", err)
	}

	w, err := round.OpenWriter[round.Record](dir, round.ResponsesFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append([]round.Record{{Provider: "A", QuestionID: 28, Run: 1, Turn: 1, Content: "這是繁體。"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "wrote 1 lines") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	out, err := os.ReadFile(filepath.Join(dir, round.MetricsFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"expr":"script_from(trad_chars, simp_chars)","value":"繁体"`) {
		t.Fatalf("metrics = %s", out)
	}
	m, _, err := round.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Metrics == nil || len(m.Metrics.Dicts) != 3 || !strings.HasPrefix(m.Metrics.Markdown, "github.com/yuin/goldmark@") {
		t.Fatalf("manifest metrics = %+v", m.Metrics)
	}

	if err := os.WriteFile(filepath.Join(dir, round.BankFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "does not match the manifest") {
		t.Fatalf("err = %v, want a bank mismatch", err)
	}
	if err := run([]string{"metrics"}, &bytes.Buffer{}); !errors.Is(err, errUsage) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}
