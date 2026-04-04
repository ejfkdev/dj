package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// SourceMapPlugin 提取 JS 文件的 source map 信息
type SourceMapPlugin struct {
	// 匹配 //# sourceMappingURL=...
	sourceMappingURLRe *regexp.Regexp
	// 匹配 //@ sourceMappingURL=... (旧版)
	sourceMappingOldRe *regexp.Regexp
	// 匹配 data:application/json;base64,... 内联 source map
	inlineSourceMapRe *regexp.Regexp
}

// NewSourceMapPlugin 创建插件
func NewSourceMapPlugin() *SourceMapPlugin {
	return &SourceMapPlugin{
		sourceMappingURLRe: regexp.MustCompile(`(?m)^//#\s*sourceMappingURL\s*=\s*(.+)$`),
		sourceMappingOldRe: regexp.MustCompile(`(?m)^//@\s*sourceMappingURL\s*=\s*(.+)$`),
		inlineSourceMapRe:  regexp.MustCompile(`data:application/json[^"'\s]+`),
	}
}

func (p *SourceMapPlugin) Name() string {
	return "SourceMapPlugin"
}

func (p *SourceMapPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	// 只处理 JS 文件
	return input.ContentType == extractor.ContentTypeJS
}

func (p *SourceMapPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	foundSourceMap := false

	// 1. 尝试从 HTTP 响应头获取 source map
	if input.Headers != nil {
		if xSourceMap := input.Headers.Get("X-SourceMap"); xSourceMap != "" {
			smURL := p.resolveURL(input.SourceURL, strings.TrimSpace(xSourceMap))
			if smURL != "" {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:     smURL,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
		if sourceMap := input.Headers.Get("SourceMap"); sourceMap != "" {
			smURL := p.resolveURL(input.SourceURL, strings.TrimSpace(sourceMap))
			if smURL != "" {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:     smURL,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	// 2. 从 JS 内容中提取 sourceMappingURL
	var smURL string

	// 2.1 新标准 //# sourceMappingURL
	if matches := p.sourceMappingURLRe.FindStringSubmatch(content); len(matches) > 1 {
		smURL = p.resolveURL(input.SourceURL, strings.TrimSpace(matches[1]))
	}

	// 2.2 旧标准 //@ sourceMappingURL
	if smURL == "" {
		if matches := p.sourceMappingOldRe.FindStringSubmatch(content); len(matches) > 1 {
			smURL = p.resolveURL(input.SourceURL, strings.TrimSpace(matches[1]))
		}
	}

	// 2.3 检查是否有内联的 data: URI
	if smURL == "" {
		if matches := p.inlineSourceMapRe.FindAllString(content, -1); len(matches) > 0 {
			// 返回最后一个（通常 source map 在文件末尾）
			smURL = matches[len(matches)-1]
		}
	}

	// 3. 如果找到了 source map URL，添加到结果
	if smURL != "" {
		foundSourceMap = true
		result.URLs = append(result.URLs, extractor.DiscoveredJS{
			URL:     smURL,
			FromURL: input.SourceURL,
			IsInline: false,
		})
	}

	// 4. 记录到 knowledge（无论是否发现 source map，都记录检查结果）
	if kb := extractor.GetKnowledgeFromContext(ctx); kb != nil {
		kb.SetJSHasSourceMap(input.SourceURL, foundSourceMap)
	}

	return result, nil
}

// resolveURL 解析 source map URL 为绝对 URL
func (p *SourceMapPlugin) resolveURL(baseURL, mapURL string) string {
	// 处理 data: URI（内联 source map）
	if strings.HasPrefix(mapURL, "data:") {
		return mapURL
	}
	// 处理绝对 URL
	if strings.HasPrefix(mapURL, "http://") || strings.HasPrefix(mapURL, "https://") {
		return mapURL
	}
	// 处理协议相对 URL (//host/path)
	if strings.HasPrefix(mapURL, "//") {
		// 根据 baseURL 的协议决定
		if strings.HasPrefix(baseURL, "https://") {
			return "https:" + mapURL
		}
		return "http:" + mapURL
	}
	// 处理绝对路径 (/path)
	if strings.HasPrefix(mapURL, "/") {
		// 提取 scheme://host
		slashIdx := strings.Index(baseURL, "://")
		if slashIdx > 0 {
			scheme := baseURL[:slashIdx+3] // includes ://
			hostAndPath := baseURL[slashIdx+3:]
			hostEnd := strings.Index(hostAndPath, "/")
			if hostEnd > 0 {
				return scheme + hostAndPath[:hostEnd] + mapURL
			}
			return scheme + hostAndPath + mapURL
		}
	}
	// 处理相对路径 (path)
	// 找到 baseURL 中最后一个 / 的位置
	lastSlash := strings.LastIndex(baseURL, "/")
	if lastSlash > 0 {
		return baseURL[:lastSlash+1] + mapURL
	}
	// 如果 baseURL 没有路径，直接附加
	return baseURL + "/" + mapURL
}

