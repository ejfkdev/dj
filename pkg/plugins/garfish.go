package plugins

import (
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// GarfishPlugin 提取 Garfish 微前端子应用入口（HTML 目录）。
//
// 注册配置形如：
//
//	Garfish.run({ apps: [{ name: "sub", entry: "/sub-app/" }] })
//
// entry 指向子应用 HTML 入口。入口本身不是 JS，入队后按 HTML 处理，
// 由 HTMLScriptPlugin/DynamicImportPlugin 继续提取子应用的 JS。
type GarfishPlugin struct {
	// Garfish.run 注册配置中的 entry 键
	entryRe *regexp.Regexp
}

// NewGarfishPlugin 创建插件
func NewGarfishPlugin() *GarfishPlugin {
	return &GarfishPlugin{
		entryRe: regexp.MustCompile(`entry["']?\s*:\s*["']([^"']+)["']`),
	}
}

func (p *GarfishPlugin) Name() string {
	return "GarfishPlugin"
}

func (p *GarfishPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	// Garfish 框架标记（minified 后仍保留字样）
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("Garfish"),
		[]byte("garfish"),
	})
}

func (p *GarfishPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	content := extractor.DecodeContent(string(input.Content))
	if len(content) == 0 {
		return result, nil
	}

	seen := make(map[string]bool)
	for _, m := range p.entryRe.FindAllStringSubmatchIndex(content, -1) {
		if len(m) < 4 {
			continue
		}
		valueStr := content[m[2]:m[3]]
		// apps: [{ name: "sub", entry: "/sub-app/" }] — 窗口内应有 name 字段
		if !hasMicroAppContext(content, m[0], m[1], []string{"name"}) {
			continue
		}
		if resolveMicroAppEntry(result, seen, input.SourceURL, valueStr) &&
			len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}

	return result, nil
}
