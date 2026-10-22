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

// LoadRecords 读取已完成的记录；文件不存在时返回空。末行不完整说明上次写入中断，须先人工处理。
// LoadRecords reads finished records; a missing file is empty. An unterminated last line
// means an interrupted write and must be resolved by hand.
func LoadRecords(dir string) ([]Record, error) {
	data, err := os.ReadFile(filepath.Join(dir, ResponsesFile))
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
		return nil, fmt.Errorf("%s ends with an incomplete line; remove it before resuming", ResponsesFile)
	}
	var recs []Record
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", ResponsesFile, i+1, err)
		}
		recs = append(recs, r)
	}
	return recs, nil
}

// Writer 以单次写入追加一段对话的全部记录 / Writer appends a conversation's records in one write
type Writer struct {
	f *os.File
}

// OpenWriter 打开记录文件用于追加 / OpenWriter opens the record file for appending
func OpenWriter(dir string) (*Writer, error) {
	f, err := os.OpenFile(filepath.Join(dir, ResponsesFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f}, nil
}

// Append 写入一段对话并落盘 / Append writes one conversation and syncs it to disk
func (w *Writer) Append(recs []Record) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	if _, err := w.f.Write(buf.Bytes()); err != nil {
		return err
	}
	return w.f.Sync()
}

// Close 关闭记录文件 / Close closes the record file
func (w *Writer) Close() error {
	return w.f.Close()
}
