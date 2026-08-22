package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ejfkdev/dj/pkg/extractor"
	"github.com/ejfkdev/dj/pkg/fetcher"
	"github.com/ejfkdev/dj/pkg/plugins"

	errs "github.com/ejfkdev/xyz-go/errors"
)

// ScanInput 是 scan 命令的参数结构：json tag 即三个前端（CLI flag / HTTP
// 请求参数 / MCP 工具入参）共享的线上名，desc 用于帮助文本与 schema 描述。
// CLI flag 名、简写与默认值全部对齐旧版 dj 命令行。
type ScanInput struct {
	URL         string   `json:"url,omitempty" desc:"目标网站 URL" required:"true" cli:"positional"`
	Concurrency int      `json:"concurrency" desc:"最大并发 HTTP 请求数（下载/探测/HEAD 共享同一预算）" default:"8" validate:"min=1,max=256" cli:"shorthand=c"`
	Timeout     int      `json:"timeout" desc:"单个 HTTP 请求超时（秒）" default:"30" validate:"gt=0" cli:"shorthand=t"`
	Proxy       string   `json:"proxy" desc:"代理 URL：http://、https://、socks5://" cli:"shorthand=x"`
	UserAgent   string   `json:"useragent" desc:"自定义 User-Agent（兼容旧别名 --ua）"`
	Format      string   `json:"format" desc:"输出格式：md（默认）| json | text（裸 URL 列表）" default:"md" enum:"md,json,text" cli:"shorthand=f"`
	Cache       bool     `json:"cache" desc:"读取磁盘缓存（--no-cache 只禁读，仍会写盘）" default:"true"`
	NoCache     bool     `json:"no-cache" desc:"禁止读取缓存（仍然保存下载产物）"`
	NoRandomTLS bool     `json:"no-random-tls" desc:"关闭 TLS 指纹随机化（固定使用 Chrome 指纹）"`
	Cookie      string   `json:"cookie" desc:"Cookie 串，用于绕过 Cloudflare 等防护（a=b; c=d）"`
	Output      string   `json:"output" desc:"把产物额外写一份到该目录（无站点子目录层级）" cli:"shorthand=o"`
	Headers     []string `json:"headers" desc:"自定义请求头 \"K: V\"，可重复，后值覆盖先值" cli:"shorthand=H"`
	Debug       bool     `json:"debug" desc:"输出调试日志" cli:"shorthand=d"`
}

// Report 是 scan 的返回值：底层 string 存放按 -f 格式渲染好的最终文本，
// xyz 的 CLI 渲染器对 string 原样输出，与旧版 dj 的输出逐字节一致。
// 实现 MarshalJSON 后，-f json 的结果在 HTTP 响应 / MCP structuredContent /
// --json 下是结构化 OutputResult JSON（而非被二次引号包裹的字符串）。
type Report string

// MarshalJSON 透传已经合法的 JSON 文本（-f json），其余格式按字符串编码。
func (r Report) MarshalJSON() ([]byte, error) {
	s := string(r)
	if strings.HasPrefix(s, "{") && json.Valid([]byte(s)) {
		return []byte(s), nil
	}
	return json.Marshal(s)
}

// buildRegistry 注册全部内置分析插件。
func buildRegistry() *extractor.PluginRegistry {
	registry := extractor.NewPluginRegistry()

	registry.Register(plugins.NewHTMLScriptPlugin())
	registry.Register(plugins.NewDynamicImportPlugin())
	registry.Register(plugins.NewWebpackPlugin())
	registry.Register(plugins.NewNextJSPlugin())
	registry.Register(plugins.NewNuxtJSPlugin())
	registry.Register(plugins.NewVitePlugin())
	registry.Register(plugins.NewSvelteKitPlugin())
	registry.Register(plugins.NewRequireJSPlugin())
	registry.Register(plugins.NewModuleFederationPlugin())
	registry.Register(plugins.NewModuleFederationManifestPlugin())
	registry.Register(plugins.NewHelMicroPlugin())
	registry.Register(plugins.NewESMImportPlugin())
	registry.Register(plugins.NewScriptCreatePlugin())
	registry.Register(plugins.NewModernJSPlugin())
	registry.Register(plugins.NewURLPatternPlugin())
	registry.Register(plugins.NewSourceMapPlugin())
	registry.Register(plugins.NewUmiJSPlugin())
	// 微前端子应用入口（entry/proEntry/url 指向子应用 HTML 目录，按框架拆分）
	registry.Register(plugins.NewQiankunPlugin())
	registry.Register(plugins.NewGarfishPlugin())
	registry.Register(plugins.NewMicroAppPlugin())
	registry.Register(plugins.NewWujiePlugin())
	registry.Register(plugins.NewIcestarkPlugin())
	// Trunk（Rust wasm 打包器）sitemap.json 清单
	registry.Register(plugins.NewTrunkPlugin())
	// 多页/iframe HTML 入口递归提取（同源链接 + .html 字面量 + 目录列表）
	registry.Register(plugins.NewHTMLPivotPlugin())
	// EMP（@efox/emp）emp.json 联邦清单探测与解析
	registry.Register(plugins.NewEmpPlugin())
	// 通用 URL 兜底提取（编码还原后做宽匹配，捕获 document.write 等其他插件未覆盖的场景）
	registry.Register(plugins.NewUniversalURLPlugin())

	return registry
}

// scanHandler 执行一次扫描：参数校验、组装 Pipeline、运行并输出格式化文本。
// handler 通过 xyz 派发器拿到 ctx（CLI 下随 SIGINT/SIGTERM 取消，HTTP 下随
// 客户端断连取消），语义与旧版 main 直连 context.Background() 兼容。
func scanHandler(ctx context.Context, in *ScanInput) (Report, error) {
	format, ok := parseFormat(in.Format)
	if !ok {
		return "", errs.Errorf(errs.KindInvalidInput,
			"invalid --format value: %q (expected: text|json|md)", in.Format)
	}
	for _, h := range in.Headers {
		if !strings.Contains(h, ":") {
			return "", errs.Errorf(errs.KindInvalidInput,
				"invalid --header value %q (expected \"Key: Value\" format)", h)
		}
	}
	// URL 形状前置校验：阻止无协议的输入深入管线（旧版会 panic），
	// 同时保护 HTTP/MCP 服务进程不被异常输入打崩
	if !extractor.IsAbsoluteURL(in.URL) {
		return "", errs.Errorf(errs.KindInvalidInput,
			"invalid URL %q (expect http:// or https://)", in.URL)
	}

	pipeline := extractor.NewPipeline(buildRegistry())
	pipeline.Debug = in.Debug

	ua := in.UserAgent
	if ua == "" {
		ua = fetcher.DefaultUserAgent
	}
	fpMode := fetcher.TLSFingerprintRandom
	if in.NoRandomTLS {
		fpMode = fetcher.TLSFingerprintChrome
	}
	pipeline.SetFetcherConfig(in.Proxy, ua, fpMode, time.Duration(in.Timeout)*time.Second)
	pipeline.SetFetchConcurrency(in.Concurrency)

	// 注入 cookie（用于绕过 Cloudflare 等防护），失败不致命
	if in.Cookie != "" {
		if err := pipeline.SetBrowserCookies(in.URL, parseCookies(in.Cookie)); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set cookies: %v\n", err)
		}
	}

	// 注入自定义 HTTP 请求头，同 key 后值覆盖先值
	if len(in.Headers) > 0 {
		headers, err := parseHeaders(in.Headers)
		if err != nil {
			return "", errs.Wrap(errs.KindInvalidInput, err)
		}
		pipeline.SetExtraHeaders(headers)
		if in.Debug {
			fmt.Fprintf(os.Stderr, "Custom headers: %d\n", len(headers))
		}
	}

	// --cache=false 时 Enable=false 但 WriteEnabled=true：每次都走网络下载
	// （不读缓存），但下载的 JS / source map / 还原源码仍然保存到磁盘。
	enableCache := in.Cache && !in.NoCache
	pipeline.SetCacheConfig(&fetcher.CacheConfig{
		Enable:       enableCache,
		WriteEnabled: !enableCache,
		BaseDir:      fetcher.GetTempDir(),
		OutputDir:    in.Output,
	})

	// text 模式：收集一行一个 URL（旧版为边跑边打，此处运行结束后统一输出）
	if format == extractor.FormatText {
		var wg sync.WaitGroup
		var lines []string
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range pipeline.GetFoundCh() {
				lines = append(lines, u)
			}
		}()
		_, err := pipeline.Run(ctx, in.URL)
		wg.Wait()
		if err != nil {
			return "", errs.Wrap(errs.KindInternal, err)
		}
		return Report(strings.Join(lines, "\n")), nil
	}

	if _, err := pipeline.Run(ctx, in.URL); err != nil {
		return "", errs.Wrap(errs.KindInternal, err)
	}
	return Report(extractor.FormatOutput(format, pipeline.GetOutputResult())), nil
}