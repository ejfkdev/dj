package plugins

import (
	"context"
	"net/url"
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
			// remote 自身入口 HTML 约定探测（固定文件名探测，非目录列表）：
			// remote 的入口页固定在 remoteEntry 所在目录或（vite 布局
			// <app>/assets/remoteEntry.js 时）其父目录的 index.html。通过它
			// 牵连出只在该 HTML 中引用的 remote 入口 chunk。覆盖两种布局：
			//   - /remote/remoteEntry.js            -> /remote/index.html
			//   - /remote/assets/remoteEntry.js     -> /remote/assets/index.html(404 无害)
			//                                         与 /remote/index.html
			if parsed, pErr := url.Parse(entryURL); pErr == nil {
				segs := strings.Split(strings.Trim(parsed.Path, "/"), "/")
				if len(segs) >= 2 && segs[len(segs)-1] == "remoteEntry.js" {
					dirs := [][]string{segs[:len(segs)-1]}
					if dirs[0][len(dirs[0])-1] == "assets" {
						dirs = append(dirs, dirs[0][:len(dirs[0])-1])
					}
					for _, d := range dirs {
						if len(d) == 0 {
							continue
						}
						htmlURL := parsed.Scheme + "://" + parsed.Host + "/" + strings.Join(d, "/") + "/index.html"
						result.Intermediates = append(result.Intermediates, extractor.Intermediate{
							URL:  htmlURL,
							Type: extractor.ContentTypeHTML,
						})
					}
				}
			}
		}
	}

	return result, nil
}
