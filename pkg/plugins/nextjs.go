package plugins

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// NextJSPlugin 提取 Next.js 相关资源
type NextJSPlugin struct {
	buildIdRe     *regexp.Regexp
	assetPrefixRe *regexp.Regexp
	nextDataRe    *regexp.Regexp
	sTypeRe       *regexp.Regexp
	numChunkIdRe  *regexp.Regexp
	numHashMapRe  *regexp.Regexp
	// Flight/RSC 相关
	flightChunkRe *regexp.Regexp // I[moduleId,["chunk1","chunk2"],...]
	flightHLRe    *regexp.Regexp // :HL["path.js","script"]
	flightBuildRe *regexp.Regexp // "b":"buildId"
	flightRouteRe *regexp.Regexp // "c":["","segment1","segment2"]
	chunkPathRe   *regexp.Regexp // 从 I[] 中提取 chunk 路径
	// HTML 链接提取
	aHrefRe *regexp.Regexp // <a href="...">
	// buildManifest 路由提取
	manifestPageRe *regexp.Regexp // "/page/path": ["chunk1.js","chunk2.js"]
}

// NewNextJSPlugin 创建插件
func NewNextJSPlugin() *NextJSPlugin {
	return &NextJSPlugin{
		// buildId 提取: "buildId":"v0.16.3-community"
		buildIdRe: regexp.MustCompile(`"buildId"\s*:\s*"([^"]+)"`),
		// assetPrefix 提取: https://cdn.example.com/_next
		assetPrefixRe: regexp.MustCompile(`https?://[^"' ]+/_next`),
		// __NEXT_DATA__ 提取
		nextDataRe: regexp.MustCompile(`<script[^>]+id="__NEXT_DATA__"[^>]*>([\s\S]*?)</script>`),
		// s.u=e=> 格式提取 (Next.js 特有的 chunk 映射)
		sTypeRe: regexp.MustCompile(`s\.u\s*=\s*e\s*=>\s*["']([^"']*)["']\s*\+\s*e\s*\+\s*["']-["']\s*\+\s*(\{[^}]+\})\s*\[\s*e\s*\]\s*\+\s*["']([^"']*)["']`),
		// 数字 ID hash 映射: {123:"abc123",456:"def456"}
		numChunkIdRe: regexp.MustCompile(`\{(\d+)\s*:\s*"([a-f0-9]+)"\}`),
		// 数字 hash map 解析: "123":"abc123"
		numHashMapRe: regexp.MustCompile(`"(\d+)"\s*:\s*"([a-f0-9]+)"`),
		// Flight: I[moduleId,["chunk1.js","chunk2.js"],"ComponentName"]
		flightChunkRe: regexp.MustCompile(`I\[\d+,\[([^\]]+)\]`),
		// Flight: :HL["/path.js","script"]
		flightHLRe: regexp.MustCompile(`:HL\["([^"]+\.js)","script"\]`),
		// Flight buildId: "b":"xxx"
		flightBuildRe: regexp.MustCompile(`"b":"([^"]+)"`),
		// Flight route: "c":["","segment1","segment2"]
		flightRouteRe: regexp.MustCompile(`"c":\[([^\]]+)\]`),
		// 从 I[] 内部提取 chunk 路径（兼容 Turbopack immutable 路径）
		chunkPathRe: regexp.MustCompile(`\\?"?(/?(?:_next/)?static/(?:immutable/)?(?:chunks|css)/[^"'\s,\]\\]+\.(?:js|css))\\?"?`),
		// HTML <a href="..."> 提取
		aHrefRe: regexp.MustCompile(`<a\s[^>]*href=["']([^"']+)["']`),
		// buildManifest 路由: "/page/path":["chunk1.js","chunk2.js"]
		manifestPageRe: regexp.MustCompile(`"(/[^"]+)":\s*\[([^\]]+)\]`),
	}
}

func (p *NextJSPlugin) Name() string {
	return "NextJSPlugin"
}

func (p *NextJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)
	// Flight 数据由 NextJSPlugin 处理
	if input.ContentType == extractor.ContentTypeFlight {
		return true
	}
	// HTML 中检测 __NEXT_DATA__ 或 /_next/
	if input.ContentType == extractor.ContentTypeHTML {
		return strings.Contains(content, "__NEXT_DATA__") || strings.Contains(content, "/_next/")
	}
	// JS 中检测 Next.js 特有特征
	if input.ContentType == extractor.ContentTypeJS {
		return strings.Contains(content, "__NEXT_DATA__") ||
			strings.Contains(content, "_next/static") ||
			strings.Contains(content, "next/dist") ||
			strings.Contains(content, "s.u=") ||
			strings.Contains(content, "turbopack")
	}
	return false
}

func (p *NextJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	if input.ContentType == extractor.ContentTypeFlight {
		return p.analyzeFlight(input, result, content)
	}

	if input.ContentType == extractor.ContentTypeHTML {
		return p.analyzeHTML(input, result, content)
	}

	return p.analyzeJS(input, result, content)
}

// analyzeHTML 分析 HTML 内容
func (p *NextJSPlugin) analyzeHTML(input *extractor.AnalyzeInput, result *extractor.Result, content string) (*extractor.Result, error) {
	// 提取 buildId
	var buildId string
	for _, match := range p.buildIdRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		buildId = string(match[1])
		if buildId != "" {
			result.PublicPaths = append(result.PublicPaths, buildId)
		}
		break
	}

	// 从 HTML 注释或 __next_f 数据中提取 buildId（Turbopack 格式）
	if buildId == "" {
		for _, match := range p.flightBuildRe.FindAllSubmatch(input.Content, -1) {
			if len(match) < 2 {
				continue
			}
			buildId = string(match[1])
			if buildId != "" {
				result.PublicPaths = append(result.PublicPaths, buildId)
			}
			break
		}
	}

	// 提取 assetPrefix
	for _, match := range p.assetPrefixRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 1 {
			continue
		}
		prefix := string(match[0])
		prefix = strings.TrimSuffix(prefix, "/_next")
		if prefix != "" && !strings.Contains(prefix, "{{") {
			result.PrependURLs = append(result.PrependURLs, prefix)
		}
	}

	// 如果有 buildId，生成 manifest 探测目标
	if buildId != "" {
		baseURL := extractor.GetBaseURL(input.SourceURL)
		manifestPaths := []string{
			"/_next/static/" + buildId + "/_buildManifest.js",
			"/_next/static/" + buildId + "/_ssgManifest.js",
			"/_next/static/" + buildId + "/_appManifest.js",
		}
		for _, path := range manifestPaths {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      path,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		}
		for _, path := range manifestPaths {
			fullURL := baseURL + path
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:      fullURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 解析 HTML 中的 __next_f 内联 flight 数据
	p.parseInlineFlightData(input, result, content)

	// 检测到 Next.js 站点，生成 RSC 探测请求
	// 对当前页面发 RSC 请求以获取 flight 数据
	result.RSCProbes = append(result.RSCProbes, extractor.RSCProbe{
		URL:     input.SourceURL,
		Headers: map[string]string{"RSC": "1"},
	})

	// 从路由树中提取路由路径，生成更多 RSC 探测目标
	p.extractRoutesForRSC(input, result, content)

	// 从 HTML 中的 <a href> 链接提取同域页面，生成 RSC 探测目标
	p.extractHTMLLinksForRSC(input, result, content)

	return result, nil
}

// parseInlineFlightData 解析 HTML 中 __next_f.push 内联的 flight 数据
func (p *NextJSPlugin) parseInlineFlightData(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	// __next_f.push 数据可能跨多个 script 标签
	// 直接在整个 HTML 内容中搜索 I[] 模式和 :HL 模式
	p.extractFlightChunksFromContent(input, result, content)
}

// analyzeFlight 分析 RSC Flight 数据
func (p *NextJSPlugin) analyzeFlight(input *extractor.AnalyzeInput, result *extractor.Result, content string) (*extractor.Result, error) {
	// 提取 I[] 中的 chunk 路径
	p.extractFlightChunksFromContent(input, result, content)

	// 提取 :HL 预加载提示中的 JS 路径
	for _, match := range p.flightHLRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		path := string(match[1])
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)
		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
			})
		}
	}

	// 提取 buildId
	for _, match := range p.flightBuildRe.FindAllSubmatch(input.Content, -1) {
		if len(match) < 2 {
			continue
		}
		buildId := string(match[1])
		if buildId != "" {
			result.PublicPaths = append(result.PublicPaths, buildId)
		}
		break
	}

	// 从 flight 数据中发现更多路由，生成 RSC 探测目标
	p.extractRoutesForRSC(input, result, content)

	return result, nil
}

// extractFlightChunksFromContent 从内容中提取 I[] 格式的 chunk 路径
func (p *NextJSPlugin) extractFlightChunksFromContent(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	for _, match := range p.flightChunkRe.FindAllSubmatch([]byte(content), -1) {
		if len(match) < 2 {
			continue
		}
		chunkList := string(match[1])
		// 从方括号内容中提取所有 chunk 路径
		for _, cm := range p.chunkPathRe.FindAllSubmatch([]byte(chunkList), -1) {
			if len(cm) < 2 {
				continue
			}
			path := string(cm[1])
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
			absoluteURL = extractor.NormalizeURL(absoluteURL)
			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     path,
					FromURL: input.SourceURL,
				})
			}
		}
	}
}

// extractRoutesForRSC 从内容中提取路由路径，生成 RSC 探测目标
func (p *NextJSPlugin) extractRoutesForRSC(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	baseURL := extractor.GetBaseURL(input.SourceURL)
	if baseURL == "" {
		return
	}

	// 推断 basePath：从 sourceURL 中提取路由段之前的路径
	// 例如 sourceURL = https://h5.qianshouapp.cn/f/matchmaker/pay
	// 路由段为 ["","matchmaker","pay"]，则 basePath = "/f"
	basePath := p.inferBasePath(input.SourceURL, content)

	seen := make(map[string]bool)
	for _, match := range p.flightRouteRe.FindAllSubmatch([]byte(content), -1) {
		if len(match) < 2 {
			continue
		}
		segments := string(match[1])
		// 解析路由段：去掉引号，过滤空段和 __PAGE__
		parts := strings.Split(segments, ",")
		var pathParts []string
		for _, part := range parts {
			part = strings.Trim(part, `" `)
			if part == "" || part == "$undefined" || part == "__PAGE__" {
				continue
			}
			// 跳过特殊标记
			if strings.HasPrefix(part, "$") {
				continue
			}
			pathParts = append(pathParts, part)
		}
		if len(pathParts) == 0 {
			continue
		}

		// 构建路由路径（包含 basePath）
		routePath := basePath + "/" + strings.Join(pathParts, "/")
		if seen[routePath] {
			continue
		}
		seen[routePath] = true

		rscURL := baseURL + routePath
		result.RSCProbes = append(result.RSCProbes, extractor.RSCProbe{
			URL:     rscURL,
			Headers: map[string]string{"RSC": "1"},
		})
	}

	// 去重 RSCProbes
	if len(result.RSCProbes) > 1 {
		seenURLs := make(map[string]bool)
		var unique []extractor.RSCProbe
		for _, probe := range result.RSCProbes {
			if !seenURLs[probe.URL] {
				seenURLs[probe.URL] = true
				unique = append(unique, probe)
			}
		}
		result.RSCProbes = unique
	}
}

// inferBasePath 从 sourceURL 和内容中的路由段推断 Next.js basePath
// 例如 sourceURL = https://h5.qianshouapp.cn/f/matchmaker/pay
// 路由段为 matchmaker/pay，则 basePath = /f
func (p *NextJSPlugin) inferBasePath(sourceURL string, content string) string {
	// 从 sourceURL 提取路径部分
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	urlPath := parsed.Path

	// 从内容中提取第一个路由的路径段
	for _, match := range p.flightRouteRe.FindAllSubmatch([]byte(content), -1) {
		if len(match) < 2 {
			continue
		}
		segments := string(match[1])
		parts := strings.Split(segments, ",")
		var pathParts []string
		for _, part := range parts {
			part = strings.Trim(part, `" `)
			if part == "" || part == "$undefined" || part == "__PAGE__" || strings.HasPrefix(part, "$") {
				continue
			}
			pathParts = append(pathParts, part)
		}
		if len(pathParts) == 0 {
			continue
		}

		// 构建路由路径
		routePath := "/" + strings.Join(pathParts, "/")
		// 在 urlPath 中找到 routePath 的位置，之前的即为 basePath
		idx := strings.Index(urlPath, routePath)
		if idx >= 0 {
			return urlPath[:idx]
		}
		// 如果精确匹配失败，尝试匹配最后一段
		// 例如 routePath="/matchmaker/pay"，urlPath="/f/matchmaker/pay"
		lastSegment := pathParts[len(pathParts)-1]
		idx = strings.Index(urlPath, "/"+lastSegment)
		if idx > 0 {
			// 回溯检查前面的段是否匹配
			candidate := urlPath[:idx]
			// 验证：routePath 的每一段都应在 urlPath 中
			for _, seg := range pathParts {
				if !strings.Contains(urlPath, "/"+seg) {
					candidate = ""
					break
				}
			}
			if candidate != "" {
				return candidate
			}
		}
		break // 只用第一个匹配的路由
	}

	return ""
}

// extractHTMLLinksForRSC 从 HTML 中的 <a href> 提取同域页面链接，生成 RSC 探测目标
func (p *NextJSPlugin) extractHTMLLinksForRSC(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	parsed, err := url.Parse(input.SourceURL)
	if err != nil {
		return
	}
	baseHost := parsed.Host

	seen := make(map[string]bool)
	// 标记已有的 RSCProbes
	for _, probe := range result.RSCProbes {
		seen[probe.URL] = true
	}

	for _, match := range p.aHrefRe.FindAllSubmatch([]byte(content), -1) {
		if len(match) < 2 {
			continue
		}
		href := string(match[1])

		// 跳过锚点、javascript:、mailto: 等
		if href == "" || href[0] == '#' || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			continue
		}

		// 解析为绝对 URL
		absURL := extractor.ResolveRelativePath(input.SourceURL, href)
		absURL = extractor.NormalizeURL(absURL)

		linkParsed, linkErr := url.Parse(absURL)
		if linkErr != nil {
			continue
		}

		// 只保留同域链接
		if linkParsed.Host != baseHost {
			continue
		}

		// 跳过静态资源链接
		ext := strings.ToLower(linkParsed.Path)
		if strings.HasSuffix(ext, ".js") || strings.HasSuffix(ext, ".css") ||
			strings.HasSuffix(ext, ".png") || strings.HasSuffix(ext, ".jpg") ||
			strings.HasSuffix(ext, ".svg") || strings.HasSuffix(ext, ".ico") ||
			strings.HasSuffix(ext, ".woff") || strings.HasSuffix(ext, ".woff2") ||
			strings.HasSuffix(ext, ".ttf") || strings.HasSuffix(ext, ".eot") {
			continue
		}

		// 跳过 _next/static 路径
		if strings.Contains(linkParsed.Path, "/_next/static") {
			continue
		}

		// 去掉 query 和 fragment
		linkParsed.RawQuery = ""
		linkParsed.Fragment = ""
		cleanURL := linkParsed.String()

		if seen[cleanURL] {
			continue
		}
		seen[cleanURL] = true

		result.RSCProbes = append(result.RSCProbes, extractor.RSCProbe{
			URL:     cleanURL,
			Headers: map[string]string{"RSC": "1"},
		})
	}
}

func (p *NextJSPlugin) analyzeJS(input *extractor.AnalyzeInput, result *extractor.Result, content string) (*extractor.Result, error) {
	// 提取 s.u=e=> 格式的 chunk 映射 (Next.js 特有)
	p.extractNextJSChunks(input, result, content)

	// 提取数字 ID hash 映射 {123:"abc123",456:"def456"}
	p.extractNumChunkIds(input, result, content)

	// 提取 flight chunk 路径: static/chunks/xxx.js
	p.extractFlightChunks(input, result, content)

	// 提取 __webpack_require__.e 动态加载
	p.extractWebpackRequireE(input, result, content)

	// 解析 _buildManifest.js 中的路由和 chunk 映射
	if strings.Contains(content, "__BUILD_MANIFEST") {
		p.extractBuildManifest(input, result, content)
	}

	return result, nil
}

// extractNextJSChunks 提取 Next.js s.u=e=> 格式的 chunk 映射
// 例如: s.u=e=>"static/chunks/"+e+"-"+{123:"abc123"}[e]+".js"
func (p *NextJSPlugin) extractNextJSChunks(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	matches := p.sTypeRe.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		prefix := match[1]  // "static/chunks/"
		hashMap := match[2] // {123:"abc123",456:"def456"}
		suffix := match[3]  // ".js"

		for _, m := range p.numHashMapRe.FindAllStringSubmatch(hashMap, -1) {
			if len(m) < 3 {
				continue
			}
			chunkID := m[1] // "123"
			hash := m[2]    // "abc123"

			chunkPath := prefix + chunkID + "-" + hash + suffix
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, chunkPath)
			absoluteURL = extractor.NormalizeURL(absoluteURL)

			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:      chunkPath,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			}
		}
	}
}

// extractNumChunkIds 提取数字 ID hash 映射
// 例如: {123:"abc123",456:"def456"}
func (p *NextJSPlugin) extractNumChunkIds(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	matches := p.numChunkIdRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		chunkID := match[1]
		hash := match[2]

		if seen[chunkID+":"+hash] {
			continue
		}
		seen[chunkID+":"+hash] = true

		pathVariants := []string{
			"static/chunks/" + chunkID + "." + hash + ".js",
			"static/" + chunkID + "." + hash + ".js",
			"chunks/" + chunkID + "." + hash + ".js",
			chunkID + "." + hash + ".function.chunk.js",
		}

		for _, path := range pathVariants {
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
			absoluteURL = extractor.NormalizeURL(absoluteURL)

			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:      "/" + path,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			}
		}
	}
}

// extractFlightChunks 提取 flight chunk 路径
func (p *NextJSPlugin) extractFlightChunks(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	flightRe := regexp.MustCompile(`(?:^|[^A-Za-z0-9_])(static/(?:immutable/)?chunks/[^"'\\\s<>()\],]+\.js)`)
	for _, match := range flightRe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		path := match[1]
		absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
		absoluteURL = extractor.NormalizeURL(absoluteURL)

		if extractor.IsAbsoluteURL(absoluteURL) {
			result.URLs = append(result.URLs, extractor.DiscoveredJS{
				URL:      absoluteURL,
				FromURL:  input.SourceURL,
				IsInline: false,
			})
		} else {
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     path,
				FromURL: input.SourceURL,
			})
		}
	}
}

// extractWebpackRequireE 提取 __webpack_require__.e 动态加载
func (p *NextJSPlugin) extractWebpackRequireE(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	webpackERe := regexp.MustCompile(`__webpack_require__\.e\s*\(\s*["']([^"']+)["']\s*\)`)
	seen := make(map[string]bool)

	for _, match := range webpackERe.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		chunkID := strings.Trim(match[1], "\"'")
		if seen[chunkID] {
			continue
		}
		seen[chunkID] = true

		result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
			URL:     "__webpack_require_e__" + chunkID,
			FromURL: input.SourceURL,
		})
	}
}

// extractBuildManifest 解析 _buildManifest.js 中的路由和 chunk 映射
// 格式: self.__BUILD_MANIFEST = {"/page":["static/chunks/xxx.js"],...}
func (p *NextJSPlugin) extractBuildManifest(input *extractor.AnalyzeInput, result *extractor.Result, content string) {
	baseURL := extractor.GetBaseURL(input.SourceURL)

	for _, match := range p.manifestPageRe.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 {
			continue
		}
		pagePath := match[1]
		chunkList := match[2]

		// 跳过内部路径
		if strings.HasPrefix(pagePath, "/_") {
			continue
		}
		// 跳过非路由字段
		if !strings.HasPrefix(pagePath, "/") {
			continue
		}

		// 生成 RSC 探测目标
		if baseURL != "" {
			rscURL := baseURL + pagePath
			result.RSCProbes = append(result.RSCProbes, extractor.RSCProbe{
				URL:     rscURL,
				Headers: map[string]string{"RSC": "1"},
			})
		}

		// 提取 chunk 路径
		for _, cm := range p.chunkPathRe.FindAllStringSubmatch(chunkList, -1) {
			if len(cm) < 2 {
				continue
			}
			path := cm[1]
			absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
			absoluteURL = extractor.NormalizeURL(absoluteURL)

			if extractor.IsAbsoluteURL(absoluteURL) {
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			} else {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     path,
					FromURL: input.SourceURL,
				})
			}
		}
	}
}
