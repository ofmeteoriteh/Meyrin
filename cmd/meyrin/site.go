package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/site"
)

// 静态站的默认输出目录与图标 / Default site output directory and icon
const (
	sitePath = "_site"
	iconPath = "assets/meyrin-icon.svg"
)

// siteBuild 把已完成统计的各轮结果与规范生成静态站；尚无 summary.json 的轮次跳过。
// siteBuild renders every round that has summary.json, plus the spec, into a static site.
func siteBuild(args []string, stdout io.Writer) error {
	fs := newFlagSet("site")
	out := fs.String("out", sitePath, "output directory")
	results := fs.String("results", resultsPath, "results directory")
	spec := fs.String("spec", specPath, "test spec")
	icon := fs.String("icon", iconPath, "site icon")
	if err := parse(fs, args); err != nil {
		return err
	}
	files, err := readAll(*spec, *icon)
	if err != nil {
		return err
	}
	rounds, skipped, err := loadRounds(*results)
	if err != nil {
		return err
	}
	if err := site.Build(*out, files[0], files[1], rounds); err != nil {
		return err
	}
	for _, s := range skipped {
		if _, err := fmt.Fprintf(stdout, "skipped round %s: no %s\n", s, round.SummaryFile); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "wrote site with %d rounds to %s\n", len(rounds), *out)
	return err
}

// loadRounds 读取结果目录下全部已统计的轮次，按开始时间从新到旧排列。
// loadRounds reads every summarized round under results, newest first.
func loadRounds(results string) ([]site.Round, []string, error) {
	entries, err := os.ReadDir(results)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var rounds []site.Round
	var skipped []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir, err := round.Dir(results, e.Name())
		if err != nil {
			return nil, nil, err
		}
		summaryData, err := os.ReadFile(filepath.Join(dir, round.SummaryFile))
		if errors.Is(err, fs.ErrNotExist) {
			skipped = append(skipped, e.Name())
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		r := site.Round{Name: e.Name()}
		if err := json.Unmarshal(summaryData, &r.Summary); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", filepath.Join(dir, round.SummaryFile), err)
		}
		m, ok, err := round.LoadManifest(dir)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("round %s has %s but no %s", e.Name(), round.SummaryFile, round.ManifestFile)
		}
		r.Manifest = m
		if r.Questions, err = loadRoundBank(dir, m); err != nil {
			return nil, nil, err
		}
		if r.Responses, err = round.LoadRecords(dir); err != nil {
			return nil, nil, err
		}
		if r.Metrics, err = round.LoadLines[metrics.Line](dir, round.MetricsFile); err != nil {
			return nil, nil, err
		}
		if r.Judgments, err = round.LoadLines[judge.Record](dir, judge.JudgmentsFile); err != nil {
			return nil, nil, err
		}
		rounds = append(rounds, r)
	}
	slices.SortStableFunc(rounds, func(a, b site.Round) int { return compareStart(b, a) })
	return rounds, skipped, nil
}

// compareStart 按开始时间比较两轮，未运行的轮次最早 / compareStart orders rounds by start time, unstarted ones first
func compareStart(a, b site.Round) int {
	start := func(r site.Round) time.Time {
		if r.Manifest.Run == nil {
			return time.Time{}
		}
		t, _ := time.Parse(time.RFC3339, r.Manifest.Run.StartedAt)
		return t
	}
	return start(a).Compare(start(b))
}
