package plugins

import (
	"net/url"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// 本文件是 entry-HTML 式微前端插件的共享工具。
// 各框架插件（Qiankun/Garfish/MicroApp/Wujie/Icestark）负责定位自己
// 框架的注册配置和入口键，公共的判断/解析逻辑集中在这里。

// looksLikeMicroAppEntry 判断入口值是否指向子应用 HTML 入口。
// 接受：
//   - 绝对路径目录: /aio/app/business/
//   - http(s) 完整 URL 目录: https://cdn.x.com/app/
//   - 显式 HTML 文件: /app/index.html（部分框架支持）
//
// 排除: dev 地址（//ip:port 协议相对形式）、模板字符串、
// 相对路径（从 bundle 内无法可靠解析目录入口）。
func looksLikeMicroAppEntry(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	// devEntry 常为 //127.0.0.1:5010/ 的协议相对地址
	if strings.HasPrefix(v, "//") {
		return false
	}
	// 只接受绝对路径或完整 URL（相对入口从 bundle 内解析不可靠）
	if !strings.HasPrefix(v, "/") &&
		!strings.HasPrefix(v, "http://") &&
		!strings.HasPrefix(v, "https://") {
		return false
	}
	// 模板变量
	if strings.Contains(v, "${") {
		return false
	}
	lower := strings.ToLower(v)
	if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
		return true
	}
	// 目录形式：以 / 结尾
	if !strings.HasSuffix(v, "/") {
		return false
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		if _, err := url.Parse(v); err != nil {
			return false
		}
	}
	return true
}

// hasMicroAppContext 在匹配位置前后窗口内查找伴生字段，降低误报率。
func hasMicroAppContext(content string, start, end int, keywords []string) bool {
	const window = 400
	lo := start - window
	if lo < 0 {
		lo = 0
	}
	hi := end + window
	if hi > len(content) {
		hi = len(content)
	}
	windowText := content[lo:hi]
	for _, kw := range keywords {
		if strings.Contains(windowText, kw) {
			return true
		}
	}
	return false
}

// resolveMicroAppEntry 把入口值解析为绝对 URL 并加入 result.Intermediates。
// seen 去重；超过 maxMicroAppEntries 时截断。
// 返回 true 表示已成功加入。
func resolveMicroAppEntry(result *extractor.Result, seen map[string]bool, sourceURL, value string) bool {
	if !looksLikeMicroAppEntry(value) {
		return false
	}
	absoluteURL := extractor.ResolveRelativePath(sourceURL, value)
	absoluteURL = extractor.NormalizeURL(absoluteURL)
	if !extractor.IsAbsoluteURL(absoluteURL) {
		return false
	}
	if seen[absoluteURL] {
		return false
	}
	seen[absoluteURL] = true
	result.Intermediates = append(result.Intermediates, extractor.Intermediate{
		URL:  absoluteURL,
		Type: extractor.ContentTypeHTML,
	})
	return true
}

// maxMicroAppEntries 单文件提取上限（防爆炸）
const maxMicroAppEntries = 50
