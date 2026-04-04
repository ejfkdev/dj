package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// VitePlugin 提取 Vite 相关资源
type VitePlugin struct {
	modulePreloadRe *regexp.Regexp
	vitePreloadRe   *regexp.Regexp
	// 匹配整个 __vite__mapDeps 函数体
	mapDepsFuncRe *regexp.Regexp
	// 从函数体中提取 JS 路径
	mapDepsJsRe   *regexp.Regexp
}

// NewVitePlugin 创建插件
func NewVitePlugin() *VitePlugin {
	return &VitePlugin{
		// <link rel="modulepreload" href="...">
		modulePreloadRe: regexp.MustCompile(`<link[^>]+rel=["']modulepreload["'][^>]+href=["']([^"']+)["']`),
		// __vitePreload(() => import("..."))
		vitePreloadRe: regexp.MustCompile(`__vitePreload\s*\(\s*\(\s*\)\s*=>\s*import\s*\(\s*["']([^"']+)["']`),
		// 匹配整个 __vite__mapDeps 函数体: 从 __vite__mapDeps= 到分号或换行
		// 使用 `(?s)` 让 . 匹配换行
		mapDepsFuncRe: regexp.MustCompile(`(?s)__vite__mapDeps\s*=\s*[^;]+`),
		// 提取 JS 路径: "xxx.js" 或 'xxx.js'
		mapDepsJsRe: regexp.MustCompile(`["']([^"']+\.js)["']`),
	}
}

func (p *VitePlugin) Name() string {
	return "VitePlugin"
}

func (p *VitePlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "@vite/client") ||
			strings.Contains(content, "type=\"module\"") ||
			strings.Contains(content, "modulepreload")
	}
	// JS 中检测 vite 特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "__vite") ||
			strings.Contains(content, "__vitePreload") ||
			strings.Contains(content, "import.meta.env")
	}
	return false
}

func (p *VitePlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	if input.ContentType == extractor.ContentTypeHTML {
		return result, p.analyzeHTML(input, result)
	}

	return result, p.analyzeJS(input, result)
}

// analyzeHTML 分析 HTML 内容
func (p *VitePlugin) analyzeHTML(input *extractor.AnalyzeInput, result *extractor.Result) error {
	// 提取 modulepreload 链接
	for _, match := range p.modulePreloadRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		url := string(match[1])
		if strings.HasSuffix(url, ".js") || strings.HasSuffix(url, ".css") {
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
	}

	// 提取 <script type="module" src="...">
	moduleScriptRe := regexp.MustCompile(`<script[^>]*type=["']module["'][^>]*src=["']([^"']+)["']`)
	for _, match := range moduleScriptRe.FindAllSubmatch(input.Content, -1) {
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

	return nil
}

// analyzeJS 分析 JS 内容
func (p *VitePlugin) analyzeJS(input *extractor.AnalyzeInput, result *extractor.Result) error {
	// 提取 __vitePreload 中的 import 路径
	for _, match := range p.vitePreloadRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		fragment := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, fragment)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 __vite__mapDeps 函数体中的 JS 路径
	for _, funcMatch := range p.mapDepsFuncRe.FindAllSubmatch(input.Content, -1) {
		if len(funcMatch) < 1 {
			continue
		}
		funcBody := string(funcMatch[0])

		// 在函数体中提取所有 JS 路径
		for _, jsMatch := range p.mapDepsJsRe.FindAllStringSubmatch(funcBody, -1) {
			if len(jsMatch) < 2 {
				continue
			}
			jsPath := jsMatch[1]

			// 跳过 CSS 文件
			if strings.HasSuffix(jsPath, ".css") {
				continue
			}

			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, jsPath)
			absoluteURL = extractor.NormalizeURL(absoluteURL)
			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:     absoluteURL,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     jsPath,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	return nil
}
