package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// NuxtJSPlugin 提取 Nuxt.js 相关资源
type NuxtJSPlugin struct {
	nuxtJSPathRe *regexp.Regexp
}

// NewNuxtJSPlugin 创建插件
func NewNuxtJSPlugin() *NuxtJSPlugin {
	return &NuxtJSPlugin{
		// 匹配 /_nuxt/xxx.js 或 https://cdn.com/_nuxt/xxx.js
		nuxtJSPathRe: regexp.MustCompile(`(?:https?://[^"' ]+)?/_nuxt/[^"'\\\s<>()\],]+\.js`),
	}
}

func (p *NuxtJSPlugin) Name() string {
	return "NuxtJSPlugin"
}

func (p *NuxtJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "__NUXT__") ||
			strings.Contains(content, "__NUXT_DATA__") ||
			strings.Contains(content, "/_nuxt/")
	}
	// JS 中检测 nuxt 特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "buildAssetsDir") ||
			strings.Contains(content, "\"buildId\"") ||
			strings.Contains(content, "process.env.NUXT")
	}
	return false
}

func (p *NuxtJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	for _, match := range p.nuxtJSPathRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 1 {
			continue
		}

		path := string(match[0])

		// 解析为完整 URL
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