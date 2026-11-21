package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ofmeteoriteh/Meyrin/mdtable"
)

// Protocol 是厂商 API 的请求协议，取值与 docs/api-endpoints.md「协议」表一致。
// Protocol is a vendor API's request protocol, named as in the endpoint document.
type Protocol string

// 四类协议各对应一个 adapter / Each protocol has one adapter
const (
	OpenAIResponses    Protocol = "OpenAI Responses"
	OpenAIChat         Protocol = "OpenAI Chat Completions"
	AnthropicMessages  Protocol = "Anthropic Messages"
	GeminiInteractions Protocol = "Gemini Interactions"
)

var protocols = []Protocol{OpenAIResponses, OpenAIChat, AnthropicMessages, GeminiInteractions}

// 端点文档中两张表的锚点 / Anchors of the two tables in the endpoint document
const (
	endpointAnchor = "# 官方 API 入口"
	protocolAnchor = "## 协议"
)

// AuthBearer 表示以 Authorization: Bearer <key> 鉴权；其余取值是直接携带 key 的请求头名。
// AuthBearer means Authorization: Bearer <key>; any other value names the header carrying the key.
const AuthBearer = "Authorization: Bearer"

// authHeader 匹配直接携带 key 的请求头名 / authHeader matches a header name that carries the key directly
var authHeader = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Endpoint 是一家厂商的请求地址、鉴权方式与协议 / Endpoint is a vendor's request URL, auth scheme and protocol
type Endpoint struct {
	URL      string
	Auth     string
	Protocol Protocol
}

// ParseEndpoints 从端点文档读取各厂商的端点与协议，两张表的厂商须一一对应。
// ParseEndpoints reads each vendor's endpoint and protocol; both tables must list the same vendors.
func ParseEndpoints(md string) (map[string]Endpoint, error) {
	urls, err := mdtable.After(md, endpointAnchor)
	if err != nil {
		return nil, fmt.Errorf("endpoints: %w", err)
	}
	if err := urls.Check("厂商", "密钥页", "端点", "鉴权头", "文档"); err != nil {
		return nil, fmt.Errorf("endpoints: %w", err)
	}
	protos, err := mdtable.After(md, protocolAnchor)
	if err != nil {
		return nil, fmt.Errorf("protocols: %w", err)
	}
	if err := protos.Check("协议", "厂商"); err != nil {
		return nil, fmt.Errorf("protocols: %w", err)
	}

	// 协议表一行列出多家厂商，以顿号分隔 / Each protocol row lists vendors separated by 、
	protocolOf := map[string]Protocol{}
	for _, row := range protos.Rows {
		p := Protocol(row[0])
		if !slices.Contains(protocols, p) {
			return nil, fmt.Errorf("protocols: unknown protocol %q", p)
		}
		for _, v := range strings.Split(row[1], "、") {
			if _, dup := protocolOf[v]; dup {
				return nil, fmt.Errorf("protocols: vendor %q listed twice", v)
			}
			protocolOf[v] = p
		}
	}

	endpoints := map[string]Endpoint{}
	for _, row := range urls.Rows {
		vendor, url, auth := row[0], mdtable.Unquote(row[2]), mdtable.Unquote(row[3])
		if _, dup := endpoints[vendor]; dup {
			return nil, fmt.Errorf("endpoints: vendor %q listed twice", vendor)
		}
		if !strings.HasPrefix(url, "https://") {
			return nil, fmt.Errorf("endpoints: %s endpoint %q is not HTTPS", vendor, url)
		}
		if auth != AuthBearer && !authHeader.MatchString(auth) {
			return nil, fmt.Errorf("endpoints: %s auth %q is neither %q nor a header name", vendor, auth, AuthBearer)
		}
		p, ok := protocolOf[vendor]
		if !ok {
			return nil, fmt.Errorf("protocols: vendor %q has no protocol", vendor)
		}
		endpoints[vendor] = Endpoint{URL: url, Auth: auth, Protocol: p}
	}
	for v := range protocolOf {
		if _, ok := endpoints[v]; !ok {
			return nil, fmt.Errorf("endpoints: vendor %q has a protocol but no endpoint", v)
		}
	}
	return endpoints, nil
}
