// Package mdtable 按锚点行从 Markdown 文档中取出 GFM 表格。
// 规范与端点文档是唯一事实来源，代码直接读取其中的表格，不另存副本。
//
// Package mdtable extracts GFM tables from Markdown documents by anchor line.
// The spec and endpoint documents are the single source of truth; code reads
// their tables directly instead of keeping copies.
package mdtable

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// delimiterRow 匹配表头下方的分隔行 / delimiterRow matches the row under the header
var delimiterRow = regexp.MustCompile(`^\|(\s*:?-+:?\s*\|)+$`)

// Table 是一张解析后的表格，Rows 不含表头与分隔行 / Table is a parsed table; Rows exclude header and delimiter
type Table struct {
	Header []string
	Rows   [][]string
}

// After 返回锚点行之后、下一个标题之前的第一张表格；锚点须与整行完全一致。
// After returns the first table after the line equal to anchor, before the next heading.
func After(md, anchor string) (Table, error) {
	lines := strings.Split(md, "\n")
	start := slices.Index(lines, anchor)
	if start < 0 {
		return Table{}, fmt.Errorf("anchor %q not found", anchor)
	}

	// 跳过锚点与表格之间的空行和说明文字 / Skip blank lines and prose between anchor and table
	i := start + 1
	for i < len(lines) && !strings.HasPrefix(lines[i], "|") {
		if strings.HasPrefix(lines[i], "#") {
			return Table{}, fmt.Errorf("no table under anchor %q", anchor)
		}
		i++
	}
	if i+1 >= len(lines) || !delimiterRow.MatchString(lines[i+1]) {
		return Table{}, fmt.Errorf("no table under anchor %q", anchor)
	}

	t := Table{Header: splitRow(lines[i])}
	for _, line := range lines[i+2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		row := splitRow(line)
		if len(row) != len(t.Header) {
			return Table{}, fmt.Errorf("anchor %q: row %q has %d cells, header has %d",
				anchor, line, len(row), len(t.Header))
		}
		t.Rows = append(t.Rows, row)
	}
	return t, nil
}

// Check 确认表头与期望一致，防止文档改版后按错列读取。
// Check verifies the header so a reworded document is not read by the wrong column.
func (t Table) Check(header ...string) error {
	if !slices.Equal(t.Header, header) {
		return fmt.Errorf("table header %q, want %q", t.Header, header)
	}
	return nil
}

// Column 返回指定列的全部单元格 / Column returns every cell of the named column
func (t Table) Column(name string) ([]string, error) {
	idx := slices.Index(t.Header, name)
	if idx < 0 {
		return nil, fmt.Errorf("table has no column %q", name)
	}
	cells := make([]string, len(t.Rows))
	for i, row := range t.Rows {
		cells[i] = row[idx]
	}
	return cells, nil
}

// splitRow 按未转义的竖线切分一行，并还原 \| / splitRow splits on unescaped pipes and restores \|
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	var cells []string
	var cell strings.Builder
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '|':
			cell.WriteByte('|')
			i++
		case line[i] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(line[i])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

// Unquote 去掉单元格两端的反引号 / Unquote strips the backticks around a cell
func Unquote(cell string) string {
	if len(cell) >= 2 && strings.HasPrefix(cell, "`") && strings.HasSuffix(cell, "`") {
		return cell[1 : len(cell)-1]
	}
	return cell
}
