package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
	"golang.org/x/net/html"
)

// HTMLScriptPlugin 提取 HTML 中的 <script src>、modulepreload、prefetch 和内联脚本
type HTMLScriptPlugin struct {
	// 外部脚本
	scriptRe *regexp.Regexp
	// modulepreload 链接
	modulePreloadRe *regexp.Regexp
	// prefetch 链接（Vite/Webpack5/Next.js 生产构建常用模式）
	// 独立于 modulePreloadRe，避免影响 modulepreload 提取逻辑
	prefetchRe *regexp.Regexp
	// ES module entry script
	entryScriptRe *regexp.Regexp
}

// NewHTMLScriptPlugin 创建插件
func NewHTMLScriptPlugin() *HTMLScriptPlugin {
	return &HTMLScriptPlugin{
		scriptRe: regexp.MustCompile(`<script[^>]*\bsrc=["']?([^"'>\s]+)["']?`),
		// 匹配 <link rel="modulepreload" href="...">
		modulePreloadRe: regexp.MustCompile(`<link[^>]+rel=["']modulepreload["'][^>]+href=["']([^"']+)["']`),
		// 匹配 <link rel="prefetch" href="..."> 或 <link href="..." rel="prefetch">
		// 兼容属性顺序：href 可在 rel 之前或之后（生产 HTML 两种都常见）
		// Vue 3 / Vite 生产构建会用此标签预取所有异步 chunk
		// 注：独立于 modulePreloadRe，按用户要求不修改 modulepreload 提取逻辑
		prefetchRe: regexp.MustCompile(`<link[^>]+(?:href=["']([^"']+\.js)["'][^>]*rel=["']prefetch["']|rel=["']prefetch["'][^>]+href=["']([^"']+\.js)["'])`),
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
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      src,
				FromURL:  input.SourceURL,
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
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 prefetch 链接（独立于 modulepreload 循环，避免影响已有逻辑）
	// Vite / Webpack 5 / Next.js 生产构建常用此标签预取所有异步 chunk
	// 正则有两个 capture group：href-在前 (group 1) 或 rel-在前 (group 2)，二选一
	for _, match := range p.prefetchRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 3 {
			continue
		}
		// 二选一取非空的那一个
		href := string(match[1])
		if href == "" {
			href = string(match[2])
		}
		if href == "" {
			continue
		}
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, href)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
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
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
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
