package config

import (
	"fmt"
	"regexp"
	"strings"
)

// envName 匹配合法的环境变量名 / envName matches a valid environment variable name
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// unquote 去掉值两端成对的单引号或双引号 / unquote strips a matching pair of single or double quotes
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// ParseEnv 解析 .env 格式：每行 KEY=VALUE，以 # 开头的行为注释，值两端成对的引号会被去掉。
// 出错信息只含行号与变量名，不含值，避免凭据进入日志。
//
// ParseEnv parses the .env format: KEY=VALUE per line, lines starting with # are comments.
// Errors carry only line numbers and names, never values, so credentials stay out of logs.
func ParseEnv(data string) (map[string]string, error) {
	env := map[string]string{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !envName.MatchString(name) {
			return nil, fmt.Errorf("env line %d: want NAME=VALUE", i+1)
		}
		if _, dup := env[name]; dup {
			return nil, fmt.Errorf("env line %d: %s set twice", i+1, name)
		}
		env[name] = unquote(strings.TrimSpace(value))
	}
	return env, nil
}
