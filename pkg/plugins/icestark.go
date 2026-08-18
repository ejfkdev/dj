package plugins

import (
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// IcestarkPlugin 提取 icestark 微前端子应用入口（HTML 目录）。
//
// 注册配置形如：
//
//	AppRoute({ path: "/", url: ["/sub-app/"] })
//	AppRoute({ path: "/routes/", url: ["/sub-app/index.html"] })
//
// url 是字符串或字符串数组，指向子应用 HTML 入口。入口本身不是 JS，
// 入队后按 HTML 处理，由 HTMLScriptPlugin/DynamicImportPlugin 继续提取
// 子应用的 JS。
type IcestarkPlugin struct {
	// url 单值或单元素数组: url: "/a/"、url:["/a/"]
	urlRe *regexp.Regexp
	// url 多元素数组: url:["/a/","/b/"]
	urlArrayRe *regexp.Regexp
	// 数组内引号字符串
	quotedRe *regexp.Regexp
}

// NewIcestarkPlugin 创建插件
func NewIcestarkPlugin() *IcestarkPlugin {
	return &IcestarkPlugin{
		urlRe:      regexp.MustCompile(`url["']?\s*:\s*(?:\[\s*)?["']([^"']+)["']`),
		urlArrayRe: regexp.MustCompile(`url["']?\s*:\s*\[([^\]]+)\]`),
		quotedRe:   regexp.MustCompile(`["']([^"']+)["']`),
	}
}

func (p *IcestarkPlugin) Name() string {
	return "IcestarkPlugin"
}

func (p *IcestarkPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	// icestark 框架标记（AppRoute 组件或包名，minified 后仍保留）
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("AppRoute"),
		[]byte("@ice/stark"),
		[]byte("icestark"),
	})
}

func (p *IcestarkPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	content := extractor.DecodeContent(string(input.Content))
	if len(content) == 0 {
		return result, nil
	}

	seen := make(map[string]bool)

	// AppRoute 配置对象内应有 path / AppRoute 伴生字段
	kw := []string{"path", "AppRoute"}
	add := func(valueStr string, start, end int) bool {
		if !hasMicroAppContext(content, start, end, kw) {
			return false
		}
		return resolveMicroAppEntry(result, seen, input.SourceURL, valueStr)
	}

	// 单值 / 单元素数组形式
	for _, m := range p.urlRe.FindAllStringSubmatchIndex(content, -1) {
		if len(m) < 4 {
			continue
		}
		if add(content[m[2]:m[3]], m[0], m[1]) && len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}
	// 多元素数组形式
	if len(result.Intermediates) < maxMicroAppEntries {
		for _, m := range p.urlArrayRe.FindAllStringSubmatchIndex(content, -1) {
			if len(m) < 4 {
				continue
			}
			for _, q := range p.quotedRe.FindAllStringSubmatch(content[m[2]:m[3]], -1) {
				if len(q) > 1 {
					if add(q[1], m[0], m[1]) && len(result.Intermediates) >= maxMicroAppEntries {
						break
					}
				}
			}
			if len(result.Intermediates) >= maxMicroAppEntries {
				break
			}
		}
	}

	return result, nil
}
