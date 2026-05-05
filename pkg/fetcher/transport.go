package fetcher

import (
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/http2"
)

// protocolCache 缓存 host 支持的协议
type protocolCache struct {
	mu    sync.RWMutex
	hosts map[string]protocol
}

type protocol int

const (
	protoUnknown protocol = iota
	protoH2 // HTTP/2 (uTLS)
	protoH1 // HTTP/1.1 (uTLS)
)

func newProtocolCache() *protocolCache {
	return &protocolCache{hosts: make(map[string]protocol)}
}

func (c *protocolCache) Get(host string) (protocol, bool) {
	c.mu.RLock()
	p, ok := c.hosts[host]
	c.mu.RUnlock()
	return p, ok
}

func (c *protocolCache) Set(host string, p protocol) {
	c.mu.Lock()
	c.hosts[host] = p
	c.mu.Unlock()
}

// multiTransport 多协议复合 RoundTripper
// HTTP/3 降级为 HTTP/2（支持 H3 的服务器必然支持 H2）
// 优先级: HTTP/2 (uTLS) > HTTP/1.1 (uTLS)
type multiTransport struct {
	h2Transport *http2.Transport // HTTP/2 (uTLS 指纹)
	h1Transport *http.Transport  // HTTP/1.1 (uTLS 指纹)
	cache       *protocolCache   // 协议缓存
}

// newMultiTransport 创建多协议传输
func newMultiTransport(proxyURL *url.URL) *multiTransport {
	return &multiTransport{
		h2Transport: newH2Transport(proxyURL),
		h1Transport: newH1Transport(proxyURL),
		cache:       newProtocolCache(),
	}
}

func (mt *multiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host

	if known, ok := mt.cache.Get(host); ok {
		return mt.roundTripWithProto(req, known)
	}

	resp, proto, err := mt.tryProtocols(req)
	if err != nil {
		return nil, err
	}

	mt.cache.Set(host, proto)
	return resp, nil
}

// tryProtocols 按优先级尝试各协议
func (mt *multiTransport) tryProtocols(req *http.Request) (*http.Response, protocol, error) {
	// 1. 尝试 HTTP/2
	resp, err := mt.h2Transport.RoundTrip(req)
	if err == nil {
		return resp, protoH2, nil
	}
	// H2 返回了响应体则关闭，避免泄漏
	if resp != nil {
		resp.Body.Close()
	}

	// 2. 回退 HTTP/1.1
	resp, err = mt.h1Transport.RoundTrip(req)
	if err != nil {
		return nil, protoUnknown, err
	}
	return resp, protoH1, nil
}

// roundTripWithProto 使用已知协议发起请求，失败时降级并更新缓存
func (mt *multiTransport) roundTripWithProto(req *http.Request, p protocol) (*http.Response, error) {
	host := req.URL.Host

	var resp *http.Response
	var err error

	switch p {
	case protoH2:
		resp, err = mt.h2Transport.RoundTrip(req)
	case protoH1:
		resp, err = mt.h1Transport.RoundTrip(req)
	default:
		resp, _, err = mt.tryProtocols(req)
		return resp, err
	}

	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		// 降级重试并更新缓存
		resp, newProto, retryErr := mt.tryProtocols(req)
		if retryErr == nil {
			mt.cache.Set(host, newProto)
		}
		return resp, retryErr
	}

	return resp, nil
}

// 确保 multiTransport 实现 http.RoundTripper
var _ http.RoundTripper = (*multiTransport)(nil)
