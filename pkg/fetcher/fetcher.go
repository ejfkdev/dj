package fetcher

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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

// FetcherConfig Fetcher 配置
type FetcherConfig struct {
	Proxy   string // HTTP 代理 URL
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
	client    *http.Client
	userAgent string
	cookieJar http.CookieJar
}

// NewFetcher 创建下载器（使用默认配置）
func NewFetcher() *Fetcher {
	f, _ := NewFetcherWithConfig(FetcherConfig{UseUTLS: true})
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
		proxyURL, _ = url.Parse(cfg.Proxy)
	}

	if cfg.UseUTLS {
		transport = newUTLSTransport(proxyURL)
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
			stdTransport.Proxy = http.ProxyFromEnvironment
		}
		transport = stdTransport
	}

	return &Fetcher{
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil // 直接跟随重定向
			},
			Jar: jar,
		},
		userAgent: DefaultUserAgent,
		cookieJar: jar,
	}, nil
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
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	return req, nil
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

	// 检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("HTTP status: " + resp.Status)
	}

	// 解压响应体
	body, err := decompressResponse(resp)
	if err != nil {
		return nil, err
	}

	// 限制响应体大小
	limited := &limitedReader{r: body, remain: MaxBodySize}
	content, err := io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			return nil, err
		}
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

	// 解压响应体
	body, err := decompressResponse(resp)
	if err != nil {
		return nil, err
	}

	// 限制响应体大小
	limited := &limitedReader{r: body, remain: MaxBodySize}
	content, err := io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			return nil, err
		}
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
