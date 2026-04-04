package plugins

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// ModuleFederationManifestPlugin 专门处理 Module Federation manifest
// 从 JS 代码中提取 baseHost + "xxx/manifest.json" 模式并解析获取 chunk 列表
type ModuleFederationManifestPlugin struct {
	// 匹配 baseHost + "/xxx/xxx-manifest.json" 模式
	manifestPathRe *regexp.Regexp
	// 匹配 manifest JSON 中的 publicPath
	publicPathRe *regexp.Regexp
	// 匹配 manifest JSON 中的 region 映射
	regionRe *regexp.Regexp
	// 匹配 js.sync 和 js.async 数组中的 chunk 路径
	chunkPathRe *regexp.Regexp
}

// NewModuleFederationManifestPlugin 创建插件
func NewModuleFederationManifestPlugin() *ModuleFederationManifestPlugin {
	return &ModuleFederationManifestPlugin{
		// 匹配 baseHost + "/xxx/xxx-manifest.json" 或直接是 manifest 路径
		manifestPathRe: regexp.MustCompile(`baseHost\s*\+\s*["']([^"']+manifest\.json)["']`),
		// 匹配 publicPath: "//__CDN_PREFIX__/path/"
		publicPathRe: regexp.MustCompile(`"publicPath"\s*:\s*"//[^"]+/([^"]+/)"`),
		// 匹配 region: { "cn": "domain/path" }
		regionRe: regexp.MustCompile(`"cn"\s*:\s*"([^"]+)"`),
		// 匹配 "static/js/async/xxx.js" 等 chunk 路径
		chunkPathRe: regexp.MustCompile(`"(static/js/[^"]+\.js)"`),
	}
}

func (p *ModuleFederationManifestPlugin) Name() string {
	return "ModuleFederationManifestPlugin"
}

func (p *ModuleFederationManifestPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	content := string(input.Content)

	// 处理 JSON content（manifest 被下载后的解析）
	if input.ContentType == extractor.ContentTypeJSON {
		return strings.Contains(content, `"metaData"`) &&
			(strings.Contains(content, `"publicPath"`) || strings.Contains(content, `"region"`))
	}

	// 处理 JS content（从 JS 中提取 manifest URL）
	if input.ContentType == extractor.ContentTypeJS {
		// 检查是否有 baseHost + "xxx/xxx-manifest.json" 模式
		// 支持任意 *manifest*.json 路径
		return p.manifestPathRe.MatchString(content)
	}

	return false
}

func (p *ModuleFederationManifestPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}
	content := string(input.Content)

	// 处理 JSON content - 直接解析 manifest
	if input.ContentType == extractor.ContentTypeJSON {
		manifestResult, err := ParseVmokManifest(input.Content)
		if err != nil || len(manifestResult.Chunks) == 0 {
			return result, nil
		}

		// 使用 manifest 中的 CDN 和 publicPath 构造 chunk URL
		chunkURLs := ConstructVmokChunkURLsFromManifest(manifestResult)
		result.URLs = append(result.URLs, chunkURLs...)
		return result, nil
	}

	// 处理 JS content - 提取 manifest URL
	// 提取 manifest URL 路径
	var manifestPath string
	for _, match := range p.manifestPathRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			manifestPath = match[1]
			break
		}
	}

	if manifestPath == "" {
		// 备用：从内容中查找任何 manifest.json 引用
		manifestRe := regexp.MustCompile(`["']([^"']*manifest\.json)["']`)
		for _, match := range manifestRe.FindAllStringSubmatch(content, -1) {
			if len(match) > 1 {
				manifestPath = match[1]
				break
			}
		}
	}

	if manifestPath == "" {
		return result, nil
	}

	// 提取 baseHost
	baseHost := p.extractBaseHost(content)

	// 构造完整的 manifest URL
	var manifestURL string
	if baseHost != "" {
		// 使用 baseHost
		manifestPath = strings.TrimPrefix(manifestPath, "/")
		manifestURL = baseHost + "/" + manifestPath
	} else {
		// 回退到从 sourceURL 推断
		manifestURL = p.constructManifestURL(input.SourceURL, manifestPath)
	}

	if manifestURL == "" {
		return result, nil
	}

	// 添加 manifest URL 到 Intermediates，让主流程下载并解析
	result.Intermediates = append(result.Intermediates, extractor.Intermediate{
		URL:  manifestURL,
		Type: extractor.ContentTypeJSON,
	})

	return result, nil
}

// extractBaseHost 从 JS 内容中提取 baseHost 的值
func (p *ModuleFederationManifestPlugin) extractBaseHost(content string) string {
	// 匹配 baseHost=location.hostname.includes("boe")?"https://www.feishu-boe.cn":"https://www.feishu.cn"
	// 提取第二个选项（生产环境版本）
	baseHostRe := regexp.MustCompile(`baseHost\s*=[^;?]+\?"[^"]+":"([^"]+)"`)
	match := baseHostRe.FindStringSubmatch(content)
	if len(match) > 1 {
		return match[1]
	}

	// 备用：直接匹配 baseHost = "https://..." 或 baseHost = 'https://...'
	simpleBaseHostRe := regexp.MustCompile(`baseHost\s*=\s*["'](https?://[^"']+)["']`)
	simpleMatch := simpleBaseHostRe.FindStringSubmatch(content)
	if len(simpleMatch) > 1 {
		return simpleMatch[1]
	}

	return ""
}

// constructManifestURL 从源 JS URL 和 manifest 路径构造完整 URL
func (p *ModuleFederationManifestPlugin) constructManifestURL(sourceURL, manifestPath string) string {
	// manifestPath 如 "/boss/order-vmok/assets/vmok-manifest.json"
	manifestPath = strings.TrimPrefix(manifestPath, "/")

	// 尝试从 sourceURL 推断网站域名
	// sourceURL 格式: https://sf3-cn.feishucdn.com/obj/hera-cn/hera/comp.xxx.js
	// 应该映射到: https://www.feishu.cn

	// 从 sourceURL 提取域名用于推断
	sourceDomain := ""
	if idx := strings.Index(sourceURL, "://"); idx != -1 {
		rest := sourceURL[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			sourceDomain = sourceURL[:idx+3+slashIdx]
		}
	}

	// 常见的 CDN 域名到源域名的映射
	cdnToOrigin := map[string]string{
		"sf3-cn.feishucdn.com":   "https://www.feishu.cn",
		"sf1-scmcdn-cn.feishucdn.com": "https://www.feishu.cn",
		"lf3-cn.feishucdn.com":    "https://www.feishu.cn",
		"lf1-cdn.feishucdn.com":   "https://www.feishu.cn",
	}

	if origin, ok := cdnToOrigin[sourceDomain]; ok {
		return origin + "/" + manifestPath
	}

	// 如果没有匹配，返回带 sourceDomain 的版本
	return sourceDomain + "/" + manifestPath
}

// VmokManifest JSON 格式
type VmokManifest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MetaData  struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		BuildInfo  struct {
			BuildVersion string `json:"buildVersion"`
			BuildName   string `json:"buildName"`
		} `json:"buildInfo"`
		RemoteEntry struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"remoteEntry"`
		Types struct {
			Name string `json:"Name"`
			Path string `json:"path"`
		} `json:"types"`
		GlobalName   string `json:"globalName"`
		PluginVersion string `json:"pluginVersion"`
		PublicPath   string `json:"publicPath"`
		Region       map[string]string `json:"region"`
	} `json:"metaData"`
	Shared   []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Assets  struct {
			JS struct {
				Sync  []string `json:"sync"`
				Async []string `json:"async"`
			} `json:"js"`
			CSS struct {
				Sync  []string `json:"sync"`
				Async []string `json:"async"`
			} `json:"css"`
		} `json:"assets"`
	} `json:"shared"`
	Remotes  []interface{} `json:"remotes"`
	Exposes  []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Assets struct {
			JS struct {
				Sync  []string `json:"sync"`
				Async []string `json:"async"`
			} `json:"js"`
			CSS struct {
				Sync  []string `json:"sync"`
				Async []string `json:"async"`
			} `json:"css"`
		} `json:"assets"`
		Path string `json:"path"`
	} `json:"exposes"`
	Region string `json:"region"`
}

// ManifestResult 包含解析后的 manifest 信息
type ManifestResult struct {
	Chunks    []string // chunk 相对路径列表
	CDNBase   string   // CDN 基础 URL
	PublicPath string  // publicPath 路径
}

// ParseVmokManifest 解析 vmok manifest JSON
func ParseVmokManifest(jsonContent []byte) (*ManifestResult, error) {
	var manifest VmokManifest
	if err := json.Unmarshal(jsonContent, &manifest); err != nil {
		return nil, err
	}

	chunks := make([]string, 0)

	// 收集所有 JS chunks
	// 1. shared 中的 chunks
	for _, shared := range manifest.Shared {
		chunks = append(chunks, shared.Assets.JS.Sync...)
		chunks = append(chunks, shared.Assets.JS.Async...)
	}

	// 2. exposes 中的 chunks
	for _, expose := range manifest.Exposes {
		chunks = append(chunks, expose.Assets.JS.Sync...)
		chunks = append(chunks, expose.Assets.JS.Async...)
	}

	// 去重
	seen := make(map[string]bool)
	uniqueChunks := make([]string, 0)
	for _, chunk := range chunks {
		if !seen[chunk] {
			seen[chunk] = true
			uniqueChunks = append(uniqueChunks, chunk)
		}
	}

	// 提取 CDN 基础 URL
	// region.cn: "sf1-scmcdn-cn.feishucdn.com/obj/feishu-static"
	cdnBase := ""
	if cn, ok := manifest.MetaData.Region["cn"]; ok {
		cdnBase = "https://" + cn
	}

	// 提取 publicPath
	// publicPath: "//__CDN_PREFIX__/lark/boss/order_vmok/"
	publicPath := ""
	if manifest.MetaData.PublicPath != "" {
		// 去掉 //__CDN_PREFIX__/ 前缀
		publicPath = strings.Replace(manifest.MetaData.PublicPath, "//__CDN_PREFIX__/", "", 1)
	}

	return &ManifestResult{
		Chunks:    uniqueChunks,
		CDNBase:   cdnBase,
		PublicPath: publicPath,
	}, nil
}

// ConstructVmokChunkURLsFromManifest 使用 manifest 中的信息构造完整的 chunk URL
func ConstructVmokChunkURLsFromManifest(manifestResult *ManifestResult) []extractor.DiscoveredJS {
	results := make([]extractor.DiscoveredJS, 0)

	// CDN 基础 URL: https://sf1-scmcdn-cn.feishucdn.com/obj/feishu-static
	// publicPath: /lark/boss/order_vmok/
	// chunk: static/js/async/236.45984be5.js
	// full URL: https://sf1-scmcdn-cn.feishucdn.com/obj/feishu-static/lark/boss/order_vmok/static/js/async/236.45984be5.js

	cdnBase := strings.TrimSuffix(manifestResult.CDNBase, "/")
	publicPath := strings.TrimPrefix(manifestResult.PublicPath, "/")
	publicPath = strings.TrimSuffix(publicPath, "/")

	for _, chunk := range manifestResult.Chunks {
		fullURL := cdnBase + "/" + publicPath + "/" + chunk
		results = append(results, extractor.DiscoveredJS{
			URL:      fullURL,
			FromURL:  "",
			IsInline: false,
		})
	}

	return results
}
