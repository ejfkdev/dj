package plugins

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ESMImportPlugin 提取 ESM 静态 import
type ESMImportPlugin struct {
	importFromRe *regexp.Regexp
	importOnlyRe *regexp.Regexp
}

// NewESMImportPlugin 创建插件
func NewESMImportPlugin() *ESMImportPlugin {
	return &ESMImportPlugin{
		// import ... from "..."
		importFromRe: regexp.MustCompile(`import\s*(?:\{[^}]*\}|\*\s*as\s+\w+|\w+)?\s*from\s*["']([^"')]+\.js)["']`),
		// import "..."
		importOnlyRe: regexp.MustCompile(`import\s*["']([^"')]+\.js)["']`),
	}
}

func (p *ESMImportPlugin) Name() string {
	return "ESMImportPlugin"
}

func (p *ESMImportPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	return bytes.Contains(input.Content, []byte("import "))
}

func (p *ESMImportPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 提取 import ... from "..."
	for _, match := range p.importFromRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		fragment := string(match[1])
		if !p.isRelevantPath(fragment) {
			continue
		}
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

	// 提取 import "..."
	for _, match := range p.importOnlyRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		fragment := string(match[1])
		if !p.isRelevantPath(fragment) {
			continue
		}
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

	return result, nil
}

// isRelevantPath 判断路径是否可能指向 JS 文件
func (p *ESMImportPlugin) isRelevantPath(path string) bool {
	// 跳过 http/https 链接（外部资源）
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return true
	}
	// 跳过 data: 或 blob:
	if strings.HasPrefix(path, "data:") || strings.HasPrefix(path, "blob:") {
		return false
	}
	// 跳过空路径
	if path == "" {
		return false
	}
	return true
}