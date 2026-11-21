// Package round 管理一轮测试的结果目录 results/<ROUND>/：manifest 记录本轮的版本与环境，
// responses.jsonl 只追加、每段对话整体写入，因此已有记录即代表该对话完成。
//
// Package round manages one round's result directory results/<ROUND>/: the manifest
// records versions and environment, and responses.jsonl is append-only with each
// conversation written at once, so a present record means a finished conversation.
package round

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// 结果目录中的文件名 / File names inside a round directory
const (
	ManifestFile  = "manifest.json"
	ResponsesFile = "responses.jsonl"
	MetricsFile   = "metrics.jsonl"
	SummaryFile   = "summary.json"
	// BankFile 是开轮时复制的题库，后续步骤与站点都读这份 / BankFile is the bank copied when a round starts, read by every later step
	BankFile = "questions.jsonl"
)

// roundName 限制轮次名只含安全字符，防止路径穿越 / roundName keeps round names path-safe
var roundName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Dir 返回轮次目录 / Dir returns the round directory
func Dir(resultsDir, name string) (string, error) {
	if !roundName.MatchString(name) {
		return "", fmt.Errorf("round name %q must match %s", name, roundName)
	}
	return filepath.Join(resultsDir, name), nil
}

// Record 是 responses.jsonl 的一行，字段见规范「结果格式」 / Record is one responses.jsonl line, as in the spec's result format
type Record struct {
	Provider      string          `json:"provider"`
	Model         string          `json:"model"`
	ModelReturned string          `json:"model_returned"`
	QuestionID    int             `json:"question_id"`
	Run           int             `json:"run"`
	Turn          int             `json:"turn"`
	Request       json.RawMessage `json:"request"`
	Content       string          `json:"content"`
	Reasoning     string          `json:"reasoning"`
	FinishReason  string          `json:"finish_reason"`
	Usage         json.RawMessage `json:"usage"`
	LatencyMS     int64           `json:"latency_ms"`
	Retries       int             `json:"retries"`
	Timestamp     string          `json:"timestamp"`
}

// Key 标识一段对话：同一厂商、同一题的第几次运行 / Key identifies one conversation
type Key struct {
	Provider   string
	QuestionID int
	Run        int
}

// Key 返回记录所属的对话 / Key returns the conversation a record belongs to
func (r Record) Key() Key {
	return Key{r.Provider, r.QuestionID, r.Run}
}

// LoadRecords 读取已完成的回答记录 / LoadRecords reads finished response records
func LoadRecords(dir string) ([]Record, error) {
	return LoadLines[Record](dir, ResponsesFile)
}

// LoadLines 读取目录中的 JSONL 文件；文件不存在时返回空。末行不完整说明上次写入中断，须先人工处理。
// LoadLines reads a JSONL file in dir; a missing file is empty. An unterminated last line
// means an interrupted write and must be resolved by hand.
func LoadLines[T any](dir, name string) ([]T, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		return nil, fmt.Errorf("%s ends with an incomplete line; remove it before resuming", name)
	}
	var rows []T
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		var r T
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", name, i+1, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// Writer 以单次写入追加一组 JSONL 行 / Writer appends a group of JSONL lines in one write
type Writer[T any] struct {
	f *os.File
}

// OpenWriter 打开目录中的 JSONL 文件用于追加 / OpenWriter opens a JSONL file in dir for appending
func OpenWriter[T any](dir, name string) (*Writer[T], error) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Writer[T]{f: f}, nil
}

// Append 写入一组行并落盘，非 ASCII 字符原样输出 / Append writes rows and syncs them, keeping non-ASCII raw
func (w *Writer[T]) Append(rows []T) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	if _, err := w.f.Write(buf.Bytes()); err != nil {
		return err
	}
	return w.f.Sync()
}

// Close 关闭文件 / Close closes the file
func (w *Writer[T]) Close() error {
	return w.f.Close()
}
