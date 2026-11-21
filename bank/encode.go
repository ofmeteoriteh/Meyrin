package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Encode 把题目编码为 JSONL：每行一个对象，冒号与逗号后带一个空格，非 ASCII 字符原样输出。
// Encode writes questions as JSONL: one object per line, a space after each
// colon and comma, non-ASCII characters unescaped.
func Encode(qs []Question) ([]byte, error) {
	var line, out bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	for _, q := range qs {
		line.Reset()
		if err := enc.Encode(q); err != nil {
			return nil, fmt.Errorf("encode question %d: %w", q.ID, err)
		}
		spaceSeparators(&out, line.Bytes())
	}
	return out.Bytes(), nil
}

// spaceSeparators 在字符串之外的冒号与逗号后补空格，并把 encoding/json 转义的
// U+2028、U+2029 还原为原字符，其余转义保持不变。
//
// spaceSeparators adds a space after colons and commas outside strings and
// restores U+2028 and U+2029, which encoding/json always escapes.
func spaceSeparators(out *bytes.Buffer, compact []byte) {
	inString := false
	for i := 0; i < len(compact); i++ {
		b := compact[i]
		switch {
		case inString && b == '\\':
			if esc := compact[i:]; bytes.HasPrefix(esc, []byte(` `)) {
				out.WriteRune(' ')
				i += 5
			} else if bytes.HasPrefix(esc, []byte(` `)) {
				out.WriteRune(' ')
				i += 5
			} else {
				out.Write(compact[i : i+2])
				i++
			}
		case b == '"':
			inString = !inString
			out.WriteByte(b)
		case !inString && (b == ':' || b == ','):
			out.WriteByte(b)
			out.WriteByte(' ')
		default:
			out.WriteByte(b)
		}
	}
}
