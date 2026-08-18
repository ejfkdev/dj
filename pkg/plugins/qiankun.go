package plugins

import (
	"context"
	"regexp"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// QiankunPlugin 提取 qiankun 微前端子应用入口（HTML 目录）。
//
// 主站 SPA 包内嵌子应用注册配置，形如：
//
//	{name:"business", devEntry:`//ip:5010/`, proEntry:"/aio/app/business/", container:"#micro-container"}
//	{name:"app", entry:"/app/", container:"#container", activeRule:"/app"}
//
// entry/proEntry 指向子应用 HTML 入口（目录或 index.html 文件）。
// 这些入口本身不是 JS：
//  1. 入队后由主流程下载（服务器返回 HTML）
//  2. HTMLScriptPlugin 提取其中的 <script src>、modulepreload、prefetch
//  3. 内联 script 中的 import('/xxx.js') 由 DynamicImportPlugin 继续提取
//
// 覆盖 qiankun（v2 registerMicroApps 的 entry 与
// vite-plugin-qiankun 的 proEntry 两种形式）。
type QiankunPlugin struct {
	// entry/proEntry 键（可选带引号的 minified key），值为字符串
	entryRe *regexp.Regexp
}

// NewQiankunPlugin 创建插件
func NewQiankunPlugin() *QiankunPlugin {
	return &QiankunPlugin{
		entryRe: regexp.MustCompile(`(?:proEntry|entry)["']?\s*:\s*["']([^"']+)["']`),
	}
}

func (p *QiankunPlugin) Name() string {
	return "QiankunPlugin"
}

func (p *QiankunPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("proEntry"),
		[]byte("entry"),
	})
}

// qiankun 注册配置的伴生字段，用作上下文校验
var qiankunContextKeywords = []string{
	"container", "activeRule", "activeWhen", "sandbox", "singular", "name",
}

func (p *QiankunPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 编码还原（minified 包中常见 \/ 转义）
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
		if !hasMicroAppContext(content, m[0], m[1], qiankunContextKeywords) {
			continue
		}
		if resolveMicroAppEntry(result, seen, input.SourceURL, valueStr) &&
			len(result.Intermediates) >= maxMicroAppEntries {
			break
		}
	}

	return result, nil
}
