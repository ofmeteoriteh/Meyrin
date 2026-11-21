package mdtable

import (
	"slices"
	"strings"
	"testing"
)

const doc = `# 文档

## 指标

除特别说明外：

| 指标 | 计算方式 |
|---|---|
| ` + "`list_items`" + ` | 匹配 ` + "`a\\|b`" + ` 的行数 |
| ` + "`chars`" + ` | 字符数 |

## 空节

说明文字

## 下一节

| 甲 | 乙 |
|:--|--:|
| 1 | 2 |
`

func TestAfter(t *testing.T) {
	tbl, err := After(doc, "## 指标")
	if err != nil {
		t.Fatal(err)
	}
	if err := tbl.Check("指标", "计算方式"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"`list_items`", "匹配 `a|b` 的行数"},
		{"`chars`", "字符数"},
	}
	if !slices.EqualFunc(tbl.Rows, want, slices.Equal) {
		t.Fatalf("rows = %q, want %q", tbl.Rows, want)
	}

	aligned, err := After(doc, "## 下一节")
	if err != nil {
		t.Fatal(err)
	}
	if got := aligned.Rows; len(got) != 1 || got[0][1] != "2" {
		t.Fatalf("aligned rows = %q", got)
	}
}

func TestAfterErrors(t *testing.T) {
	tests := []struct {
		name, md, anchor, want string
	}{
		{"锚点不存在", doc, "## 不存在", "not found"},
		{"锚点须整行匹配", doc, "## 指", "not found"},
		{"表格在下一个标题之后", doc, "## 空节", "no table"},
		{"缺分隔行", "## a\n\n| x |\n| y |\n", "## a", "no table"},
		{"单元格数不一致", "## a\n\n| x | y |\n|---|---|\n| 1 |\n", "## a", "has 1 cells"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := After(tt.md, tt.anchor)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckAndColumn(t *testing.T) {
	tbl := Table{Header: []string{"厂商", "端点"}, Rows: [][]string{{"OpenAI", "u1"}, {"Google", "u2"}}}
	if err := tbl.Check("厂商"); err == nil {
		t.Fatal("Check accepted a mismatched header")
	}
	col, err := tbl.Column("端点")
	if err != nil || !slices.Equal(col, []string{"u1", "u2"}) {
		t.Fatalf("Column = %q, %v", col, err)
	}
	if _, err := tbl.Column("文档"); err == nil {
		t.Fatal("Column accepted a missing column")
	}
}

func TestUnquote(t *testing.T) {
	tests := map[string]string{"`寒暄`": "寒暄", "寒暄": "寒暄", "`": "`", "``": ""}
	for in, want := range tests {
		if got := Unquote(in); got != want {
			t.Errorf("Unquote(%q) = %q, want %q", in, got, want)
		}
	}
}
