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
// 返回保存的路径
func (c *CacheConfig) SaveDataURI(baseURL, urlPath, dataURI string) (string, error) {
	if !c.Enable {
		return "", nil
	}

	// 解析 data URI
	if !strings.HasPrefix(dataURI, "data:") {
		return "", nil
	}

	// 提取 content
	rest := strings.TrimPrefix(dataURI, "data:")
	parts := strings.SplitN(rest, ",", 2)
	if len(parts) != 2 {
		return "", nil
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
		return "", fmt.Errorf("decode data uri failed: %w", err)
	}

	// 保存为 .map 文件
	mapPath := c.GetCachePath(baseURL, "source_map", urlPath+".map")
	dir := filepath.Dir(mapPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir failed: %w", err)
	}

	if err := os.WriteFile(mapPath, content, 0644); err != nil {
		return "", fmt.Errorf("write file failed: %w", err)
	}

	return mapPath, nil
}
