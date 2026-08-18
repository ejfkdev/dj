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

// RebaseLoopbackOrigin 将 loopback 绝对地址（127.0.0.1 / localhost，端口与来源
// 页面不同）重映射到来源页面的 origin，路径不变。覆盖"构建期把 dev server
// 地址烧进产物、扫描期跑在另一端口"的同源镜像部署场景。
// 目标非 loopback、或与来源同源时返回原值。
func RebaseLoopbackOrigin(rawURL, sourceURL string) string {
	target, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	targetHostname := target.Hostname()
	if targetHostname != "127.0.0.1" && targetHostname != "localhost" {
		return rawURL
	}
	src, err := url.Parse(sourceURL)
	if err != nil || src.Host == "" {
		return rawURL
	}
	if target.Host == src.Host {
		return rawURL
	}
	target.Scheme, target.Host = src.Scheme, src.Host
	return target.String()
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

// ExpandComboLoader 展开 CDN combo-loader URL（含 `??` 语法）。
// 支持 WordPress 和 阿里云 CDN 的 combo-loader 格式：
//   - WordPress: https://example.com/_static/??/js/a.js,/js/b.js
//     → https://example.com/_static/js/a.js, https://example.com/_static/js/b.js
//   - 阿里云 CDN: https://g.alicdn.com/path/??a.js,b.js
//     → https://g.alicdn.com/path/a.js, https://g.alicdn.com/path/b.js
//
// 如果不含 `??` 或展开失败，返回 [url]（原样）。
func ExpandComboLoader(rawURL string) []string {
	if !strings.Contains(rawURL, "??") {
		return []string{rawURL}
	}

	// 不能用 url.Parse 因为 `?` 会被当作 query string
	// 直接在原始字符串上处理

	// 找到 `??` 的位置
	idx := strings.Index(rawURL, "??")
	if idx < 0 {
		return []string{rawURL}
	}

	// 前缀部分（scheme://host + path 前段）
	prefix := rawURL[:idx] // 如 "https://example.com/_static/" 或 "https://g.alicdn.com/path/"

	// 后缀部分（文件列表）
	fileList := rawURL[idx+2:] // 如 "/js/a.js,/js/b.js" 或 "a.js,b.js"

	// 可能还有 query string 在文件列表后面
	// 如 https://example.com/??a.js,b.js?v=1
	var querySuffix string
	if qidx := strings.IndexAny(fileList, "?#"); qidx >= 0 {
		querySuffix = fileList[qidx:]
		fileList = fileList[:qidx]
	}

	// 分割文件列表
	files := strings.Split(fileList, ",")
	var urls []string
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// 拼接前缀 + 文件名
		fullURL := prefix + f
		// 如果有 query suffix，添加回去
		if querySuffix != "" {
			fullURL += querySuffix
		}
		// 清理路径中的多余斜杠（前缀以 / 结尾，文件以 / 开头时）
		// 但保留正常的单斜杠
		urls = append(urls, NormalizeURL(fullURL))
	}

	if len(urls) == 0 {
		return []string{rawURL}
	}
	return urls
}
