package extractor

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// DecodeContent 对文本进行常见编码的"机械式"还原，目的是让被编码的 JS URL
// 字符串（如 "https:\/\/foo.com\/bar.js"、"%2Fpath%2Fchunk.js"、"\u003Cscript src=..."）
// 变成普通可被 URL 正则直接命中的字符串。
//
// 还原的编码类型（顺序很重要：先做 URL 编码 → JS 转义 → Unicode → HTML 实体）：
//  1. URL 编码: %xx → 字符（仅在 %xx 出现频率较高时整段 QueryUnescape，避免误改哈希）
//  2. JS 字符串字面量转义: \/ \" \' \\ \n \r \t
//  3. Unicode 转义: \uXXXX → 字符（BMP 范围）
//  4. HTML 实体: &lt; &gt; &amp; &quot; &#39; &#x2F; 等
//
// 注意：
//   - 本函数不解释 JS 语义（不解析字符串拼接、不执行函数）
//   - 输入可能是 JS 字符串字面量、HTML 文本、或其他文本，统一做"最坏情况"还原
//   - 如果内容不含任何目标编码模式，函数应极快返回（避免无谓的字符串分配）
//   - 不抛错（编码可能不完整），失败时静默跳过该步骤
//
// 典型调用方：UniversalURLPlugin 等需要在做正则匹配前对原始 JS/HTML 文本
// 做"机械式"字符还原的下游插件。
func DecodeContent(content string) string {
	if content == "" {
		return content
	}
	// 快速路径：完全不含任何目标编码特征，直接返回
	if !needsDecode(content) {
		return content
	}

	result := content

	// 1. URL 编码还原（用 QueryUnescape，覆盖 %3C %3E %2F %3D 等常见编码）
	//    仅在 % 字符出现且上下文像 URL 编码时才处理（避免误改哈希或字符串字面量）
	if strings.Contains(result, "%") && looksLikeURLEncoded(result) {
		if decoded, err := url.QueryUnescape(result); err == nil {
			result = decoded
		}
	}

	// 2. JS 字符串字面量转义还原
	//    注意：\/ 必须在 \n 之前处理，因为两者都以 \ 开头
	if strings.Contains(result, `\`) {
		result = decodeJSEscapes(result)
	}

	// 3. Unicode 转义还原 \uXXXX
	if strings.Contains(result, `\u`) {
		result = unicodeEscapeRe.ReplaceAllStringFunc(result, unicodeEscapeReplace)
	}

	// 4. HTML 实体还原（仅处理 JS/HTML 文本中常见的几种，避免误改代码）
	if strings.Contains(result, "&") {
		result = decodeCommonHTMLEntities(result)
	}

	return result
}

// needsDecode 快速判断 content 是否含任何目标编码特征。
// 用于快速路径优化：当内容完全不含任何编码符号时直接返回原字符串，
// 避免对 41KB 的 CSS/JS 启动正则引擎。
func needsDecode(content string) bool {
	return strings.ContainsAny(content, "%\\&") ||
		strings.Contains(content, `\u`)
}

// looksLikeURLEncoded 简单判断一段文本是否"看起来像 URL 编码"：
// 至少含 1 个 %xx 模式（% 后跟 2 个十六进制字符）。
// 避免误改哈希值（如 chunk-5913e2749eb6975859bb 中的字母虽含 % 含义但不是 URL 编码）。
func looksLikeURLEncoded(s string) bool {
	return percentEncodedRe.MatchString(s)
}

// decodeJSEscapes 还原 JS 字符串字面量中的常见转义序列
// 支持：\/ \" \' \\ \n \r \t
// 不支持的（如 \x41、\b、\f 等）保持原样
func decodeJSEscapes(s string) string {
	// 顺序：先处理双字符转义（避免 \" 处理后 \n 又被错误处理）
	// 用 strings.ReplaceAll 是最简单稳定的方式
	s = strings.ReplaceAll(s, `\\`, "\x00") // 暂存 \\
	s = strings.ReplaceAll(s, `\/`, "/")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, "\x00", `\`) // 还原 \\
	return s
}

// unicodeEscapeReplace 还原单个 \uXXXX Unicode 转义
// 失败时（如格式不合法）保持原样
func unicodeEscapeReplace(match string) string {
	// match 形如 \uXXXX
	if len(match) < 6 {
		return match
	}
	hex := match[2:]
	r, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return match
	}
	return string(rune(r))
}

// decodeCommonHTMLEntities 还原 JS/HTML 文本中常见的 HTML 实体
// 只处理 URL/JS 提取真正需要的那几个，避免过度展开
func decodeCommonHTMLEntities(s string) string {
	// 命名实体
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&") // 注意：&amp; 必须在 &lt; &gt; 之后处理
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&apos;", "'")
	// 数字实体 &#39; &#x2F;
	s = numericEntityRe.ReplaceAllStringFunc(s, decodeNumericEntity)
	return s
}

// decodeNumericEntity 还原 &#NNN; 或 &#xHH; 形式的数字实体
func decodeNumericEntity(match string) string {
	// match 形如 &#39; 或 &#x2F;
	if !strings.HasPrefix(match, "&#") || !strings.HasSuffix(match, ";") {
		return match
	}
	body := match[2 : len(match)-1]
	var n int64
	var err error
	if strings.HasPrefix(body, "x") || strings.HasPrefix(body, "X") {
		n, err = strconv.ParseInt(body[1:], 16, 32)
	} else {
		n, err = strconv.ParseInt(body, 10, 32)
	}
	if err != nil || n < 0 {
		return match
	}
	return string(rune(n))
}

// 预编译的正则（在包初始化时编译一次）
var (
	// 匹配 % 后跟两个十六进制字符（URL 编码的特征）
	percentEncodedRe = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

	// 匹配 \uXXXX Unicode 转义（4 位十六进制）
	unicodeEscapeRe = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)

	// 匹配 HTML 数字实体 &#NNN; 或 &#xHH;
	numericEntityRe = regexp.MustCompile(`(?:#[0-9]+|#[xX][0-9a-fA-F]+);`)
)
