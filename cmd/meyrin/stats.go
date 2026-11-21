package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"path/filepath"

	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/metrics"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/stats"
)

// defaultBootstrap 是 bootstrap 重采样次数的默认值 / defaultBootstrap is the default number of bootstrap resamples
const defaultBootstrap = 2000

// statsRound 汇总一轮的回答、程序指标与评审，写入 summary.json。种子与 bootstrap 次数在首次统计时
// 记入 manifest，之后沿用，重算结果逐字节相同。
//
// statsRound aggregates a round's answers, metrics and judgments into summary.json. The
// seed and bootstrap count are recorded in the manifest on the first run and reused, so
// recomputing gives identical bytes.
func statsRound(args []string, stdout io.Writer) error {
	fs := newFlagSet("stats")
	name := fs.String("round", "", "round name, the directory under results/")
	resamples := fs.Int("bootstrap", defaultBootstrap, "bootstrap resamples, used on the first run only")
	results := fs.String("results", resultsPath, "results directory")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("stats: -round is required: %w", errUsage)
	}
	if *resamples < 100 {
		return fmt.Errorf("stats: -bootstrap must be at least 100: %w", errUsage)
	}

	dir, err := round.Dir(*results, *name)
	if err != nil {
		return err
	}
	manifest, ok, err := round.LoadManifest(dir)
	if err != nil {
		return err
	}
	switch {
	case !ok || manifest.Run == nil:
		return fmt.Errorf("round %s has not been run", *name)
	case manifest.Metrics == nil:
		return fmt.Errorf("round %s has no metrics; run meyrin metrics first", *name)
	case manifest.Judge == nil:
		return fmt.Errorf("round %s has not been judged; run meyrin judge first", *name)
	}
	questions, err := loadRoundBank(dir, manifest)
	if err != nil {
		return err
	}
	responses, err := round.LoadRecords(dir)
	if err != nil {
		return err
	}
	lines, err := round.LoadLines[metrics.Line](dir, round.MetricsFile)
	if err != nil {
		return err
	}
	judgments, err := round.LoadLines[judge.Record](dir, judge.JudgmentsFile)
	if err != nil {
		return err
	}

	if manifest.Stats == nil {
		manifest.Stats = &round.StatsInfo{Seed: rand.Uint64(), Bootstrap: *resamples}
	}
	summary, err := stats.Build(stats.Input{
		Questions: questions, Responses: responses, Metrics: lines, Judgments: judgments,
		Seed: manifest.Stats.Seed, Bootstrap: manifest.Stats.Bootstrap,
	})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		return err
	}
	if err := round.WriteAtomic(dir, round.SummaryFile, buf.Bytes()); err != nil {
		return err
	}
	if err := round.SaveManifest(dir, manifest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "wrote %s (%d cells, %d answers for review)\n",
		filepath.Join(dir, round.SummaryFile), len(summary.Cells), len(summary.Review))
	return err
}
