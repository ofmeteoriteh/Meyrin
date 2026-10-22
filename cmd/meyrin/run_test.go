package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/runner"
)

func TestSelectQuestions(t *testing.T) {
	data, err := os.ReadFile(repo(bankPath))
	if err != nil {
		t.Fatal(err)
	}
	qs, err := selectQuestions(data, "q24, 1")
	if err != nil || len(qs) != 2 || qs[0].ID != 24 || qs[1].ID != 1 {
		t.Fatalf("got %d questions, %v", len(qs), err)
	}
	for _, bad := range []string{"q0", "q38", "x"} {
		if _, err := selectQuestions(data, bad); err == nil {
			t.Errorf("selectQuestions accepted %q", bad)
		}
	}
}

func TestSelectTargets(t *testing.T) {
	o := runOptions{models: repo(modelsPath), endpoints: repo(endpointsPath)}
	all, err := selectTargets(o)
	if err != nil || len(all) != 13 {
		t.Fatalf("got %d targets, %v", len(all), err)
	}
	o.vendors = "MiniMax,智谱"
	some, err := selectTargets(o)
	if err != nil || len(some) != 2 || some[0].Vendor.Name != "智谱" || some[1].Endpoint.Protocol != config.AnthropicMessages {
		t.Fatalf("targets = %+v, %v", some, err)
	}
	o.vendors = "Nobody"
	if _, err := selectTargets(o); err == nil {
		t.Fatal("selectTargets accepted an unknown vendor")
	}
}

func TestAttachClientsListsEveryGap(t *testing.T) {
	env := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(env, []byte("A_API_KEY=sk-a\n"), 0o644)
	targets := []runner.Target{
		{Vendor: config.Vendor{Name: "A", KeyEnv: "A_API_KEY", Model: "m"}},
		{Vendor: config.Vendor{Name: "B", KeyEnv: "B_API_KEY"}},
	}
	err := attachClients(targets, env)
	if err == nil || !strings.Contains(err.Error(), "B: model") || !strings.Contains(err.Error(), "B: B_API_KEY") ||
		strings.Contains(err.Error(), "sk-a") {
		t.Fatalf("err = %v", err)
	}
	if err := attachClients(targets[:1], env); err != nil || targets[0].Sender == nil {
		t.Fatalf("err = %v, sender %v", err, targets[0].Sender)
	}
}

func TestOpenManifest(t *testing.T) {
	dir := t.TempDir()
	o := runOptions{round: "r1", kernel: "7.3"}
	targets := []runner.Target{{
		Vendor:   config.Vendor{Name: "A", Model: "m1"},
		Endpoint: config.Endpoint{URL: "https://a/v1/responses", Protocol: config.OpenAIResponses},
	}}
	ctx := context.Background()

	m, err := openManifest(ctx, dir, o, "v0.1", "sha", targets)
	if err != nil {
		t.Fatal(err)
	}
	if m.Run.KernelStable != "7.3" || m.Run.Vendors[0].Store != "disabled" || m.Run.StartedAt == "" {
		t.Fatalf("manifest = %+v", m.Run)
	}

	// 续跑时追加新厂商，已有厂商须一致 / Resuming may add vendors but not change existing ones
	more := append(targets, runner.Target{
		Vendor:   config.Vendor{Name: "B", Model: "m2"},
		Endpoint: config.Endpoint{URL: "https://b/v1/messages", Protocol: config.AnthropicMessages},
	})
	m, err = openManifest(ctx, dir, runOptions{round: "r1"}, "v0.1", "sha", more)
	if err != nil || len(m.Run.Vendors) != 2 || m.Run.Vendors[1].Store != "no option" || m.Run.KernelStable != "7.3" {
		t.Fatalf("resume = %+v, %v", m.Run, err)
	}
	saved, _, _ := round.LoadManifest(dir)
	if len(saved.Run.Vendors) != 2 {
		t.Fatalf("saved vendors = %d", len(saved.Run.Vendors))
	}

	changed := []runner.Target{targets[0]}
	changed[0].Vendor.Model = "m1-new"
	if _, err := openManifest(ctx, dir, o, "v0.1", "sha", changed); err == nil || !strings.Contains(err.Error(), "new round") {
		t.Fatalf("model change err = %v", err)
	}
	if _, err := openManifest(ctx, dir, o, "v0.1", "other", targets); err == nil {
		t.Fatal("openManifest accepted a different bank")
	}
}
