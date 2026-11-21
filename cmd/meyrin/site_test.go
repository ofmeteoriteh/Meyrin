package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/round"
)

func TestSiteBuild(t *testing.T) {
	results := t.TempDir()
	newRound(t, results, round.Manifest{Round: "unfinished", Run: &round.RunInfo{}})
	out := filepath.Join(t.TempDir(), "_site")
	var stdout bytes.Buffer
	args := []string{"site", "-results", results, "-out", out, "-spec", repo(specPath), "-icon", repo(iconPath)}
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "skipped round unfinished") || !strings.Contains(stdout.String(), "0 rounds") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	spec, err := os.ReadFile(filepath.Join(out, "spec.html"))
	if err != nil || !strings.Contains(string(spec), "AI 风格测试集") {
		t.Fatalf("spec page = %.200s, %v", spec, err)
	}
}

func TestSaveRoundBank(t *testing.T) {
	dir := t.TempDir()
	if err := saveRoundBank(dir, []byte("a\n")); err != nil {
		t.Fatal(err)
	}
	if err := saveRoundBank(dir, []byte("a\n")); err != nil {
		t.Fatalf("saving the same bank again: %v", err)
	}
	if err := saveRoundBank(dir, []byte("b\n")); err == nil || !strings.Contains(err.Error(), "start a new round") {
		t.Fatalf("err = %v, want a mismatch", err)
	}
}
