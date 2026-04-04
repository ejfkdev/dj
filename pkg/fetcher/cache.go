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

// NormalizeOrigin 规范化 origin 部分
// https://test.com:8080 → https_test.com:8080
func NormalizeOrigin(url string) string {
	// 将 scheme:// 替换为 scheme_
	// https://test.com:8080/aa → https_test.com:8080_aa
	url = strings.ReplaceAll(url, "https://", "https_")
	url = strings.ReplaceAll(url, "http://", "http_")
	// 将路径中的 / 替换为 _
	// 如果有路径，保留 host:port 格式
	return url
}

// NormalizePathForFile 规范化路径用于文件名
// /aa/bb/static/js/app.js → aa_bb_static_js_app.js
func NormalizePathForFile(path string) string {
	// 去掉前导 /
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	// 去掉尾随 /
	if strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	// 防止路径遍历：移除 .. 和 . 等特殊成分
	// 清理任何 .. 父目录引用和 . 当前目录引用
	path = strings.ReplaceAll(path, "..", "_")
	path = strings.ReplaceAll(path, "./", "_")
	path = strings.ReplaceAll(path, "/./", "_")

	// 将 / 替换为 _
	path = strings.ReplaceAll(path, "/", "_")

	// 再次检查清理后的路径是否安全
	// 如果包含 .. 返回空字符串让调用方处理
	if strings.Contains(path, "..") {
		return ""
	}

	return path
}

// GetCacheRoot 获取缓存根目录
// 输入: https://test.com:8080/aa
// 输出: /tmp/ejfkdev/dj/https_test.com:8080_aa
func (c *CacheConfig) GetCacheRoot(baseURL string) string {
	normalized := NormalizeOrigin(baseURL)
	return filepath.Join(c.BaseDir, normalized)
}

// GetCachePath 获取缓存文件路径
// baseURL: https://test.com:8080/aa
// subDir: "js" 或 "source_map"
// urlPath: /static/js/app.js
// 返回: /tmp/ejfkdev/dj/https_test.com:8080_aa/js/static_js_app.js
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
