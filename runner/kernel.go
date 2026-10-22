package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// KernelReleasesURL 是 kernel.org 的机读版本列表，latest_stable 即首页的 Latest Release。
// KernelReleasesURL is kernel.org's machine-readable release list; latest_stable is the
// homepage's Latest Release.
const KernelReleasesURL = "https://www.kernel.org/releases.json"

// FetchKernelStable 获取第 7 题的参考值：当前稳定版版本号 / FetchKernelStable fetches question 7's reference
func FetchKernelStable(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// 与厂商请求一致，不读取环境中的代理 / Like vendor calls, ignore proxies from the environment
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}

	var releases struct {
		LatestStable struct {
			Version string `json:"version"`
		} `json:"latest_stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("%s: %w", url, err)
	}
	if releases.LatestStable.Version == "" {
		return "", errors.New("kernel.org releases have no latest_stable version")
	}
	return releases.LatestStable.Version, nil
}
