package plugins

import (
	"context"
	"encoding/json"
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
	mapDepsJsRe *regexp.Regexp
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
	// JSON: vite build manifest（build.manifest=true 时生成 .vite/manifest.json，
	// Vite 8/Rolldown 与 vite-plus 沿用同一形状）
	if input.ContentType == extractor.ContentTypeJSON {
		return strings.Contains(content, `"isEntry"`) &&
			strings.Contains(content, `"file"`)
	}
	return false
}

// viteManifestEntry .vite/manifest.json 的一个条目（只取需要的字段）
type viteManifestEntry struct {
	File string `json:"file"`
}

func (p *VitePlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	if input.ContentType == extractor.ContentTypeJSON {
		return result, p.analyzeManifest(input, result)
	}

	if input.ContentType == extractor.ContentTypeHTML {
		return result, p.analyzeHTML(input, result)
	}

	return result, p.analyzeJS(input, result)
}

// analyzeManifest 解析 .vite/manifest.json，提取全部入口/产物 JS 路径。
// 清单形如 {"index.html":{"file":"assets/index-abc123.js","src":"main.ts","isEntry":true,...}}，
// file 是相对站点根的产物路径；本探测固定从站点根发起（见 analyzeHTML），
// 因此用 manifest 自身的 origin 作为解析基准。
func (p *VitePlugin) analyzeManifest(input *extractor.AnalyzeInput, result *extractor.Result) error {
	var manifest map[string]viteManifestEntry
	if err := json.Unmarshal(input.Content, &manifest); err != nil {
		return nil
	}
	root := extractor.GetBaseURL(input.SourceURL) + "/"
	for _, entry := range manifest {
		if entry.File == "" || !strings.HasSuffix(entry.File, ".js") {
			continue
		}
		absoluteURL := extractor.NormalizeURL(root + strings.TrimPrefix(entry.File, "/"))
		if !extractor.IsAbsoluteURL(absoluteURL) {
			continue
		}
		result.URLs = append(result.URLs, extractor.DiscoveredJS{
			URL:      absoluteURL,
			FromURL:  input.SourceURL,
			IsInline: false,
		})
	}
	return nil
}

// analyzeHTML 分析 HTML 内容
func (p *VitePlugin) analyzeHTML(input *extractor.AnalyzeInput, result *extractor.Result) error {
	moduleish := 0

	// 提取 modulepreload 链接
	for _, match := range p.modulePreloadRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		moduleish++
		url := string(match[1])
		if strings.HasSuffix(url, ".js") || strings.HasSuffix(url, ".css") {
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, url)
			absoluteURL = extractor.NormalizeURL(absoluteURL)
			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:      url,
					FromURL:  input.SourceURL,
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
		moduleish++
		url := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, url)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      url,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		}
	}

	// vite 风格页面：固定文件名探测站点根的构建清单 .vite/manifest.json
	// （Vite build.manifest=true 时生成；Vite 8/Rolldown 与 vite-plus 同形状）
	if moduleish > 0 {
		if base := extractor.GetBaseURL(input.SourceURL); base != "" {
			result.Intermediates = append(result.Intermediates, extractor.Intermediate{
				URL:     base + "/.vite/manifest.json",
				Type:    extractor.ContentTypeJSON,
				FromURL: input.SourceURL,
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
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      fragment,
				FromURL:  input.SourceURL,
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
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:      jsPath,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	return nil
}
