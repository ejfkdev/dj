package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// RequireJSPlugin 提取 RequireJS 相关资源
type RequireJSPlugin struct {
	scriptSrcRe *regexp.Regexp
	dataMainRe  *regexp.Regexp
	requireRe   *regexp.Regexp
	defineRe    *regexp.Regexp
}

// NewRequireJSPlugin 创建插件
func NewRequireJSPlugin() *RequireJSPlugin {
	return &RequireJSPlugin{
		// <script src="...require.js">
		scriptSrcRe: regexp.MustCompile(`<script[^>]*src=["']([^"']*require(?:\.min)?\.js[^"']*)["']`),
		// data-main="js/main"
		dataMainRe: regexp.MustCompile(`data-main=["']([^"']+)["']`),
		// require([...])
		requireRe: regexp.MustCompile(`require\s*\(\s*\[([^\]]+)\]`),
		// define([...])
		defineRe: regexp.MustCompile(`define\s*\(\s*\[([^\]]+)\]`),
	}
}

func (p *RequireJSPlugin) Name() string {
	return "RequireJSPlugin"
}

func (p *RequireJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "require.min.js") ||
			strings.Contains(content, "require.js") ||
			strings.Contains(content, "data-main=") ||
			strings.Contains(content, "require.config")
	}
	// JS 中检测
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "require([") ||
			strings.Contains(content, "define([") ||
			strings.Contains(content, "require.config")
	}
	return false
}

func (p *RequireJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	if input.ContentType == extractor.ContentTypeHTML {
		return p.analyzeHTML(input, result)
	}

	return p.analyzeJS(input, result)
}

// analyzeHTML 分析 HTML 内容
func (p *RequireJSPlugin) analyzeHTML(input *extractor.AnalyzeInput, result *extractor.Result) (*extractor.Result, error) {
	// 提取 require.js 路径
	for _, match := range p.scriptSrcRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		url := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, url)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     url,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 data-main
	for _, match := range p.dataMainRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		// 自动添加 .js 后缀
		if !strings.HasSuffix(path, ".js") {
			path = path + ".js"
		}
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	return result, nil
}

// analyzeJS 分析 JS 内容
func (p *RequireJSPlugin) analyzeJS(input *extractor.AnalyzeInput, result *extractor.Result) (*extractor.Result, error) {
	// 提取 require([...]) 中的依赖
	for _, match := range p.requireRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		depsContent := string(match[1])
		p.extractDeps(depsContent, input.SourceURL, result)
	}

	// 提取 define([...]) 中的依赖
	for _, match := range p.defineRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		depsContent := string(match[1])
		p.extractDeps(depsContent, input.SourceURL, result)
	}

	return result, nil
}

// extractDeps 从依赖数组中提取 JS 路径
func (p *RequireJSPlugin) extractDeps(content string, sourceURL string, result *extractor.Result) {
	// 匹配 "xxx.js" 或 'xxx'
	depRe := regexp.MustCompile(`["']([^"']+)["']`)
	for _, match := range depRe.FindAllSubmatch([]byte(content), -1) {
		if len(match) < 2 {
			continue
		}
		dep := string(match[1])
		// 跳过已经完整的 URL
		if strings.HasPrefix(dep, "http://") || strings.HasPrefix(dep, "https://") {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     dep,
				FromURL: sourceURL,
				IsInline: false,
			})
			continue
		}
		// 添加 .js 后缀
		path := dep
		if !strings.HasSuffix(path, ".js") {
			path = path + ".js"
		}
		absoluteURL := extractor.ResolveRelativePath(sourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: sourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: sourceURL,
				IsInline: false,
			})
		}
	}
}