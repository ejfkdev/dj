package fetcher

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CacheConfig 缓存配置
type CacheConfig struct {
	Enable  bool
	BaseDir string // 缓存根目录，如 /tmp/ejfkdev/dj
}

// GetTempDir 获取临时目录
func GetTempDir() string {
	if runtime.GOOS == "windows" {
		if temp := os.Getenv("TEMP"); temp != "" {
			return temp
		}
		// Windows fallback
		return "C:\\Windows\\Temp"
	}
	return "/tmp/ejfkdev/dj"
}

// NormalizeOrigin 规范化 origin 部分（Windows 安全）
// https://test.com:8080 → https_test.com_8080
func NormalizeOrigin(url string) string {
	url = strings.ReplaceAll(url, "https://", "https_")
	url = strings.ReplaceAll(url, "http://", "http_")
	url = strings.ReplaceAll(url, ":", "_")
	url = strings.ReplaceAll(url, "/", "_")
	return url
}

// NormalizePathForFile 规范化路径用于文件名（Windows 安全）
// /aa/bb/static/js/app.js → aa_bb_static_js_app.js
func NormalizePathForFile(path string) string {
	path = strings.TrimPrefix(path, "/")
	path, _ = strings.CutSuffix(path, "/")

	// 防止路径遍历
	path = strings.ReplaceAll(path, "..", "_")
	path = strings.ReplaceAll(path, "./", "_")
	path = strings.ReplaceAll(path, "/./", "_")

	// 替换路径分隔符和 Windows 不安全字符
	path = strings.Map(sanitizeFilenameRune, path)

	if strings.Contains(path, "..") {
		return ""
	}

	return path
}

// sanitizeFilenameRune 将文件名不安全字符替换为下划线
func sanitizeFilenameRune(r rune) rune {
	switch r {
	case '/', '\\', '<', '>', ':', '"', '|', '?', '*', '\x00':
		return '_'
	default:
		return r
	}
}

// GetCacheRoot 获取缓存根目录
// 输入: https://test.com:8080/aa
// 输出: /tmp/ejfkdev/dj/https_test.com_8080_aa
func (c *CacheConfig) GetCacheRoot(baseURL string) string {
	normalized := NormalizeOrigin(baseURL)
	return filepath.Join(c.BaseDir, normalized)
}

// GetCachePath 获取缓存文件路径
// baseURL: https://test.com:8080/aa
// subDir: "js" 或 "source_map"
// urlPath: /static/js/app.js
// 返回: /tmp/ejfkdev/dj/https_test.com_8080_aa/js/static_js_app.js
func (c *CacheConfig) GetCachePath(baseURL, subDir, urlPath string) string {
	root := c.GetCacheRoot(baseURL)
	normalizedPath := NormalizePathForFile(urlPath)
	return filepath.Join(root, subDir, normalizedPath)
}

// SaveToCache 保存内容到缓存
func (c *CacheConfig) SaveToCache(baseURL, subDir, urlPath string, content []byte) error {
	if !c.Enable {
		return nil
	}

	cachePath := c.GetCachePath(baseURL, subDir, urlPath)

	// 确保目录存在
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(cachePath, content, 0644); err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	return nil
}

// SaveMetadata 保存元数据 JSON 到缓存
// 如果 urlPath 为空，保存到根目录的 meta.json
func (c *CacheConfig) SaveMetadata(baseURL, urlPath string, metadata []byte) error {
	if !c.Enable {
		return nil
	}

	var cachePath string
	if urlPath == "" {
		// 保存到根目录的 meta.json
		root := c.GetCacheRoot(baseURL)
		cachePath = filepath.Join(root, "meta.json")
	} else {
		// 元数据保存在单独的 metadata 目录
		cachePath = c.GetCachePath(baseURL, "metadata", urlPath)
		// 改为保存为 .json 文件
		cachePath = cachePath + ".json"
	}

	// 确保目录存在
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(cachePath, metadata, 0644); err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	return nil
}

// SaveDataURI 保存 data URI 内容到缓存
// dataURI: data:application/json;base64,... 或 data:application/json,...
// baseURL: 来源页面的 base URL
// urlPath: 来源 JS 的路径
// 返回保存的路径和解码后的原始内容（content 用于后续 source map 还原）
func (c *CacheConfig) SaveDataURI(baseURL, urlPath, dataURI string) (string, []byte, error) {
	if !c.Enable {
		return "", nil, nil
	}

	// 解析 data URI
	if !strings.HasPrefix(dataURI, "data:") {
		return "", nil, nil
	}

	// 提取 content
	rest := strings.TrimPrefix(dataURI, "data:")
	parts := strings.SplitN(rest, ",", 2)
	if len(parts) != 2 {
		return "", nil, nil
	}

	mimePart := parts[0]
	dataPart := parts[1]

	// 判断是否是 base64
	isBase64 := strings.Contains(mimePart, "base64")

	var content []byte
	var err error

	if isBase64 {
		content, err = base64.StdEncoding.DecodeString(dataPart)
	} else {
		content = []byte(dataPart)
	}
	if err != nil {
		return "", nil, fmt.Errorf("decode data uri failed: %w", err)
	}

	// 保存为 .map 文件
	mapPath := c.GetCachePath(baseURL, "source_map", urlPath+".map")
	dir := filepath.Dir(mapPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, fmt.Errorf("mkdir failed: %w", err)
	}

	if err := os.WriteFile(mapPath, content, 0644); err != nil {
		return "", nil, fmt.Errorf("write file failed: %w", err)
	}

	return mapPath, content, nil
}

// SaveSourceFile 保存还原出的原始源码文件到缓存，保留 sources 路径的目录层级。
//
// 与 SaveToCache 不同，本方法不拍平路径，而是按 sourcePath 的 / 分隔符创建子目录，
// 以还原原始项目结构。路径安全由调用方（sourcemap.NormalizeSourcePath）保证，
// 此处再做一次 filepath.Clean + ".." 段校验作为最后防线。
//
// baseURL: 站点 URL（用于定位缓存根目录）
// sourcePath: 规范化后的源码相对路径（如 src/App.jsx），必须是相对路径
// content: 文件内容
// 返回写入的完整文件路径
func (c *CacheConfig) SaveSourceFile(baseURL, sourcePath string, content []byte) (string, error) {
	if !c.Enable {
		return "", nil
	}
	if sourcePath == "" {
		return "", fmt.Errorf("empty source path")
	}

	// 安全校验：清理路径，确保不含 .. 段（防止路径穿越）
	cleanPath := filepath.Clean(sourcePath)
	if filepath.IsAbs(cleanPath) {
		// 去掉前导分隔符，转为相对路径
		cleanPath = strings.TrimPrefix(cleanPath, string(filepath.Separator))
	}
	// 逐段检查，禁止 .. 段
	for _, seg := range strings.Split(filepath.ToSlash(cleanPath), "/") {
		if seg == ".." {
			return "", fmt.Errorf("source path contains '..' segment: %q", sourcePath)
		}
	}

	root := c.GetCacheRoot(baseURL)
	fullPath := filepath.Join(root, "sources", cleanPath)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir failed: %w", err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return "", fmt.Errorf("write file failed: %w", err)
	}

	return fullPath, nil
}

// GetSourcesDir 获取还原源码的根目录路径
func (c *CacheConfig) GetSourcesDir(baseURL string) string {
	root := c.GetCacheRoot(baseURL)
	return filepath.Join(root, "sources")
}

// LoadFromCache 读取缓存内容（通用方法，按 subDir + urlPath 定位）。
// 路径计算与 SaveToCache 完全一致（复用 GetCachePath），确保读写路径对应。
// 文件不存在、为空、或读取失败时返回 (nil, false)。
func (c *CacheConfig) LoadFromCache(baseURL, subDir, urlPath string) ([]byte, bool) {
	if !c.Enable {
		return nil, false
	}
	cachePath := c.GetCachePath(baseURL, subDir, urlPath)
	content, err := os.ReadFile(cachePath)
	if err != nil || len(content) == 0 {
		return nil, false
	}
	return content, true
}

// LoadHTML 读取缓存的起始页 HTML（固定文件名 web.html）。
// 用于第二次运行同一站点时跳过起始页的网络下载。
func (c *CacheConfig) LoadHTML(baseURL string) ([]byte, bool) {
	return c.LoadFromCache(baseURL, "html", "web.html")
}

// LoadSourceMap 读取缓存的 source map 内容。
// jsRelPath 是关联 JS 的相对路径（与 SaveToCache 写入时的 urlPath 参数一致，
// 即 getRelativePath(jsURL, baseURL) 的结果，带前导 /）。
// 内部会拼接 ".map" 后缀并经 NormalizePathForFile 拍平，与 saveSourceMapToCache 写入路径一致。
func (c *CacheConfig) LoadSourceMap(baseURL, jsRelPath string) ([]byte, bool) {
	return c.LoadFromCache(baseURL, "source_map", jsRelPath+".map")
}

// LoadMetadata 读取缓存的站点元数据 meta.json。
// 返回原始 JSON 字节；文件不存在或读取失败返回 (nil, false)。
// 用于第二次运行时从缓存恢复已发现的 JS URL 列表，跳过网络发现流程。
func (c *CacheConfig) LoadMetadata(baseURL string) ([]byte, bool) {
	if !c.Enable {
		return nil, false
	}
	root := c.GetCacheRoot(baseURL)
	metaPath := filepath.Join(root, "meta.json")
	content, err := os.ReadFile(metaPath)
	if err != nil || len(content) == 0 {
		return nil, false
	}
	return content, true
}
