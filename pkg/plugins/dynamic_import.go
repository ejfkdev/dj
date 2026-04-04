package plugins

import (
	"bytes"
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// DynamicImportPlugin 提取 import() 动态导入
type DynamicImportPlugin struct {
	importRe *regexp.Regexp
}

// NewDynamicImportPlugin 创建插件
func NewDynamicImportPlugin() *DynamicImportPlugin {
	return &DynamicImportPlugin{
		importRe: regexp.MustCompile(`import\s*\(\s*["']([^"']+)["']\s*\)`),
	}
}

func (p *DynamicImportPlugin) Name() string {
	return "DynamicImportPlugin"
}

func (p *DynamicImportPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	return bytes.Contains(input.Content, []byte("import"))
}

func (p *DynamicImportPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	for _, match := range p.importRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}

		fragment := string(match[1])

		// 尝试解析为完整 URL
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
