package plugins

import (
	"context"
	"regexp"
	"strconv"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// UmiJSPlugin 提取 UmiJS 动态加载的 chunk 文件
// 这种模式常见于使用 umi.js 框架的网站，如 Ant Design Pro
// preload_helper.js 文件包含路由映射和 chunk 文件映射
type UmiJSPlugin struct {
	// 检测 umijs preload helper 特征
	preloadHelperRe *regexp.Regexp
	// publicPath 提取: publicPath:"/"
	publicPathRe *regexp.Regexp
	// 文件映射表提取: f:[["p__Login.94493893.async.js",57],...]
	fileMapRe *regexp.Regexp
	// 路由映射表提取: r:{"/*":[9,25],"/":[8,9,12,...],"/login":[3],...}
	routeMapRe *regexp.Regexp
	// chunk ID 到文件名的映射表
	chunkMapRe *regexp.Regexp
	// 匹配 location.pathname 的使用
	pathnameRe *regexp.Regexp
	// 匹配 {id}:"{name}" 映射 (用于 umi.js 的 name+hash 模式)
	nameMapRe *regexp.Regexp
	// 匹配 {id}:"{hash}" 映射 (8位以上的hex)
	hashMapRe *regexp.Regexp
}

// NewUmiJSPlugin 创建插件
func NewUmiJSPlugin() *UmiJSPlugin {
	return &UmiJSPlugin{
		preloadHelperRe: regexp.MustCompile(`preload_helper|umi\.js`),
		publicPathRe:    regexp.MustCompile(`publicPath["']?\s*:\s*["']([^"']+)["']`),
		// 匹配 f:[["文件名", 索引], ...] 格式的文件映射表
		// 支持嵌套数组: [["name.js", 0], ["name2.js", 1], ...]
		fileMapRe: regexp.MustCompile(`f\s*:\s*(\[\[[^\]]+\](?:\s*,\s*\[[^\]]+\])*\])`),
		// 匹配 r:{"/*":[...], "/":[...], "/login":[...]} 格式的路由映射
		routeMapRe: regexp.MustCompile(`r\s*:\s*\{([^}]+)\}`),
		// 匹配 ["chunkId1", "chunkId2", ...] 格式的 chunk ID 数组
		chunkMapRe: regexp.MustCompile(`\[(\d+)\]`),
		// 匹配 location.pathname
		pathnameRe: regexp.MustCompile(`location\.pathname`),
		// 匹配 {或, 分隔的 {id}:"{name}" 映射
		// 实际格式: {46:"p__h5__bindConfirm",104:"p__sso__logout",...}
		// names contain underscore (p__xxx or xxx__xxx)
		nameMapRe: regexp.MustCompile(`[{,](\d+):"([a-zA-Z0-9]*_[a-zA-Z0-9_]+)"`),
		// 匹配 {或, 分隔的 {id}:"{hash}" 映射 (8位以上的纯hex)
		// 实际格式: {46:"b891323a",71:"fc71ff65",...}
		// hashes are ONLY hex digits, no underscore
		hashMapRe: regexp.MustCompile(`[{,](\d+):"([a-f0-9]{8,})"`),
	}
}

func (p *UmiJSPlugin) Name() string {
	return "UmiJSPlugin"
}

func (p *UmiJSPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("preload_helper"),
		[]byte("publicPath"),
		[]byte("location.pathname"),
		[]byte("p__"),
	})
}

func (p *UmiJSPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	// 1. 提取 publicPath
	publicPath := "/"
	if matches := p.publicPathRe.FindStringSubmatch(content); len(matches) > 1 {
		publicPath = matches[1]
	}
	result.PublicPaths = append(result.PublicPaths, publicPath)

	// 2. 提取文件映射表 f: [["name1.js", 0], ["name2.js", 1], ...]
	// 建立 chunk ID -> 文件名 的映射
	chunkIDToFile := make(map[int]string)
	if fileMapMatch := p.fileMapRe.FindStringSubmatch(content); len(fileMapMatch) > 1 {
		fileMapStr := fileMapMatch[1]
		// 提取所有 ["文件名", 索引] 对
		// 格式: ["name.js", 57] 或 ["p__Login.94493893.async.js", 57]
		fileEntryRe := regexp.MustCompile(`\[?"([^"]+\.js)"?\s*,\s*(\d+)\]?`)
		for _, match := range fileEntryRe.FindAllStringSubmatch(fileMapStr, -1) {
			if len(match) > 2 {
				filename := match[1]
				chunkID := parseInt(match[2])
				chunkIDToFile[chunkID] = filename
			}
		}
	}

	// 3. 提取路由映射表 r: {"/login": [3], "/*": [9, 25], ...}
	if routeMapMatch := p.routeMapRe.FindStringSubmatch(content); len(routeMapMatch) > 1 {
		routeMapStr := routeMapMatch[1]

		// 提取所有路由条目: "/path": [1, 2, 3]
		// 使用更宽松的正则匹配路由映射
		routeEntryRe := regexp.MustCompile(`"([^"]+)"\s*:\s*\[([^\]]+)\]`)
		for _, routeMatch := range routeEntryRe.FindAllStringSubmatch(routeMapStr, -1) {
			if len(routeMatch) > 2 {
				route := routeMatch[1]
				chunkIDsStr := routeMatch[2]

				// 提取 chunk ID 数组
				for _, idMatch := range p.chunkMapRe.FindAllStringSubmatch(chunkIDsStr, -1) {
					if len(idMatch) > 1 {
						chunkID := parseInt(idMatch[1])
						if filename, ok := chunkIDToFile[chunkID]; ok {
							// 根据 publicPath 和文件名构造完整 URL
							url := publicPath + filename
							result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
								URL:      url,
								FromURL:  input.SourceURL,
								IsInline: false,
							})
						}
					}
				}

				// 如果是通配符路由 /*，收集所有 chunk 文件
				if route == "/*" || route == "/" {
					for chunkID, filename := range chunkIDToFile {
						_ = chunkID // 已使用的变量
						url := publicPath + filename
						result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
							URL:      url,
							FromURL:  input.SourceURL,
							IsInline: false,
						})
					}
				}
			}
		}
	}

	// 5. umi.js name+hash 映射模式: ({46:"p__h5__bindConfirm",...}[e]||e)+"."+{46:"b891323a",...}[e]+".async.js"
	// 提取所有 {id}:"{name}" 映射
	idToName := make(map[int]string)
	for _, match := range p.nameMapRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			id := parseInt(match[1])
			name := match[2]
			idToName[id] = name
		}
	}
	// 提取所有 {id}:"{hash}" 映射
	idToHash := make(map[int]string)
	for _, match := range p.hashMapRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			id := parseInt(match[1])
			hash := match[2]
			idToHash[id] = hash
		}
	}
	// 如果找到了映射表，生成所有可能的 chunk URL
	if len(idToName) > 0 && len(idToHash) > 0 {
		generated := make(map[string]bool)
		for id, name := range idToName {
			if hash, ok := idToHash[id]; ok {
				// 格式: {name}.{hash}.async.js
				filename := name + "." + hash + ".async.js"
				if !generated[filename] {
					generated[filename] = true
					url := publicPath + filename
					if !containsProbeTarget(result.ProbeTargets, url) {
						result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
							URL:      url,
							FromURL:  input.SourceURL,
							IsInline: false,
						})
					}
				}
			}
		}
		// 同时生成只有 hash 的情况（如 {id}.{hash}.async.js）
		for id, hash := range idToHash {
			if _, ok := idToName[id]; !ok {
				filename := strconv.Itoa(id) + "." + hash + ".async.js"
				if !generated[filename] {
					generated[filename] = true
					url := publicPath + filename
					if !containsProbeTarget(result.ProbeTargets, url) {
						result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
							URL:      url,
							FromURL:  input.SourceURL,
							IsInline: false,
						})
					}
				}
			}
		}
	}

	// 4. 备用：直接提取所有 f 数组中的文件名（用于没有完整路由映射的情况）
	// 匹配 "p__Login.94493893.async.js" 或 "99.e61fca3c.async.js" 格式
	asyncFileRe := regexp.MustCompile(`"([a-zA-Z0-9_]+\.[a-f0-9]+\.async\.js)"`)
	for _, match := range asyncFileRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			filename := match[1]
			url := publicPath + filename
			// 去重
			if !containsProbeTarget(result.ProbeTargets, url) {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:      url,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	return result, nil
}

// parseInt 简单解析 int
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// containsProbeTarget 检查是否已存在相同的 URL
func containsProbeTarget(targets []extractor.DiscoveredJS, url string) bool {
	for _, t := range targets {
		if t.URL == url {
			return true
		}
	}
	return false
}

