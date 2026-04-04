package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ModuleFederationPlugin 提取 Module Federation manifest
type ModuleFederationPlugin struct {
	manifestRe *regexp.Regexp
}

// NewModuleFederationPlugin 创建插件
func NewModuleFederationPlugin() *ModuleFederationPlugin {
	return &ModuleFederationPlugin{
		// 匹配 manifest.json 引用
		manifestRe: regexp.MustCompile(`["']([^"']*manifest[^"']*\.json)["']`),
	}
}

func (p *ModuleFederationPlugin) Name() string {
	return "ModuleFederationPlugin"
}

func (p *ModuleFederationPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	content := string(input.Content)
	return strings.Contains(content, "remoteEntry.js") ||
		strings.Contains(content, "__webpack_share_scopes__") ||
		strings.Contains(content, "__webpack_init_sharing__")
}

func (p *ModuleFederationPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	for _, match := range p.manifestRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		// 过滤明显的 non-JSON 引用
		if strings.Contains(path, "{{") || strings.Contains(path, "}}") {
			continue
		}
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