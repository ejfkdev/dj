package plugins

import (
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// MicroAppPlugin 提取 micro-app 微前端子应用入口（HTML 目录）。
//
// 注册配置形如：
//
//	microApp.start({ name: "sub", url: "/sub-app/", container: "#sub-app-container" })
//
// url 指向子应用 HTML 入口。入口本身不是 JS，入队后按 HTML 处理，
// 由 HTMLScriptPlugin/DynamicImportPlugin 继续提取子应用的 JS。
type MicroAppPlugin struct {
	// microApp.start 配置中的 url 键
	urlRe *regexp.Regexp
}

// NewMicroAppPlugin 创建插件
func NewMicroAppPlugin() *MicroAppPlugin {
	return &MicroAppPlugin{
		urlRe: regexp.MustCompile(`url["']?\s*:\s*["']([^"']+)["']`),
	}
}

func (p *MicroAppPlugin) Name() string {
	return "MicroAppPlugin"
}

func (p *MicroAppPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	// micro-app 框架标记（start 调用或包名，minified 后仍保留）
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("microApp.start"),
		[]byte("micro-app"),
	})
}

func (p *MicroAppPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
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
		// start 配置对象内应有 name / container 伴生字段
		if !hasMicroAppContext(content, m[0], m[1], []string{"name", "container"}) {
			continue
		}
		if resolveMicroAppEntry(result, seen, input.SourceURL, valueStr) &&
			len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}

	return result, nil
}
