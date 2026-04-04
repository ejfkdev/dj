package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ModernJSPlugin 提取 ModernJS/ByteDance 系路由 manifest
type ModernJSPlugin struct {
}

// NewModernJSPlugin 创建插件
func NewModernJSPlugin() *ModernJSPlugin {
	return &ModernJSPlugin{}
}

func (p *ModernJSPlugin) Name() string {
	return "ModernJSPlugin"
}

func (p *ModernJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeHTML {
		return false
	}
	content := string(input.Content)
	return strings.Contains(content, "_MODERNJS_ROUTE_MANIFEST")
}

func (p *ModernJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	// 使用括号计数提取完整的 JSON 对象
	manifestJSON := p.extractManifestJSON(content)
	if manifestJSON == "" {
		return result, nil
	}

	// 提取 CDN base URL
	cdnBase := p.extractCDNBase(content, input.SourceURL)
	if cdnBase != "" {
		result.PrependURLs = append(result.PrependURLs, cdnBase)
	}

	// 解析 JSON 并提取 JS 路径
	jsPaths := p.extractJSPaths(manifestJSON)

	for _, path := range jsPaths {
		var absoluteURL string
		if cdnBase != "" {
			// 使用 CDN base URL 构建完整 URL
			absoluteURL = cdnBase + strings.TrimPrefix(path, "/")
		} else {
			// 转换为完整 URL
			absoluteURL = extractor.ResolveRelativePath(input.SourceURL, path)
		}
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

// extractCDNBase 从 HTML/JS 内容中提取 CDN base URL
// 优先从 b.p 或 __webpack_public_path__ 中提取
func (p *ModernJSPlugin) extractCDNBase(content string, sourceURL string) string {
	// 方法1: 查找 b.p = "..." 或 __webpack_public_path__ = "..."
	// b.p 是 Webpack 5 Modern.js 常用的变量名
	publicPathRe := regexp.MustCompile(`(?:b\.p|__webpack_public_path__)\s*=\s*["']([^"']+)["']`)
	match := publicPathRe.FindStringSubmatch(content)
	if len(match) >= 2 {
		publicPath := match[1]
		// 如果是协议相对路径 (//cdn.xxx.com)，转换为 https://
		if strings.HasPrefix(publicPath, "//") {
			publicPath = "https:" + publicPath
		}
		// 去掉末尾的 /
		publicPath = strings.TrimSuffix(publicPath, "/") + "/"
		return publicPath
	}

	return ""
}

// extractManifestJSON 使用括号计数从 content 中提取 _MODERNJS_ROUTE_MANIFEST 的完整 JSON
func (p *ModernJSPlugin) extractManifestJSON(content string) string {
	// 查找 window._MODERNJS_ROUTE_MANIFEST 开始位置
	startMarker := "window._MODERNJS_ROUTE_MANIFEST"
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return ""
	}

	// 找到等号位置
	equalIdx := strings.Index(content[startIdx:], "=")
	if equalIdx == -1 {
		return ""
	}
	jsonStart := startIdx + equalIdx + 1

	// 找到第一个 {
	braceStart := -1
	for i := jsonStart; i < len(content); i++ {
		if content[i] == '{' {
			braceStart = i
			break
		}
		// 跳过空白字符
		if content[i] != ' ' && content[i] != '\t' && content[i] != '\n' && content[i] != '\r' {
			break
		}
	}
	if braceStart == -1 {
		return ""
	}

	// 使用括号计数找到匹配的关闭括号
	depth := 1
	for i := braceStart + 1; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				// 返回 JSON 内容（不包含大括号）
				return content[braceStart+1 : i]
			}
		}
	}

	return ""
}

// extractJSPaths 从 manifest JSON 中提取所有 JS 文件路径
func (p *ModernJSPlugin) extractJSPaths(manifestJSON string) []string {
	var paths []string
	seen := make(map[string]bool)

	// 匹配 "static/js/..." 或 "/static/js/..." 路径
	jsPathRe := regexp.MustCompile(`"((?:static|/static)/[^"]+\.js)"`)
	for _, match := range jsPathRe.FindAllStringSubmatch(manifestJSON, -1) {
		if len(match) < 2 {
			continue
		}
		path := match[1]
		// 去重
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}

	return paths
}
