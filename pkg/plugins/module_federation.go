package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ModuleFederationPlugin 提取 Module Federation manifest 和 remoteEntry
type ModuleFederationPlugin struct {
	manifestRe *regexp.Regexp
	// 任何引号字符串形式的 remoteEntry.js 引用：
	//   initOptions remotes: entry:"/remote/remoteEntry.js"
	//   webpack script loader: r.l("/remote/remoteEntry.js", ...)
	remoteEntryRe *regexp.Regexp
}

// NewModuleFederationPlugin 创建插件
func NewModuleFederationPlugin() *ModuleFederationPlugin {
	return &ModuleFederationPlugin{
		// 匹配 manifest.json 引用
		manifestRe:    regexp.MustCompile(`["']([^"']*manifest[^"']*\.json)["']`),
		remoteEntryRe: regexp.MustCompile(`["']([^"']*remoteEntry\.js)["']`),
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

	addPath := func(path string) {
		// 过滤明显的 non-JSON 引用
		if strings.Contains(path, "{{") || strings.Contains(path, "}}") {
			return
		}
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      path,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		}
	}

	for _, match := range p.manifestRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		addPath(string(match[1]))
	}

	// remoteEntry.js 本身也必须下载，其 runtime 才携带 remote 侧 chunk 映射
	for _, match := range p.remoteEntryRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		entryPath := string(match[1])
		addPath(entryPath)

		// 同一目录下的 remote manifest（mf-manifest.json / federation-manifest.json）
		// 标准 MF manifest 携带 remote side 全部 chunk 的 sync/async 清单。
		entryURL := extractor.ResolveRelativePath(input.SourceURL, entryPath)
		entryURL = extractor.NormalizeURL(entryURL)
		if idx := strings.LastIndex(entryURL, "/"); idx >= 0 {
			dir := entryURL[:idx+1]
			for _, name := range []string{"mf-manifest.json", "federation-manifest.json"} {
				addPath(dir + name)
			}
		}
	}

	return result, nil
}
