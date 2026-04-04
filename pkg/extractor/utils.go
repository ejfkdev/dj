package extractor

import (
	"net/url"
	"path"
	"strings"
)

// ResolveRelativePath 解析相对路径为绝对路径
func ResolveRelativePath(baseURL, relative string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	rel, err := url.Parse(relative)
	if err != nil {
		return relative
	}

	return base.ResolveReference(rel).String()
}

// IsAbsoluteURL 判断是否为完整 URL
func IsAbsoluteURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// NormalizeURL 规范化 URL，去除双斜杠、清理路径、清除 fragment
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// 清除 fragment（# 后面的内容对网络请求无意义）
	u.Fragment = ""
	// 只有 Path 不为空时才清理
	if u.Path != "" {
		u.Path = path.Clean(u.Path)
	}
	return u.String()
}

// GetDirFromURL 获取 URL 的目录部分
func GetDirFromURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	// 如果没有路径，返回基础 URL
	if parsed.Path == "" {
		return parsed.Scheme + "://" + parsed.Host + "/"
	}
	dir := path.Dir(parsed.Path)
	if dir == "." {
		dir = ""
	}
	// 重建 URL
	if parsed.Scheme != "" {
		return parsed.Scheme + "://" + parsed.Host + dir + "/"
	}
	return dir + "/"
}

// GetBaseURL 获取 URL 的基础部分 (scheme://host)
func GetBaseURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
