package round

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Manifest 记录一轮测试的版本与环境，供复现与核对 / Manifest records a round's versions and environment
type Manifest struct {
	Round       string       `json:"round"`
	SpecVersion string       `json:"spec_version"`
	BankSHA256  string       `json:"bank_sha256"`
	Run         *RunInfo     `json:"run,omitempty"`
	Metrics     *MetricsInfo `json:"metrics,omitempty"`
	Judge       *JudgeInfo   `json:"judge,omitempty"`
	Stats       *StatsInfo   `json:"stats,omitempty"`
}

// StatsInfo 是统计这一步的随机种子与 bootstrap 次数；种子只在首次统计时生成。
// StatsInfo records the statistics step's seed and bootstrap count; the seed is drawn once.
type StatsInfo struct {
	Seed      uint64 `json:"seed"`
	Bootstrap int    `json:"bootstrap"`
}

// JudgeInfo 是调用评审这一步的记录；Seed 决定各席位的评审顺序 / JudgeInfo records the judging step; Seed fixes each seat's order
type JudgeInfo struct {
	StartedAt    string      `json:"started_at"`
	FinishedAt   string      `json:"finished_at"`
	PromptSHA256 string      `json:"prompt_sha256"`
	Seed         uint64      `json:"seed"`
	Seats        []JudgeSeat `json:"seats"`
}

// JudgeSeat 是一个评审席位；订阅席位的地址只在 .env 中，不记录。
// JudgeSeat is one judge seat; a subscription seat's address lives only in .env and is not recorded.
type JudgeSeat struct {
	Vendor    string     `json:"vendor"`
	Access    string     `json:"access"`
	Model     string     `json:"model"`
	Endpoint  string     `json:"endpoint"`
	SelfCheck *SelfCheck `json:"self_check,omitempty"`
}

// SelfCheck 是订阅席位最近一次自检的结果 / SelfCheck is a subscription seat's latest self-check result
type SelfCheck struct {
	Echo           bool   `json:"echo"`
	InjectedTokens int    `json:"injected_tokens"`
	CheckedAt      string `json:"checked_at"`
}

// MetricsInfo 是计算程序指标这一步所用的数据与库 / MetricsInfo records the data and library used for program metrics
type MetricsInfo struct {
	Dicts    []DictInfo `json:"dicts"`
	Markdown string     `json:"markdown"`
}

// DictInfo 是一个固定版本的数据文件 / DictInfo is one pinned data file
type DictInfo struct {
	File   string `json:"file"`
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

// RunInfo 是调用受测模型这一步的记录 / RunInfo records the step that calls the models under test
type RunInfo struct {
	StartedAt    string       `json:"started_at"`
	FinishedAt   string       `json:"finished_at"`
	KernelStable string       `json:"kernel_stable"`
	MaxTokens    MaxTokens    `json:"max_tokens"`
	History      string       `json:"history"`
	Vendors      []VendorInfo `json:"vendors"`
}

// MaxTokens 是输出上限的起始值与截断重跑时的上限 / MaxTokens holds the initial output limit and the cap for truncation reruns
type MaxTokens struct {
	Initial int `json:"initial"`
	Cap     int `json:"cap"`
}

// VendorInfo 是一家厂商在本轮的调用方式 / VendorInfo is how one vendor was called in this round
type VendorInfo struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint"`
	Protocol string `json:"protocol"`
	Store    string `json:"store"`
}

// LoadManifest 读取 manifest，不存在时 ok 为 false / LoadManifest reads the manifest; ok is false when absent
func LoadManifest(dir string) (m Manifest, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, false, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return m, true, nil
}

// SaveManifest 写入 manifest / SaveManifest writes the manifest
func SaveManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(dir, ManifestFile, append(data, '\n'))
}

// WriteAtomic 先写临时文件再改名，避免中断后留下半个文件。
// WriteAtomic writes a temporary file and renames it, so an interruption never leaves half a file.
func WriteAtomic(dir, name string, data []byte) error {
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}
