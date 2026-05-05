package fetcher

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

const (
	// MaxBodySize 最大响应体大小 100MB
	MaxBodySize = 100 * 1024 * 1024
)

// ErrBodyTooLarge 响应体超过限制
var ErrBodyTooLarge = errors.New("response body exceeds max size limit")

// DefaultUserAgent 默认 User-Agent（模拟 Chrome，不暴露工具标识）
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"

// chromeSecChUA 与 DefaultUserAgent 对应的 Sec-CH-UA 值
const chromeSecChUA = `"Chromium";v="148", "Google Chrome";v="148", "Not-A.Brand";v="99"`

// FetcherConfig Fetcher 配置
type FetcherConfig struct {
	Proxy   string // 代理 URL (http/https/socks5)
	UseUTLS bool   // 启用 uTLS TLS 指纹伪装
}

// FetchResult 包含内容和状态码
type FetchResult struct {
	Content     []byte
	StatusCode  int
	ContentType string
	Headers     http.Header // HTTP 响应头
}

// Fetcher HTTP 下载器
type Fetcher struct {
	client         *http.Client
	userAgent      string
	cookieJar      http.CookieJar
	browserHeaders bool
}

// NewFetcher 创建下载器（使用默认配置）
func NewFetcher() *Fetcher {
	f, err := NewFetcherWithConfig(FetcherConfig{UseUTLS: true})
	if err != nil {
		panic(fmt.Sprintf("failed to create fetcher: %v", err))
	}
	return f
}

// NewFetcherWithConfig 创建下载器，支持 uTLS 指纹伪装、Cookie Jar 和代理
func NewFetcherWithConfig(cfg FetcherConfig) (*Fetcher, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	var transport http.RoundTripper

	var proxyURL *url.URL
	if cfg.Proxy != "" {
		var parseErr error
		proxyURL, parseErr = parseProxyURL(cfg.Proxy)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.Proxy, parseErr)
		}
	}

	if cfg.UseUTLS {
		transport = newMultiTransport(proxyURL)
	} else {
		stdTransport := &http.Transport{
			MaxIdleConns:        2000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		}
		if proxyURL != nil {
			stdTransport.Proxy = http.ProxyURL(proxyURL)
		} else {
			stdTransport.Proxy = proxyForStdTransport
		}
		transport = stdTransport
	}

	return &Fetcher{
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 20 {
					return fmt.Errorf("too many redirects (%d)", len(via))
				}
				return nil
			},
			Jar: jar,
		},
		userAgent:      DefaultUserAgent,
		cookieJar:      jar,
		browserHeaders: cfg.UseUTLS,
	}, nil
}

// parseProxyURL 解析代理 URL，自动补全 http:// scheme
func parseProxyURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return url.Parse(raw)
}

// proxyForStdTransport 标准传输代理函数，支持 ALL_PROXY 环境变量
func proxyForStdTransport(req *http.Request) (*url.URL, error) {
	u, err := http.ProxyFromEnvironment(req)
	if u != nil || err != nil {
		return u, err
	}
	// 回退到 ALL_PROXY
	for _, key := range []string{"ALL_PROXY", "all_proxy"} {
		if v := os.Getenv(key); v != "" {
			noProxy := getEnvAny("NO_PROXY", "no_proxy")
			if !shouldBypassProxy(req.URL.Hostname(), noProxy) {
				if !strings.Contains(v, "://") {
					v = "http://" + v
				}
				if pu, parseErr := url.Parse(v); parseErr == nil {
					return pu, nil
				}
			}
		}
	}
	return nil, nil
}

// SetUserAgent 设置自定义 User-Agent
func (f *Fetcher) SetUserAgent(ua string) {
	if ua != "" {
		f.userAgent = ua
	}
}

// SetCookies 向 Fetcher 的 Cookie Jar 中注入 cookie
func (f *Fetcher) SetCookies(targetURL string, cookies []*http.Cookie) error {
	u, err := url.Parse(targetURL)
	if err != nil {
		return err
	}
	f.cookieJar.SetCookies(u, cookies)
	return nil
}

// newRequest 创建一个带默认请求头的 GET 请求
func (f *Fetcher) newRequest(rawURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	if f.browserHeaders {
		setBrowserHeaders(req, f.userAgent)
	} else {
		req.Header.Set("User-Agent", f.userAgent)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	}

	return req, nil
}

// setBrowserHeaders 设置完整浏览器仿真请求头，模拟 Chrome 浏览器
func setBrowserHeaders(req *http.Request, ua string) {
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", chromeSecChUA)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

// limitedReader 限制读取大小
type limitedReader struct {
	r      io.Reader
	remain int64
}

func (lr *limitedReader) Read(p []byte) (int, error) {
	if lr.remain <= 0 {
		return 0, ErrBodyTooLarge
	}
	toRead := lr.remain
	if int64(len(p)) < toRead {
		toRead = int64(len(p))
	}
	n, err := lr.r.Read(p[:toRead])
	lr.remain -= int64(n)
	return n, err
}

// Fetch 获取 URL 内容
func (f *Fetcher) Fetch(rawURL string) ([]byte, error) {
	req, err := f.newRequest(rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("HTTP status: " + resp.Status)
	}

	body, err := decompressResponse(resp)
	if err != nil {
		return nil, err
	}
	if rc, ok := body.(io.Closer); ok {
		defer rc.Close()
	}

	limited := &limitedReader{r: body, remain: MaxBodySize}
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// FetchWithStatus 获取 URL 内容和状态码
func (f *Fetcher) FetchWithStatus(rawURL string) (*FetchResult, error) {
	req, err := f.newRequest(rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := decompressResponse(resp)
	if err != nil {
		return nil, err
	}
	if rc, ok := body.(io.Closer); ok {
		defer rc.Close()
	}

	limited := &limitedReader{r: body, remain: MaxBodySize}
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		Content:     content,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     resp.Header,
	}, nil
}

// FetchWithStatusHead 使用 HEAD 请求探测 URL 是否存在
func (f *Fetcher) FetchWithStatusHead(rawURL string) (*FetchResult, error) {
	resp, err := f.client.Head(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return &FetchResult{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     resp.Header,
	}, nil
}

// decompressResponse 根据 Content-Encoding 解压响应体
func decompressResponse(resp *http.Response) (io.Reader, error) {
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		return gz, nil
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		return zr, nil
	case "br":
		return brotli.NewReader(resp.Body), nil
	default:
		return resp.Body, nil
	}
}
