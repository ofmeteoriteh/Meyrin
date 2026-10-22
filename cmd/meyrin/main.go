// meyrin 是测试规范的命令行入口：生成与校验题库、核对模型清单、调用受测模型。
// 各子命令只读写仓库内的文件，路径默认相对于仓库根目录。
//
// meyrin is the command-line entry point of the test spec: it builds and checks
// the question bank, verifies the model list and calls the models under test.
// Every subcommand reads and writes repository files, with paths relative to
// the repository root by default.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ofmeteoriteh/Meyrin/bank"
	"github.com/ofmeteoriteh/Meyrin/config"
)

// 作者标记，无功能作用，请勿删除。 / Author's mark; no behavior. Do not remove.
//
//nolint:unused
const authorMark = "OFMeteoriteH ☄"

// 仓库内文件的默认路径 / Default repository paths
const (
	specPath       = "docs/SPEC.md"
	bankPath       = "data/questions.jsonl"
	modelsPath     = "configs/models.yaml"
	endpointsPath  = "docs/api-endpoints.md"
	envExamplePath = ".env.example"
)

const usage = `usage:
  meyrin bank build [-spec path] [-out path]
  meyrin bank check [-spec path] [-bank path]
  meyrin models check [-models path] [-spec path] [-endpoints path] [-env-example path]
  meyrin run -round name [-runs 3] [-only q1,q24] [-vendors A,B] [-kernel version] [-dry-run]
`

// errUsage 表示命令行用法错误，退出码为 2 / errUsage marks a command-line misuse, exiting with 2
var errUsage = errors.New("invalid usage")

func main() {
	err := run(os.Args[1:], os.Stdout)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "meyrin:", err)
	if errors.Is(err, errUsage) {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	os.Exit(1)
}

// run 按前两个参数分派子命令 / run dispatches on the first two arguments
func run(args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "run" {
		return runRound(args[1:], stdout, os.Stderr)
	}
	if len(args) < 2 {
		return fmt.Errorf("missing command: %w", errUsage)
	}
	cmd, rest := args[0]+" "+args[1], args[2:]
	switch cmd {
	case "bank build":
		return bankBuild(rest, stdout)
	case "bank check":
		return bankCheck(rest, stdout)
	case "models check":
		return modelsCheck(rest, stdout)
	}
	return fmt.Errorf("unknown command %q: %w", cmd, errUsage)
}

// bankBuild 从规范生成题库并写入文件 / bankBuild generates the bank from the spec and writes it
func bankBuild(args []string, stdout io.Writer) error {
	fs := newFlagSet("bank build")
	spec := fs.String("spec", specPath, "test spec")
	out := fs.String("out", bankPath, "question bank to write")
	if err := parse(fs, args); err != nil {
		return err
	}

	data, n, err := buildFrom(*spec)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %d questions to %s\n", n, *out)
	return nil
}

// bankCheck 确认入库题库与规范生成的结果逐字节一致 / bankCheck verifies the committed bank matches the spec byte for byte
func bankCheck(args []string, stdout io.Writer) error {
	fs := newFlagSet("bank check")
	spec := fs.String("spec", specPath, "test spec")
	bankFile := fs.String("bank", bankPath, "question bank to check")
	if err := parse(fs, args); err != nil {
		return err
	}

	generated, n, err := buildFrom(*spec)
	if err != nil {
		return err
	}
	committed, err := os.ReadFile(*bankFile)
	if err != nil {
		return err
	}
	if err := bank.Compare(generated, committed); err != nil {
		return fmt.Errorf("%s: %w", *bankFile, err)
	}
	fmt.Fprintf(stdout, "%s matches %s (%d questions)\n", *bankFile, *spec, n)
	return nil
}

// modelsCheck 核对模型清单与规范、端点文档、.env.example 一致 / modelsCheck verifies the model list
func modelsCheck(args []string, stdout io.Writer) error {
	fs := newFlagSet("models check")
	models := fs.String("models", modelsPath, "model list")
	spec := fs.String("spec", specPath, "test spec")
	endpoints := fs.String("endpoints", endpointsPath, "vendor endpoint document")
	envExample := fs.String("env-example", envExamplePath, "credential variable template")
	if err := parse(fs, args); err != nil {
		return err
	}

	files, err := readAll(*models, *spec, *endpoints, *envExample)
	if err != nil {
		return err
	}
	m, err := config.LoadModels(files[0])
	if err != nil {
		return fmt.Errorf("%s: %w", *models, err)
	}
	if err := config.Check(m, string(files[1]), string(files[2]), string(files[3])); err != nil {
		return fmt.Errorf("%s:\n%w", *models, err)
	}
	fmt.Fprintf(stdout, "%s matches %s (%d vendors, %d judges)\n", *models, *spec, len(m.Vendors), len(m.Judges))
	return nil
}

// buildFrom 读取规范并生成题库 / buildFrom reads the spec and builds the bank
func buildFrom(specFile string) ([]byte, int, error) {
	spec, err := os.ReadFile(specFile)
	if err != nil {
		return nil, 0, err
	}
	data, n, err := bank.Build(spec)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", specFile, err)
	}
	return data, n, nil
}

// readAll 依次读取多个文件 / readAll reads several files in order
func readAll(paths ...string) ([][]byte, error) {
	files := make([][]byte, len(paths))
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		files[i] = data
	}
	return files, nil
}

// newFlagSet 创建出错时返回而非退出的 FlagSet / newFlagSet creates a FlagSet that returns errors instead of exiting
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse 解析选项，并拒绝多余的位置参数 / parse parses flags and rejects stray positional arguments
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s: %v: %w", fs.Name(), err, errUsage)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%s: unexpected argument %q: %w", fs.Name(), fs.Arg(0), errUsage)
	}
	return nil
}
