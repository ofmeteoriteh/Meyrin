package round

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir(t *testing.T) {
	if got, err := Dir("results", "2026-10-a"); err != nil || got != filepath.Join("results", "2026-10-a") {
		t.Fatalf("Dir = %q, %v", got, err)
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden"} {
		if _, err := Dir("results", bad); err == nil {
			t.Errorf("Dir accepted %q", bad)
		}
	}
}

func TestAppendAndLoadRecords(t *testing.T) {
	dir := t.TempDir()
	if recs, err := LoadRecords(dir); err != nil || recs != nil {
		t.Fatalf("missing file = %v, %v", recs, err)
	}

	w, err := OpenWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	conv := []Record{
		{Provider: "V", QuestionID: 10, Run: 1, Turn: 1, Content: "<一>", Request: json.RawMessage(`{"a":1}`)},
		{Provider: "V", QuestionID: 10, Run: 1, Turn: 2, Content: "二"},
	}
	if err := w.Append(conv); err != nil {
		t.Fatal(err)
	}
	w.Close()

	data, _ := os.ReadFile(filepath.Join(dir, ResponsesFile))
	if !strings.Contains(string(data), `"content":"<一>"`) || !strings.Contains(string(data), `"usage":null`) {
		t.Fatalf("file = %s", data)
	}
	recs, err := LoadRecords(dir)
	if err != nil || len(recs) != 2 || recs[1].Key() != (Key{"V", 10, 1}) {
		t.Fatalf("records = %+v, %v", recs, err)
	}

	// 末行不完整须报错 / An unterminated last line is an error
	if err := os.WriteFile(filepath.Join(dir, ResponsesFile), append(data, `{"provider":`...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecords(dir); err == nil || !strings.Contains(err.Error(), "incomplete line") {
		t.Fatalf("err = %v", err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := LoadManifest(dir); ok || err != nil {
		t.Fatalf("missing manifest: ok %v, err %v", ok, err)
	}
	m := Manifest{Round: "r1", SpecVersion: "v0.1", BankSHA256: "abc", Run: &RunInfo{
		KernelStable: "7.3",
		MaxTokens:    MaxTokens{Initial: 8192, Cap: 65536},
		Vendors:      []VendorInfo{{Name: "V", Model: "m", Store: "disabled"}},
	}}
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadManifest(dir)
	if err != nil || !ok || got.Run.KernelStable != "7.3" || got.Run.Vendors[0].Store != "disabled" {
		t.Fatalf("manifest = %+v, %v, %v", got, ok, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestFile+".tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary manifest left behind")
	}
}
