package plugins

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// QiankunPlugin 提取 qiankun/single-spa 微前端子应用入口（HTML 目录）。
//
// 主站 SPA 包内嵌微应用注册配置，形如：
//
//	{name:"business", devEntry:`//ip:5010/`, proEntry:"/aio/app/business/", container:"#micro-container"}
//	{name:"app", entry:"/app/", container:"#container", activeRule:"/app"}
//
// entry/proEntry 指向子应用 HTML 入口（目录或 index.html 文件）。
// 与普通 JS URL 不同，这些入口本身不是 JS：
//  1. 入队后由主流程下载（服务器返回 HTML）
//  2. HTMLScriptPlugin 提取其中的 <script src>、modulepreload、prefetch
//  3. 内联 script 中的 import('/xxx.js') 由 DynamicImportPlugin 继续提取
//
// 覆盖 single-spa 生态：qiankun（含 vite-plugin-qiankun 的 proEntry）、
// garfish、icestark 等 entry-HTML 式微前端框架。
// 纯 single-spa 应用直接 write import('/path/app.js') 的形式已由
// DynamicImportPlugin 覆盖，本插件不重复处理。
type QiankunPlugin struct {
	// entry/proEntry 键（可选带引号的 minified key），值为字符串
	entryRe *regexp.Regexp
}

// NewQiankunPlugin 创建插件
func NewQiankunPlugin() *QiankunPlugin {
	return &QiankunPlugin{
		entryRe: regexp.MustCompile(`(?:proEntry|entry)["']?\s*:\s*["']([^"']+)["']`),
	}
}

func (p *QiankunPlugin) Name() string {
	return "QiankunPlugin"
}

func (p *QiankunPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	// 快速过滤：必须出现微应用注册关键字
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("proEntry"),
		[]byte("entry"),
	})
}

func (p *QiankunPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 编码还原（minified 包中常见 \/ 转义）
	content := extractor.DecodeContent(string(input.Content))
	if len(content) == 0 {
		return result, nil
	}

	seen := make(map[string]bool)
	for _, m := range p.entryRe.FindAllStringSubmatchIndex(content, -1) {
		if len(m) < 4 {
			continue
		}
		valueStr := content[m[2]:m[3]]

		// 只接受指向 HTML 入口的值：目录（以 / 结尾）或 index.html
		if !looksLikeMicroAppEntry(valueStr) {
			continue
		}

		// 上下文校验：附近必须有微应用注册的伴生字段
		// （container/activeRule/activeWhen/sandbox/singular），
		// 避免把普通 JS 对象里的 entry:"/xxx/" 误判为微应用入口
		if !hasMicroAppContext(content, m[0], m[1]) {
			continue
		}

		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, valueStr)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if !extractor.IsAbsoluteURL(absoluteURL) {
			continue
		}

		if seen[absoluteURL] {
			continue
		}
		seen[absoluteURL] = true

		result.Intermediates = append(result.Intermediates, extractor.Intermediate{
			URL:  absoluteURL,
			Type: extractor.ContentTypeHTML,
		})

		if len(result.Intermediates) >= maxEntries {
			break
		}
	}

	return result, nil
}

// looksLikeMicroAppEntry 判断 entry 值是否指向子应用 HTML 入口。
// 接受：
//   - 绝对路径目录: /aio/app/business/
//   - http(s) 完整 URL 目录: https://cdn.x.com/app/
//   - 显式 HTML 文件: /app/index.html（部分框架支持）
//
// 排除: dev 地址（//ip:port 协议相对形式）、模板字符串、
// 相对路径（从 bundle 内无法可靠解析目录入口）。
func looksLikeMicroAppEntry(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	// devEntry 常为 //127.0.0.1:5010/ 的协议相对地址
	if strings.HasPrefix(v, "//") {
		return false
	}
	// 只接受绝对路径或完整 URL（相对入口从 bundle 内解析不可靠）
	if !strings.HasPrefix(v, "/") &&
		!strings.HasPrefix(v, "http://") &&
		!strings.HasPrefix(v, "https://") {
		return false
	}
	// 模板变量
	if strings.Contains(v, "${") {
		return false
	}
	lower := strings.ToLower(v)
	if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
		return true
	}
	// 目录形式：以 / 结尾
	if !strings.HasSuffix(v, "/") {
		return false
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		if _, err := url.Parse(v); err != nil {
			return false
		}
	}
	return true
}

// hasMicroAppContext 在匹配位置前后窗口内查找微应用伴生字段，
// 降低误报率。
func hasMicroAppContext(content string, start, end int) bool {
	const window = 400
	lo := start - window
	if lo < 0 {
		lo = 0
	}
	hi := end + window
	if hi > len(content) {
		hi = len(content)
	}
	windowText := content[lo:hi]
	for _, kw := range []string{
		"container", "activeRule", "activeWhen", "sandbox", "singular",
	} {
		if strings.Contains(windowText, kw) {
			return true
		}
	}
	return false
}

// maxEntries 单文件提取上限（防爆炸）
const maxEntries = 50
