package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
)

// dictPath 是程序指标所用固定版本数据的默认目录 / dictPath is the default directory of the pinned metric data
const dictPath = "data/dict"

// metricsRound 计算一轮全部回答的程序指标并写入 metrics.jsonl，所用数据与库记入 manifest。
// 每次全量重算，不依赖上次的输出。
//
// metricsRound computes program metrics for every answer of a round into metrics.jsonl and
// records the data and library used in the manifest. Every call recomputes from scratch.
func metricsRound(args []string, stdout io.Writer) error {
	fs := newFlagSet("metrics")
	name := fs.String("round", "", "round name, the directory under results/")
	dictDir := fs.String("dict", dictPath, "pinned data for program metrics")
	results := fs.String("results", resultsPath, "results directory")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("metrics: -round is required: %w", errUsage)
	}

	dir, err := round.Dir(*results, *name)
	if err != nil {
		return err
	}
	manifest, ok, err := round.LoadManifest(dir)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("round %s has no %s; run it first", *name, round.ManifestFile)
	}
	questions, err := loadRoundBank(dir, manifest)
	if err != nil {
		return err
	}
	recs, err := round.LoadRecords(dir)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return fmt.Errorf("round %s has no responses", *name)
	}

	dict, err := metrics.LoadDict(*dictDir)
	if err != nil {
		return err
	}
	lines, err := metrics.Compute(dict, questions, recs)
	if err != nil {
		return err
	}
	data, err := metrics.Encode(lines)
	if err != nil {
		return err
	}
	lib, err := metrics.MarkdownLibrary()
	if err != nil {
		return err
	}
	if err := round.WriteAtomic(dir, round.MetricsFile, data); err != nil {
		return err
	}
	manifest.Metrics = &round.MetricsInfo{Dicts: metrics.DictFiles, Markdown: lib}
	if err := round.SaveManifest(dir, manifest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote %d lines to %s\n", len(lines), filepath.Join(dir, round.MetricsFile))
	return err
}
