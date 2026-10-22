package config

import (
	"os"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRepoFilesAreConsistent(t *testing.T) {
	m, err := LoadModels([]byte(readRepoFile(t, "configs/models.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	err = Check(m, readRepoFile(t, "docs/SPEC.md"), readRepoFile(t, "docs/api-endpoints.md"),
		readRepoFile(t, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
}

const (
	fixtureSpec = "受测模型：\n\n| 厂商 |\n|---|\n| OpenAI |\n| 智谱 |\n\n" +
		"评审模型：\n\n| 厂商 | 调用方式 |\n|---|---|\n| OpenAI | 订阅 |\n| 智谱 | 官方 API |\n"
	fixtureEndpoints = "# 官方 API 入口\n\n说明\n\n| 厂商 | 密钥页 | 端点 | 鉴权头 | 文档 |\n|---|---|---|---|---|\n" +
		"| OpenAI | k | `https://api.openai.com/v1/responses` | `Authorization: Bearer` | d |\n" +
		"| 智谱 | k | `https://open.bigmodel.cn/api/paas/v4/chat/completions` | `api-key` | d |\n\n" +
		"## 协议\n\n| 协议 | 厂商 |\n|---|---|\n| OpenAI Responses | OpenAI |\n| OpenAI Chat Completions | 智谱 |\n"
	fixtureModels = "vendors:\n" +
		"  - {name: OpenAI, key_env: OPENAI_API_KEY, model: \"\"}\n" +
		"  - {name: 智谱, key_env: ZHIPU_API_KEY, model: \"\"}\n" +
		"judges:\n" +
		"  - {vendor: OpenAI, access: 订阅, key_env: OPENAI_JUDGE_API_KEY, base_url_env: OPENAI_JUDGE_BASE_URL, model: \"\"}\n" +
		"  - {vendor: 智谱, access: 官方 API, key_env: ZHIPU_JUDGE_API_KEY, model: \"\"}\n"
	fixtureEnv = "# 注释\nOPENAI_API_KEY=<x>\n\nZHIPU_API_KEY=<y>\n" +
		"OPENAI_JUDGE_API_KEY=<a>\nOPENAI_JUDGE_BASE_URL=<b>\nZHIPU_JUDGE_API_KEY=<c>\n"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name                         string
		spec, endpoints, models, env string
		want                         []string
	}{
		{name: "一致"},
		{
			name:   "厂商顺序与规范不同",
			models: strings.Replace(strings.Replace(fixtureModels, "OpenAI, key_env", "X, key_env", 1), "智谱, key_env", "OpenAI, key_env", 1),
			want:   []string{"as in the spec", `endpoint vendor "智谱" is not in the model list`},
		},
		{
			name: "评审调用方式与规范不同",
			spec: strings.Replace(fixtureSpec, "| OpenAI | 订阅 |", "| OpenAI | 官方 API |", 1),
			want: []string{"judges"},
		},
		{
			name:   "官方 API 评审缺端点",
			models: strings.Replace(fixtureModels, "vendor: 智谱, access: 官方 API", "vendor: Google, access: 官方 API", 1),
			want:   []string{`judge "Google" uses the official API but has no endpoint`},
		},
		{
			name:   "评审复用受测凭据",
			models: strings.Replace(fixtureModels, "key_env: ZHIPU_JUDGE_API_KEY", "key_env: ZHIPU_API_KEY", 1),
			want:   []string{"judges need their own keys"},
		},
		{
			name:   "订阅评审缺地址变量",
			models: strings.Replace(fixtureModels, "base_url_env: OPENAI_JUDGE_BASE_URL", "base_url_env: \"\"", 1),
			want:   []string{"NAME_BASE_URL", "OPENAI_JUDGE_BASE_URL, which no vendor or judge uses"},
		},
		{
			name:   "官方 API 评审带地址变量",
			models: strings.Replace(fixtureModels, "key_env: ZHIPU_JUDGE_API_KEY,", "key_env: ZHIPU_JUDGE_API_KEY, base_url_env: X_BASE_URL,", 1),
			want:   []string{"only for subscriptions"},
		},
		{
			name:   "未知调用方式",
			models: strings.Replace(fixtureModels, "access: 订阅", "access: 网页", 1),
			want:   []string{`access "网页"`},
		},
		{
			name:      "厂商缺端点",
			endpoints: strings.Replace(strings.Replace(fixtureEndpoints, "| 智谱 | k |", "| 小米 | k |", 1), "| 智谱 |\n", "| 小米 |\n", 1),
			want:      []string{`vendor "智谱" has no endpoint`, `endpoint vendor "小米"`},
		},
		{
			name:   "凭据变量重复且 .env.example 无效",
			models: strings.Replace(fixtureModels, "ZHIPU_API_KEY", "OPENAI_API_KEY", 1),
			env:    "OPENAI_API_KEY=x\nzhipu=y\n",
			want:   []string{"used twice", ".env.example"},
		},
		{
			name:   "凭据变量格式错误",
			models: strings.Replace(fixtureModels, "ZHIPU_API_KEY", "ZHIPU_TOKEN", 1),
			want:   []string{"NAME_API_KEY", "lacks ZHIPU_TOKEN", "ZHIPU_API_KEY, which no vendor or judge uses"},
		},
		{
			name: "规范缺表",
			spec: "评审模型：\n\n| 厂商 | 调用方式 |\n|---|---|\n| OpenAI | 订阅 |\n",
			want: []string{"受测模型"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, endpoints, models, env := or(tt.spec, fixtureSpec), or(tt.endpoints, fixtureEndpoints),
				or(tt.models, fixtureModels), or(tt.env, fixtureEnv)
			m, err := LoadModels([]byte(models))
			if err != nil {
				t.Fatal(err)
			}
			err = Check(m, spec, endpoints, env)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Check passed, want errors containing %q", tt.want)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestLoadModelsRejectsUnknownField(t *testing.T) {
	if _, err := LoadModels([]byte("vendors:\n  - {name: OpenAI, base_url: x}\n")); err == nil {
		t.Fatal("LoadModels accepted an unknown field")
	}
}

func TestParseEndpoints(t *testing.T) {
	eps, err := ParseEndpoints(fixtureEndpoints)
	if err != nil {
		t.Fatal(err)
	}
	if got := eps["智谱"]; got.Protocol != OpenAIChat || got.Auth != "api-key" ||
		got.URL != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
		t.Fatalf("智谱 = %+v", got)
	}
	if got := eps["OpenAI"].Auth; got != AuthBearer {
		t.Fatalf("OpenAI auth = %q", got)
	}

	tests := []struct {
		name, md, want string
	}{
		{"未知协议", strings.Replace(fixtureEndpoints, "| OpenAI Responses |", "| SOAP |", 1), "unknown protocol"},
		{"厂商无协议", strings.Replace(fixtureEndpoints, "| OpenAI Responses | OpenAI |\n", "", 1), `"OpenAI" has no protocol`},
		{"协议厂商无端点", strings.Replace(fixtureEndpoints, "| OpenAI Responses | OpenAI |", "| OpenAI Responses | OpenAI、xAI |", 1), `"xAI" has a protocol but no endpoint`},
		{"协议表重复厂商", strings.Replace(fixtureEndpoints, "| OpenAI Chat Completions | 智谱 |", "| OpenAI Chat Completions | 智谱、OpenAI |", 1), "listed twice"},
		{"非 HTTPS", strings.Replace(fixtureEndpoints, "`https://api.openai.com", "`http://api.openai.com", 1), "not HTTPS"},
		{"表头改动", strings.Replace(fixtureEndpoints, "| 端点 |", "| 地址 |", 1), "header"},
		{"鉴权头无效", strings.Replace(fixtureEndpoints, "`api-key`", "`Bearer`", 1), "neither"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEndpoints(tt.md)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestParseEnv(t *testing.T) {
	env, err := ParseEnv("# 注释\n\n  A_KEY=x=y \nB_KEY=\nC_URL=\"http://h:1/v1\"\nD_KEY='s'\nE_KEY=\"unbalanced\nF_KEY=\"\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A_KEY": "x=y", "B_KEY": "", "C_URL": "http://h:1/v1", "D_KEY": "s", "E_KEY": `"unbalanced`, "F_KEY": `"`}
	if len(env) != len(want) {
		t.Fatalf("env = %q", env)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}

	for name, data := range map[string]string{
		"缺等号":   "A_KEY\n",
		"小写变量名": "a_key=secret\n",
		"重复变量":  "A_KEY=1\nA_KEY=secret\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseEnv(data)
			if err == nil {
				t.Fatal("ParseEnv accepted invalid input")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaks the value: %v", err)
			}
		})
	}
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func TestVendorProxyURL(t *testing.T) {
	tests := []struct {
		proxy   string
		wantErr bool
	}{
		{"", false},
		{"http://127.0.0.1:7890", false},
		{"socks5://127.0.0.1:1080", false},
		{"127.0.0.1:7890", true},
		{"ftp://127.0.0.1", true},
		{"http://", true},
	}
	for _, tt := range tests {
		u, err := Vendor{Name: "X", Proxy: tt.proxy}.ProxyURL()
		if (err != nil) != tt.wantErr {
			t.Errorf("ProxyURL(%q) err = %v, wantErr %v", tt.proxy, err, tt.wantErr)
		}
		if tt.proxy == "" && u != nil {
			t.Errorf("ProxyURL(\"\") = %v, want nil", u)
		}
	}
}
