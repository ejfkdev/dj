package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// URLPatternPlugin 从 JS 内容中提取 URL 模式，用于探测
// 提取 protocol-relative URL (//host/path) 作为 probe base URL
type URLPatternPlugin struct {
	// 匹配 protocol-relative URL: "//domain/path"，排除文件后缀
	cdnPrefixRe *regexp.Regexp
	// 匹配字符串中的 .js 文件路径
	jsStringRe *regexp.Regexp
}

// NewURLPatternPlugin 创建插件
func NewURLPatternPlugin() *URLPatternPlugin {
	// 匹配带引号的 //domain/path 模式
	// 路径段中不能包含 . (排除文件扩展名)
	// 引号可以是双引号、单引号或反引号
	cdnPrefixRe := regexp.MustCompile(`["'\x60](//[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z0-9]+(?:/[a-zA-Z0-9_-]+)*/?)["'\x60]`)
	// 匹配字符串中的 .js 文件路径
	// 支持: "xxx.js", 'xxx.js', `xxx.js`
	// 支持: http://, https://, //, / 开头或无协议
	jsStringRe := regexp.MustCompile(`["'\x60]([^"'\x60]+\.js)["'\x60]`)

	return &URLPatternPlugin{
		cdnPrefixRe: cdnPrefixRe,
		jsStringRe:  jsStringRe,
	}
}

func (p *URLPatternPlugin) Name() string {
	return "URLPatternPlugin"
}

func (p *URLPatternPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	return input.ContentType == extractor.ContentTypeJS
}

func (p *URLPatternPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 处理 protocol-relative URL 前缀
	matches := p.cdnPrefixRe.FindAllSubmatch(input.Content, -1)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		prefix := string(match[1]) // 捕获组1是去掉引号后的内容

		// 跳过已处理过的
		if seen[prefix] {
			continue
		}
		seen[prefix] = true

		// 转换为完整 URL（添加 https:）
		fullURL := "https:" + prefix
		// 去掉末尾斜杠保持一致
		fullURL = strings.TrimSuffix(fullURL, "/")

		result.PrependURLs = append(result.PrependURLs, fullURL)
	}

	// 处理 .js 字符串路径，添加到探测目标
	jsMatches := p.jsStringRe.FindAllSubmatch(input.Content, -1)
	seenJS := make(map[string]bool)

	for _, match := range jsMatches {
		if len(match) < 2 {
			continue
		}

		jsPath := string(match[1]) // 捕获组1是去掉引号后的 .js 路径

		// 跳过已处理过的
		if seenJS[jsPath] {
			continue
		}
		seenJS[jsPath] = true

		result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
			URL:     jsPath,
			FromURL: input.SourceURL,
			IsInline: false,
		})
	}

	return result, nil
}
