package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/runner"
)

// 运行所需的默认路径与次数 / Default paths and run count
const (
	envPath     = ".env"
	resultsPath = "results"
	defaultRuns = 3
)

// runOptions 是 meyrin run 的参数 / runOptions holds the arguments of meyrin run
type runOptions struct {
	round, only, vendors, kernel     string
	bankFile, models, endpoints, env string
	results                          string
	runs                             int
	dryRun                           bool
}

// runRound 调用受测模型，或在 dry-run 时只输出请求 / runRound calls the models under test, or prints requests in dry-run
func runRound(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("run")
	var o runOptions
	fs.StringVar(&o.round, "round", "", "round name, the directory under results/")
	fs.IntVar(&o.runs, "runs", defaultRuns, "runs per question")
	fs.StringVar(&o.only, "only", "", "comma-separated question IDs, e.g. q1,q24")
	fs.StringVar(&o.vendors, "vendors", "", "comma-separated vendor names; all when empty")
	fs.StringVar(&o.kernel, "kernel", "", "question 7 reference, skipping the kernel.org lookup")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print requests without sending them")
	fs.StringVar(&o.bankFile, "bank", bankPath, "question bank")
	fs.StringVar(&o.models, "models", modelsPath, "model list")
	fs.StringVar(&o.endpoints, "endpoints", endpointsPath, "vendor endpoint document")
	fs.StringVar(&o.env, "env", envPath, "credential file")
	fs.StringVar(&o.results, "results", resultsPath, "results directory")
	if err := parse(fs, args); err != nil {
		return err
	}
	if o.round == "" && !o.dryRun {
		return fmt.Errorf("run: -round is required: %w", errUsage)
	}
	if o.runs < 1 {
		return fmt.Errorf("run: -runs must be at least 1: %w", errUsage)
	}

	bankData, err := os.ReadFile(o.bankFile)
	if err != nil {
		return err
	}
	questions, err := selectQuestions(bankData, o.only)
	if err != nil {
		return err
	}
	targets, err := selectTargets(o)
	if err != nil {
		return err
	}
	if o.dryRun {
		return runner.DryRun(stdout, questions, targets)
	}

	if err := attachClients(targets, o.env); err != nil {
		return err
	}
	dir, err := round.Dir(o.results, o.round)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sum := sha256.Sum256(bankData)
	manifest, err := openManifest(ctx, dir, o, questions[0].Version, hex.EncodeToString(sum[:]), targets)
	if err != nil {
		return err
	}

	recs, err := round.LoadRecords(dir)
	if err != nil {
		return err
	}
	done := map[round.Key]bool{}
	for _, r := range recs {
		done[r.Key()] = true
	}
	w, err := round.OpenWriter(dir)
	if err != nil {
		return err
	}
	defer w.Close()

	r := &runner.Runner{
		Questions: questions,
		Targets:   targets,
		Runs:      o.runs,
		Log:       stderr,
		Sleep:     runner.SleepContext,
		Now:       time.Now,
	}
	failed, runErr := r.Run(ctx, w, done)

	manifest.Run.FinishedAt = time.Now().Format(time.RFC3339)
	if err := round.SaveManifest(dir, manifest); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return runErr
	}
	if failed > 0 {
		return fmt.Errorf("%d conversations failed; rerun the same command to retry them", failed)
	}
	fmt.Fprintf(stdout, "round %s complete in %s\n", o.round, dir)
	return nil
}

// selectQuestions 读取题库并按 -only 过滤 / selectQuestions loads the bank and applies -only
func selectQuestions(data []byte, only string) ([]bank.Question, error) {
	qs, err := bank.Load(data)
	if err != nil {
		return nil, err
	}
	if only == "" {
		return qs, nil
	}
	var picked []bank.Question
	for _, f := range strings.Split(only, ",") {
		id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(f), "q"))
		if err != nil || id < 1 || id > len(qs) {
			return nil, fmt.Errorf("run: -only has invalid question %q: %w", f, errUsage)
		}
		picked = append(picked, qs[id-1])
	}
	return picked, nil
}

// selectTargets 按 -vendors 选出厂商并配上端点 / selectTargets picks vendors by -vendors and attaches endpoints
func selectTargets(o runOptions) ([]runner.Target, error) {
	files, err := readAll(o.models, o.endpoints)
	if err != nil {
		return nil, err
	}
	m, err := config.LoadModels(files[0])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.models, err)
	}
	endpoints, err := config.ParseEndpoints(string(files[1]))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", o.endpoints, err)
	}

	var wanted []string
	if o.vendors != "" {
		wanted = strings.Split(o.vendors, ",")
	}
	var targets []runner.Target
	for _, v := range m.Vendors {
		if wanted != nil && !slices.Contains(wanted, v.Name) {
			continue
		}
		ep, ok := endpoints[v.Name]
		if !ok {
			return nil, fmt.Errorf("vendor %q has no endpoint in %s", v.Name, o.endpoints)
		}
		targets = append(targets, runner.Target{Vendor: v, Endpoint: ep})
	}
	for _, name := range wanted {
		if !slices.ContainsFunc(m.Vendors, func(v config.Vendor) bool { return v.Name == name }) {
			return nil, fmt.Errorf("run: unknown vendor %q: %w", name, errUsage)
		}
	}
	return targets, nil
}

// attachClients 确认模型 ID 与凭据齐全并创建客户端；缺失项一次列全。
// attachClients checks model IDs and credentials, listing every gap at once, then creates clients.
func attachClients(targets []runner.Target, envFile string) error {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return err
	}
	env, err := config.ParseEnv(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", envFile, err)
	}

	var missing []string
	for i, t := range targets {
		if t.Vendor.Model == "" {
			missing = append(missing, t.Vendor.Name+": model")
		}
		key := env[t.Vendor.KeyEnv]
		if key == "" {
			missing = append(missing, t.Vendor.Name+": "+t.Vendor.KeyEnv)
		}
		proxy, err := t.Vendor.ProxyURL()
		if err != nil {
			return err
		}
		targets[i].Sender = provider.NewClient(t.Endpoint, key, proxy)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing model IDs or credentials:\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}

// openManifest 新建本轮 manifest，或核对续跑时题库与各厂商配置未变。
// openManifest creates the round manifest, or on resume checks that the bank and vendor setup are unchanged.
func openManifest(ctx context.Context, dir string, o runOptions, specVersion, bankSHA string, targets []runner.Target) (round.Manifest, error) {
	m, ok, err := round.LoadManifest(dir)
	if err != nil {
		return round.Manifest{}, err
	}
	if !ok {
		m = round.Manifest{Round: o.round, SpecVersion: specVersion, BankSHA256: bankSHA}
	}
	if m.SpecVersion != specVersion || m.BankSHA256 != bankSHA {
		return round.Manifest{}, fmt.Errorf("round %s was started with a different spec or bank; start a new round", o.round)
	}

	if m.Run == nil {
		kernel := o.kernel
		if kernel == "" {
			if kernel, err = runner.FetchKernelStable(ctx, runner.KernelReleasesURL); err != nil {
				return round.Manifest{}, fmt.Errorf("question 7 reference: %w; pass -kernel <version> to fill it by hand", err)
			}
		}
		m.Run = &round.RunInfo{
			StartedAt:    time.Now().Format(time.RFC3339),
			KernelStable: kernel,
			MaxTokens:    round.MaxTokens{Initial: runner.InitialMaxTokens, Cap: runner.MaxTokensCap},
			History:      runner.HistoryPolicy,
		}
	}

	// 同一厂商在一轮内不得更换模型或端点 / A vendor may not change model or endpoint within a round
	for _, t := range targets {
		info := round.VendorInfo{
			Name:     t.Vendor.Name,
			Model:    t.Vendor.Model,
			Endpoint: t.Endpoint.URL,
			Protocol: string(t.Endpoint.Protocol),
			Store:    "no option",
		}
		if provider.StoreDisabled(t.Endpoint.Protocol) {
			info.Store = "disabled"
		}
		i := slices.IndexFunc(m.Run.Vendors, func(v round.VendorInfo) bool { return v.Name == info.Name })
		switch {
		case i < 0:
			m.Run.Vendors = append(m.Run.Vendors, info)
		case m.Run.Vendors[i] != info:
			return round.Manifest{}, fmt.Errorf("round %s ran %s as %+v; start a new round to change it",
				o.round, info.Name, m.Run.Vendors[i])
		}
	}
	return m, round.SaveManifest(dir, m)
}
