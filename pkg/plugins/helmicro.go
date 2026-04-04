package plugins

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// HelMicroPlugin 提取 Hel-Micro metadata JSON 并解析其中的组件
type HelMicroPlugin struct {
	metadataRe         *regexp.Regexp
	metadataConcatRe  *regexp.Regexp
	componentPathRe    *regexp.Regexp
	cdnPrefixRe        *regexp.Regexp
	cdnFallbackRe      *regexp.Regexp
}

// HelComponent Hel-Micro 组件配置
type HelComponent struct {
	URL           string   `json:"url"`
	OfflineChunks []string `json:"offlineChunks"`
}

// NewHelMicroPlugin 创建插件
func NewHelMicroPlugin() *HelMicroPlugin {
	return &HelMicroPlugin{
		// 匹配 metadata*.json 路径
		metadataRe: regexp.MustCompile(`["']([^"']*metadata[^"']*\.json)["']`),
		// 从 concat 动态构造中提取 metadata 路径
		// 如: "".concat(Kv,"//").concat($v,"/components/docs/metadata{{specifiedVersionKey}}.json?t={{time}}
		metadataConcatRe: regexp.MustCompile(`["']([^"']*/components/docs/metadata[^"']+\.json[^"']*)["']`),
		// 匹配 hel-micro 组件路径
		componentPathRe: regexp.MustCompile(`["']([^"']*/components/[^"']+\.js)["']`),
		// COMPONENT_CDN_PREFIX = "..." 或 window.COMPONENT_CDN_PREFIX = "..."
		// 或 componentCdnPrefix: "..."
		cdnPrefixRe: regexp.MustCompile(`(?:window\.)?COMPONENT_CDN_PREFIX\s*=\s*["']([^"']+)["']`),
		// 匹配 CDN fallback URL 模式: xxx || "//host/path"
		// 用于提取协议相对 URL 作为 CDN baseURL
		cdnFallbackRe: regexp.MustCompile(`\|\|\s*["'](//[^"'\s]+)["']`),
	}
}

func (p *HelMicroPlugin) Name() string {
	return "HelMicroPlugin"
}

func (p *HelMicroPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)

	// JS 内容：检查 Hel-Micro 特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "hel-micro") ||
			strings.Contains(content, "helMicro") ||
			strings.Contains(content, "hel_meta") ||
			strings.Contains(content, "components/docs/metadata") ||
			strings.Contains(content, "COMPONENT_CDN_PREFIX") ||
			strings.Contains(content, "componentCdnPrefix") ||
			strings.Contains(content, "window.COMPONENT_CDN_PREFIX")
	}

	// JSON 内容：检查是否是 Hel-Micro metadata 格式
	if input.ContentType == extractor.ContentTypeJSON {
		return p.isHelMicroMetadata(content)
	}

	return false
}

// isHelMicroMetadata 检查 JSON 内容是否是 Hel-Micro metadata 格式
func (p *HelMicroPlugin) isHelMicroMetadata(content string) bool {
	// Hel-Micro metadata 格式是 {"组件名": {"url": "xxx.js", ...}, ...}
	// 检查是否是 JSON 对象格式
	var jsonData any
	if err := json.Unmarshal([]byte(content), &jsonData); err != nil {
		return false
	}

	jsonMap, ok := jsonData.(map[string]any)
	if !ok {
		return false
	}

	// 检查是否包含至少一个包含 url 字段的对象
	for _, v := range jsonMap {
		if comp, ok := v.(map[string]any); ok {
			if _, hasURL := comp["url"]; hasURL {
				return true
			}
			if _, hasChunks := comp["offlineChunks"]; hasChunks {
				return true
			}
		}
	}

	return false
}

func (p *HelMicroPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// JS 内容：提取 metadata JSON 路径
	if input.ContentType == extractor.ContentTypeJS {
		// 提取 metadata JSON 路径
		for _, match := range p.metadataRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			path := string(match[1])
			// 过滤掉包含 window 或 location 的路径
			if strings.Contains(path, "window") || strings.Contains(path, "location") {
				continue
			}
			// 清理 placeholder
			path = strings.ReplaceAll(path, "{{specifiedVersionKey}}", ".v1")
			path = strings.ReplaceAll(path, "{{", "")
			path = strings.ReplaceAll(path, "}}", "")

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

		// 从 concat 动态构造中提取 metadata 路径
		// 如: "".concat(Kv,"//").concat($v,"/components/docs/metadata{{specifiedVersionKey}}.json
		for _, match := range p.metadataConcatRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			path := string(match[1])
			// 清理 placeholder
			path = strings.ReplaceAll(path, "{{specifiedVersionKey}}", ".v1")
			path = strings.ReplaceAll(path, "{{", "")
			path = strings.ReplaceAll(path, "}}", "")

			// 添加为路径片段，让探测机制与已知域名组合
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}

		// 提取 hel-micro 组件路径
		for _, match := range p.componentPathRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			path := string(match[1])
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

		// 提取 CDN prefix 作为 BaseURL
		for _, match := range p.cdnPrefixRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			prefix := string(match[1])
			result.PrependURLs = append(result.PrependURLs, prefix)
		}

		// 提取 CDN fallback URL: xxx || "//host/path" 格式的协议相对 URL
		// 这类 fallback 通常用于 CDN 前缀配置
		for _, match := range p.cdnFallbackRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			prefix := string(match[1])
			// 确保 prefix 以 // 开头，转换为完整的 baseURL
			if strings.HasPrefix(prefix, "//") {
				prefix = "https:" + prefix
			}
			result.PrependURLs = append(result.PrependURLs, prefix)
		}

		// 如果检测到 hel-micro 特征，添加标准探测路径
		content := string(input.Content)
		if strings.Contains(content, "hel-micro") || strings.Contains(content, "helMicro") || strings.Contains(content, "components/docs/metadata") {
			// 添加标准 metadata 路径片段，让探测机制与已知域名组合
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     "/components/docs/metadata.v1.json",
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// JSON 内容：解析 metadata JSON，提取组件 URL
	if input.ContentType == extractor.ContentTypeJSON {
		var jsonData any
		if err := json.Unmarshal(input.Content, &jsonData); err != nil {
			return result, nil
		}

		jsonMap, ok := jsonData.(map[string]any)
		if !ok {
			return result, nil
		}

		// 提取组件 URL 作为 ProbeTargets
		for name, value := range jsonMap {
			// 跳过测试组件
			if strings.HasPrefix(name, "test") || strings.HasPrefix(name, "mock") {
				continue
			}

			comp, ok := value.(map[string]any)
			if !ok {
				continue
			}

			// 提取 url 字段
			if url, ok := comp["url"].(string); ok && url != "" {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     url,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}

			// 提取 offlineChunks 字段
			if chunks, ok := comp["offlineChunks"].([]any); ok {
				for _, chunk := range chunks {
					if chunkStr, ok := chunk.(string); ok {
						result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
							URL:     chunkStr,
							FromURL: input.SourceURL,
							IsInline: false,
						})
					}
				}
			}
		}
	}

	return result, nil
}
