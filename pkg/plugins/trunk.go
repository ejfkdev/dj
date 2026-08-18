package plugins

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// TrunkPlugin 提取 Trunk（Rust wasm-bindgen 打包器）的动态加载资源。
//
// Trunk SPA 的入口 HTML 内联脚本通过 fetch('./static/sitemap.json') 获取
// 路由/worker/chunk 清单，静态产物不含字面量 chunk 依赖。清单结构：
//
//	{
//	  "routes":  [ { "name": "home", "file": "js/lazy-home.js" }, ... ],
//	  "chunks":  { "w-0": "x1y2z3w4", "w-1": "a5b6c7d8" },
//	  "workers": [ "js/worker.js" ]
//	}
//
// 步骤：
//  1. 从 JS/HTML 内容中提取 sitemap.json 引用（fetch() 或裸字符串）
//  2. sitemap 作为 JSON Intermediate 入队下载
//  3. 解析 JSON：routes[].file 直接是 chunk 路径；chunks 映射组合成
//     js/<name>.<hash>.js；workers 数组直接是路径
type TrunkPlugin struct {
	// sitemap.json 引用
	sitemapRe *regexp.Regexp
	// fetch(...json) 形式
	fetchJSONRe *regexp.Regexp
}

// NewTrunkPlugin 创建插件
func NewTrunkPlugin() *TrunkPlugin {
	return &TrunkPlugin{
		sitemapRe:   regexp.MustCompile(`["']([^"']*sitemap\.json)["']`),
		fetchJSONRe: regexp.MustCompile(`fetch\s*\(\s*["']([^"']+\.json)["']`),
	}
}

func (p *TrunkPlugin) Name() string {
	return "TrunkPlugin"
}

func (p *TrunkPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	switch input.ContentType {
	case extractor.ContentTypeJS, extractor.ContentTypeHTML:
		return strings.Contains(string(input.Content), "sitemap.json") ||
			strings.Contains(string(input.Content), "trunk_app")
	case extractor.ContentTypeJSON:
		content := string(input.Content)
		return strings.Contains(content, `"routes"`) &&
			(strings.Contains(content, `"chunks"`) || strings.Contains(content, `"workers"`))
	}
	return false
}

func (p *TrunkPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// JSON: 解析 sitemap 清单
	if input.ContentType == extractor.ContentTypeJSON {
		return result, p.parseSitemap(input.SourceURL, input.Content, result)
	}

	content := extractor.DecodeContent(string(input.Content))
	if len(content) == 0 {
		return result, nil
	}

	// 提取 sitemap 引用
	var sitemapPath string
	if m := p.sitemapRe.FindStringSubmatch(content); len(m) > 1 {
		sitemapPath = m[1]
	} else if m := p.fetchJSONRe.FindStringSubmatch(content); len(m) > 1 {
		sitemapPath = m[1]
	}
	if sitemapPath == "" {
		return result, nil
	}

	absoluteURL := extractor.ResolveRelativePath(input.SourceURL, sitemapPath)
	absoluteURL = extractor.NormalizeURL(absoluteURL)
	if !extractor.IsAbsoluteURL(absoluteURL) {
		return result, nil
	}
	result.Intermediates = append(result.Intermediates, extractor.Intermediate{
		URL:  absoluteURL,
		Type: extractor.ContentTypeJSON,
	})
	return result, nil
}

// trunkSitemap Trunk sitemap.json 结构
type trunkSitemap struct {
	Routes []struct {
		Name string `json:"name"`
		File string `json:"file"`
	} `json:"routes"`
	Chunks  map[string]string `json:"chunks"`
	Workers []string          `json:"workers"`
}

// parseSitemap 解析 Trunk sitemap JSON，把 routes/chunks/workers 转成 chunk 绝对 URL。
// 路径相对 sitemap.json 所在目录解析。
func (p *TrunkPlugin) parseSitemap(sourceURL string, content []byte, result *extractor.Result) error {
	var sm trunkSitemap
	if err := json.Unmarshal(content, &sm); err != nil {
		return err
	}

	add := func(path string) {
		if path == "" {
			return
		}
		absoluteURL := extractor.ResolveRelativePath(sourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if !extractor.IsAbsoluteURL(absoluteURL) {
			return
		}
		for _, u := range result.URLs {
			if u.URL == absoluteURL {
				return
			}
		}
		result.URLs = append(result.URLs, extractor.DiscoveredJS{
			URL:      absoluteURL,
			FromURL:  sourceURL,
			IsInline: false,
		})
	}

	for _, route := range sm.Routes {
		add(route.File)
	}
	for name, hash := range sm.Chunks {
		add("js/" + name + "." + hash + ".js")
	}
	for _, worker := range sm.Workers {
		add(worker)
	}
	return nil
}
