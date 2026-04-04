package plugins

import (
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ScriptCreatePlugin 提取 createElement('script') 模式动态加载的 JS
type ScriptCreatePlugin struct {
	setAttributeRe *regexp.Regexp
	srcAssignRe   *regexp.Regexp
	newURLRe      *regexp.Regexp
	oFunctionRe   *regexp.Regexp
}

// NewScriptCreatePlugin 创建插件
func NewScriptCreatePlugin() *ScriptCreatePlugin {
	return &ScriptCreatePlugin{
		// script.setAttribute('src', 'xxx.js')
		setAttributeRe: regexp.MustCompile(`script\.setAttribute\s*\(\s*["']src["']\s*,\s*["']([^"']+\.js)["']\s*\)`),
		// script.src = 'xxx.js'
		srcAssignRe: regexp.MustCompile(`script\.src\s*=\s*["']([^"']+\.js)["']`),
		// new URL('xxx.js', base)
		newURLRe: regexp.MustCompile(`new\s+URL\s*\(\s*["']([^"')]+\.js)["']`),
		// o('xxx') 函数调用模式（腾讯云等）
		oFunctionRe: regexp.MustCompile(`\bo\s*\(\s*["']([^"')]+)`),
	}
}

func (p *ScriptCreatePlugin) Name() string {
	return "ScriptCreatePlugin"
}

func (p *ScriptCreatePlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	content := string(input.Content)
	return strings.Contains(content, "createElement") ||
		strings.Contains(content, ".src") ||
		strings.Contains(content, "setAttribute")
}

func (p *ScriptCreatePlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// 提取 setAttribute('src', 'xxx.js')
	for _, match := range p.setAttributeRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 script.src = 'xxx.js'
	for _, match := range p.srcAssignRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 new URL('xxx.js')
	for _, match := range p.newURLRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 o('xxx') 模式
	for _, match := range p.oFunctionRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		// 只处理看起来像 JS 路径的
		if !strings.HasSuffix(path, ".js") && !strings.Contains(path, ".js?") {
			continue
		}
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:     absoluteURL,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	return result, nil
}