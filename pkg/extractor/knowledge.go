package extractor

import "sync"

// KnowledgeBase 全局知识库
type KnowledgeBase struct {
	mu sync.RWMutex

	// PublicPaths 发现的 publicPath 列表
	PublicPaths []string

	// PrependURLs 用于拼接在文件名前面的 URL 列表
	PrependURLs []string

	// SeenURLs 已尝试过的 URL（无论成功/404）
	SeenURLs map[string]bool

	// SeenFragments 已探测过的 JS 路径片段
	SeenFragments map[string]bool

	// KnownPaths 已知存在的 JS 路径（用于优先拼接）
	KnownPaths []string

	// JSHasSourceMap JS URL 是否有 source map（通过 sourceMappingURL 或 HTTP 头发现）
	hasSourceMapURLs map[string]bool
}

// NewKnowledgeBase 创建知识库
func NewKnowledgeBase() *KnowledgeBase {
	return &KnowledgeBase{
		SeenURLs:        make(map[string]bool),
		SeenFragments:    make(map[string]bool),
		hasSourceMapURLs: make(map[string]bool),
		PublicPaths:     make([]string, 0),
		PrependURLs:     make([]string, 0),
		KnownPaths:      make([]string, 0),
	}
}

// AddPublicPath 添加 publicPath
func (kb *KnowledgeBase) AddPublicPath(paths ...string) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	for _, p := range paths {
		if p == "" {
			continue
		}
		if !contains(kb.PublicPaths, p) {
			kb.PublicPaths = append(kb.PublicPaths, p)
		}
	}
}

// GetPublicPaths 获取所有 publicPath
func (kb *KnowledgeBase) GetPublicPaths() []string {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	result := make([]string, len(kb.PublicPaths))
	copy(result, kb.PublicPaths)
	return result
}

// AddPrependURL 添加用于拼接的 URL
func (kb *KnowledgeBase) AddPrependURL(urls ...string) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	for _, u := range urls {
		if u == "" {
			continue
		}
		if !contains(kb.PrependURLs, u) {
			kb.PrependURLs = append(kb.PrependURLs, u)
		}
	}
}

// GetPrependURLs 获取所有用于拼接的 URL
func (kb *KnowledgeBase) GetPrependURLs() []string {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	result := make([]string, len(kb.PrependURLs))
	copy(result, kb.PrependURLs)
	return result
}

// MarkSeenURL 标记 URL 已尝试
func (kb *KnowledgeBase) MarkSeenURL(url string) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.SeenURLs[url] = true
}

// IsSeenURL 检查 URL 是否已尝试
func (kb *KnowledgeBase) IsSeenURL(url string) bool {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.SeenURLs[url]
}

// MarkSeenFragment 标记片段已探测
func (kb *KnowledgeBase) MarkSeenFragment(fragment string) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.SeenFragments[fragment] = true
}

// IsSeenFragment 检查片段是否已探测
func (kb *KnowledgeBase) IsSeenFragment(fragment string) bool {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.SeenFragments[fragment]
}

// AddKnownPath 添加已知存在的 JS 路径
func (kb *KnowledgeBase) AddKnownPath(paths ...string) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	for _, p := range paths {
		if p == "" {
			continue
		}
		if !contains(kb.KnownPaths, p) {
			kb.KnownPaths = append(kb.KnownPaths, p)
		}
	}
}

// GetKnownPaths 获取所有已知路径
func (kb *KnowledgeBase) GetKnownPaths() []string {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	result := make([]string, len(kb.KnownPaths))
	copy(result, kb.KnownPaths)
	return result
}

// SetJSHasSourceMap 标记 JS 是否有 source map
func (kb *KnowledgeBase) SetJSHasSourceMap(url string, has bool) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.hasSourceMapURLs[url] = has
}

// JSHasSourceMap 检查 JS 是否有 source map
func (kb *KnowledgeBase) JSHasSourceMap(url string) bool {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.hasSourceMapURLs[url]
}

// contains 检查切片是否包含某个元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
