package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
	"github.com/ejfkdev/dj/pkg/fetcher"

	xyz "github.com/ejfkdev/xyz-go"
	"github.com/ejfkdev/xyz-go/registry"
	"github.com/ejfkdev/xyz-go/spec"
)

var version = "dev" // 版本号，通过 -ldflags "-X main.version=x.x.x" 设置

func init() {
	// 未通过 ldflags 注入版本时，回退到模块构建信息：
	// go install github.com/ejfkdev/dj@v0.6.0 这类安装会带上标签版本，而不是 dev
	if version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
	}
}

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
	fmt.Printf("dj %s - Dynamic JS File Extractor\n", version)
	fmt.Printf("\n")
	fmt.Printf("Extract JS URLs and source maps from websites: discover dynamically\n")
	fmt.Printf("loaded JS (webpack chunks, import(), framework manifests...), restore\n")
	fmt.Printf("original sources from source maps, and reuse results from a local cache.\n")
	fmt.Printf("The scan is one definition exposed as CLI / HTTP API / MCP tool (xyz-go).\n")
	fmt.Printf("\n")
	fmt.Printf("GitHub:  https://github.com/ejfkdev/dj\n")
	fmt.Printf("License: MPL-2.0\n")
	fmt.Printf("\n")
	fmt.Printf("Usage:\n")
	fmt.Printf("  dj <url> [options]              scan a website (same as dj scan <url>)\n")
	fmt.Printf("  dj scan <url> [options]         canonical subcommand form\n")
	fmt.Printf("  dj serve [flags]                HTTP REST API (POST /scan, /openapi.json,\n")
	fmt.Printf("                                  /mcp, /healthz on one port)\n")
	fmt.Printf("  dj mcp stdio|sse|http [flags]   MCP tool server (scan exposed as a tool)\n")
	fmt.Printf("  dj completion <bash|zsh|fish>   shell completion script\n")
	fmt.Printf("  dj version                      print version and exit\n")
	fmt.Printf("\n")
	fmt.Printf("Scan options:\n")
	fmt.Printf("  -v, --version            print version and exit\n")
	fmt.Printf("  -d, --debug              enable debug output\n")
	fmt.Printf("  -f, --format <fmt>       output format: md (default) | json | text (bare URL list)\n")
	fmt.Printf("      --json               global flag: output raw JSON instead of rendered text\n")
	fmt.Printf("      --no-cache           disable cache reads (still saves to disk)\n")
	fmt.Printf("      --cache[=bool]       legacy compat: --cache / --cache=false / --cache=yes\n")
	fmt.Printf("      --useragent <UA>     custom User-Agent string (non-ASCII supported)\n")
	fmt.Printf("      --ua <UA>            short alias for --useragent\n")
	fmt.Printf("  -x, --proxy <URL>        proxy URL: http://, https://, socks5://\n")
	fmt.Printf("      --cookie <cookies>   cookies for bypassing Cloudflare\n")
	fmt.Printf("  -H, --header <K: V>      custom HTTP header, repeatable (curl-style, non-ASCII supported)\n")
	fmt.Printf("      --no-random-tls      disable randomized TLS fingerprint (use fixed Chrome)\n")
	fmt.Printf("  -o, --output <dir>       output directory (saves a copy without site subdir)\n")
	fmt.Printf("  -t, --timeout <secs>     per-request timeout in seconds (default: 30)\n")
	fmt.Printf("  -c, --concurrency <N>    max concurrent HTTP requests (default: 8)\n")
	fmt.Printf("  -h, --help               show this help\n")
	fmt.Printf("\n")
	fmt.Printf("xyz-go commands (serve/mcp):\n")
	fmt.Printf("  dj serve [flags]             HTTP REST API: POST /scan, /healthz, /openapi.json, /mcp\n")
	fmt.Printf("      --addr <host:port>       listen address (default: :8080)\n")
	fmt.Printf("      --bearer <token>         bearer token(s), comma-separated (default: none)\n")
	fmt.Printf("      --tls-cert <file>        TLS certificate file (use with --tls-key for HTTPS)\n")
	fmt.Printf("      --tls-key <file>         TLS key file\n")
	fmt.Printf("      --cors <origins>         allowed CORS origins, comma-separated\n")
	fmt.Printf("  dj mcp stdio|sse|http [flags]\n")
	fmt.Printf("                               MCP tool server; stdio needs no flags,\n")
	fmt.Printf("                               sse/http take --addr, http adds --stateless\n")
	fmt.Printf("                               (protocol 2026-07-28 without sessions)\n")
	fmt.Printf("\n")
	fmt.Printf("Notes:\n")
	fmt.Printf("  - URL is the first positional argument; flags can appear before or after it\n")
	fmt.Printf("  - Flag values can be passed as --flag=value or as the next argument\n")
	fmt.Printf("  - --header can be specified multiple times; later values override earlier ones\n")
	fmt.Printf("  - --header overrides default browser headers (e.g. User-Agent, Accept)\n")
	fmt.Printf("  - environment proxies are honored: HTTPS_PROXY, HTTP_PROXY, ALL_PROXY, NO_PROXY\n")
	fmt.Printf("  - -o saves a copy of all files to the output dir (js/, html/, source_map/, sources/)\n")
	fmt.Printf("    without the site subdirectory level; cache dir is still written normally\n")
	fmt.Printf("  - 'dj scan -h' shows the auto-generated per-flag help with defaults\n")
	fmt.Printf("  - exit codes: 0 success, 1 usage or runtime error\n")
	fmt.Printf("\n")
	fmt.Printf("Examples:\n")
	fmt.Printf("  dj https://example.com\n")
	fmt.Printf("  dj -f md https://example.com\n")
	fmt.Printf("  dj --debug --no-cache https://example.com\n")
	fmt.Printf("  dj --useragent='Mozilla/5.0 ...' https://example.com\n")
	fmt.Printf("  dj -x socks5://127.0.0.1:7890 https://example.com\n")
	fmt.Printf("  dj -f json --cookie 'cf_clearance=xxx; key=val' https://example.com\n")
	fmt.Printf("  dj -H 'Referer: https://google.com' -H 'X-Token: abc' https://example.com\n")
	fmt.Printf("  dj -o ./output -t 60 -c 16 https://example.com\n")
	fmt.Printf("  dj https://example.com -f text\n")
	fmt.Printf("  # HTTP API: start the server, then scan via curl\n")
	fmt.Printf("  dj serve --addr 127.0.0.1:8080\n")
	fmt.Printf("  curl -X POST http://127.0.0.1:8080/scan -H 'Content-Type: application/json' \\\n")
	fmt.Printf("       -d '{\"url\": \"https://example.com\", \"format\": \"json\"}'\n")
	fmt.Printf("  # MCP over streamable HTTP (protocol 2026-07-28, no sessions)\n")
	fmt.Printf("  dj mcp http --addr 127.0.0.1:8080 --stateless\n")
	fmt.Printf("  # Shell completion\n")
	fmt.Printf("  source <(dj completion zsh)\n")
	fmt.Printf("Cache path: %s\n", fetcher.GetTempDir())
}

func main() {
	reg := registry.New()
	if _, err := spec.Define("scan", scanHandler).
		Summary("Extract JS URLs and source maps from a website").
		Description("Crawl a URL, discover its JS files and source maps, and restore original sources when available. All input fields mirror the legacy dj flags (concurrency, timeout, proxy, headers, output dir, format...).").
		CLI(spec.CliHints{Usage: "<url>", Default: true}).
		HTTP(spec.HTTPHints{Method: "POST", Path: "/scan"}).
		MCP(spec.MCPHints{Annotations: []string{"read", "title:scan a website for JS and source maps"}}).
		Register(reg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	args, scanMode := buildArgs(os.Args[1:])
	code := xyz.RunConfig(reg, args, xyz.Config{})
	// 旧版 dj 的用法错误退出码是 1（xyz 的 invalid_input 是 2），扫描路径统一回 1
	if scanMode && code == 2 {
		code = 1
	}
	os.Exit(code)
}

// buildArgs 把进程参数归一化为 xyz 派发器能识别的形式，并保持旧版 dj 的参数习惯：
//   - 首参是 serve/mcp/completion 时原样交给 xyz（HTTP/MCP 模式）
//   - -h/--help/help 与无参数走旧版 printHelp（-h 退出 0，无参数退出 1）
//   - 其余情况前面补 "scan"，因此 `dj <flags> <url>` 与 `dj scan <flags> <url>` 等价
//   - 旧拼写兼容：--ua → --useragent，-debug → --debug，--cache 的布尔词归一化
//   - scan 参数里出现 -v/--version 立即打印版本退出（与旧版一致）
func buildArgs(args []string) ([]string, bool) {
	if len(args) == 0 {
		printHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "serve", "mcp", "completion":
		return args, false
	case "scan":
		return args, true
	case "version":
		fmt.Printf("dj %s\n", version)
		os.Exit(0)
	case "-h", "--help", "help":
		printHelp()
		os.Exit(0)
	}

	out := make([]string, 0, len(args)+1)
	// 首段以 - 开头（flag 在 URL 前）时显式补 "scan"；首段是 URL 等普通词时
	// 不补——交给 xyz-go 的默认子命令（CLI.Default）整串转发
	if strings.HasPrefix(args[0], "-") {
		out = append(out, "scan")
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		switch {
		case a == "-v" || a == "--version":
			fmt.Printf("dj %s\n", version)
			os.Exit(0)
		case a == "--ua":
			out = append(out, "--useragent")
		case strings.HasPrefix(a, "--ua="):
			out = append(out, "--useragent="+strings.TrimPrefix(a, "--ua="))
		case a == "-debug":
			out = append(out, "--debug")
		case a == "--cache":
			// 旧语义：裸 --cache 会吞掉下一个非 - 参数，是布尔词则按值处理，否则报错
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				switch cacheWordToFlag(args[i]) {
				case "--cache":
					out = append(out, "--cache")
				case "--cache=false":
					out = append(out, "--cache=false")
				default:
					fmt.Fprintf(os.Stderr, "invalid --cache value: %q\n", args[i])
					os.Exit(1)
				}
				continue
			}
			out = append(out, "--cache")
		case strings.HasPrefix(a, "--cache="):
			switch cacheWordToFlag(strings.TrimPrefix(a, "--cache=")) {
			case "--cache":
				out = append(out, "--cache")
			case "--cache=false":
				out = append(out, "--cache=false")
			default:
				fmt.Fprintf(os.Stderr, "invalid --cache value: %q\n", strings.TrimPrefix(a, "--cache="))
				os.Exit(1)
			}
		default:
			out = append(out, a)
		}
	}
	return out, true
}

// cacheWordToFlag 把旧版 --cache 的布尔词映射为 xyz bool flag 形态。
// 空串在旧版语义中等价于 true。
func cacheWordToFlag(word string) string {
	switch word {
	case "", "true", "1", "yes", "on":
		return "--cache"
	case "false", "0", "no", "off":
		return "--cache=false"
	}
	return "invalid"
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