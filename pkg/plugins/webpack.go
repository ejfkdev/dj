package plugins

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// WebpackPlugin 提取 Webpack 相关的动态加载
type WebpackPlugin struct {
	// 检测特征
	webpackMarkerRe *regexp.Regexp
	// publicPath 提取 - 通用模式，不依赖变量名
	// 匹配: X.p="..." 或 X.p="..."+... 形式
	publicPathRe *regexp.Regexp
	// chunk hash map 提取
	chunkHashMapRe *regexp.Regexp
	// 数字 ID hash map (如 bilibili)
	chunkNumericHashRe *regexp.Regexp
	// 箭头函数格式: X.u=Y=>"prefix/"+Y+"-"+{...}[Y]+".suffix"
	lTypeRe *regexp.Regexp
	// 通用 webpack runtime chunk URL 函数: X.u=function(e){...{id:hash}[e]...}
	webpackRuntimeChunkUrlRe *regexp.Regexp
	// webpackChunk_xxx 全局变量注册
	webpackChunkRe *regexp.Regexp
	// 从 push([[id], {...}]) 提取 chunk ID
	chunkPushRe *regexp.Regexp
	// 数字 hash 映射表提取: {10:"ce0ccf4f",209:"d48a70eb",...}
	numericHashMapRe *regexp.Regexp
	// 箭头函数 chunk 路径前缀提取: l.u=e=>"js/"+e+"-"+...+".js"
	arrowFunctionPathPrefixRe *regexp.Regexp
	// rspack/webpack runtime hash mapping: {1124:"hash",...} (数字开头)
	rspackHashMapRe *regexp.Regexp
	// webpackChunk_xxx.push([[id], {...}]) 格式的 chunk 内部注册
	// 匹配: webpackChunk_tencent_docs.push([[449],{85449:function...}])
	webpackChunkSelfRegRe *regexp.Regexp
	// webpackChunk_xxx.push([[id], {...}]) 格式的 chunk hash 映射
	// 匹配: webpackChunk_xxx.push([[449],{...}]) 后面的 {449:{hash}} 映射
	webpackChunkHashMapRe *regexp.Regexp
	// webpack chunk URL 构造函数: u.u=function(t){return t+".{hash}.js"}
	webpackChunkURLBuilderRe *regexp.Regexp
	// Webpack 5 chunk URL 函数: function(e){return""+e+"."+{id:hash}[e]+".js"}
	webpackChunkURLBuilderV5Re *regexp.Regexp
	// 匹配 Webpack chunk 动态加载调用: .e(449) 或 .e(449).then(...)
	webpackChunkLoadCallRe *regexp.Regexp
	// 匹配字符串 chunk ID 的动态加载: xxx("chunk-xxx") 或 xxx("chunk-xxx").then(...)
	webpackChunkLoadStringIdRe *regexp.Regexp
	// 字符串 chunk ID -> hash 映射提取: "chunk-xxx":"hash"
	stringChunkHashMapRe *regexp.Regexp
	// 从 webpack runtime 提取路径前缀: (__webpack_require__.p||"")+"static/js/"
	webpackPathPrefixRe *regexp.Regexp
	// RuoYi-Vue 格式: t.p+"static/js/" 路径前缀提取
	// 备用路径前缀正则（用于 X.p+"path/" 格式）
	webpackPathPrefixAltRe *regexp.Regexp
	// 通用 publicPath 赋值: X.p="..." (任意变量名)
	webpackPublicPathAssignRe *regexp.Regexp
	// webpack runtime 中 "prefix" + e + "-" + {...}[e] + ".suffix" 模式
	webpackChunkMapPatternRe *regexp.Regexp
	// h.u=e=>"prefix/"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js" 模式
	// 匹配: "prefix"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js"
	webpackStaticChunkPatternRe *regexp.Regexp
	// Module Federation 格式: e===id?"path.js":"prefix/"+({name}[e]||e)+"."+{hash}[e]+".js"
	// 匹配三元表达式后面的通用 chunk 映射模式
	webpackFederationChunkPatternRe *regexp.Regexp
	// __webpack_require__.u 函数中的 chunk URL 映射模式
	// 匹配: "prefix/"+({name}[e]||e)+"."+{hash}[e]+".js"
	webpackRequireUChunkPatternRe *regexp.Regexp
	// a.u=function(c){return"js/chunk/"+{name:path}[c]+"."+{id:hash}[c]+".js"} 格式
	// 匹配: "js/chunk/"+{1:"icons/xxx",...}[c]+"."+{1:"hash",...}[c]+".js"
	webpackChunkReturnPatternRe *regexp.Regexp

	// 备用路径前缀正则（避免循环内重新编译）
	fallbackRe *regexp.Regexp
	// ID -> value 正则
	idValueRe *regexp.Regexp
	// js/ 前缀正则
	jsPrefixRe *regexp.Regexp
	// 查询参数后缀正则
	querySuffixRe *regexp.Regexp

	// 字符串 chunk ID -> hash 映射表（跨 JS 共享）
	stringChunkHashMaps []map[string]string
}

// NewWebpackPlugin 创建插件
func NewWebpackPlugin() *WebpackPlugin {
	return &WebpackPlugin{
		webpackMarkerRe:      regexp.MustCompile(`__webpack_require__|__webpack_exports__|webpackJsonp`),
		publicPathRe:         regexp.MustCompile(`(?:__webpack_require__\.\w+|window\.__webpack_public_path__|window\.resourceBaseUrl|a\.p)\s*=\s*["']([^"']+)["']`),
		chunkHashMapRe:       regexp.MustCompile(`"(chunk-[0-9a-f]+)"\s*:\s*"([0-9a-f]+)"`),
		chunkNumericHashRe:   regexp.MustCompile(`[\{,]\s*(\d+)\s*:\s*["']([a-f0-9]{20,24})["']`),
		// 匹配箭头函数格式: X.u=Y=>"prefix/"+Y+"-"+{...}[Y]+".suffix"
		lTypeRe:              regexp.MustCompile(`\.\w+\s*=\s*\w+\s*=>\s*"([^"]+)"\s*\+\s*\w+\s*\+\s*"([^"]+)"\s*\+\s*\{([^}]+)\}\[\w+\]\s*\+\s*"([^"]+)"`),
		// 匹配 webpack runtime chunk URL 函数: X.u=function(Y){...或 X.u=Y=>...
		webpackRuntimeChunkUrlRe: regexp.MustCompile(`\.\w+\s*=\s*(?:function\s*\([^)]*\)|\w+\s*=>)\s*[^}]+\{\s*[^}]*\[\s*[^\]]*\]\s*[^}]*\}`),
		webpackChunkRe:       regexp.MustCompile(`webpackChunk_([a-zA-Z_][a-zA-Z0-9_]*)\s*=`),
		chunkPushRe:          regexp.MustCompile(`\.\s*push\s*\(\s*\[\s*\[\s*(\d+)\s*\]`),
		// 匹配数字 hash 映射表中的单个条目: 10:"ce0ccf4f" 或 209:"d48a70eb"
		numericHashMapRe:      regexp.MustCompile(`(\d+):"([a-f0-9]{6,10})"`),
		// 匹配箭头函数 chunk 路径前缀: X.u=Y=>"js/"+Y+"-"+...+".js"
		arrowFunctionPathPrefixRe: regexp.MustCompile(`\.\w+\s*=\s*\w+\s*=>\s*"([^"]+)"\s*\+\s*\w+\s*\+\s*"-"`),
		// 匹配 rspack/webpack runtime hash mapping: {1124:"7ca513dc",...} (数字作为 key)
		rspackHashMapRe:     regexp.MustCompile(`\{(\d+):"([a-f0-9]{5,40})"`),
		// 匹配 webpackChunk_xxx.push([[id], {...}]) 的 chunk ID 和 hash
		// 格式: webpackChunk_xxx.push([[449],{85449:function(e,t,n){"use strict"...}])
		// 或带 hash 的: webpackChunk_xxx.push([[449],{...}]) 后续有 {449:"hash"}
		webpackChunkSelfRegRe: regexp.MustCompile(`webpackChunk_([a-zA-Z_][a-zA-Z0-9_]*)\s*\.\s*push\s*\(\s*\[\s*\[\s*(\d+)\s*\]`),
		// 匹配 webpackChunk 内部定义的 {id:"hash"} 映射
		webpackChunkHashMapRe: regexp.MustCompile(`\{(\d+):"([a-f0-9]{7,40})"\}`),
		// 匹配 webpack chunk URL 构造函数: X.u=function(Y){return Y+".{hash}.js"}
		// X.u=function(Y){return Y+".95f73b75.js"} 格式
		webpackChunkURLBuilderRe: regexp.MustCompile(`\.\w+\s*=\s*function\s*\(\w+\)\s*\{\s*return\s*\w+\s*\+\s*"\.([[:alnum:]]+)\.js"\s*\}`),
		// 匹配 Webpack 5 chunk URL 函数: function(X){return""+X+"."+{id:hash}[X]+".js"}
		webpackChunkURLBuilderV5Re: regexp.MustCompile(`\.\w+\s*=\s*function\s*\(\s*\w+\s*\)\s*\{\s*return\s*[^}]+\+"\.([[:alnum:]]+)\.js"\s*\}`),
		// 匹配 Webpack chunk 动态加载调用: .X(449) 或 .X(449).then(...)
		webpackChunkLoadCallRe: regexp.MustCompile(`\.\w+\s*\(\s*(\d+)\s*\)`),
		// 匹配字符串 chunk ID 的动态加载: xxx("chunk-xxx") 或 xxx("chunk-xxx").then(...)
		// 例如: n.e("chunk-2d0b2b28") - 变量名任意，只要匹配 chunk-{id} 格式
		webpackChunkLoadStringIdRe: regexp.MustCompile(`\.(\w+)\s*\(\s*["'](chunk-[a-z0-9]+)["']\s*\)`),
		// 匹配字符串 chunk ID -> hash 映射: "chunk-xxx":"hash"
		// 例如: {"chunk-2d0b2b28":"6267aaf1","chunk-0abfe318":"3e5e9dc2",...}
		stringChunkHashMapRe: regexp.MustCompile(`"(chunk-[a-z0-9]+)"\s*:\s*"([a-f0-9]{5,10})"`),
		// 匹配 webpack runtime 中的路径前缀: (X.Y||"")+"static/js/"
		// 提取 + 号后面的路径部分，如 "static/js/" 或 "js/"
		webpackPathPrefixRe: regexp.MustCompile(`\(\s*\w+\.\w+\s*\|\|\s*""\s*\)\s*\+\s*["']([^"']+/)["']`),
		// RuoYi-Vue 格式: X.p+"static/js/" 路径前缀提取（不依赖变量名）
		// 匹配: +"static/js/" 或 +"js/" 等路径拼接模式
		// 备用路径前缀正则（用于 X.p+"path/" 格式）
		webpackPathPrefixAltRe: regexp.MustCompile(`\+\s*["'](/?static/js/)["']`),
		// 通用 publicPath 赋值: X.Y="..." 或 X.Y="..."+... 形式 (不依赖变量名)
		webpackPublicPathAssignRe: regexp.MustCompile(`\.\w+\s*=\s*(?:function\s*\([^)]*\)\s*\{[^}]*\}|"[^"]*"|[^;,]+)`),
		// webpack runtime 中 "prefix" + X + "-" + {...}[X] + ".suffix" 模式
		// 匹配: "js/"+e+"-"+{...}[e]+".js"
		webpackChunkMapPatternRe: regexp.MustCompile(`"([^"]+)"\s*\+\s*\w+\s*\+\s*"([^"]+)"\s*\+\s*\{([^}]+)\}\[\w+\]\s*\+\s*"([^"]+)"`),
		// h.u=e=>"prefix/"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js" 模式
		// 匹配: "prefix"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js"
		webpackStaticChunkPatternRe: regexp.MustCompile(`"([^"]+/)"\s*\+\s*\(\s*\(([^)]+)\)\[\w+\]\s*\|\|\s*\w+\s*\)\s*\+\s*"\."\s*\+\s*\(([^)]+)\)\[\w+\]\s*\+\s*"\.js"`),
		// a.u=function(c){return"js/chunk/"+{name:path}[c]+"."+{id:hash}[c]+".js"} 格式
		// 匹配: "prefix/"+{id:"name"}[c]+"."+{id:"hash"}[c]+".js"
		webpackChunkReturnPatternRe: regexp.MustCompile(`"([^"]+/)"\s*\+\s*\{[^}]+\}\[\w+\]\s*\+\s*"\."\s*\+\s*\{[^}]+\}\[\w+\]\s*\+\s*"\.js"`),
		// ({name}[e]||e)+"."+{hash}[e]+".js" 格式
		// 匹配: ({812:"name",...}[e]||e)+"."+{236:"hash",...}[e]+".js"
		webpackFederationChunkPatternRe: regexp.MustCompile(`\(\s*\{[^}]+\}\[\w+\]\s*\|\|\s*\w+\s*\)\s*\+\s*"\."\s*\+\s*\{[^}]+\}\[\w+\]\s*\+\s*"\.js"`),
		// __webpack_require__.u 函数中的 chunk URL 映射模式
		// 匹配: "prefix/"+({name}[e]||e)+"."+{hash}[e]+".js"
		webpackRequireUChunkPatternRe: regexp.MustCompile(`"([^"]+/)"\s*\+\s*\(\s*\{[^}]+\}\[\w+\]\s*\|\|\s*\w+\s*\)\s*\+\s*"\."\s*\+\s*\{[^}]+\}\[\w+\]\s*\+\s*"\.js"`),
		// 备用路径前缀正则
		fallbackRe: regexp.MustCompile(`\+\s*["']([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+/)["']`),
		// ID -> value 正则
		idValueRe: regexp.MustCompile(`(\d+):"([^"]+)"`),
		// js/ 前缀正则
		jsPrefixRe: regexp.MustCompile(`\+\s*["'](\w+/)["']`),
		// 查询参数后缀正则
		querySuffixRe: regexp.MustCompile(`\.js\?([a-zA-Z0-9_=]+)"`),
	}
}

func (p *WebpackPlugin) Name() string {
	return "WebpackPlugin"
}

func (p *WebpackPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	if input.ContentType != extractor.ContentTypeJS {
		return false
	}
	return bytesContainsAny(input.Content, [][]byte{
		[]byte("__webpack_require__"),
		[]byte("webpackJsonp"),
		[]byte("chunk-"),
		[]byte("webpackChunk"),
		[]byte("__webpack_public_path__"),
		[]byte("resourceBaseUrl"),
		[]byte("webpackChunk_"),
	})
}

func (p *WebpackPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	// 提取 publicPath
	if matches := p.publicPathRe.FindStringSubmatch(content); len(matches) > 1 {
		result.PublicPaths = append(result.PublicPaths, matches[1])
	}

	// 检查通用 webpack runtime chunk URL 映射模式
	// 模式: "prefix"+e+"-"+{id:hash}[e]+".suffix"
	// 例如: l.u=e=>"js/"+e+"-"+{10:"ce0cc4f",...}[e]+".js"
	// 或: X.u=function(e){return"prefix/"+e+"-"+{id:hash}[e]+".suffix"}
	if chunkMapMatch := p.webpackChunkMapPatternRe.FindStringSubmatch(content); len(chunkMapMatch) > 4 {
		prefix := chunkMapMatch[1]  // "js/"
		suffix := chunkMapMatch[4]  // ".js"
		hashMapStr := chunkMapMatch[3] // {10:"ce0cc4f",...}

		// 从 hashMapStr 中提取所有 id:hash 映射
		hashMap := make(map[string]string)
		for _, m := range p.numericHashMapRe.FindAllStringSubmatch(hashMapStr, -1) {
			if len(m) > 2 {
				hashMap[m[1]] = m[2]
			}
		}

		// 如果 hashMap 中没有足够的条目，尝试从整个文件中提取
		if len(hashMap) < 3 {
			for _, m := range p.numericHashMapRe.FindAllStringSubmatch(content, -1) {
				if len(m) > 2 {
					hashMap[m[1]] = m[2]
				}
			}
		}

		// 生成所有 chunk URL
		// 格式: prefix + chunkID + "-" + hash + suffix
		for chunkID, hash := range hashMap {
			chunkPath := prefix + chunkID + "-" + hash + suffix
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     chunkPath,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
		// 通用 webpack runtime chunk 映射已处理
		return result, nil
	}

	// 检查 h.u=e=>"prefix/"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js" 模式
	// 格式: "prefix"+((nameMap)[e]||e)+"."+(hashMap)[e]+".js"
	// 生成: prefix/name.hash.js 或 prefix/id.hash.js
	if staticMatch := p.webpackStaticChunkPatternRe.FindStringSubmatch(content); len(staticMatch) > 3 {
		prefix := staticMatch[1]        // 路径前缀，如 "static/"
		nameMapVar := staticMatch[2]   // name 映射表变量名
		hashMapVar := staticMatch[3]   // hash 映射表变量名

		// 从整个内容中搜索 nameMap 和 hashMap 的定义
		// 格式可能是: var xxx={...} 或 xxx={...}
		nameMap := make(map[string]string)
		hashMap := make(map[string]string)

		// 构造搜索模式: 变量名 = {...}
		nameMapDefRe := regexp.MustCompile(nameMapVar + `\s*=\s*\{([^}]+)\}`)
		hashMapDefRe := regexp.MustCompile(hashMapVar + `\s*=\s*\{([^}]+)\}`)

		// 提取 nameMap 定义
		if nameMapMatch := nameMapDefRe.FindStringSubmatch(content); len(nameMapMatch) > 1 {
			for _, m := range p.idValueRe.FindAllStringSubmatch(nameMapMatch[1], -1) {
				if len(m) > 2 {
					nameMap[m[1]] = m[2]
				}
			}
		}

		// 提取 hashMap 定义
		if hashMapMatch := hashMapDefRe.FindStringSubmatch(content); len(hashMapMatch) > 1 {
			for _, m := range p.idValueRe.FindAllStringSubmatch(hashMapMatch[1], -1) {
				if len(m) > 2 {
					hashMap[m[1]] = m[2]
				}
			}
		}

		// 如果映射表条目少于 3，从整个内容中提取所有数字到值的映射
		if len(hashMap) < 3 {
			hashMap = make(map[string]string)
			for _, m := range p.idValueRe.FindAllStringSubmatch(content, -1) {
				if len(m) > 2 {
					id := m[1]
					val := m[2]
					// 检查是否是 hash (纯 hex，长度合适)
					if len(val) >= 6 && len(val) <= 40 {
						isHex := true
						for _, c := range val {
							if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
								isHex = false
								break
							}
						}
						if isHex {
							hashMap[id] = val
						}
					}
				}
			}
		}

		// 生成 chunk URL
		for id, hash := range hashMap {
			var chunkPath string
			if name, ok := nameMap[id]; ok {
				chunkPath = prefix + name + "." + hash + ".js"
			} else {
				chunkPath = prefix + id + "." + hash + ".js"
			}
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     chunkPath,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
		return result, nil
	}

	// a.u=function(c){return"js/chunk/"+{name:path}[c]+"."+{id:hash}[c]+".js"} 格式
	// 生成: js/chunk/icons/File_Word.307a7e9e693993b38d7f.js
	if match := p.webpackChunkReturnPatternRe.FindStringSubmatch(content); len(match) > 1 {
		prefix := match[1] // "js/chunk/"

		// 提取所有 {id:"value"} 映射
		allMappings := p.idValueRe.FindAllStringSubmatch(content, -1)

		// 分离 name 和 hash 映射
		nameMap := make(map[string]string)
		hashMap := make(map[string]string)
		isHex := func(s string) bool {
			for _, c := range s {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					return false
				}
			}
			return true
		}
		for _, m := range allMappings {
			if len(m) > 2 {
				id := m[1]
				val := m[2]
				if isHex(val) {
					hashMap[id] = val
				} else {
					nameMap[id] = val
				}
			}
		}

		// 生成 chunk URL: prefix + name + "." + hash + ".js"
		for id, hash := range hashMap {
			var chunkPath string
			if name, ok := nameMap[id]; ok {
				chunkPath = prefix + name + "." + hash + ".js"
			} else {
				chunkPath = prefix + id + "." + hash + ".js"
			}
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     chunkPath,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
		return result, nil
	}

	// __webpack_require__.u 函数中的 chunk URL 映射模式
	// 匹配: "prefix/"+({name}[e]||e)+"."+{hash}[e]+".js"
	if match := p.webpackRequireUChunkPatternRe.FindStringSubmatch(content); len(match) > 1 {
		prefix := match[1] // "static/js/async/"

		// 提取所有 {id:"value"} 映射
		allMappings := p.idValueRe.FindAllStringSubmatch(content, -1)

		// 分离 name 和 hash 映射
		nameMap := make(map[string]string)
		hashMap := make(map[string]string)
		isHex := func(s string) bool {
			for _, c := range s {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					return false
				}
			}
			return true
		}
		for _, m := range allMappings {
			if len(m) > 2 {
				id := m[1]
				val := m[2]
				if isHex(val) {
					hashMap[id] = val
				} else {
					nameMap[id] = val
				}
			}
		}

		// 生成 chunk URL: prefix + name + "." + hash + ".js"
		for id, hash := range hashMap {
			var chunkPath string
			if name, ok := nameMap[id]; ok {
				chunkPath = prefix + name + "." + hash + ".js"
			} else {
				chunkPath = prefix + id + "." + hash + ".js"
			}
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     chunkPath,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}

		// 提取 __webpack_require__.u 函数中的直接路径映射
		// 匹配: ===id?"path.js" 模式（如 740===e?"path.js"）
		directPathRe := regexp.MustCompile(`(\d+)===e\?"([^"]+\.js)"`)
		for _, dm := range directPathRe.FindAllStringSubmatch(content, -1) {
			if len(dm) > 2 {
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     dm[2],
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}

		return result, nil
	}

	// ({name}[e]||e)+"."+{hash}[e]+".js" 格式（无前缀版本）
	if match := p.webpackFederationChunkPatternRe.FindStringSubmatch(content); len(match) > 0 {
		// 提取前缀 - 查找匹配部分之前的 "prefix/" 模式
		prefix := ""
		idx := strings.Index(content, match[0])
		if idx > 0 {
			// 提取前缀：查找 "..."+ 模式
			prefixRe := regexp.MustCompile(`"([^"]+/)"\s*\+\s*$`)
			before := content[:idx]
			prefixMatches := prefixRe.FindAllStringSubmatch(before, -1)
			if len(prefixMatches) > 0 {
				prefix = prefixMatches[len(prefixMatches)-1][1]
			}
		}
		if prefix == "" {
			prefix = "static/js/async/" // 默认前缀
		}

		// 提取所有 {id:"value"} 映射
		allMappings := p.idValueRe.FindAllStringSubmatch(content, -1)

		// 分离 name 和 hash 映射
		nameMap := make(map[string]string)
		hashMap := make(map[string]string)
		isHex := func(s string) bool {
			for _, c := range s {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					return false
				}
			}
			return true
		}
		for _, m := range allMappings {
			if len(m) > 2 {
				id := m[1]
				val := m[2]
				if isHex(val) {
					hashMap[id] = val
				} else {
					nameMap[id] = val
				}
			}
		}

		// 生成 chunk URL: prefix + name + "." + hash + ".js"
		for id, hash := range hashMap {
			var chunkPath string
			if name, ok := nameMap[id]; ok {
				chunkPath = prefix + name + "." + hash + ".js"
			} else {
				chunkPath = prefix + id + "." + hash + ".js"
			}
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     chunkPath,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
		return result, nil
	}

	// 备用：检查箭头函数格式
	var arrowFnPrefix string
	if prefixMatch := p.arrowFunctionPathPrefixRe.FindStringSubmatch(content); len(prefixMatch) > 1 {
		arrowFnPrefix = prefixMatch[1]
	}

	if arrowFnPrefix != "" {
		// 提取所有 {id:"hash"} 映射
		for _, hashMatch := range p.numericHashMapRe.FindAllStringSubmatch(content, -1) {
			if len(hashMatch) > 2 {
				chunkID := hashMatch[1]
				hash := hashMatch[2]
				chunkPath := arrowFnPrefix + chunkID + "-" + hash + ".js"
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     chunkPath,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
		return result, nil
	}

	// 以下是通用格式处理

	// 提取路径前缀
	// 典型模式: (__webpack_require__.p||"")+"static/js/"+({}[c]||c)+"."+{...}[c]+".js"
	// 从 webpack runtime 代码中动态提取路径前缀
	pathPrefix := ""
	if match := p.webpackPathPrefixRe.FindStringSubmatch(content); len(match) > 1 {
		pathPrefix = match[1]
	}

	// 备用：如果没找到，尝试 RuoYi-Vue 格式: X.p+"static/js/"
	if pathPrefix == "" {
		if match := p.webpackPathPrefixAltRe.FindStringSubmatch(content); len(match) > 1 {
			pathPrefix = match[1]
		}
	}

	// 备用：如果没找到，尝试提取任何看起来像路径的引号字符串
	if pathPrefix == "" {
		// 匹配 + "path/" 模式，提取路径部分
		// 例如: +"static/js/" 或 +"js/"
		if matches := p.fallbackRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
			for _, m := range matches {
				if len(m) > 1 {
					candidate := m[1]
					// 过滤掉明显不是路径的字符串
					if candidate != "use " && !strings.HasPrefix(candidate, "use ") {
						pathPrefix = candidate
						break
					}
				}
			}
		}
	}

	// 提取 chunk hash map (chunk-xxx: "hash")
	for _, match := range p.chunkHashMapRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			chunkID := match[1]
			hash := match[2]
			// 添加 pathPrefix 以匹配 webpack runtime 的实际路径格式
			fragment := pathPrefix + chunkID + "." + hash + ".js"
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取数字 ID hash map (rspack/webpack runtime 格式，如 ChatGLM、bilibili)
	// 格式: d.u=function(e){return""+({name_mapping})[e]||e+"."+{id:hash}[e]+".js"}
	// 生成: {name}.{hash}.js 或 {id}.{hash}.js
	// 提取 name mapping 和 hash mapping
	// 格式: {1:"common",3:"album",...} 或 {1:"5baf607c7074b3816f02",3:"...",...}
	// 用简单的正则匹配所有 id:"value" 对
	nameMap := make(map[string]string)
	isHex := func(s string) bool {
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
		return true
	}
	for _, match := range p.idValueRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			id := match[1]
			value := match[2]
			// 如果值包含非 hex 字符，则是 name（如 "index"、"common"）
			// 如果值是纯 hex，则是 hash
			if !isHex(value) {
				nameMap[id] = value
			}
		}
	}

	// 提取 js/ 前缀 (来自 + "js/" 模式)
	jsPrefix := ""
	if matches := p.jsPrefixRe.FindStringSubmatch(content); len(matches) > 1 {
		jsPrefix = matches[1]
	}

	// 提取查询参数 (来自 .js?max_age=... 模式，在字符串拼接中)
	querySuffix := ""
	if matches := p.querySuffixRe.FindStringSubmatch(content); len(matches) > 1 {
		querySuffix = matches[1]
	}

	// 再提取 hash mapping 并生成 chunk URL
	for _, match := range p.chunkNumericHashRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			chunkID := match[1]
			hash := match[2]
			// 如果有 name mapping，使用 name.hash.js 格式
			var fragment string
			if name, ok := nameMap[chunkID]; ok {
				fragment = name + ".chunk." + hash + ".js"
			} else {
				fragment = chunkID + ".chunk." + hash + ".js"
			}
			// 添加 js/ 前缀和查询参数
			if jsPrefix != "" {
				fragment = jsPrefix + fragment
			}
			if querySuffix != "" {
				fragment = fragment + "?" + querySuffix
			}
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取箭头函数格式
	for _, match := range p.lTypeRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 3 {
			prefix := match[1]
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     prefix + "_placeholder_.js",
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 webpackChunk_xxx.push([[id], {...}]) 中的 chunk ID（通用）
	for _, match := range p.chunkPushRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			chunkID := match[1]
			// 生成探测目标: {chunkID}-*.js
			fragment := chunkID + "-*.js"
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 webpackChunk_xxx.push([[id], {...}]) 格式的 chunk 自注册模式
	// 例如: webpackChunk_tencent_docs_component_official_footer.push([[449],{85449:function...}])
	// 这种模式下，chunk URL 格式为 {chunkId}.{hash}.js
	// 先收集所有 webpackChunk 全局变量名
	webpackChunkNames := make(map[string]bool)
	for _, match := range p.webpackChunkSelfRegRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			webpackChunkNames[match[1]] = true
		}
	}

	// 如果检测到 webpackChunk_xxx 全局变量，提取关联的 hash 映射
	if len(webpackChunkNames) > 0 {
		// 提取 {id:"hash"} 映射 (hash 长度 7-10 位)
		chunkHashMap := make(map[string]string)
		for _, match := range p.webpackChunkHashMapRe.FindAllStringSubmatch(content, -1) {
			if len(match) > 2 {
				chunkID := match[1]
				hash := match[2]
				chunkHashMap[chunkID] = hash
			}
		}

		// 生成探测目标: {chunkId}.{hash}.js
		for chunkID, hash := range chunkHashMap {
			fragment := chunkID + "." + hash + ".js"
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	}

	// 提取 webpack chunk URL 构造函数中的 hash
	// 模式: u.u=function(t){return t+".{hash}.js"}
	// 例如: u.u=function(t){return t+".95f73b75.js"}
	var chunkHash string
	if match := p.webpackChunkURLBuilderRe.FindStringSubmatch(content); len(match) > 1 {
		chunkHash = match[1]
	}

	// 提取 .e(chunkId) 形式的 chunk 动态加载调用
	// 例如: .e(449) 或 .e(449).then(...)
	// 如果同时找到 hash 和 chunk ID，生成完整的 chunk URL
	for _, callMatch := range p.webpackChunkLoadCallRe.FindAllStringSubmatch(content, -1) {
		if len(callMatch) > 1 {
			chunkID := callMatch[1]
			if chunkHash != "" {
				fragment := chunkID + "." + chunkHash + ".js"
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     fragment,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			} else {
				// 只有 chunk ID，没有 hash，生成带通配符的探测目标
				fragment := chunkID + "-*.js"
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     fragment,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	// 提取字符串 chunk ID -> hash 映射
	// 例如: {"chunk-2d0b2b28":"6267aaf1","chunk-0abfe318":"3e5e9dc2",...}
	// 先从插件自己的 stringChunkHashMaps 获取已存储的 hash 映射
	stringChunkHashMap := make(map[string]string)
	for _, storedMap := range p.stringChunkHashMaps {
		for k, v := range storedMap {
			stringChunkHashMap[k] = v
		}
	}
	// 从当前内容提取 hash 映射
	// 注意：同一个 chunk ID 可能出现多次（JS hash 和 CSS hash），我们应该保留第一个
	hashMapMatches := p.stringChunkHashMapRe.FindAllStringSubmatch(content, -1)
	for _, match := range hashMapMatches {
		if len(match) > 2 {
			chunkID := match[1]  // "chunk-2d0b2b28"
			hash := match[2]      // "6267aaf1"
			// 如果已经存在 hash，不要覆盖它（第一个出现的才是 JS hash）
			if _, exists := stringChunkHashMap[chunkID]; exists {
				continue
			}
			stringChunkHashMap[chunkID] = hash
		}
	}
	// 将当前内容的 hash 映射存储到插件自己的 map
	if len(stringChunkHashMap) > 0 {
		p.stringChunkHashMaps = append(p.stringChunkHashMaps, stringChunkHashMap)
	}


	// 如果有 hash 映射表，生成所有 chunk 的探测目标
	// 这是 webpack runtime 的标准模式：hash 映射表在 inline script 或主 JS 中
	// 生成格式: static/js/{chunkId}.{hash}.js
	if len(stringChunkHashMap) > 0 {
		// 用于去重，避免与原有逻辑生成的 ProbeTargets 重复
		generatedChunks := make(map[string]bool)
		for chunkID, hash := range stringChunkHashMap {
			// 构造成完整的 chunk 路径: static/js/chunk-xxx.hash.js 或 chunk-xxx.hash.js
			fragment := pathPrefix + chunkID + "." + hash + ".js"
			if generatedChunks[fragment] {
				continue
			}
			generatedChunks[fragment] = true
			result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
				URL:     fragment,
				FromURL: input.SourceURL,
				IsInline: false,
			})
		}
	} else {
		// 备用逻辑：如果没有 hash 映射表，提取字符串 chunk ID 的动态加载调用
		// 例如: n.e("chunk-2d0b2b28") - 匹配任意函数名 + chunk-{id} 格式
		seenChunks := make(map[string]bool)
		for _, callMatch := range p.webpackChunkLoadStringIdRe.FindAllStringSubmatch(content, -1) {
			if len(callMatch) > 2 {
				funcName := callMatch[1]  // 函数名，如 "e"
				chunkID := callMatch[2]   // chunk ID，如 "chunk-2d0b2b28"
				_ = funcName // 函数名不用于 chunk URL 构建

				if seenChunks[chunkID] {
					continue
				}
				seenChunks[chunkID] = true

				// 判断 chunkID 是否已经是完整的 chunk 文件名格式
				// chunk-{8+字符hex} 格式如 chunk-2d0b2b28 可能是完整文件名
				// 需要判断是否像 hash（8位以上hex）而不是模块 ID
				var fragment string
				if isLikelyChunkHash(chunkID) {
					// chunk-{hash} 格式，可能是完整文件名
					fragment = pathPrefix + chunkID + ".js"
				} else {
					// 普通模块 ID，生成带通配符的探测目标
					fragment = pathPrefix + chunkID + "-*.js"
				}
				result.ProbeTargets = append(result.ProbeTargets, extractor.DiscoveredJS{
					URL:     fragment,
					FromURL: input.SourceURL,
					IsInline: false,
				})
			}
		}
	}

	return result, nil
}

// bytesContainsAny 检查 content 是否包含 any of the needles
func bytesContainsAny(content []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if bytes.Contains(content, needle) {
			return true
		}
	}
	return false
}

// isLikelyChunkHash 判断 chunk ID 是否看起来像完整的 chunk hash
// 例如: chunk-2d0b2b28 看起来像完整的文件名（chunk-{hash}.js）
// 而: chunk-vendor 只是一个通用的模块名
func isLikelyChunkHash(chunkID string) bool {
	// chunk-{8位以上hex} 格式通常是完整的 chunk 文件名
	// 例如: chunk-2d0b2b28, chunk-abc123def
	if strings.HasPrefix(chunkID, "chunk-") && len(chunkID) > len("chunk-")+7 {
		hashPart := strings.TrimPrefix(chunkID, "chunk-")
		// 检查是否主要是 hex 字符
		hexCount := 0
		for _, c := range hashPart {
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				hexCount++
			}
		}
		// 如果超过70%的字符是hex，认为是hash
		return float64(hexCount)/float64(len(hashPart)) > 0.7
	}
	return false
}
