package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// SvelteKitPlugin 提取 SvelteKit 相关资源
type SvelteKitPlugin struct {
	nodesRe *regexp.Regexp
	chunksRe *regexp.Regexp
}

// NewSvelteKitPlugin 创建插件
func NewSvelteKitPlugin() *SvelteKitPlugin {
	return &SvelteKitPlugin{
		// 匹配 ../nodes/xxx.js 或 /_app/immutable/nodes/xxx.js
		nodesRe: regexp.MustCompile(`["']\.\.?/nodes/([0-9a-zA-Z_-]+\.js)["']`),
		chunksRe: regexp.MustCompile(`["']\.\.?/chunks/([0-9a-zA-Z_-]+\.js)["']`),
	}
}

func (p *SvelteKitPlugin) Name() string {
	return "SvelteKitPlugin"
}

func (p *SvelteKitPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "_app/immutable") ||
			strings.Contains(content, "__svelte") ||
			strings.Contains(content, "sveltekit")
	}
	// JS 中检测 sveltekit 特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "_app/immutable") ||
			strings.Contains(content, "__sveltekit") ||
			strings.Contains(content, ".svelte")
	}
	return false
}

func (p *SvelteKitPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 提取 nodes 路径
	for _, match := range p.nodesRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		filename := string(match[1])
		// 转换为完整路径
		path := "/_app/immutable/nodes/" + filename
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

	// 提取 chunks 路径
	for _, match := range p.chunksRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		filename := string(match[1])
		// 转换为完整路径
		path := "/_app/immutable/chunks/" + filename
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
