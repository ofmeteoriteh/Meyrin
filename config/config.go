// Package config 读取模型清单、厂商端点与 .env，并核对它们与测试规范一致。
// 厂商名单以 docs/SPEC.md 为准，端点与协议以 docs/api-endpoints.md 为准，
// configs/models.yaml 只补充凭据变量名与模型 ID。
//
// Package config loads the model list, vendor endpoints and .env, and checks
// them against the test spec. Vendor lists follow docs/SPEC.md, endpoints and
// protocols follow docs/api-endpoints.md; configs/models.yaml only adds
// credential variable names and model IDs.
package config

import (
	"bytes"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Models 是 configs/models.yaml 的内容 / Models is the content of configs/models.yaml
type Models struct {
	Vendors []Vendor `yaml:"vendors"`
	Judges  []Judge  `yaml:"judges"`
}

// Vendor 是一家受测厂商；Model 为空表示尚未选定模型，Proxy 为空表示直连。
// Vendor is one vendor under test; an empty Model is unselected and an empty Proxy connects directly.
type Vendor struct {
	Name   string `yaml:"name"`
	KeyEnv string `yaml:"key_env"`
	Model  string `yaml:"model"`
	Proxy  string `yaml:"proxy"`
}

// proxySchemes 是 net/http 支持的代理协议 / proxySchemes are the proxy schemes net/http supports
var proxySchemes = []string{"http", "https", "socks5"}

// ProxyURL 解析厂商代理地址，未配置时返回 nil / ProxyURL parses the vendor proxy, returning nil when unset
func (v Vendor) ProxyURL() (*url.URL, error) {
	if v.Proxy == "" {
		return nil, nil
	}
	u, err := url.Parse(v.Proxy)
	if err != nil || !slices.Contains(proxySchemes, u.Scheme) || u.Host == "" {
		return nil, fmt.Errorf("vendor %q: proxy must be a %v URL with a host", v.Name, proxySchemes)
	}
	return u, nil
}

// 评审的调用方式，取值与规范「评审模型：」表一致 / Judge access modes, named as in the spec's judge table
const (
	AccessAPI          = "官方 API"
	AccessSubscription = "订阅"
)

// Judge 是一个评审席位，使用独立凭据；订阅方式的地址取自 BaseURLEnv 指向的环境变量。
// Judge is one judge seat with its own credential; a subscription seat reads its address
// from the variable named by BaseURLEnv.
type Judge struct {
	Vendor     string `yaml:"vendor"`
	Access     string `yaml:"access"`
	KeyEnv     string `yaml:"key_env"`
	BaseURLEnv string `yaml:"base_url_env"`
	Model      string `yaml:"model"`
}

// judgePath 是订阅席位 base URL 之后的请求路径 / judgePath follows a subscription seat's base URL
const judgePath = "/chat/completions"

// Endpoint 返回评审席位的端点：官方 API 取自端点文档且须为 Chat Completions；订阅席位的请求
// 发往 BaseURLEnv 所指 base URL 之后的 judgePath，以 Bearer 鉴权。
//
// Endpoint resolves a judge seat: official API seats come from the endpoint document and
// must speak Chat Completions; subscription seats post to judgePath under the base URL
// named by BaseURLEnv, with Bearer auth.
func (j Judge) Endpoint(endpoints map[string]Endpoint, env map[string]string) (Endpoint, error) {
	if j.Access == AccessAPI {
		ep, ok := endpoints[j.Vendor]
		if !ok || ep.Protocol != OpenAIChat {
			return Endpoint{}, fmt.Errorf("judge %q needs an %s endpoint", j.Vendor, OpenAIChat)
		}
		return ep, nil
	}
	base := env[j.BaseURLEnv]
	u, err := url.Parse(base)
	if base == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Endpoint{}, fmt.Errorf("judge %q: %s must be an http(s) base URL", j.Vendor, j.BaseURLEnv)
	}
	return Endpoint{URL: strings.TrimSuffix(base, "/") + judgePath, Auth: AuthBearer, Protocol: OpenAIChat}, nil
}

// LoadModels 解析模型清单，拒绝未知字段 / LoadModels parses the model list and rejects unknown fields
func LoadModels(data []byte) (Models, error) {
	var m Models
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Models{}, fmt.Errorf("parse model list: %w", err)
	}
	return m, nil
}
