package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ofmeteoriteh/Meyrin/round"
)

// DictFiles 是 data/dict/ 中固定版本的数据文件，哈希不符即拒绝加载。
// DictFiles lists the pinned data files under data/dict/; a hash mismatch refuses to load.
var DictFiles = []round.DictInfo{
	{File: "STCharacters.txt", Source: "OpenCC ver.1.4.2", SHA256: "a0ca1601c70648cf48b33c3c6210ccbecc5c7eead4b4c3daf76587ba2c03582b"},
	{File: "TSCharacters.txt", Source: "OpenCC ver.1.4.2", SHA256: "737c21c66f55a419dd6956cb3089476cdefc5a36877452631617696df1e5d925"},
	{File: "emoji-test.txt", Source: "Unicode Emoji 18.0", SHA256: "8f3735cda1f92a779d78af67cf86066bb1f07143dc22f2ac29394d9bc57ab21a"},
}

// tofuRiskTag 标记 OpenCC 默认配置经 TSCharactersExt 转成生僻简体形的行。
// tofuRiskTag marks lines that OpenCC's default config converts to a rare simplified form through TSCharactersExt.
const tofuRiskTag = "# @tofu-risk"

// Dict 是程序指标所需的单字简繁表与 emoji 序列 / Dict holds the single-character tables and emoji sequences
type Dict struct {
	trad  map[rune]bool // 繁转简后会改变的字 / characters that change under traditional-to-simplified
	simp  map[rune]bool // 简转繁后会改变的字 / characters that change under simplified-to-traditional
	emoji *emojiNode
}

// LoadDict 从目录读取并校验 DictFiles / LoadDict reads and verifies DictFiles from dir
func LoadDict(dir string) (*Dict, error) {
	files := map[string][]byte{}
	for _, f := range DictFiles {
		data, err := os.ReadFile(filepath.Join(dir, f.File))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
			return nil, fmt.Errorf("%s: sha256 %s, want %s", f.File, got, f.SHA256)
		}
		files[f.File] = data
	}
	var d Dict
	var err error
	if d.simp, err = parseOpenCC(files["STCharacters.txt"]); err != nil {
		return nil, fmt.Errorf("STCharacters.txt: %w", err)
	}
	if d.trad, err = parseOpenCC(files["TSCharacters.txt"]); err != nil {
		return nil, fmt.Errorf("TSCharacters.txt: %w", err)
	}
	if d.emoji, err = parseEmojiTest(files["emoji-test.txt"]); err != nil {
		return nil, fmt.Errorf("emoji-test.txt: %w", err)
	}
	return &d, nil
}

// parseOpenCC 读取单字表，返回 OpenCC 默认配置下会被改写的字：首个候选与原字不同，
// 或该行带 tofuRiskTag（默认配置先查 TSCharactersExt，把它转成生僻形）。
//
// parseOpenCC reads a single-character table and returns the characters OpenCC's default
// config rewrites: the first candidate differs from the key, or the line carries
// tofuRiskTag, which the default config converts through TSCharactersExt.
func parseOpenCC(data []byte) (map[rune]bool, error) {
	changed := map[rune]bool{}
	tagged := false
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, tofuRiskTag) {
			tagged = true
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, values, ok := strings.Cut(line, "\t")
		candidates := strings.Fields(values)
		r, size := utf8.DecodeRuneInString(key)
		if !ok || len(candidates) == 0 || size != len(key) {
			return nil, fmt.Errorf("line %d: want a single character, a tab and candidates", i+1)
		}
		if tagged && candidates[0] != key {
			return nil, fmt.Errorf("line %d: %s line does not keep the key first", i+1, tofuRiskTag)
		}
		if tagged || candidates[0] != key {
			changed[r] = true
		}
		tagged = false
	}
	return changed, nil
}

// emojiNode 是 emoji 序列按码位组成的前缀树 / emojiNode is a code-point trie of emoji sequences
type emojiNode struct {
	next    map[rune]*emojiNode
	end     bool // 此处结束一个序列 / a sequence ends here
	counted bool // 该序列计入 emoji / the sequence counts as an emoji
}

// parseEmojiTest 读取 emoji-test.txt。单码位的 unqualified 是默认以文本显示的字符（如 ©、✔），
// 只用于最长匹配、不计数；其余序列都计数。
//
// parseEmojiTest reads emoji-test.txt. Single-code-point unqualified entries are characters
// shown as text by default (such as © and ✔): they take part in longest match but do not count.
func parseEmojiTest(data []byte) (*emojiNode, error) {
	root := &emojiNode{next: map[rune]*emojiNode{}}
	for i, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		points, rest, ok := strings.Cut(line, ";")
		status, _, _ := strings.Cut(rest, "#")
		status = strings.TrimSpace(status)
		if !ok || !knownStatus[status] {
			return nil, fmt.Errorf("line %d: want code points ; status", i+1)
		}
		var seq []rune
		for _, p := range strings.Fields(points) {
			v, err := strconv.ParseUint(p, 16, 32)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			seq = append(seq, rune(v))
		}
		n := root
		for _, r := range seq {
			if n.next[r] == nil {
				n.next[r] = &emojiNode{next: map[rune]*emojiNode{}}
			}
			n = n.next[r]
		}
		n.end = true
		n.counted = status != "unqualified" || len(seq) > 1
	}
	return root, nil
}

// knownStatus 是 emoji-test.txt 的限定状态 / knownStatus lists the qualification statuses in emoji-test.txt
var knownStatus = map[string]bool{
	"component": true, "fully-qualified": true, "minimally-qualified": true, "unqualified": true,
}

// scanEmoji 按最长匹配统计 emoji 序列数，并返回多码位序列比单个字符多出的码位数。
// scanEmoji counts emoji sequences by longest match and returns how many extra code points
// multi-code-point sequences carry beyond one character each.
func (d *Dict) scanEmoji(rs []rune) (count, extra int) {
	for i := 0; i < len(rs); {
		best, counted := 0, false
		n := d.emoji
		for j := i; j < len(rs); j++ {
			if n = n.next[rs[j]]; n == nil {
				break
			}
			if n.end {
				best, counted = j-i+1, n.counted
			}
		}
		if best == 0 {
			i++
			continue
		}
		if counted {
			count++
		}
		extra += best - 1
		i += best
	}
	return count, extra
}
