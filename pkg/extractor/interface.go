package extractor

import (
	"context"
	"net/http"
)

// ContentType 类型安全常量
type ContentType string

const (
	ContentTypeHTML ContentType = "html"
	ContentTypeJS   ContentType = "js"
	ContentTypeJSON ContentType = "json"
)

// AnalyzeInput 插件分析的上下文输入
type AnalyzeInput struct {
	SourceURL   string      // 当前内容来源 URL，绝对路径
	ContentType ContentType // "html" | "js" | "json"
	Content     []byte      // 资源内容
	Headers     http.Header // HTTP 响应头（用于 source map 等场景）
}

// Intermediate 发现的中间配置资源
type Intermediate struct {
	URL  string      // 资源的绝对 URL
	Type ContentType // 期望的资源类型
}

// InlineScript 行内脚本内容
type InlineScript struct {
	SourceURL string // 来源 HTML 的 URL
	Index     int    // 脚本在 HTML 中的顺序索引
	Content   []byte // 脚本内容
}

// Result 插件分析的返回结果
type Result struct {
	// FromPlugin 来源插件名称
	FromPlugin string

	// URLs 明确且完整的 JS URL，直接加入下载队列
	URLs []DiscoveredJS

	// ProbeTargets 提取到的路径片段，待主流程探测
	ProbeTargets []DiscoveredJS

	// PublicPaths 发现的 publicPath 路径
	PublicPaths []string

	// PrependURLs 用于拼接在文件名前面的 URL（通常是 protocol-relative URL）
	PrependURLs []string

	// Intermediates 需要主流程去下载并重新分发的配置文件
	Intermediates []Intermediate

	// InlineScripts 行内脚本内容，需要主流程分析
	InlineScripts []InlineScript
}

// DiscoveredJS 发现的 JS 详细信息
type DiscoveredJS struct {
	// URL 发现的 JS URL
	URL string
	// FromURL 发现该 JS 的来源 URL（分析的是哪个 HTML/JS 文件）
	FromURL string
	// FromPlugin 发现该 JS 的插件名称
	FromPlugin string
	// IsInline 是否来自内联脚本
	IsInline bool
}

// JSMetadata JS 文件的元数据信息
type JSMetadata struct {
	// URL 完整的 JS URL
	URL string `json:"url"`
	// LocalPath 本地文件路径
	LocalPath string `json:"local_path"`
	// SourceURL 发现该 JS 的来源 URL
	SourceURL string `json:"source_url"`
	// IsInline 是否来自内联脚本
	IsInline bool `json:"is_inline"`
	// SourceMapURL 关联的 source map URL（如果有）
	SourceMapURL string `json:"source_map_url,omitempty"`
	// SourceMapPath 本地 source map 文件路径（如果有）
	SourceMapPath string `json:"source_map_path,omitempty"`
	// FromPlugin 发现该 JS 的插件名称
	FromPlugin string `json:"from_plugin,omitempty"`
	// DiscoveredAt 发现时间（Unix timestamp）
	DiscoveredAt int64 `json:"discovered_at"`
}

// SiteMetadata 站点元数据（所有信息保存到一个 JSON 文件）
type SiteMetadata struct {
	// URLs 所有发现的 JS URL 信息
	URLs []JSMetadata `json:"urls"`
	// PrependURLs 探测时使用的 PrependURLs（拼接用的 CDN 前缀）
	PrependURLs []string `json:"prepend_urls"`
	// PublicPaths 探测时使用的 PublicPaths
	PublicPaths []string `json:"public_paths"`
	// DiscoveredAt 探测开始时间
	DiscoveredAt int64 `json:"discovered_at"`
}

// Plugin 核心插件接口
type Plugin interface {
	// Name 插件的唯一标识名称
	Name() string

	// Precheck 快速过滤，返回 true 才执行 Analyze
	Precheck(ctx context.Context, input *AnalyzeInput) bool

	// Analyze 正式分析逻辑
	Analyze(ctx context.Context, input *AnalyzeInput) (*Result, error)
}
