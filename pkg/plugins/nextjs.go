package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// NextJSPlugin 提取 Next.js 相关资源
type NextJSPlugin struct {
	buildIdRe      *regexp.Regexp
	assetPrefixRe  *regexp.Regexp
	nextDataRe     *regexp.Regexp
	sTypeRe        *regexp.Regexp
	numChunkIdRe   *regexp.Regexp
	numHashMapRe   *regexp.Regexp
}

// NewNextJSPlugin 创建插件
func NewNextJSPlugin() *NextJSPlugin {
	return &NextJSPlugin{
		// buildId 提取: "buildId":"v0.16.3-community"
		buildIdRe: regexp.MustCompile(`"buildId"\s*:\s*"([^"]+)"`),
		// assetPrefix 提取: https://cdn.example.com/_next
		assetPrefixRe: regexp.MustCompile(`https?://[^"' ]+/_next`),
		// __NEXT_DATA__ 提取
		nextDataRe: regexp.MustCompile(`<script[^>]+id="__NEXT_DATA__"[^>]*>([\s\S]*?)</script>`),
		// s.u=e=> 格式提取 (Next.js 特有的 chunk 映射)
		sTypeRe: regexp.MustCompile(`s\.u\s*=\s*e\s*=>\s*["']([^"']*)["']\s*\+\s*e\s*\+\s*["']-["']\s*\+\s*(\{[^}]+\})\s*\[\s*e\s*\]\s*\+\s*["']([^"']*)["']`),
		// 数字 ID hash 映射: {123:"abc123",456:"def456"}
		numChunkIdRe: regexp.MustCompile(`\{(\d+)\s*:\s*"([a-f0-9]+)"\}`),
		// 数字 hash map 解析: "123":"abc123"
		numHashMapRe: regexp.MustCompile(`"(\d+)"\s*:\s*"([a-f0-9]+)"`),
	}
}

func (p *NextJSPlugin) Name() string {
	return "NextJSPlugin"
}

func (p *NextJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	// HTML 中检测 __NEXT_DATA__ 或 /_next/
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "__NEXT_DATA__") || strings.Contains(content, "/_next/")
	}
	// JS 中检测 Next.js 特有特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "__NEXT_DATA__") ||
			strings.Contains(content, "_next/static") ||
			strings.Contains(content, "next/dist") ||
			strings.Contains(content, "s.u=") ||
			strings.Contains(content, "turbopack")
	}
	return false
}

func (p *NextJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	if input.ContentType == extractor.ContentTypeHTML {
		return p.analyzeHTML(input, result, content)
	}

	return p.analyzeJS(input, result, content)
}

// analyzeHTML 分析 HTML 内容
func (p *NextJSPlugin) analyzeHTML(input *extractor.AnalyzeInput, result *extractor.Result, content string) (*extractor.Result, error) {
	// 提取 buildId
	var buildId string
	for _, match := range p.buildIdRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		buildId = string(match[1])
		// buildId 不是域名，不能加入 PrependURLs，但可以加入 PublicPaths 供后续使用
		if buildId != "" {
			result.PublicPaths = append(result.PublicPaths, buildId)
		}
		break
	}

	// 提取 assetPrefix
	for _, match := range p.assetPrefixRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 1 {
			continue
		}
		prefix := string(match[0])
		// 移除末尾的 /_next
		prefix = strings.TrimSuffix(prefix, "/_next")
		if prefix != "" && !strings.Contains(prefix, "{{") {
			result.PrependURLs = append(result.PrependURLs, prefix)
		}
	}

	// 如果有 buildId，生成 manifest 探测目标
	if buildId != "" {
		baseURL := extractor.GetBaseURL(input.SourceURL)
		manifestPaths := []string{
			"/_next/static/" + buildId + "/_buildManifest.js",
			"/_next/static/" + buildId + "/_ssgManifest.js",
			"/_next/static/" + buildId + "/_appManifest.js",
		}
		for _, path := range manifestPaths {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}

		// 同时添加完整 URL 作为探测目标
		for _, path := range manifestPaths {
			fullURL := baseURL + path
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fullURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	return result, nil
}

// analyzeJS 分析 JS 内容
func (p *NextJSPlugin) analyzeJS(input *extractor.AnalyzeInput, result *extractor.Result, content string) (*extractor.Result, error) {
	// 提取 s.u=e=> 格式的 chunk 映射 (Next.js 特有)
	p.extractNextJSChunks(input, result, content)

	// 提取数字 ID hash 映射 {123:"abc123",456:"def456"}
	p.extractNumChunkIds(input, result, content)

	// 提取 flight chunk 路径: static/chunks/xxx.js
	p.extractFlightChunks(input, result, content)

	// 提取 __webpack_require__.e 动态加载
	p.extractWebpackRequireE(input, result, content)

	return result, nil
}

// extractNextJSChunks 提取 Next.js s.u=e=> 格式的 chunk 映射
// 例如: s.u=e=>"static/chunks/"+e+"-"+{123:"abc123"}[e]+".js"
func (p *NextJSPlugin) extractNextJSChunks(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	matches := p.sTypeRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		prefix := match[1]   // "static/chunks/"
		hashMap := match[2]  // {123:"abc123",456:"def456"}
		suffix := match[3]   // ".js"

		// 解析 hash map
		for _, m := range p.numHashMapRe.FindAllStringSubmatch(hashMap, -1) {
			if len(m) < 3 {
				continue
			}
			chunkID := m[1]   // "123"
			hash := m[2]       // "abc123"

			// 构建 chunk URL: prefix + chunkID + "-" + hash + suffix
			chunkPath := prefix + chunkID + "-" + hash + suffix
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, chunkPath)
			absoluteURL = extractor.NormalizeURL(absoluteURL)

			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:     absoluteURL,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     chunkPath,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}
}

// extractNumChunkIds 提取数字 ID hash 映射
// 例如: {123:"abc123",456:"def456"}
func (p *NextJSPlugin) extractNumChunkIds(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	matches := p.numChunkIdRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		chunkID := match[1]
		hash := match[2]

		if seen[chunkID+":"+hash] {
			continue
		}
		seen[chunkID+":"+hash] = true

		// 尝试提取前缀路径
		// 先尝试 "static/chunks/" + id + "." + hash + ".js"
		pathVariants := []string{
			"static/chunks/" + chunkID + "." + hash + ".js",
			"static/" + chunkID + "." + hash + ".js",
			"chunks/" + chunkID + "." + hash + ".js",
			chunkID + "." + hash + ".function.chunk.js",
		}

		for _, path := range pathVariants {
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
					URL:     "/" + path,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}
}

// extractFlightChunks 提取 flight chunk 路径
func (p *NextJSPlugin) extractFlightChunks(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	flightRe := regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(static/chunks/[^"'\\\s<>()\],]+\.js)`)
	for _, match := range flightRe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		path := match[1]
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
}

// extractWebpackRequireE 提取 __webpack_require__.e 动态加载
func (p *NextJSPlugin) extractWebpackRequireE(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	webpackERe := regexp.MustCompile(`__webpack_require__\.e\s*\(\s*["']([^"']+)["']\s*\)`)
	seen := make(map[string]bool)

	for _, match := range webpackERe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		chunkID := strings.Trim(match[1], "\"'")
		if seen[chunkID] {
			continue
		}
		seen[chunkID] = true

		// __webpack_require__.e 返回的 chunk ID 通常需要通过 chunk map 来解析
		// 作为探测目标上报，让主流程探测
		result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
			URL:     "__webpack_require_e__" + chunkID,
			FromURL: input.SourceURL,
			IsInline: false,
		})
	}
}
