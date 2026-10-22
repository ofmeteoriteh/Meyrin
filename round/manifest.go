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
	Round       string   `json:"round"`
	SpecVersion string   `json:"spec_version"`
	BankSHA256  string   `json:"bank_sha256"`
	Run         *RunInfo `json:"run,omitempty"`
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

// SaveManifest 先写临时文件再改名，避免中断后留下半个 manifest。
// SaveManifest writes a temporary file and renames it, so an interruption never leaves half a manifest.
func SaveManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ManifestFile+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, ManifestFile))
}
