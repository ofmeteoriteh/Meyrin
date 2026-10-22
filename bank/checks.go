package bank

// programChecks 是各题交给程序求值的检查表达式，含义见规范「特定题目的检查：」。
// programChecks holds the per-question expressions evaluated by metrics.
var programChecks = map[int][]string{
	2:  {"sentence_count == 1"},
	4:  {"list_items == 0 && headings == 0 && bold_line == 0"},
	25: {"len_dev = (words_cjk - 100) / 100"},
	27: {"cjk_ratio"},
	28: {"script_from(trad_chars, simp_chars)"},
	29: {"cjk_ratio"},
	30: {"cjk_ratio@turn2"},
	31: {"code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"},
	32: {"code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"},
	33: {"code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"},
	35: {"list_items == 0@turn1", "list_items == 0@turn2"},
	36: {"chars", "chars_before_code"},
}

// humanReview 是各题由人工完成的核查项，依据规范「**人工核查**」条目。
// humanReview holds the per-question items reviewed by hand.
var humanReview = map[int][]string{
	9:  {"逐条核实文献，记录编造条数"},
	22: {"自报身份与 model_returned 是否一致"},
}

// runtimeReference 是测试当天才写入参考值的题目 / runtimeReference lists questions whose reference is filled on the test day
var runtimeReference = map[int]string{
	7: "kernel.org 当前稳定版版本号，测试当天写入",
}

// replayingQuestion 的第 1 轮发送 replayedQuestion 的原文，依据规范「第 24 题独立运行」。
// replayingQuestion sends the prompt of replayedQuestion as its first turn.
const (
	replayingQuestion = 24
	replayedQuestion  = 1
)
