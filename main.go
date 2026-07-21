package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ejfkdev/dj/pkg/extractor"
	"github.com/ejfkdev/dj/pkg/fetcher"
	"github.com/ejfkdev/dj/pkg/plugins"
)

var version = "dev" // 版本号，通过 -ldflags "-X main.version=x.x.x" 设置

// parseFormat 输出格式字符串转换为内部常量
func parseFormat(s string) (extractor.OutputFormat, bool) {
	switch s {
	case "json":
		return extractor.FormatJSON, true
	case "md":
		return extractor.FormatMD, true
	case "text":
		return extractor.FormatText, true
	}
	return "", false
}

// printHelp 输出帮助信息
func printHelp() {
	fmt.Printf("dj - JS/SourceMap Extractor %s\n", version)
	fmt.Printf("Extract JS URLs and source maps from websites\n\n")
	fmt.Printf("Usage: dj [options] <url>\n\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -d, --debug              enable debug output\n")
	fmt.Printf("  -f, --format <fmt>       output format: text | json | md (default: text)\n")
	fmt.Printf("      --cache              enable cache (default: on)\n")
	fmt.Printf("      --cache=false        disable cache\n")
	fmt.Printf("      --useragent <UA>     custom User-Agent string (non-ASCII supported)\n")
	fmt.Printf("      --ua <UA>            short alias for --useragent\n")
	fmt.Printf("      --proxy <URL>        proxy URL: http://, https://, socks5://\n")
	fmt.Printf("      --cookie <cookies>   cookies for bypassing Cloudflare\n")
	fmt.Printf("  -H, --header <K: V>      custom HTTP header, repeatable (curl-style, non-ASCII supported)\n")
	fmt.Printf("  -h, --help               show this help\n\n")
	fmt.Printf("Notes:\n")
	fmt.Printf("  - URL is the first non-flag argument; flags can appear before or after it\n")
	fmt.Printf("  - Flag values can be passed as --flag=value or as the next argument\n")
	fmt.Printf("  - --header can be specified multiple times; later values override earlier ones\n")
	fmt.Printf("  - --header overrides default browser headers (e.g. User-Agent, Accept)\n\n")
	fmt.Printf("Examples:\n")
	fmt.Printf("  dj https://example.com\n")
	fmt.Printf("  dj -f md https://example.com\n")
	fmt.Printf("  dj --debug --cache=false https://example.com\n")
	fmt.Printf("  dj --useragent='Mozilla/5.0 ...' https://example.com\n")
	fmt.Printf("  dj --proxy=socks5://127.0.0.1:7890 https://example.com\n")
	fmt.Printf("  dj -f json --cookie 'cf_clearance=xxx; key=val' https://example.com\n")
	fmt.Printf("  dj -H 'Referer: https://google.com' -H 'X-Token: abc' https://example.com\n")
	fmt.Printf("  dj https://example.com -f md --debug\n\n")
	fmt.Printf("Cache path: %s\n", fetcher.GetTempDir())
}

func main() {
	// 解析参数
	var debug bool
	var enableCache = true // 默认开启缓存
	var showHelp bool
	var outputFormat = extractor.FormatText
	var userAgent string
	var proxy string
	var cookie string
	var url string
	var rawHeaders []string // 收集所有 -H/--header 值，最后一次性解析

	// 取下一段参数值（支持 --flag value 和 --flag=value 两种形式）
	nextValue := func(i *int) (string, bool) {
		if *i+1 >= len(os.Args) {
			return "", false
		}
		v := os.Args[*i+1]
		if strings.HasPrefix(v, "-") && v != "-" {
			return "", false
		}
		*i++
		return v, true
	}

	// 分割 --flag=value 形式
	splitEq := func(arg string) (string, string) {
		if idx := strings.Index(arg, "="); idx > 0 {
			return arg[:idx], arg[idx+1:]
		}
		return arg, ""
	}

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		name, val := splitEq(arg)

		switch {
		case name == "--debug" || name == "-d" || name == "-debug":
			debug = true
		case name == "--cache":
			// 显式 --cache=true/false 可在 val 中给值
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				}
			}
			switch val {
			case "", "true", "1", "yes", "on":
				enableCache = true
			case "false", "0", "no", "off":
				enableCache = false
			default:
				fmt.Fprintf(os.Stderr, "invalid --cache value: %q\n", val)
				os.Exit(1)
			}
		case name == "--format" || name == "-f":
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				} else {
					fmt.Fprintln(os.Stderr, "missing value for --format/-f")
					os.Exit(1)
				}
			}
			if f, ok := parseFormat(val); ok {
				outputFormat = f
			} else {
				fmt.Fprintf(os.Stderr, "invalid --format value: %q (expected: text|json|md)\n", val)
				os.Exit(1)
			}
		case name == "--useragent" || name == "--ua":
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				} else {
					fmt.Fprintln(os.Stderr, "missing value for --useragent")
					os.Exit(1)
				}
			}
			userAgent = val
		case name == "--proxy":
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				} else {
					fmt.Fprintln(os.Stderr, "missing value for --proxy")
					os.Exit(1)
				}
			}
			proxy = val
		case name == "--cookie":
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				} else {
					fmt.Fprintln(os.Stderr, "missing value for --cookie")
					os.Exit(1)
				}
			}
			cookie = val
		case name == "--header" || name == "-H":
			if val == "" {
				if v, ok := nextValue(&i); ok {
					val = v
				} else {
					fmt.Fprintln(os.Stderr, "missing value for --header/-H")
					os.Exit(1)
				}
			}
			if !strings.Contains(val, ":") {
				fmt.Fprintf(os.Stderr, "invalid --header value %q (expected \"Key: Value\" format)\n", val)
				os.Exit(1)
			}
			rawHeaders = append(rawHeaders, val)
		case name == "--help" || name == "-h":
			showHelp = true
		case strings.HasPrefix(name, "-"):
			// 未知 flag
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			os.Exit(1)
		default:
			// 第一个非 flag 位置参数视为 URL
			if url == "" {
				url = arg
			}
		}
	}

	if showHelp {
		printHelp()
		os.Exit(0)
	}

	if url == "" {
		printHelp()
		os.Exit(1)
	}

	// 初始化插件注册中心
	registry := extractor.NewPluginRegistry()

	// 注册内置插件
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
	// 通用 URL 兜底提取（编码还原后做宽匹配，捕获 document.write 等其他插件未覆盖的场景）
	registry.Register(plugins.NewUniversalURLPlugin())

	// 创建 Pipeline
	pipeline := extractor.NewPipeline(registry)
	pipeline.Debug = debug

	// 设置 Fetcher 配置（代理和 User-Agent）
	ua := userAgent
	if ua == "" {
		ua = fetcher.DefaultUserAgent
	}
	pipeline.SetFetcherConfig(proxy, ua)

	// 注入 cookie（用于绕过 Cloudflare 等防护）
	if cookie != "" {
		if err := pipeline.SetBrowserCookies(url, parseCookies(cookie)); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set cookies: %v\n", err)
		}
	}

	// 注入自定义 HTTP 请求头
	if len(rawHeaders) > 0 {
		headers, err := parseHeaders(rawHeaders)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse --header: %v\n", err)
			os.Exit(1)
		}
		pipeline.SetExtraHeaders(headers)
		if debug {
			fmt.Fprintf(os.Stderr, "Custom headers: %d\n", len(headers))
		}
	}

	// 设置缓存配置
	if enableCache {
		cacheConfig := &fetcher.CacheConfig{
			Enable:  true,
			BaseDir: fetcher.GetTempDir(),
		}
		pipeline.SetCacheConfig(cacheConfig)
	}

	// 执行
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// text 模式：实时输出；json/md 模式：收集后统一输出
	if outputFormat == extractor.FormatText {
		// 实时打印模式
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range pipeline.GetFoundCh() {
				fmt.Println(url)
			}
		}()

		_, err := pipeline.Run(ctx, url)
		wg.Wait()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Pipeline error: %v\n", err)
			os.Exit(1)
		}

		// 末尾追加汇总信息（JS 数、source map、源码还原、缓存目录）
		result := pipeline.GetOutputResult()
		fmt.Print(extractor.FormatTextSummary(result))
	} else {
		// json/md 模式：收集所有 URL 后统一输出
		_, err := pipeline.Run(ctx, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Pipeline error: %v\n", err)
			os.Exit(1)
		}

		// 输出格式化结果
		result := pipeline.GetOutputResult()
		fmt.Print(extractor.FormatOutput(outputFormat, result))
	}
}

// parseCookies 解析 cookie 字符串为 http.Cookie 切片
// 格式: "name1=value1; name2=value2"
func parseCookies(cookieStr string) []*http.Cookie {
	var cookies []*http.Cookie
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "="); idx > 0 {
			cookies = append(cookies, &http.Cookie{
				Name:  strings.TrimSpace(part[:idx]),
				Value: strings.TrimSpace(part[idx+1:]),
			})
		}
	}
	return cookies
}

// parseHeaders 解析 curl 风格的 header 列表为 map
// 格式: ["Key1: Value1", "Key2: Value2", ...]
// 同 key 后出现的值覆盖先前的值（与 net/http Header.Add/Set 行为一致）
func parseHeaders(rawList []string) (map[string]string, error) {
	headers := make(map[string]string, len(rawList))
	for _, raw := range rawList {
		idx := strings.Index(raw, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid header %q (expected \"Key: Value\" format)", raw)
		}
		key := strings.TrimSpace(raw[:idx])
		value := strings.TrimSpace(raw[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q (empty key)", raw)
		}
		headers[key] = value
	}
	return headers, nil
}
