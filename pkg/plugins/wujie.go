package plugins

import (
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// WujiePlugin 提取 wujie 微前端子应用入口（HTML 目录）。
//
// 注册配置形如：
//
//	startApp({ name: "sub", url: "/sub-app/", el: "#sub-app-container" })
//
// url 指向子应用 HTML 入口。入口本身不是 JS，入队后按 HTML 处理，
// 由 HTMLScriptPlugin/DynamicImportPlugin 继续提取子应用的 JS。
type WujiePlugin struct {
	// startApp 配置中的 url 键
	urlRe *regexp.Regexp
}

// NewWujiePlugin 创建插件
func NewWujiePlugin() *WujiePlugin {
	return &WujiePlugin{
		urlRe: regexp.MustCompile(`url["']?\s*:\s*["']([^"']+)["']`),
	}
}

func (p *WujiePlugin) Name() string {
	return "WujiePlugin"
}

func (p *WujiePlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	// wujie 框架标记（startApp 调用或包名，minified 后仍保留）
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("startApp"),
		[]byte("wujie"),
	})
}

func (p *WujiePlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	content := extractor.DecodeContent(string(input.Content))
	if len(content) == 0 {
		return result, nil
	}

	seen := make(map[string]bool)
	for _, m := range p.urlRe.FindAllStringSubmatchIndex(content, -1) {
		if len(m) < 4 {
			continue
		}
		valueStr := content[m[2]:m[3]]
		// startApp 配置对象内应有 name / el 伴生字段
		if !hasMicroAppContext(content, m[0], m[1], []string{"name", "el"}) {
			continue
		}
		if resolveMicroAppEntry(result, seen, input.SourceURL, valueStr) &&
			len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}

	return result, nil
}
