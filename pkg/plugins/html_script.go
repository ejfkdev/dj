package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
	"golang.org/x/net/html"
)

// HTMLScriptPlugin 提取 HTML 中的 <script src>、modulepreload 和内联脚本
type HTMLScriptPlugin struct {
	// 外部脚本
	scriptRe *regexp.Regexp
	// modulepreload 链接
	modulePreloadRe *regexp.Regexp
	// ES module entry script
	entryScriptRe *regexp.Regexp
}

// NewHTMLScriptPlugin 创建插件
func NewHTMLScriptPlugin() *HTMLScriptPlugin {
	return &HTMLScriptPlugin{
		scriptRe: regexp.MustCompile(`<script[^>]*\bsrc=["']?([^"'>\s]+)["']?`),
		// 匹配 <link rel="modulepreload" href="...">
		modulePreloadRe: regexp.MustCompile(`<link[^>]+rel=["']modulepreload["'][^>]+href=["']([^"']+)["']`),
		// 匹配 <script type="module" src="...">
		entryScriptRe: regexp.MustCompile(`<script[^>]+type=["']module["'][^>]*src=["']([^"']+)["']`),
	}
}

func (p *HTMLScriptPlugin) Name() string {
	return "HTMLScriptPlugin"
}

func (p *HTMLScriptPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	return input.ContentType == extractor.ContentTypeHTML
}

func (p *HTMLScriptPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 使用 html parser 提取内联脚本
	inlineScripts := p.parseInlineScripts(string(input.Content))

	// 提取外部脚本（使用正则，保持原有逻辑）
	for _, match := range p.scriptRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}

		src := string(match[1])

		// 解析为完整 URL
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, src)
		absoluteURL = extractor.NormalizeURL(absoluteURL)

		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     src,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 modulepreload 链接
	for _, match := range p.modulePreloadRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		href := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, href)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 ES module entry script (type="module" src="...)
	for _, match := range p.entryScriptRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		src := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, src)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 添加内联脚本
	for i, content := range inlineScripts {
		if len(content) == 0 {
			continue
		}
		result.InlineScripts = append(result.InlineScripts, extractor.InlineScript{
			SourceURL: input.SourceURL,
			Index:     i,
			Content:   []byte(content),
		})
	}

	return result, nil
}

// parseInlineScripts 使用 html parser 提取所有内联 script 内容
func (p *HTMLScriptPlugin) parseInlineScripts(htmlContent string) []string {
	var scripts []string

	// 使用 strings.Reader 包装 htmlContent
	reader := strings.NewReader(htmlContent)
	tokenizer := html.NewTokenizer(reader)

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}

		if tokenType == html.StartTagToken || tokenType == html.SelfClosingTagToken {
			token := tokenizer.Token()
			if token.Data == "script" {
				// 检查是否有 src 属性，如果有则跳过（外部脚本）
				hasSrc := false
				for _, attr := range token.Attr {
					if attr.Key == "src" {
						hasSrc = true
						break
					}
				}
				if hasSrc {
					continue
				}

				// 这是一个内联脚本，获取其内容
				// 消费 tokenizer 直到 </script>
				content := p.extractScriptContent(tokenizer)
				if content != "" {
					scripts = append(scripts, content)
				}
			}
		}
	}

	return scripts
}

// extractScriptContent 从 tokenizer 提取 script 标签的内容
func (p *HTMLScriptPlugin) extractScriptContent(tokenizer *html.Tokenizer) string {
	var content strings.Builder

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}

		if tokenType == html.EndTagToken {
			token := tokenizer.Token()
			if token.Data == "script" {
				break
			}
		}

		if tokenType == html.TextToken {
			content.WriteString(tokenizer.Token().Data)
		}
	}

	return content.String()
}
