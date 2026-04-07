package fetcher

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxBodySize 最大响应体大小 100MB
	MaxBodySize = 100 * 1024 * 1024
)

// ErrBodyTooLarge 响应体超过限制
var ErrBodyTooLarge = errors.New("response body exceeds max size limit")

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
}

// DefaultUserAgentTemplate 默认 User-Agent 模板
// 使用 BuildUserAgent(version) 生成完整 UA
const DefaultUserAgentTemplate = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36 dj/{version} (+https://github.com/ejfkdev/dj)"

// BuildUserAgent 生成完整的 User-Agent 字符串
func BuildUserAgent(version string) string {
	if version == "" {
		version = "dev"
	}
	return strings.ReplaceAll(DefaultUserAgentTemplate, "{version}", version)
}

// NewFetcher 创建下载器（使用默认配置）
func NewFetcher() *Fetcher {
	return NewFetcherWithConfig("")
}

// NewFetcherWithConfig 创建下载器，支持自定义 User-Agent 和代理
// userAgent 为空使用默认 UA，proxy 为空使用环境变量代理
func NewFetcherWithConfig(proxy string) *Fetcher {
	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	// 设置代理
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}

	return &Fetcher{
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil // 直接跟随重定向
			},
		},
		userAgent: BuildUserAgent("dev"),
	}
}

// SetUserAgent 设置自定义 User-Agent
func (f *Fetcher) SetUserAgent(ua string) {
	if ua != "" {
		f.userAgent = ua
	}
}

// newRequest 创建一个带默认请求头的 GET 请求
func (f *Fetcher) newRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	return req, nil
}

// limitedReader 限制读取大小
type limitedReader struct {
	r    io.Reader
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
func (f *Fetcher) Fetch(url string) ([]byte, error) {
	req, err := f.newRequest(url)
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
func (f *Fetcher) FetchWithStatus(url string) (*FetchResult, error) {
	req, err := f.newRequest(url)
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
func (f *Fetcher) FetchWithStatusHead(url string) (*FetchResult, error) {
	resp, err := f.client.Head(url)
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
		// brotli - Go 1.23+ has built-in brotli support via compress/brotli
		// 如果不支持 brotli，使用 identity (不压缩)
		return resp.Body, nil
	default:
		return resp.Body, nil
	}
}
