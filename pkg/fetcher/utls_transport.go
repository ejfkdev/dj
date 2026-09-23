package fetcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

// utlsConnWrapper 包装 utls.UConn，暴露 tls.ConnectionState
// golang.org/x/net/http2.Transport 会检查 ConnectionState() 来验证 ALPN 协商
type utlsConnWrapper struct {
	*utls.UConn
}

func (w *utlsConnWrapper) ConnectionState() tls.ConnectionState {
	cs := w.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:                     cs.Version,
		HandshakeComplete:           cs.HandshakeComplete,
		DidResume:                   cs.DidResume,
		CipherSuite:                 cs.CipherSuite,
		NegotiatedProtocol:          cs.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  cs.NegotiatedProtocolIsMutual,
		ServerName:                  cs.ServerName,
		PeerCertificates:            cs.PeerCertificates,
		VerifiedChains:              cs.VerifiedChains,
		SignedCertificateTimestamps: cs.SignedCertificateTimestamps,
		OCSPResponse:                cs.OCSPResponse,
		TLSUnique:                   cs.TLSUnique,
	}
}

// BrowserProfile 一个「自洽」的浏览器画像：TLS 指纹与请求头（UA / Sec-CH-*）
// 同源。此前随机模式只随机 TLS 指纹、UA 恒为 Chrome，等于部分连接处于
// 「TLS 说 Safari、UA 说 Chrome」的自相矛盾状态而被 CDN/WAF 识别；
// 真实站点实测固定 Chrome 比随机多找回 10+ 个 JS 文件（svelte.dev 73→83）。
type BrowserProfile struct {
	Name          string
	HelloID       utls.ClientHelloID
	UserAgent     string
	SecChUA       string // 空 = 非 Chromium 内核，不发送 Sec-CH-* 系列头
	SecChPlatform string
}

var browserProfiles = []BrowserProfile{
	{
		Name:          "chrome",
		HelloID:       utls.HelloChrome_Auto,
		UserAgent:     DefaultUserAgent,
		SecChUA:       chromeSecChUA,
		SecChPlatform: `"Windows"`,
	},
	{
		Name:      "firefox",
		HelloID:   utls.HelloFirefox_Auto,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:142.0) Gecko/20100101 Firefox/142.0",
	},
	{
		Name:      "safari",
		HelloID:   utls.HelloSafari_Auto,
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15",
	},
	{
		Name:          "edge",
		HelloID:       utls.HelloEdge_Auto,
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36 Edg/148.0.0.0",
		SecChUA:       `"Microsoft Edge";v="148", "Chromium";v="148", "Not_A Brand";v="24"`,
		SecChPlatform: `"Windows"`,
	},
	{
		Name:      "ios",
		HelloID:   utls.HelloIOS_Auto,
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1",
	},
}

// ChromeProfile 固定 Chrome 画像（--no-random-tls、或用户自定义 UA 时使用）
var ChromeProfile = browserProfiles[0]

// pickProfile 按模式挑选一个自洽画像：随机模式每次请求随机选一个，
// Chrome 模式固定 Chrome。
func pickProfile(mode TLSFingerprintMode) BrowserProfile {
	if mode == TLSFingerprintChrome {
		return ChromeProfile
	}
	return browserProfiles[rand.Intn(len(browserProfiles))]
}

// profileCtxKey 用 context 把「本次请求选定的画像」传给传输层，
// 保证 TLS 指纹与请求头严格同源。
type profileCtxKey struct{}

func withProfile(ctx context.Context, p BrowserProfile) context.Context {
	return context.WithValue(ctx, profileCtxKey{}, p)
}

func profileFromContext(ctx context.Context) (BrowserProfile, bool) {
	p, ok := ctx.Value(profileCtxKey{}).(BrowserProfile)
	return p, ok
}

// proxyFromEnvironment 从环境变量解析代理 URL
func proxyFromEnvironment() *url.URL {
	for _, key := range []string{
		"HTTPS_PROXY", "https_proxy",
		"HTTP_PROXY", "http_proxy",
		"ALL_PROXY", "all_proxy",
	} {
		if v := os.Getenv(key); v != "" {
			if !strings.Contains(v, "://") {
				v = "http://" + v
			}
			if u, err := url.Parse(v); err == nil {
				return u
			}
		}
	}
	return nil
}

// getEnvAny 返回第一个非空的环境变量
func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// shouldBypassProxy 检查 host 是否匹配 NO_PROXY 列表
func shouldBypassProxy(host, noProxy string) bool {
	if noProxy == "*" {
		return true
	}
	for _, pattern := range strings.Split(noProxy, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == host {
			return true
		}
		if strings.HasPrefix(pattern, ".") && (strings.HasSuffix(host, pattern) || host == pattern[1:]) {
			return true
		}
	}
	return false
}

// resolveProxy 按请求解析代理配置，支持 NO_PROXY
func resolveProxy(proxyURL *url.URL, targetAddr string) *url.URL {
	if proxyURL != nil {
		return proxyURL
	}
	p := proxyFromEnvironment()
	if p == nil {
		return nil
	}
	// 环境变量代理检查 NO_PROXY
	noProxy := getEnvAny("NO_PROXY", "no_proxy")
	host, _, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
	}
	if shouldBypassProxy(host, noProxy) {
		return nil
	}
	return p
}

// newH2Transport 创建 HTTP/2 传输，使用 uTLS 指纹伪装
func newH2Transport(proxyURL *url.URL, fpMode TLSFingerprintMode) *http2.Transport {
	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			p := resolveProxy(proxyURL, addr)
			return dialUTLS(ctx, network, addr, p, []string{"h2", "http/1.1"}, fpMode)
		},
		AllowHTTP: false,
	}
}

// newH1Transport 创建 HTTP/1.1 传输，使用 uTLS 指纹伪装
func newH1Transport(proxyURL *url.URL, fpMode TLSFingerprintMode) *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		p := resolveProxy(proxyURL, addr)
		return dialUTLS(ctx, network, addr, p, []string{"http/1.1"}, fpMode)
	}

	return transport
}

// dialUTLS 建立 uTLS 连接，可选通过代理。
// fpMode 控制指纹模式：随机模式下每次连接从浏览器指纹池随机选取。
func dialUTLS(ctx context.Context, network, addr string, proxyURL *url.URL, alpnProtos []string, fpMode TLSFingerprintMode) (net.Conn, error) {
	var rawConn net.Conn
	var err error

	if proxyURL != nil {
		rawConn, err = dialProxy(ctx, proxyURL, addr)
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

	// 优先使用请求 context 里选定的画像（与请求头严格同源）；
	// 缺省回退为按模式随机/固定选择。
	helloID := pickProfile(fpMode).HelloID
	if prof, ok := profileFromContext(ctx); ok {
		helloID = prof.HelloID
	}
	spec, err := utls.UTLSIdToSpec(helloID)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS spec error: %w", err)
	}

	// 替换 ALPN 为指定协议列表
	spec.Extensions = append([]utls.TLSExtension{}, spec.Extensions...)
	filtered := make([]utls.TLSExtension, 0, len(spec.Extensions))
	for _, ext := range spec.Extensions {
		if _, ok := ext.(*utls.ALPNExtension); !ok {
			filtered = append(filtered, ext)
		}
	}
	filtered = append(filtered, &utls.ALPNExtension{AlpnProtocols: alpnProtos})
	spec.Extensions = filtered

	utlsConn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
	}, utls.HelloCustom)

	if err := utlsConn.ApplyPreset(&spec); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS apply preset failed: %w", err)
	}

	// 握手单独限时：TCP 拨号已有 30s 超时，但握手此前只受整请求超时约束，
	// 卡住的握手会把 http2 传输层的连接闸门一起拖住（表现为整次扫描
	// 6s/100s 双峰）。10s 上限足够真实站点握手，病态情况快速失败。
	hsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := utlsConn.HandshakeContext(hsCtx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS handshake failed: %w", err)
	}

	return &utlsConnWrapper{utlsConn}, nil
}

// dialProxy 根据代理 URL scheme 选择拨号方式
func dialProxy(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	switch proxyURL.Scheme {
	case "socks5", "socks5h":
		return dialViaSOCKS5(ctx, proxyURL, targetAddr)
	case "https":
		return dialViaHTTPSProxy(ctx, proxyURL, targetAddr)
	default:
		return dialViaHTTPProxy(ctx, proxyURL, targetAddr)
	}
}

// dialViaSOCKS5 通过 SOCKS5 代理建立连接
func dialViaSOCKS5(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		auth = &proxy.Auth{
			User: proxyURL.User.Username(),
		}
		if p, ok := proxyURL.User.Password(); ok {
			auth.Password = p
		}
	}

	forward := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, forward)
	if err != nil {
		return nil, fmt.Errorf("create SOCKS5 dialer for %s: %w", proxyURL.Host, err)
	}

	var conn net.Conn
	if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
		conn, err = ctxDialer.DialContext(ctx, "tcp", targetAddr)
	} else {
		conn, err = dialer.Dial("tcp", targetAddr)
	}
	if err != nil {
		return nil, fmt.Errorf("socks5 connect %s via %s: %w", targetAddr, proxyURL.Host, err)
	}

	return conn, nil
}

// dialViaHTTPProxy 通过 HTTP CONNECT 隧道建立连接
func dialViaHTTPProxy(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	proxyConn, err := dialer.DialContext(ctx, "tcp", proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy %s: %w", proxyURL.Host, err)
	}

	if err := sendConnectRequest(proxyConn, ctx, proxyURL, targetAddr); err != nil {
		proxyConn.Close()
		return nil, err
	}

	return proxyConn, nil
}

// dialViaHTTPSProxy 通过 HTTPS 代理的 CONNECT 隧道建立连接（TLS 到代理 + CONNECT）
func dialViaHTTPSProxy(ctx context.Context, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
	}

	rawConn, err := dialer.DialContext(ctx, "tcp", proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy %s: %w", proxyURL.Host, err)
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: proxyURL.Hostname(),
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("TLS handshake with proxy %s: %w", proxyURL.Host, err)
	}

	if err := sendConnectRequest(tlsConn, ctx, proxyURL, targetAddr); err != nil {
		tlsConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// sendConnectRequest 发送 HTTP CONNECT 请求并读取响应
func sendConnectRequest(conn net.Conn, ctx context.Context, proxyURL *url.URL, targetAddr string) error {
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetReadDeadline(deadline)
	} else {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	}

	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}

	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		if username != "" {
			connectReq.SetBasicAuth(username, password)
		}
	}

	if err := connectReq.Write(conn); err != nil {
		return fmt.Errorf("write CONNECT to proxy: %w", err)
	}

	// 逐字节读取 CONNECT 响应，避免 bufio 缓冲 TLS 数据
	buf := make([]byte, 1)
	var respBuf []byte
	for range 4096 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("read CONNECT response: %w", err)
		}
		respBuf = append(respBuf, buf[0])
		if len(respBuf) >= 4 && string(respBuf[len(respBuf)-4:]) == "\r\n\r\n" {
			break
		}
	}

	conn.SetReadDeadline(time.Time{})

	respStr := string(respBuf)
	if len(respStr) < 12 {
		return fmt.Errorf("malformed CONNECT response: %s", respStr)
	}
	statusCode := respStr[9:12]
	if statusCode != "200" {
		return fmt.Errorf("proxy CONNECT failed: %s", respStr)
	}

	return nil
}
