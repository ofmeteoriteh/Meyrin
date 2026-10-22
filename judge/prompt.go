// Package judge 实现规范「评审规范」中由 LLM 完成的部分：按「评审输入」组织每条回答，
// 发给两个评审席位，按「评审输出」校验结果并写入 judgments.jsonl，再按「汇总」计票。
// 订阅席位在使用前自检，确认请求未被改写、未被注入额外的 system prompt。
//
// Package judge implements the LLM part of the spec's judging rules: it builds each
// answer's input, sends it to both judge seats, validates the output and appends it to
// judgments.jsonl, and tallies votes. Subscription seats are self-checked first to confirm
// requests reach the model unaltered and without an injected system prompt.
package judge

import (
	"fmt"
	"strings"
)

// promptSections 是评审 prompt 须逐字包含的规范小节 / promptSections are spec sections the prompt must contain verbatim
var promptSections = []string{"风格标签", "评审输出"}

// CheckPrompt 确认评审 prompt 逐字包含规范中 promptSections 各节的正文。
// CheckPrompt verifies the judge prompt contains the body of each promptSections section verbatim.
func CheckPrompt(spec, prompt string) error {
	for _, h := range promptSections {
		body, err := specSection(spec, h)
		if err != nil {
			return err
		}
		if !strings.Contains(prompt, body) {
			return fmt.Errorf("judge prompt differs from the spec section %q", h)
		}
	}
	return nil
}

// specSection 返回规范中三级标题 h 之下、下一个三级标题之前的正文 / specSection returns the body under the level-3 heading h
func specSection(spec, h string) (string, error) {
	_, rest, ok := strings.Cut(spec, "\n### "+h+"\n")
	if !ok {
		return "", fmt.Errorf("spec has no section %q", h)
	}
	body, _, _ := strings.Cut(rest, "\n### ")
	return strings.Trim(body, "\n"), nil
}
