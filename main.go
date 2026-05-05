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
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--debug" || arg == "-d" || arg == "-debug" {
			debug = true
		} else if strings.HasPrefix(arg, "--cache") {
			// 支持 --cache 和 --cache=false
			if strings.HasSuffix(arg, "=false") {
				enableCache = false
			} else if arg == "--cache" {
				enableCache = true
			}
		} else if strings.HasPrefix(arg, "--format=") {
			format := strings.TrimPrefix(arg, "--format=")
			switch format {
			case "json":
				outputFormat = extractor.FormatJSON
			case "md":
				outputFormat = extractor.FormatMD
			case "text":
				outputFormat = extractor.FormatText
			}
		} else if strings.HasPrefix(arg, "-f") {
			// -f json / -f md / -f text
			format := strings.TrimPrefix(arg, "-f")
			if format == "" && i+1 < len(os.Args) {
				i++
				format = os.Args[i]
			}
			switch format {
			case "json":
				outputFormat = extractor.FormatJSON
			case "md":
				outputFormat = extractor.FormatMD
			case "text":
				outputFormat = extractor.FormatText
			}
		} else if strings.HasPrefix(arg, "--useragent=") {
			userAgent = strings.TrimPrefix(arg, "--useragent=")
		} else if strings.HasPrefix(arg, "--proxy=") {
			proxy = strings.TrimPrefix(arg, "--proxy=")
		} else if strings.HasPrefix(arg, "--cookie=") {
			cookie = strings.TrimPrefix(arg, "--cookie=")
		} else if arg == "--help" || arg == "-h" {
			showHelp = true
		} else if !strings.HasPrefix(arg, "-") {
			url = arg
		}
	}

	if showHelp {
		fmt.Printf("dj - JS/SourceMap Extractor %s\n", version)
		fmt.Printf("Extract JS URLs and source maps from websites\n\n")
		fmt.Printf("Usage: dj [--debug] [--cache[=false]] [-f format] [--useragent=<UA>] [--proxy=<proxy>] [--cookie=<cookies>] <url>\n")
		fmt.Printf("  --debug: enable debug output\n")
		fmt.Printf("  -f: output format (text, json, md), default: text\n")
		fmt.Printf("      --cache is enabled by default, use --cache=false to disable\n")
		fmt.Printf("  --useragent: custom User-Agent string for HTTP requests\n")
		fmt.Printf("  --proxy: proxy URL (http/https/socks5, e.g., http://127.0.0.1:7890, socks5://127.0.0.1:1080)\n")
		fmt.Printf("  --cookie: cookies for bypassing Cloudflare (e.g., \"cf_clearance=xxx; key=val\")\n")
		fmt.Printf("Cache path: %s\n", fetcher.GetTempDir())
		os.Exit(0)
	}

	if url == "" {
		fmt.Printf("dj - JS/SourceMap Extractor %s\n", version)
		fmt.Printf("Extract JS URLs and source maps from websites\n\n")
		fmt.Printf("Usage: dj [--debug] [--cache[=false]] [-f format] [--useragent=<UA>] [--proxy=<proxy>] [--cookie=<cookies>] <url>\n")
		fmt.Printf("  --debug: enable debug output\n")
		fmt.Printf("  -f: output format (text, json, md), default: text\n")
		fmt.Printf("      --cache is enabled by default, use --cache=false to disable\n")
		fmt.Printf("  --useragent: custom User-Agent string for HTTP requests\n")
		fmt.Printf("  --proxy: proxy URL (http/https/socks5, e.g., http://127.0.0.1:7890, socks5://127.0.0.1:1080)\n")
		fmt.Printf("  --cookie: cookies for bypassing Cloudflare (e.g., \"cf_clearance=xxx; key=val\")\n")
		fmt.Printf("Cache path: %s\n", fetcher.GetTempDir())
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
