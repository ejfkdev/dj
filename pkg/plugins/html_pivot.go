package plugins

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// HTMLPivotPlugin 发现更多 HTML 入口，形成多页/iframe 递归提取。
//
// 覆盖四个来源：
//  1. HTML 内容中的同源超链接/iframe（<a href>、<link href>、<iframe src>）——
//     覆盖 marko 这类多页路由站点（<a href=/page1>，server 目录自动落 index.html）
//  2. JS 内容中引号包裹的 .html 字面量——覆盖 storybook 等把预览页
//     （iframe.html）当字符串引用、运行时才拼接进 iframe src 的站点
//  3. JS 中表达式 import 的字面量前缀目录（import('./templates/'+name)）——
//     前缀目录本身可以被探测（静态服务器目录列表）枚举出真实 chunk 文件
//  4. 目录列表页（如 python http.server 的 Directory listing）——
//     解析 <a href="x.js"> 得到该目录下全部 JS 文件
//
// 提取出的入口作为 Intermediate 入队；主流程对 HTML Intermediate 有
// 全局预算（防无限爬取），IsSeenURL 天然防重复解析同一 URL。
type HTMLPivotPlugin struct {
	// JS 中引号包裹的 .html 字面量
	jsHtmlRe *regexp.Regexp
	// 表达式 import 前缀: import('./templates/' + name) / import(`./templates/${x}.js`)
	exprPrefixConcatRe *regexp.Regexp
	exprPrefixTplRe    *regexp.Regexp
	// 目录列表 href
	dirListingHrefRe *regexp.Regexp
}

// NewHTMLPivotPlugin 创建插件
func NewHTMLPivotPlugin() *HTMLPivotPlugin {
	return &HTMLPivotPlugin{
		jsHtmlRe:           regexp.MustCompile(`["']([^"'<>\\\s]{1,200}\.html?)["']`),
		exprPrefixConcatRe: regexp.MustCompile("import\\s*\\(\\s*[\"'`](\\.{0,2}[^\"'`]*?/)[\"'`]\\s*\\+"),
		exprPrefixTplRe:    regexp.MustCompile("import\\s*\\(\\s*`(\\.[^`]*?)\\${"),
		dirListingHrefRe:   regexp.MustCompile(`href=["']([^"']+)["']`),
	}
}

func (p *HTMLPivotPlugin) Name() string {
	return "HTMLPivotPlugin"
}

func (p *HTMLPivotPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	switch input.ContentType {
	case extractor.ContentTypeHTML:
		return bytesContainsAny(input.Content, [][]byte{[]byte("href="), []byte("iframe")})
	case extractor.ContentTypeJS:
		return bytesContainsAny(input.Content, [][]byte{[]byte(".html"), []byte("import(")})
	}
	return false
}

func (p *HTMLPivotPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	seen := make(map[string]bool)

	sourceParsed, sourceErr := url.Parse(input.SourceURL)
	sourceHost := ""
	if sourceErr == nil {
		sourceHost = sourceParsed.Host
	}

	addOne := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "javascript:") ||
			strings.HasPrefix(raw, "mailto:") || strings.Contains(raw, "${") {
			return
		}
		if strings.Contains(raw, "?") {
			raw = strings.SplitN(raw, "?", 2)[0]
		}
		if raw == "" {
			return
		}
		lower := strings.ToLower(raw)
		// 排除明显资源后缀；无扩展名/目录或 .html/.htm 才作为 HTML 入口
		for _, ext := range []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg",
			".ico", ".woff", ".woff2", ".ttf", ".json", ".xml", ".map", ".pdf"} {
			if strings.HasSuffix(lower, ext) {
				return
			}
		}
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, raw)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if !extractor.IsAbsoluteURL(absoluteURL) {
			return
		}
		// 只跟同源 HTML（避免爬出站外）
		if parsed, err := url.Parse(absoluteURL); err != nil || (sourceHost != "" && parsed.Host != sourceHost) {
			return
		}
		if seen[absoluteURL] {
			return
		}
		seen[absoluteURL] = true
		result.Intermediates = append(result.Intermediates, extractor.Intermediate{
			URL:  absoluteURL,
			Type: extractor.ContentTypeHTML,
		})
	}

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		// 目录型链接（无扩展名、无尾斜杠）：多页框架对路由的落盘方式不一，
		// 三种候选都试（失败的多余请求无害）：
		//   /page1                （服务端路由映射）
		//   /page1/index.html     （marko 新版多页目录结构）
		//   /page1.html           （marko 旧版平铺 html 文件）
		if !strings.Contains(strings.TrimSuffix(raw, "/"), ".") && !strings.HasSuffix(raw, "/") {
			addOne(raw)
			addOne(raw + "/index.html")
			addOne(raw + ".html")
			return
		}
		addOne(raw)
	}

	if input.ContentType == extractor.ContentTypeJS {
		content := extractor.DecodeContent(string(input.Content))
		for _, m := range p.jsHtmlRe.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				add(m[1])
			}
		}
		// 表达式 import 的字面量前缀目录：import('./templates/'+name) 或
		// import(`./templates/${x}.js`)。文件名运行时才拼出，静态提取拿不到；
		// 但前缀目录本身可以直接探测（静态服务器会返回目录列表），
		// 由目录列表解析分支枚举出真实 chunk 文件（snowpack 等无打包产物的站点）。
		prefixDirs := make(map[string]bool)
		addPrefixDir := func(prefix string) {
			prefix = strings.TrimSpace(prefix)
			if prefix == "" || strings.Contains(prefix, "${") {
				return
			}
			if prefixDirs[prefix] {
				return
			}
			prefixDirs[prefix] = true
			// 目录探测必须以尾斜杠入队：既让静态文件服务器返回目录列表，
			// 也让后续相对解析把该 URL 当作目录基准
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, prefix)
			absoluteURL = extractor.NormalizeURL(absoluteURL)
			if !extractor.IsAbsoluteURL(absoluteURL) {
				return
			}
			if !strings.HasSuffix(absoluteURL, "/") {
				absoluteURL += "/"
			}
			if parsed, err := url.Parse(absoluteURL); err != nil || (sourceHost != "" && parsed.Host != sourceHost) {
				return
			}
			result.Intermediates = append(result.Intermediates, extractor.Intermediate{
				URL:  absoluteURL,
				Type: extractor.ContentTypeHTML,
			})
		}
		for _, m := range p.exprPrefixConcatRe.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				addPrefixDir(m[1])
			}
		}
		for _, m := range p.exprPrefixTplRe.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				addPrefixDir(m[1])
			}
		}
		if len(result.Intermediates) >= maxMicroAppEntries {
			return result, nil
		}
		return result, nil
	}

	// HTML: 目录列表页（python http.server 等）→ 直接产出该目录下全部 JS
	if strings.Contains(string(input.Content), "Directory listing for") {
		// NormalizeURL 的 path.Clean 会吃掉尾斜杠，导致相对解析把目录当文件；
		// 这里强制补回尾斜杠作为解析基准
		dirBase := input.SourceURL
		if !strings.HasSuffix(dirBase, "/") {
			dirBase += "/"
		}
		for _, m := range p.dirListingHrefRe.FindAllStringSubmatch(string(input.Content), -1) {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			lower := strings.ToLower(name)
			if strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".mjs") {
				absoluteURL := extractor.ResolveRelativePath(dirBase, name)
				absoluteURL = extractor.NormalizeURL(absoluteURL)
				if extractor.IsAbsoluteURL(absoluteURL) {
					result.URLs = append(result.URLs, extractor.DiscoveredJS{
						URL:      absoluteURL,
						FromURL:  input.SourceURL,
						IsInline: false,
					})
				}
			}
		}
		return result, nil
	}

	// HTML: 解析 <a href> / <link href> / <iframe src>（支持无引号属性值）
	hrefRe := regexp.MustCompile(`(?:<a\b[^>]*\bhref\s*=\s*|(?:<link\b[^>]*\bhref\s*=\s*)|(?:<iframe\b[^>]*\bsrc\s*=\s*))(?:"([^"]+)"|'([^']+)'|([^"'\s>]+))`)
	for _, m := range hrefRe.FindAllStringSubmatch(string(input.Content), -1) {
		if len(m) > 1 {
			val := m[1]
			if val == "" {
				val = m[2]
			}
			if val == "" {
				val = m[3]
			}
			add(val)
		}
		if len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}
	return result, nil
}
