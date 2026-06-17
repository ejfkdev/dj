package extractor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OutputFormat 输出格式
type OutputFormat string

const (
	FormatText OutputFormat = "text"
	FormatJSON OutputFormat = "json"
	FormatMD   OutputFormat = "md"
)

// OutputResult 输出结果结构
type OutputResult struct {
	Summary   Summary    `json:"summary"`
	JSURLs    []string   `json:"jsURLs"`
	CacheBase string     `json:"cacheBase,omitempty"`
	CacheDirs *CacheDirs `json:"cacheDirs,omitempty"`
}

// Summary 统计摘要
type Summary struct {
	JSCount        int `json:"jsCount"`
	SourceMapCount int `json:"sourceMapCount,omitempty"`
	SourceCount    int `json:"sourceCount,omitempty"`
}

// HasSourceMap 是否发现 source map
func (s Summary) HasSourceMap() bool {
	return s.SourceMapCount > 0
}

// SourcesRestored 是否成功还原出源码
func (s Summary) SourcesRestored() bool {
	return s.SourceCount > 0
}

// CacheDirs 缓存目录
type CacheDirs struct {
	JS        string `json:"js"`
	SourceMap string `json:"sourceMap,omitempty"`
	Source    string `json:"source,omitempty"`
	HTML      string `json:"html,omitempty"`
}

// FormatOutput 格式化输出
func FormatOutput(format OutputFormat, result *OutputResult) string {
	switch format {
	case FormatJSON:
		return formatJSON(result)
	case FormatMD:
		return formatMD(result)
	default:
		return formatText(result)
	}
}

func formatText(result *OutputResult) string {
	return strings.Join(result.JSURLs, "\n")
}

func formatJSON(result *OutputResult) string {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal: %v"}`, err)
	}
	return string(data)
}

func formatMD(result *OutputResult) string {
	var sb strings.Builder

	sb.WriteString("## Summary\n")
	sb.WriteString(fmt.Sprintf("- **JS files**: %d\n", result.Summary.JSCount))

	// source map 状态
	if result.Summary.HasSourceMap() {
		sb.WriteString(fmt.Sprintf("- **Source maps**: %d (found)\n", result.Summary.SourceMapCount))
	} else {
		sb.WriteString("- **Source maps**: 0 (not found)\n")
	}

	// 源码还原状态
	if result.Summary.SourcesRestored() {
		sb.WriteString(fmt.Sprintf("- **Restored sources**: %d files (restored)\n", result.Summary.SourceCount))
	} else {
		sb.WriteString("- **Restored sources**: 0 (not restored)\n")
	}

	sb.WriteString("\n## JS URLs\n")
	for _, url := range result.JSURLs {
		sb.WriteString("- ")
		sb.WriteString(url)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Cache Directories\n")
	if result.CacheDirs == nil {
		sb.WriteString("- cache disabled\n")
	} else {
		if result.CacheDirs.HTML != "" {
			sb.WriteString(fmt.Sprintf("- **html**: %s\n", result.CacheDirs.HTML))
		}
		if result.CacheDirs.JS != "" {
			sb.WriteString(fmt.Sprintf("- **js**: %s\n", result.CacheDirs.JS))
		}
		if result.CacheDirs.SourceMap != "" {
			sb.WriteString(fmt.Sprintf("- **sourceMap**: %s\n", result.CacheDirs.SourceMap))
		}
		if result.CacheDirs.Source != "" {
			sb.WriteString(fmt.Sprintf("- **sources**: %s\n", result.CacheDirs.Source))
		}
	}

	return sb.String()
}

// FormatTextSummary 生成 text 格式的末尾汇总信息。
// text 模式下 JS URL 是实时流式打印的，此函数用于在末尾追加统计摘要，
// 让用户一眼看到 source map 和源码还原情况。
func FormatTextSummary(result *OutputResult) string {
	var sb strings.Builder

	sb.WriteString("\n--- Summary ---\n")
	sb.WriteString(fmt.Sprintf("JS files: %d\n", result.Summary.JSCount))

	if result.Summary.HasSourceMap() {
		sb.WriteString(fmt.Sprintf("Source maps: %d (found)\n", result.Summary.SourceMapCount))
	} else {
		sb.WriteString("Source maps: 0 (not found)\n")
	}

	if result.Summary.SourcesRestored() {
		sb.WriteString(fmt.Sprintf("Restored sources: %d files (restored)\n", result.Summary.SourceCount))
	} else {
		sb.WriteString("Restored sources: 0 (not restored)\n")
	}

	// 缓存目录
	if result.CacheDirs != nil {
		if result.CacheDirs.SourceMap != "" {
			sb.WriteString(fmt.Sprintf("Source map dir: %s\n", result.CacheDirs.SourceMap))
		}
		if result.CacheDirs.Source != "" {
			sb.WriteString(fmt.Sprintf("Sources dir: %s\n", result.CacheDirs.Source))
		}
	}

	return sb.String()
}
