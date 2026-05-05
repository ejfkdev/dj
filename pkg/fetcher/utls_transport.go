package fetcher

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	utls "github.com/refraction-networking/utls"
)

// proxyFromEnvironment 从环境变量解析代理 URL
func proxyFromEnvironment() *url.URL {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(key); v != "" {
			if u, err := url.Parse(v); err == nil {
				return u
			}
		}
	}
	return nil
}

// newUTLSTransport 创建使用 uTLS 的 http.Transport，模拟 Chrome TLS 指纹
// 代理由 Transport 自行处理（CONNECT 隧道），DialTLSContext 只负责 TLS 握手
func newUTLSTransport(proxyURL *url.URL) *http.Transport {
	// 解析代理：优先显式指定的代理，其次从环境变量读取
	resolvedProxy := proxyURL
	if resolvedProxy == nil {
		resolvedProxy = proxyFromEnvironment()
	}

	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	// 设置代理（Transport 会自行处理 CONNECT 隧道）
	if resolvedProxy != nil {
		transport.Proxy = http.ProxyURL(resolvedProxy)
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}

	// 重写 TLS 拨号，使用 uTLS
	// Transport 处理完代理 CONNECT 后，在此连接上进行 TLS 握手
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Transport 在有代理时会先建立到代理的 TCP 连接，发送 CONNECT，
		// 然后在此调用中进行 TLS 握手。
		// 但实际上 Transport 会先调用 DialContext 建立 TCP 连接，
		// 然后对 HTTPS 请求调用 DialTLSContext 来建立 TLS。
		// 所以这里的 conn 需要我们自行建立 TCP 连接。
		//
		// 注意：当设置了 DialTLSContext 后，Transport 不再使用 DialContext
		// 来建立 HTTPS 连接，而是直接使用 DialTLSContext。
		// Transport 会在 DialTLSContext 返回的连接上处理代理 CONNECT（如果需要）。
		//
		// 但实际上 Go 的实现是：DialTLSContext 完全接管连接建立，
		// Transport 不会再做代理 CONNECT。
		// 所以我们需要在 DialTLSContext 中处理代理。

		return dialUTLS(ctx, network, addr, resolvedProxy, utls.HelloChrome_Auto)
	}

	return transport
}

// dialUTLS 建立 uTLS 连接，可选通过代理
func dialUTLS(ctx context.Context, network, addr string, proxyURL *url.URL, clientHelloID utls.ClientHelloID) (net.Conn, error) {
	var rawConn net.Conn
	var err error

	if proxyURL != nil {
		rawConn, err = dialViaProxy(ctx, proxyURL, addr)
	} else {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		rawConn, err = dialer.DialContext(ctx, network, addr)
	}
	if err != nil {
		return nil, err
	}

	// 提取 hostname 用于 SNI
	host, _, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		host = addr
	}

	spec, err := utls.UTLSIdToSpec(clientHelloID)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS spec error: %w", err)
	}

	// 强制 ALPN 只使用 http/1.1，避免与 Go HTTP/2 transport 不兼容
	spec.Extensions = append([]utls.TLSExtension{}, spec.Extensions...)
	// 过滤掉 ALPN extension 并替换为只支持 http/1.1 的版本
	filtered := make([]utls.TLSExtension, 0, len(spec.Extensions))
	for _, ext := range spec.Extensions {
		if _, ok := ext.(*utls.ALPNExtension); !ok {
			filtered = append(filtered, ext)
		}
	}
	filtered = append(filtered, &utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}})
	spec.Extensions = filtered

	utlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
	}, utls.HelloCustom)

	if err := utlsConn.ApplyPreset(&spec); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS apply preset failed: %w", err)
	}

	if err := utlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS handshake failed: %w", err)
	}

	return utlsConn, nil
}

// dialViaProxy 通过 HTTP 代理的 CONNECT 隧道建立连接
func dialViaProxy(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	proxyConn, err := dialer.DialContext(ctx, "tcp", proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy %s: %w", proxyURL.Host, err)
	}

	// 构建 CONNECT 请求
	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}

	// 代理认证
	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		if username != "" {
			connectReq.SetBasicAuth(username, password)
		}
	}

	if err := connectReq.Write(proxyConn); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("write CONNECT to proxy: %w", err)
	}

	// 逐字节读取 CONNECT 响应，避免 bufio 缓冲 TLS 数据
	// CONNECT 响应格式: HTTP/1.1 200 Connection Established\r\n\r\n
	buf := make([]byte, 1)
	var respBuf []byte
	for i := 0; i < 4096; i++ {
		_, err := proxyConn.Read(buf)
		if err != nil {
			proxyConn.Close()
			return nil, fmt.Errorf("read CONNECT response: %w", err)
		}
		respBuf = append(respBuf, buf[0])
		// 检查是否读到响应结尾（\r\n\r\n）
		if len(respBuf) >= 4 && string(respBuf[len(respBuf)-4:]) == "\r\n\r\n" {
			break
		}
	}

	// 解析状态码
	respStr := string(respBuf)
	if len(respStr) < 12 {
		proxyConn.Close()
		return nil, fmt.Errorf("malformed CONNECT response: %s", respStr)
	}
	statusCode := respStr[9:12]
	if statusCode != "200" {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed: %s", respStr)
	}

	return proxyConn, nil
}
