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

// repo 返回仓库根目录下的路径 / repo returns a path under the repository root
func repo(path string) string {
	return filepath.Join("..", "..", path)
}

func TestBankBuildAndCheck(t *testing.T) {
	out := filepath.Join(t.TempDir(), "questions.jsonl")
	var stdout bytes.Buffer
	if err := run([]string{"bank", "build", "-spec", repo(specPath), "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "wrote 37 questions") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	built, _ := os.ReadFile(out)
	committed, _ := os.ReadFile(repo(bankPath))
	if !bytes.Equal(built, committed) {
		t.Fatal("bank build output differs from the committed bank")
	}

	if err := run([]string{"bank", "check", "-spec", repo(specPath), "-bank", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(built, "{}\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"bank", "check", "-spec", repo(specPath), "-bank", out}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "line 38") {
		t.Fatalf("err = %v, want a line 38 mismatch", err)
	}
}

func TestModelsCheck(t *testing.T) {
	var stdout bytes.Buffer
	args := []string{"models", "check", "-models", repo(modelsPath), "-spec", repo(specPath),
		"-endpoints", repo(endpointsPath), "-env-example", repo(envExamplePath)}
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "(13 vendors, 2 judges)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"bank"},
		{"bank", "publish"},
		{"bank", "build", "-unknown"},
		{"bank", "check", "extra"},
	} {
		if err := run(args, &bytes.Buffer{}); !errors.Is(err, errUsage) {
			t.Errorf("run(%q) = %v, want a usage error", args, err)
		}
	}
}

// newRound 建立含 manifest 与题库副本的轮次目录 / newRound creates a round directory with a manifest and a bank copy
func newRound(t *testing.T, results string, m round.Manifest) string {
	t.Helper()
	dir := filepath.Join(results, m.Round)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bankData, err := os.ReadFile(repo(bankPath))
	if err != nil {
		t.Fatal(err)
	}
	m.BankSHA256 = sha256Hex(bankData)
	if err := round.SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := saveRoundBank(dir, bankData); err != nil {
		t.Fatal(err)
	}
	return dir
}
