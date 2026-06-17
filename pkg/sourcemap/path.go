package sourcemap

import (
	"path"
	"path/filepath"
	"strings"
)

// 路径保护相关常量
const (
	// maxSourcePathDepth 源码路径最大目录深度，防止恶意 map 创建极深目录
	maxSourcePathDepth = 32
	// maxSourcePathLen 源码路径最大长度（字符数）
	maxSourcePathLen = 512
)

// NormalizeSourcePath 规范化 source map 中 sources 字段的路径，
// 使其可作为安全的相对文件路径写入磁盘，保留原始目录层级。
//
// 输入示例：
//
//	webpack:///./src/App.jsx          -> src/App.jsx
//	webpack:///webpack/bootstrap       -> webpack/bootstrap
//	webpack-internal:///./node_modules -> node_modules/foo.js
//	./src/utils.js                     -> src/utils.js
//	/static/js/app.js                  -> static/js/app.js
//	../../../../node_modules/foo.js    -> node_modules/foo.js  (.. 按相对路径回溯)
//	../../etc/passwd                   -> etc/passwd  (超出根的 .. 被丢弃，不逃逸)
//	file:///home/user/a.js             -> home/user/a.js
//
// sourceRoot 非空时会作为前缀拼接到规范化后的路径前（同样经过清理）。
// 返回空字符串表示路径无法规范化（如纯 scheme 无实际路径）。
func NormalizeSourcePath(source, sourceRoot string) string {
	cleaned := stripSourcePrefix(source)
	rootPart := stripSourcePrefix(sourceRoot)
	if rootPart != "" {
		cleaned = rootPart + "/" + cleaned
	}
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "./")

	// 规范化：消除多余的 . 和处理 .. 段
	cleaned = sanitizePathSegments(cleaned)

	if cleaned == "" || cleaned == "/" || cleaned == "." {
		return ""
	}

	// 长度与深度限制
	if len(cleaned) > maxSourcePathLen {
		// 保留文件名后缀，截断中间
		cleaned = truncatePathImpl(cleaned, maxSourcePathLen)
	}
	depth := strings.Count(cleaned, "/")
	if depth > maxSourcePathDepth {
		cleaned = flattenExcessDepth(cleaned, maxSourcePathDepth)
	}

	return cleaned
}

// stripSourcePrefix 剥离 source 路径的 scheme / 协议前缀
// 例如 webpack:///./src/App.jsx -> ./src/App.jsx
//
//	webpack-internal:///./src -> ./src
//	app:///index.js -> index.js
//	file:///home/user/a.js -> home/user/a.js
//	~/config -> config
func stripSourcePrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// 处理 ~/ 前缀（home 目录引用）
	s = strings.TrimPrefix(s, "~/")

	// 处理 scheme:// 或 scheme:/// 前缀
	// 形如 webpack:///、webpack-internal:///、app:///、file:///
	if idx := strings.Index(s, "://"); idx > 0 {
		rest := s[idx+3:]
		// 去掉开头的 authority（host）部分。source map 里 scheme:// 后通常直接是 /
		// 或空 authority（scheme:///path）。这里去掉 // 后到第一个 / 之间的部分
		rest = strings.TrimPrefix(rest, "//")
		// 如果还有内容且不以 / 开头，可能是 authority，跳到下一个 /
		if rest != "" && !strings.HasPrefix(rest, "/") {
			if slash := strings.Index(rest, "/"); slash >= 0 {
				rest = rest[slash+1:]
			} else {
				rest = ""
			}
		}
		s = rest
	}

	// 去掉前导 / 和 ./
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimPrefix(s, "./")
	return s
}

// sanitizePathSegments 清理路径中的段，按相对路径语义解析 ".."（向上回溯），
// 消除 "." 段，合并多余的 "/"。
//
// 关键设计：source map 的 sources 字段常含 "../" 前缀（Vite/webpack 用相对路径
// 表示 chunk 到源文件的回溯，如 "../../../../node_modules/foo.js"）。这是合法的
// 相对路径表达，应当按标准语义回溯，而不是简单替换为占位符——否则会丢失真实的项目
// 目录结构（变成 _/_/_/_/node_modules/...）。
//
// 防穿越安全：当 ".." 超过已有目录深度（即试图逃出 sources/ 根目录）时，多余的
// ".." 被丢弃，而不是回溯到根之上。最终结果始终是 sources/ 下的相对路径。
func sanitizePathSegments(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	// 统一路径分隔符
	p = strings.ReplaceAll(p, "\\", "/")

	// 用栈模拟路径解析：遇到普通段入栈，遇到 ".." 弹栈（栈空则丢弃，防止逃逸）
	parts := strings.Split(p, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case part == "" || part == ".":
			// 跳过空段和当前目录段
			continue
		case part == "..":
			// 按相对路径语义向上回溯：弹出栈顶目录
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			// 栈空时丢弃 ".."，防止逃逸到 sources/ 根目录之上
		default:
			// 清理段内的不安全字符（保留正常文件名字符）
			stack = append(stack, sanitizeFilename(part))
		}
	}

	return strings.Join(stack, "/")
}

// sanitizeFilename 清理单个文件名段中的不安全字符
// 保留字母、数字、点、连字符、下划线、加号、百分号等常见字符，
// 替换控制字符和路径分隔符为下划线
func sanitizeFilename(name string) string {
	if name == "" {
		return "_"
	}
	var sb strings.Builder
	sb.Grow(len(name))
	for _, r := range name {
		switch {
		case r < 0x20:
			// 控制字符
			sb.WriteByte('_')
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			// 路径分隔符和 Windows 不安全字符
			sb.WriteByte('_')
		default:
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		return "_"
	}
	return result
}

// truncatePathImpl 截断过长的路径，尽量保留末尾的文件名
func truncatePathImpl(p string, maxLen int) string {
	if len(p) <= maxLen {
		return p
	}
	// 保留最后一段（文件名）和部分目录
	lastSlash := strings.LastIndex(p, "/")
	var filename string
	if lastSlash >= 0 {
		filename = p[lastSlash+1:]
	} else {
		filename = p
	}
	keep := maxLen - len(filename) - 1
	if keep < 0 {
		// 文件名本身就超长，直接截断文件名
		if maxLen < 1 {
			maxLen = 1
		}
		return p[:maxLen]
	}
	return p[:keep] + "_" + filename
}

// flattenExcessDepth 将过深的目录路径扁平化
// 例如 a/b/c/d/e（depth=4）限制为 3 时 -> a/b/c_d_e
func flattenExcessDepth(p string, maxDepth int) string {
	parts := strings.Split(p, "/")
	if len(parts)-1 <= maxDepth {
		return p
	}
	// 保留前 maxDepth 个目录，剩余合并为扁平文件名
	keep := parts[:maxDepth]
	tail := strings.Join(parts[maxDepth:], "_")
	return strings.Join(keep, "/") + "/" + tail
}

// SafeFilePath 将规范化的 source path 转为当前 OS 的文件路径
// 确保结果始终是相对路径，不会逃逸到上级目录
func SafeFilePath(normalizedPath string) string {
	if normalizedPath == "" {
		return ""
	}
	// filepath.Clean 后再次校验，确保没有 .. 段
	cleaned := filepath.Clean(normalizedPath)
	// 转换为 path（用 / ）做段检查
	cleanedPath := filepath.ToSlash(cleaned)
	for _, part := range strings.Split(cleanedPath, "/") {
		if part == ".." {
			// 不应出现，但作为最后防线
			return ""
		}
	}
	// 确保是相对路径
	if filepath.IsAbs(cleaned) {
		cleaned = strings.TrimPrefix(cleaned, string(filepath.Separator))
	}
	// 最终再用 path.Join 确保分隔符正确
	return path.Join(filepath.ToSlash(cleaned))
}
