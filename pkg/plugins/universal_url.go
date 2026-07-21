package plugins

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// UniversalURLPlugin 通用 JS URL 提取插件（兜底）
//
// 设计目的：先用 extractor.DecodeContent 对内容做编码还原
// (JS 转义/URL 编码/Unicode/HTML 实体)，再用宽松正则匹配任何看起来像 JS URL 的字符串。
// 用于覆盖现有插件未识别的场景（如 document.write 字符串中的 <script src>、
// 编码/转义隐藏的 URL、罕见自定义加载逻辑等）。
//
// 注意：与现有插件并行运行，重复入队由 pipeline 的 knowledge.IsSeenURL 去重，
// 不会重复下载。
type UniversalURLPlugin struct {
	// 1. <script src="..."> 任意引号、任意属性顺序
	scriptSrcRe *regexp.Regexp
	// 2. import("...")
	importRe *regexp.Regexp
	// 3. require("...")
	requireRe *regexp.Regexp
	// 4. loader!path 或 !path 形式 (webpack require.context 等)
	loaderRe *regexp.Regexp
	// 5. src="..." / src='...' 直接赋值
	directSrcRe *regexp.Regexp
	// 6. 通用宽松匹配: "..." 或 '...' 中含 .js 扩展名的字符串
	//    注意：仅作为兜底，前面规则都未命中时才用
	bareJSRe *regexp.Regexp
	// 7. 协议相对 URL 模式
	protocolRelativeRe *regexp.Regexp
}

// NewUniversalURLPlugin 创建插件
func NewUniversalURLPlugin() *UniversalURLPlugin {
	return &UniversalURLPlugin{
		// <script src="..."> 或 <script type=... src="..."> 等
		// 兼容单引号/双引号/无引号（无引号在 HTML 里少见，先不处理）
		scriptSrcRe: regexp.MustCompile(`<script\b[^>]*\bsrc\s*=\s*["']([^"']+\.js[^"']*)["']`),
		// import("path") 动态 import
		importRe: regexp.MustCompile(`\bimport\s*\(\s*["']([^"']+\.js)["']`),
		// require("path") CommonJS
		requireRe: regexp.MustCompile(`\brequire\s*\(\s*["']([^"']+\.js)["']`),
		// loader!path 或 !path 形式 (webpack 等)
		loaderRe: regexp.MustCompile(`["']([^"']*!/[^"']+\.js)["']`),
		// src="..." 直接赋值
		directSrcRe: regexp.MustCompile(`\bsrc\s*=\s*["']([^"']+\.js)["']`),
		// 宽松兜底: 任意引号字符串中含 .js
		bareJSRe: regexp.MustCompile(`["']([^"'\\\s]*?\.js(?:\?[^"'\\]*?)?)["']`),
		// 协议相对 URL
		protocolRelativeRe: regexp.MustCompile(`["'](//[a-zA-Z0-9.\-]+/[^"']+\.js[^"']*)["']`),
	}
}

// Name 插件名
func (p *UniversalURLPlugin) Name() string {
	return "UniversalURLPlugin"
}

// Precheck 快速判断：内容可能含 JS URL
func (p *UniversalURLPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS && input.ContentType != extractor.ContentTypeHTML {
		return false
	}
	content := string(input.Content)
	// 快速过滤：至少含 .js 或 document.write 或 import/require
	return strings.Contains(content, ".js") ||
		strings.Contains(content, "document.write") ||
		strings.Contains(content, "import(") ||
		strings.Contains(content, "require(") ||
		strings.Contains(content, "<script")
}

// Analyze 提取所有匹配到的 JS URL（去重后写入 result.URLs）
func (p *UniversalURLPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 关键步骤：对内容做编码还原
	// 这样 document.write 里的 https:\/\/foo.com\/bar.js 会被还原成 https://foo.com/bar.js
	// %3Cscript src=...%3E 会被还原成 <script src=...>
	// \u003Cscript 会被还原成 <script
	decoded := extractor.DecodeContent(string(input.Content))

	seen := make(map[string]bool)
	add := func(rawURL string) {
		// 协议相对 URL 补全 scheme
		if strings.HasPrefix(rawURL, "//") {
			if sourceURLParsed, err := url.Parse(input.SourceURL); err == nil && sourceURLParsed.Scheme != "" {
				rawURL = sourceURLParsed.Scheme + ":" + rawURL
			}
		}
		// 清理尾部标点
		rawURL = cleanURLTrailing(rawURL)
		// 只保留 .js URL（避免误把 .css/.png 误判）
		if !looksLikeJSURL(rawURL) {
			return
		}
		// 解析为绝对 URL
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, rawURL)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if !extractor.IsAbsoluteURL(absoluteURL) {
			return
		}
		// 去重（同一 content 内）
		if seen[absoluteURL] {
			return
		}
		seen[absoluteURL] = true
		result.URLs = append(result.URLs, extractor.DiscoveredJS{
			URL:      absoluteURL,
			FromURL:  input.SourceURL,
			IsInline: false,
		})
	}

	// 1. <script src="..."> （含解码后出现的）
	for _, m := range p.scriptSrcRe.FindAllStringSubmatch(decoded, -1) {
		add(m[1])
	}
	// 2. import("...")
	for _, m := range p.importRe.FindAllStringSubmatch(decoded, -1) {
		add(m[1])
	}
	// 3. require("...")
	for _, m := range p.requireRe.FindAllStringSubmatch(decoded, -1) {
		add(m[1])
	}
	// 4. src="..." 直接赋值
	for _, m := range p.directSrcRe.FindAllStringSubmatch(decoded, -1) {
		add(m[1])
	}
	// 5. 协议相对 URL（单独处理，避免与其他正则重复）
	for _, m := range p.protocolRelativeRe.FindAllStringSubmatch(decoded, -1) {
		add(m[1])
	}
	// 6. loader!path（webpack 等）
	for _, m := range p.loaderRe.FindAllStringSubmatch(decoded, -1) {
		// 提取 ! 后面的 path
		if idx := strings.LastIndex(m[1], "!"); idx >= 0 {
			add(m[1][idx+1:])
		}
	}
	// 7. 兜底: 宽松匹配所有 .js 字符串（仅在前面规则未命中的情况下使用）
	//    限制：URL 长度 < 500 字符，避免误匹配 CSS/注释中的长字符串
	if len(seen) == 0 {
		for _, m := range p.bareJSRe.FindAllStringSubmatch(decoded, -1) {
			if len(m[1]) < 500 {
				add(m[1])
			}
		}
	}

	return result, nil
}

// cleanURLTrailing 清理 URL 尾部可能的杂质字符（来自 CSS 块等场景的右括号/逗号等）
func cleanURLTrailing(rawURL string) string {
	// 去掉尾部常见标点
	for len(rawURL) > 0 {
		last := rawURL[len(rawURL)-1]
		if last == ')' || last == ',' || last == ';' || last == '"' || last == '\'' || last == '>' || last == ' ' {
			rawURL = rawURL[:len(rawURL)-1]
			continue
		}
		break
	}
	return rawURL
}

// looksLikeJSURL 判断 URL 看起来像 JS 文件
// 排除: .css .png .jpg .gif .svg .ico .json .xml .txt .html .htm .map 等
func looksLikeJSURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	// 必须含 .js（可能在 query 之前或之后）
	// 排除常见的非 JS 扩展名
	excludeExts := []string{".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
		".json", ".xml", ".txt", ".pdf", ".zip", ".rar", ".7z",
		".html", ".htm", ".map", ".woff", ".ttf", ".otf", ".eot"}
	for _, ext := range excludeExts {
		// 检查 .css 等是否在 .js 之前出现
		jsIdx := strings.Index(lower, ".js")
		if jsIdx < 0 {
			return false
		}
		extIdx := strings.Index(lower, ext)
		if extIdx >= 0 && extIdx < jsIdx {
			return false
		}
	}
	// 必须含 .js（允许 .js?... 这种带 query 的形式）
	if !strings.Contains(lower, ".js") {
		return false
	}
	// 排除以 .map 结尾的（已被 SourceMapPlugin 处理）
	if strings.HasSuffix(lower, ".map") {
		return false
	}
	// 排除 data: / blob: / about: 等伪协议
	if strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") || strings.HasPrefix(lower, "about:") {
		return false
	}
	return true
}
