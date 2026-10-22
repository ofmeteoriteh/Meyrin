package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
	"github.com/ofmeteoriteh/Meyrin/judge"
	"github.com/ofmeteoriteh/Meyrin/provider"
	"github.com/ofmeteoriteh/Meyrin/round"
	"github.com/ofmeteoriteh/Meyrin/runner"
)

// promptPath 是评审 system prompt 的默认路径 / promptPath is the default judge system prompt
const promptPath = "prompts/judge-system.md"

// judgeOptions 是 meyrin judge 的参数 / judgeOptions holds the arguments of meyrin judge
type judgeOptions struct {
	round, judges                   string
	spec, prompt                    string
	models, endpoints, env, results string
	dryRun                          bool
}

// judgeRound 让评审席位评审一轮的全部回答，或在 dry-run 时只输出请求。
// judgeRound has the judge seats judge every answer of a round, or prints requests in dry-run.
func judgeRound(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("judge")
	var o judgeOptions
	fs.StringVar(&o.round, "round", "", "round name, the directory under results/")
	fs.StringVar(&o.judges, "judges", "", "comma-separated judge vendors; all when empty")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print judge requests without sending them or reading credentials")
	fs.StringVar(&o.spec, "spec", specPath, "test spec")
	fs.StringVar(&o.prompt, "prompt", promptPath, "judge system prompt")
	fs.StringVar(&o.models, "models", modelsPath, "model list")
	fs.StringVar(&o.endpoints, "endpoints", endpointsPath, "vendor endpoint document")
	fs.StringVar(&o.env, "env", envPath, "credential file")
	fs.StringVar(&o.results, "results", resultsPath, "results directory")
	if err := parse(fs, args); err != nil {
		return err
	}
	if o.round == "" {
		return fmt.Errorf("judge: -round is required: %w", errUsage)
	}

	files, err := readAll(o.spec, o.prompt, o.models)
	if err != nil {
		return err
	}
	spec, prompt := string(files[0]), string(files[1])
	if err := judge.CheckPrompt(spec, prompt); err != nil {
		return fmt.Errorf("%s: %w", o.prompt, err)
	}
	m, err := config.LoadModels(files[2])
	if err != nil {
		return fmt.Errorf("%s: %w", o.models, err)
	}
	seats, err := selectJudges(m.Judges, o.judges)
	if err != nil {
		return err
	}

	dir, err := round.Dir(o.results, o.round)
	if err != nil {
		return err
	}
	manifest, ok, err := round.LoadManifest(dir)
	if err != nil {
		return err
	}
	if !ok || manifest.Run == nil {
		return fmt.Errorf("round %s has not been run", o.round)
	}
	qs, err := loadRoundBank(dir, manifest)
	if err != nil {
		return err
	}
	questions := map[int]bank.Question{}
	for _, q := range qs {
		questions[q.ID] = q
	}
	recs, err := round.LoadRecords(dir)
	if err != nil {
		return err
	}
	answers, err := judge.Answers(questions, recs)
	if err != nil {
		return err
	}
	if len(answers) == 0 {
		return fmt.Errorf("round %s has no answers to judge", o.round)
	}

	j := &judge.Judger{
		Prompt:           prompt,
		Questions:        questions,
		RuntimeReference: manifest.Run.KernelStable,
		Seats:            seats,
		Log:              log.New(stderr, "", 0),
		Sleep:            runner.SleepContext,
		Now:              time.Now,
	}
	if o.dryRun {
		return j.DryRun(stdout, answers)
	}

	endpoints, err := readEndpoints(o.endpoints)
	if err != nil {
		return err
	}
	if err := attachJudges(seats, endpoints, o.env); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	info, err := openJudgeInfo(manifest.Judge, m.Judges, endpoints, sha256Hex(files[1]))
	if err != nil {
		return fmt.Errorf("round %s: %w", o.round, err)
	}
	manifest.Judge = info
	j.Seed = info.Seed
	if err := selfCheck(ctx, j, info); err != nil {
		return errors.Join(err, round.SaveManifest(dir, manifest))
	}
	if err := round.SaveManifest(dir, manifest); err != nil {
		return err
	}

	prior, err := round.LoadLines[judge.Record](dir, judge.JudgmentsFile)
	if err != nil {
		return err
	}
	done := map[judge.Done]bool{}
	for _, r := range prior {
		done[judge.Done{Judge: r.Judge, Answer: r.Answer()}] = true
	}
	w, err := round.OpenWriter[judge.Record](dir, judge.JudgmentsFile)
	if err != nil {
		return err
	}
	failed, runErr := j.Run(ctx, w, answers, done)
	runErr = errors.Join(runErr, w.Close())

	info.FinishedAt = time.Now().Format(time.RFC3339)
	if err := round.SaveManifest(dir, manifest); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return runErr
	}
	if failed > 0 {
		return fmt.Errorf("%d judgments failed; rerun the same command to retry them", failed)
	}
	all, err := round.LoadLines[judge.Record](dir, judge.JudgmentsFile)
	if err != nil {
		return err
	}
	invalid := 0
	for _, r := range all {
		if !r.Valid {
			invalid++
		}
	}
	_, err = fmt.Fprintf(stdout, "round %s judged: %d judgments, %d invalid\n", o.round, len(all), invalid)
	return err
}

// selectJudges 按 -judges 选出评审席位 / selectJudges picks judge seats by -judges
func selectJudges(judges []config.Judge, names string) ([]judge.Seat, error) {
	var wanted []string
	if names != "" {
		wanted = strings.Split(names, ",")
	}
	var seats []judge.Seat
	for _, jd := range judges {
		if wanted == nil || slices.Contains(wanted, jd.Vendor) {
			seats = append(seats, judge.Seat{Judge: jd})
		}
	}
	for _, name := range wanted {
		if !slices.ContainsFunc(judges, func(jd config.Judge) bool { return jd.Vendor == name }) {
			return nil, fmt.Errorf("judge: unknown judge %q: %w", name, errUsage)
		}
	}
	return seats, nil
}

// readEndpoints 读取端点文档 / readEndpoints reads the endpoint document
func readEndpoints(path string) (map[string]config.Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	eps, err := config.ParseEndpoints(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return eps, nil
}

// attachJudges 确认评审的模型 ID、凭据与地址齐全并创建客户端；缺失项一次列全。
// attachJudges checks judge model IDs, credentials and addresses, listing every gap at once, then creates clients.
func attachJudges(seats []judge.Seat, endpoints map[string]config.Endpoint, envFile string) error {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return err
	}
	env, err := config.ParseEnv(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", envFile, err)
	}
	var missing []string
	for i, s := range seats {
		if s.Judge.Model == "" {
			missing = append(missing, s.Judge.Vendor+": model")
		}
		key := env[s.Judge.KeyEnv]
		if key == "" {
			missing = append(missing, s.Judge.Vendor+": "+s.Judge.KeyEnv)
		}
		ep, err := s.Judge.Endpoint(endpoints, env)
		if err != nil {
			missing = append(missing, err.Error())
			continue
		}
		seats[i].Sender = provider.NewClient(ep, key, nil)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing judge model IDs, credentials or addresses:\n  %s", strings.Join(missing, "\n  "))
	}
	return nil
}

// openJudgeInfo 新建评审记录，或核对续评时 prompt 与全部席位未变；随机种子只在新建时生成。
// openJudgeInfo creates the judging record, or on resume checks the prompt and seats are
// unchanged; the seed is drawn only on creation.
func openJudgeInfo(prev *round.JudgeInfo, judges []config.Judge, endpoints map[string]config.Endpoint, promptSHA string) (*round.JudgeInfo, error) {
	var seats []round.JudgeSeat
	for _, jd := range judges {
		s := round.JudgeSeat{Vendor: jd.Vendor, Access: jd.Access, Model: jd.Model}
		if jd.Access == config.AccessAPI {
			s.Endpoint = endpoints[jd.Vendor].URL
		}
		seats = append(seats, s)
	}
	if prev == nil {
		return &round.JudgeInfo{
			StartedAt:    time.Now().Format(time.RFC3339),
			PromptSHA256: promptSHA,
			Seed:         rand.Uint64(),
			Seats:        seats,
		}, nil
	}
	same := prev.PromptSHA256 == promptSHA && slices.EqualFunc(prev.Seats, seats, func(a, b round.JudgeSeat) bool {
		return a.Vendor == b.Vendor && a.Access == b.Access && a.Model == b.Model && a.Endpoint == b.Endpoint
	})
	if !same {
		return nil, errors.New("judged with a different prompt or judge seats; rejudging needs a new round")
	}
	return prev, nil
}

// selfCheck 对选中的订阅席位自检，结果写入 info / selfCheck runs the self-check on selected subscription seats into info
func selfCheck(ctx context.Context, j *judge.Judger, info *round.JudgeInfo) error {
	for _, s := range j.Seats {
		if s.Judge.Access != config.AccessSubscription {
			continue
		}
		sc, err := judge.SelfCheck(ctx, s, j.Sleep, j.Now)
		for i := range info.Seats {
			if info.Seats[i].Vendor == s.Judge.Vendor && sc.CheckedAt != "" {
				info.Seats[i].SelfCheck = &sc
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
