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
	Summary   Summary        `json:"summary"`
	JSURLs   []string       `json:"jsURLs"`
	CacheBase string        `json:"cacheBase,omitempty"`
	CacheDirs *CacheDirs    `json:"cacheDirs,omitempty"`
}

// Summary 统计摘要
type Summary struct {
	JSCount        int `json:"jsCount"`
	SourceMapCount int `json:"sourceMapCount,omitempty"`
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
	if result.Summary.SourceMapCount > 0 {
		sb.WriteString(fmt.Sprintf("- **Source maps**: %d\n", result.Summary.SourceMapCount))
	}
	sb.WriteString("\n## JS URLs\n")
	for _, url := range result.JSURLs {
		sb.WriteString("- ")
		sb.WriteString(url)
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Cache Directories\n")
	sb.WriteString("- **js**: ")
	sb.WriteString(result.CacheDirs.JS)
	sb.WriteString("\n")
	if result.Summary.SourceMapCount > 0 {
		sb.WriteString("- **sourceMap**: ")
		sb.WriteString(result.CacheDirs.SourceMap)
		sb.WriteString("\n")
	}
	if result.CacheDirs.Source != "" {
		sb.WriteString("- **source**: ")
		sb.WriteString(result.CacheDirs.Source)
		sb.WriteString("\n")
	}

	return sb.String()
}
