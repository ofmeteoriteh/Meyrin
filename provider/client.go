package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ofmeteoriteh/Meyrin/config"
)

const (
	// requestTimeout 覆盖长推理的最慢响应 / requestTimeout covers the slowest long-reasoning responses
	requestTimeout = 15 * time.Minute
	// errorBodyLimit 限制错误信息中保留的响应体长度 / errorBodyLimit caps the response body kept in errors
	errorBodyLimit = 2048
	redacted       = "[REDACTED]"
)

// Client 向一家厂商发送请求；代理只取自厂商配置，不读取环境变量。
// Client sends requests to one vendor; the proxy comes only from vendor config, never the environment.
type Client struct {
	endpoint config.Endpoint
	key      string
	http     *http.Client
}

// NewClient 创建厂商客户端，proxy 为 nil 时直连 / NewClient creates a vendor client; a nil proxy connects directly
func NewClient(ep config.Endpoint, key string, proxy *url.URL) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &Client{
		endpoint: ep,
		key:      key,
		http:     &http.Client{Transport: transport, Timeout: requestTimeout},
	}
}

// HTTPError 是非 2xx 响应，Body 已脱敏并截断 / HTTPError is a non-2xx response with a redacted, truncated body
type HTTPError struct {
	Status     int
	RetryAfter time.Duration
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

// Retryable 判断限流与服务端错误 / Retryable reports rate limits and server errors
func (e *HTTPError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError
}

// IsRetryable 判断错误能否以同一请求体重试：限流、5xx 与网络故障可以，解析失败与 4xx 不可以。
// IsRetryable reports whether the same body may be resent: rate limits, 5xx and network faults may.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable()
	}
	var netErr *url.Error
	return errors.As(err, &netErr)
}

// Send 发送已编码的请求体，返回统一后的响应与耗时 / Send posts an encoded body and returns the normalized response and latency
func (c *Client) Send(ctx context.Context, body []byte) (Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return Response{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.endpoint.Auth == config.AuthBearer {
		req.Header.Set("Authorization", "Bearer "+c.key)
	} else {
		req.Header.Set(c.endpoint.Auth, c.key)
	}
	for k, v := range adapters[c.endpoint.Protocol].headers() {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	latency := time.Since(start)
	if err != nil {
		return Response{}, latency, &url.Error{Op: "read", URL: c.endpoint.URL, Err: err}
	}

	if resp.StatusCode/100 != 2 {
		return Response{}, latency, &HTTPError{
			Status:     resp.StatusCode,
			RetryAfter: retryAfter(resp.Header.Get("Retry-After")),
			Body:       c.redact(data),
		}
	}
	parsed, err := Parse(c.endpoint.Protocol, data)
	if err != nil {
		return Response{}, latency, fmt.Errorf("%w: %s", err, c.redact(data))
	}
	return parsed, latency, nil
}

// redact 去掉响应体中可能回显的 key 并截断 / redact strips any echoed key and truncates
func (c *Client) redact(data []byte) string {
	s := string(data)
	if c.key != "" {
		s = strings.ReplaceAll(s, c.key, redacted)
	}
	if len(s) > errorBodyLimit {
		s = strings.ToValidUTF8(s[:errorBodyLimit], "") + "…"
	}
	return s
}

// retryAfter 解析以秒为单位的 Retry-After / retryAfter parses a Retry-After given in seconds
func retryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
