package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ejfkdev/dj/pkg/fetcher"
)

const (
	// Worker 并发数
	workerCount = 100
)

// contextKey 用于在 context 中传递数据的 key
type contextKey string

const (
	knowledgeKey contextKey = "knowledge"
)

// debugLog 输出 debug 日志（仅在 debug=true 时）
func (p *Pipeline) debugLog(format string, v ...interface{}) {
	if p.Debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Pipeline 主流程调度器
type Pipeline struct {
	knowledge *KnowledgeBase
	registry  *PluginRegistry
	fetcher   *fetcher.Fetcher

	// 缓存配置
	cacheConfig *fetcher.CacheConfig
	baseURL    string // 起始 URL，用于缓存路径计算

	// 调试模式
	Debug bool

	// 任务队列（无限制 slice）
	taskMu sync.Mutex
	tasks  []string

	// 待探测片段队列（无限制 slice）
	fragmentMu sync.Mutex
	fragments  []DiscoveredJS

	// JS 处理 WaitGroup（每个 URL 一个 goroutine，直接处理 fetch+验证+分发）
	jsWg sync.WaitGroup

	// 发现 JS URL 的通知 channel
	foundCh      chan string
	foundChMu    sync.Mutex
	foundChSeen  map[string]bool  // 去重
	foundChClose sync.Once        // 安全关闭 channel

	// 收集的 JS URL（用于格式化输出）
	jsURLsMu sync.Mutex
	jsURLs   []DiscoveredJS

	// URL 上下文映射（用于在 processURL/processFragment 中添加 jsURLs 时获取上下文）
	urlContext   map[string]DiscoveredJS // key: 规范化后的 URL
	urlContextMu sync.RWMutex
}

// NewPipeline 创建 Pipeline
func NewPipeline(reg *PluginRegistry) *Pipeline {
	return &Pipeline{
		knowledge:   NewKnowledgeBase(),
		registry:   reg,
		fetcher:    fetcher.NewFetcher(),
		tasks:      make([]string, 0),
		fragments:  make([]DiscoveredJS, 0),
		foundCh:    make(chan string, 100),
		foundChSeen: make(map[string]bool),
		urlContext: make(map[string]DiscoveredJS),
	}
}

// SetCacheConfig 设置缓存配置
func (p *Pipeline) SetCacheConfig(cfg *fetcher.CacheConfig) {
	p.cacheConfig = cfg
}

// SetFetcherConfig 设置 Fetcher 配置（代理和 User-Agent）
func (p *Pipeline) SetFetcherConfig(proxy, userAgent string) {
	f, err := fetcher.NewFetcherWithConfig(fetcher.FetcherConfig{
		Proxy:   proxy,
		UseUTLS: true,
	})
	if err != nil {
		// 降级为不使用 uTLS
		f, _ = fetcher.NewFetcherWithConfig(fetcher.FetcherConfig{
			Proxy:   proxy,
			UseUTLS: false,
		})
	}
	if userAgent != "" {
		f.SetUserAgent(userAgent)
	}
	p.fetcher = f
}

// SetBrowserCookies 注入浏览器 cookie 到 Fetcher
func (p *Pipeline) SetBrowserCookies(targetURL string, cookies []*http.Cookie) error {
	return p.fetcher.SetCookies(targetURL, cookies)
}

// SetBaseURL 设置起始 URL
func (p *Pipeline) SetBaseURL(url string) {
	p.baseURL = url
}

// Run 执行主流程
func (p *Pipeline) Run(ctx context.Context, startURL string) ([]string, error) {
	// 保存起始 URL 用于缓存路径
	p.baseURL = startURL

	// 添加起始 URL 的 baseURL 到知识库
	if baseURL := GetBaseURL(startURL); baseURL != "" {
		p.knowledge.AddPrependURL(baseURL)
	}

	// 下载并保存起始页 HTML
	if p.cacheConfig != nil && p.cacheConfig.Enable {
		if htmlContent, err := p.fetcher.Fetch(startURL); err == nil {
			p.saveHTMLToCache(htmlContent)
		}
	}

	// 将起始任务放入队列
	p.tryEnqueue(startURL, nil) // 起始 URL 没有插件上下文

	// 迭代处理：直到没有新 URLs 为止
	for {
		// 收集当前批次的 tasks
		p.taskMu.Lock()
		tasks := p.tasks
		p.tasks = make([]string, 0)
		p.taskMu.Unlock()

		p.debugLog("MainLoop: tasks=%d, fragments=%d", len(tasks), len(p.fragments))
		if len(tasks) == 0 {
			// 没有任务了，处理 pending fragments
			p.debugLog("Calling processFragmentBatch with %d fragments", len(p.fragments))
			p.processFragmentBatch(ctx)
			p.debugLog("After processFragmentBatch, tasks=%d, fragments=%d", len(p.tasks), len(p.fragments))

			// 检查是否还有新任务（fragments 探测可能产生新任务）
			p.taskMu.Lock()
			hasMoreTasks := len(p.tasks) > 0 || len(p.fragments) > 0
			p.taskMu.Unlock()
			if !hasMoreTasks {
				// 等待一段时间让已启动的 goroutines 处理完并发现新任务
				time.Sleep(500 * time.Millisecond)

				// 再次检查
				p.taskMu.Lock()
				hasMoreTasks = len(p.tasks) > 0 || len(p.fragments) > 0
				p.taskMu.Unlock()
				if !hasMoreTasks {
					break
				}
			}
			continue
		}

		// 为每个 URL 启动一个 goroutine 处理完整流程（fetch + 验证 + 插件分发）
		for _, urlStr := range tasks {
			p.jsWg.Add(1)
			go func(url string) {
				defer p.jsWg.Done()
				p.processJSContentURL(ctx, url)
			}(urlStr)
		}

		// 等待所有 goroutine 完成
		p.jsWg.Wait()
	}

	// 安全关闭 found channel
	p.foundChClose.Do(func() {
		close(p.foundCh)
	})

	// 保存站点元数据
	p.saveSiteMetadata()

	// 返回所有发现的 JS URL
	return p.knowledge.GetKnownPaths(), nil
}

// tryEnqueue 安全地添加 URL 到队列，返回是否成功
// discovered 参数可选（用于保留上下文）
func (p *Pipeline) tryEnqueue(url string, discovered *DiscoveredJS) bool {
	p.taskMu.Lock()
	defer p.taskMu.Unlock()

	// 在锁内检查，避免竞态
	if p.knowledge.IsSeenURL(url) {
		return false
	}

	p.knowledge.MarkSeenURL(url)
	p.tasks = append(p.tasks, url)

	// 存储上下文到 urlContext map（用于后续添加 jsURLs 时获取）
	if discovered != nil {
		p.urlContextMu.Lock()
		p.urlContext[url] = *discovered
		p.urlContextMu.Unlock()
	}

	if p.Debug {
		p.debugLog("tryEnqueue: %s", url)
	}
	return true
}

// probeSourceMap 使用 HEAD 请求探测 .map 文件是否存在
func (p *Pipeline) probeSourceMap(jsURL string) {
	// 如果 knowledge 中记录该 JS 有 source map，则不探测
	if p.knowledge.JSHasSourceMap(jsURL) {
		return
	}
	mapURL := buildSourceMapURL(jsURL)
	// 检查是否已探测过
	if p.knowledge.IsSeenURL(mapURL) {
		return
	}
	p.knowledge.MarkSeenURL(mapURL)

	result, err := p.fetcher.FetchWithStatusHead(mapURL)
	if err != nil {
		if p.Debug {
			p.debugLog("probeSourceMap HEAD error: url=%s, err=%v", mapURL, err)
		}
		return
	}
	// 检查状态码和 Content-Type
	if result.StatusCode >= 200 && result.StatusCode < 300 {
		// Content-Type 不能是 text/html（服务器错误可能返回 HTML 页面）
		contentType := strings.ToLower(result.ContentType)
		if strings.Contains(contentType, "text/html") {
			return
		}

		if p.Debug {
			p.debugLog("probeSourceMap: found .map: %s", mapURL)
		}
		p.foundChMu.Lock()
		if !p.foundChSeen[mapURL] {
			p.foundChSeen[mapURL] = true
			select {
			case p.foundCh <- mapURL:
			default:
			}
		}
		p.foundChMu.Unlock()

		// 下载 .map 文件内容并保存到缓存
		mapContent, err := p.fetcher.Fetch(mapURL)
		if err == nil {
			p.saveSourceMapToCache(jsURL, mapContent)
		}
	}
}

// getRelativePath 获取 URL 相对于 baseURL 的路径部分
// 如果 fullURL 是 https://test.com:8080/aa/static/js/app.js
// baseURL 是 https://test.com:8080/aa
// 返回 /static/js/app.js
func (p *Pipeline) getRelativePath(fullURL, baseURL string) string {
	// 去掉 baseURL 的 origin 部分
	baseOrigin := GetBaseURL(baseURL)
	if baseOrigin == "" {
		return fullURL
	}

	// 边界检查：确保 baseOrigin 不比 fullURL 长
	if len(baseOrigin) > len(fullURL) {
		return fullURL
	}

	// 提取 fullURL 的路径部分
	fullPath := fullURL[len(baseOrigin):]
	return fullPath
}

// saveJSToCache 保存 JS 到缓存
func (p *Pipeline) saveJSToCache(url string, content []byte) {
	if p.cacheConfig == nil || !p.cacheConfig.Enable {
		return
	}
	if p.baseURL == "" {
		return
	}

	// 获取相对路径
	path := p.getRelativePath(url, p.baseURL)
	if path == "" || path == url {
		return
	}

	// 提取 host 部分（用于生成扁平化文件名）
	host := extractHost(p.baseURL)

	// 去掉路径前导 /，将路径中的 / 替换为 -
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "-")

	// 组合文件名: host-path.js (如果 path 不以 .js/.mjs/.css 结尾才加)
	filename := host + "-" + path
	if !strings.HasSuffix(filename, ".js") && !strings.HasSuffix(filename, ".mjs") && !strings.HasSuffix(filename, ".css") {
		filename += ".js"
	}
	if err := p.cacheConfig.SaveToCache(p.baseURL, "js", filename, content); err != nil {
		if p.Debug {
			p.debugLog("saveJSToCache failed: %v", err)
		}
	}
}

// saveHTMLToCache 保存 HTML 到缓存
func (p *Pipeline) saveHTMLToCache(content []byte) {
	if p.cacheConfig == nil || !p.cacheConfig.Enable {
		return
	}
	if p.baseURL == "" {
		return
	}

	if err := p.cacheConfig.SaveToCache(p.baseURL, "html", "web.html", content); err != nil {
		if p.Debug {
			p.debugLog("saveHTMLToCache failed: %v", err)
		}
	}
}

// saveSourceMapToCache 保存 source map 到缓存
func (p *Pipeline) saveSourceMapToCache(jsURL string, content []byte) {
	if p.cacheConfig == nil || !p.cacheConfig.Enable {
		return
	}
	if p.baseURL == "" {
		return
	}

	// 获取相对路径
	path := p.getRelativePath(jsURL, p.baseURL)
	if path == "" || path == jsURL {
		return
	}

	if err := p.cacheConfig.SaveToCache(p.baseURL, "source_map", path+".map", content); err != nil {
		if p.Debug {
			p.debugLog("saveSourceMapToCache failed: %v", err)
		}
	}
}

// saveDataURIToCache 保存 data URI 到缓存
func (p *Pipeline) saveDataURIToCache(sourceJSURL, dataURI string) {
	if p.cacheConfig == nil || !p.cacheConfig.Enable {
		return
	}
	if p.baseURL == "" {
		return
	}

	// 获取 source JS 的相对路径
	path := p.getRelativePath(sourceJSURL, p.baseURL)
	if path == "" || path == sourceJSURL {
		return
	}

	// 保存到缓存
	cachePath, err := p.cacheConfig.SaveDataURI(p.baseURL, path, dataURI)
	if err != nil {
		if p.Debug {
			p.debugLog("saveDataURIToCache failed: %v", err)
		}
		return
	}

	if p.Debug {
		p.debugLog("saveDataURIToCache: saved inline source map to %s", cachePath)
	}
}

// saveSiteMetadata 保存站点元数据到 JSON 文件
func (p *Pipeline) saveSiteMetadata() {
	if p.cacheConfig == nil || !p.cacheConfig.Enable {
		return
	}
	if p.baseURL == "" {
		return
	}

	// 从 jsURLs 构建 URLs 列表（保留上下文）
	p.jsURLsMu.Lock()
	jsMetadataList := make([]JSMetadata, 0, len(p.jsURLs))
	for _, js := range p.jsURLs {
		// 计算本地路径
		localPath := ""
		if path := p.getRelativePath(js.URL, p.baseURL); path != "" && path != js.URL {
			localPath = p.cacheConfig.GetCachePath(p.baseURL, "js", path)
		}
		jsMetadataList = append(jsMetadataList, JSMetadata{
			URL:         js.URL,
			LocalPath:   localPath,
			SourceURL:   js.FromURL,
			IsInline:    js.IsInline,
			FromPlugin:  js.FromPlugin,
			DiscoveredAt: time.Now().Unix(),
		})
	}
	p.jsURLsMu.Unlock()

	// 从 KnownPaths 提取路径前缀（如 /ryqq/js/）
	prependURLs := p.knowledge.GetPrependURLs()
	knownPaths := p.knowledge.GetKnownPaths()
	baseOrigin := GetBaseURL(p.baseURL)
	seenPrefixes := make(map[string]bool)
	for _, path := range knownPaths {
		// 提取路径前缀（去掉文件名，保留目录部分）
		// 例如: https://y.qq.com/ryqq/js/runtime~Page.xxx.js -> /ryqq/js/
		if strings.HasPrefix(path, baseOrigin) {
			relativePath := path[len(baseOrigin):]
			if idx := strings.LastIndex(relativePath, "/"); idx > 0 {
				prefix := relativePath[:idx+1] // 包含前导 /
				if !seenPrefixes[prefix] {
					seenPrefixes[prefix] = true
					// 添加为完整 URL 前缀
					prependURLs = append(prependURLs, baseOrigin+prefix)
				}
			}
		}
	}

	// 去重
	uniquePrependURLs := make([]string, 0, len(prependURLs))
	seen := make(map[string]bool)
	for _, u := range prependURLs {
		if !seen[u] {
			seen[u] = true
			uniquePrependURLs = append(uniquePrependURLs, u)
		}
	}

	// 从 knowledge 收集全局信息
	metadata := SiteMetadata{
		URLs:         jsMetadataList,
		PrependURLs:  uniquePrependURLs,
		PublicPaths:  p.knowledge.GetPublicPaths(),
		DiscoveredAt: time.Now().Unix(),
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		if p.Debug {
			p.debugLog("saveSiteMetadata failed to marshal: %v", err)
		}
		return
	}

	// 保存到缓存根目录的 meta.json 文件
	if err := p.cacheConfig.SaveMetadata(p.baseURL, "", data); err != nil {
		if p.Debug {
			p.debugLog("saveSiteMetadata failed: %v", err)
		}
	}
}

// processFragmentBatch 处理所有待探测的片段
// 使用 worker pool 并行处理所有片段
func (p *Pipeline) processFragmentBatch(ctx context.Context) {
	// 从 slice 收集所有待处理的片段
	p.fragmentMu.Lock()
	fragments := p.fragments
	p.fragments = make([]DiscoveredJS, 0)
	p.fragmentMu.Unlock()

	if len(fragments) == 0 {
		return
	}

	if p.Debug {
		p.debugLog("processFragmentBatch: processing %d fragments in parallel", len(fragments))
	}

	// 使用信号量限制并发数
	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup

	for _, discovered := range fragments {
		sem <- struct{}{}
		wg.Add(1)
		go func(d DiscoveredJS) {
			defer wg.Done()
			defer func() { <-sem }()
			p.processFragment(ctx, d)
		}(discovered)
	}

	wg.Wait()
}

// processFragment 处理单个片段：展开并为每个候选启动 goroutine 处理
func (p *Pipeline) processFragment(ctx context.Context, discovered DiscoveredJS) {
	// 展开片段为候选 URLs
	candidateURLs := p.probeFragment(discovered.URL, discovered.FromURL)
	if p.Debug {
		p.debugLog("processFragment: fragment=%s -> %d candidates", discovered.URL, len(candidateURLs))
	}

	// 为每个候选启动 goroutine 处理
	for _, candidateURL := range candidateURLs {
		normalizedURL := NormalizeURL(candidateURL)

		// 存储上下文
		p.urlContextMu.Lock()
		if _, ok := p.urlContext[normalizedURL]; !ok {
			p.urlContext[normalizedURL] = DiscoveredJS{
				URL:        normalizedURL,
				FromURL:    discovered.FromURL,
				FromPlugin: discovered.FromPlugin,
				IsInline:   discovered.IsInline,
			}
		}
		p.urlContextMu.Unlock()

		// 启动 goroutine 处理
		p.jsWg.Add(1)
		go func(url string) {
			defer p.jsWg.Done()
			p.processJSContentURL(ctx, url)
		}(normalizedURL)
	}

	if p.Debug && len(candidateURLs) == 0 {
		p.debugLog("processFragment: no candidates for fragment: %s", discovered.URL)
	}
}

// processJSContentURL 处理单个 URL 的完整流程（由 goroutine 直接调用）
func (p *Pipeline) processJSContentURL(ctx context.Context, urlStr string) {
	normalizedURL := NormalizeURL(urlStr)

	// 查找上下文
	p.urlContextMu.RLock()
	discovered, ok := p.urlContext[normalizedURL]
	p.urlContextMu.RUnlock()
	if !ok {
		discovered = DiscoveredJS{URL: normalizedURL}
	}

	p.processJSContent(ctx, discovered)
}

// processJSContent 处理单个 JS URL 的完整流程
func (p *Pipeline) processJSContent(ctx context.Context, discovered DiscoveredJS) {
	normalizedURL := discovered.URL

	// 下载内容
	result, err := p.fetcher.FetchWithStatus(normalizedURL)
	if err != nil {
		if p.Debug {
			p.debugLog("processJSContent fetch error: url=%s, err=%v", normalizedURL, err)
		}
		return
	}

	// 只处理 2xx 响应
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return
	}

	// 检测 Content-Type（优先使用响应头）
	contentType := p.detectContentTypeFromHeader(result.ContentType, result.Content)

	// 如果 URL 以 .js/.mjs/.jsonp 结尾，但 Content-Type 是 HTML，直接跳过
	if contentType == ContentTypeHTML && isLikelyStaticResource(normalizedURL) {
		return
	}

	// 分发给所有插件分析
	input := &AnalyzeInput{
		SourceURL:   normalizedURL,
		ContentType: contentType,
		Content:     result.Content,
		Headers:     result.Headers,
	}
	results := p.dispatchPlugins(ctx, input)

	// 先处理插件返回结果，收集 PrependURLs 和 PublicPaths
	p.processResults(ctx, results, normalizedURL)

	// 如果是 JS，加入 jsURLs、foundCh、knownPaths
	if contentType == ContentTypeJS {
		// 保存 JS 到缓存
		p.saveJSToCache(normalizedURL, result.Content)

		// 添加到 jsURLs（使用 discovered 上下文，去重）
		p.jsURLsMu.Lock()
		found := false
		for _, js := range p.jsURLs {
			if js.URL == normalizedURL {
				found = true
				break
			}
		}
		if !found {
			p.jsURLs = append(p.jsURLs, DiscoveredJS{
				URL:        normalizedURL,
				FromURL:    discovered.FromURL,
				FromPlugin: discovered.FromPlugin,
				IsInline:   discovered.IsInline,
			})
		}
		p.jsURLsMu.Unlock()

		// 发送到 foundCh
		p.foundChMu.Lock()
		if !p.foundChSeen[normalizedURL] {
			p.foundChSeen[normalizedURL] = true
			select {
			case p.foundCh <- normalizedURL:
			default:
			}
		}
		p.foundChMu.Unlock()

		// 添加到 knownPaths 供后续探测使用
		p.knowledge.AddKnownPath(normalizedURL)

		if p.Debug {
			p.debugLog("processJSContent: found valid JS: %s", normalizedURL)
		}

		// 探测 .map 文件
		if !p.knowledge.JSHasSourceMap(normalizedURL) {
			go p.probeSourceMap(normalizedURL)
		}
	}
}

// dispatchPlugins 分发插件分析
func (p *Pipeline) dispatchPlugins(ctx context.Context, input *AnalyzeInput) []*Result {
	if p.Debug {
		p.debugLog("dispatchPlugins: sourceURL=%s, contentType=%s, contentLen=%d", input.SourceURL, input.ContentType, len(input.Content))
	}
	// 将 knowledgeBase 添加到 context 中，以便插件访问
	ctx = context.WithValue(ctx, knowledgeKey, p.knowledge)

	var results []*Result
	var mu sync.Mutex

	var wg sync.WaitGroup
	for _, plugin := range p.registry.GetAll() {
		// 跳过不满足前提条件的插件
		if !plugin.Precheck(ctx, input) {
			continue
		}

		wg.Add(1)
		go func(pl Plugin) {
			defer wg.Done()

			result, err := pl.Analyze(ctx, input)
			if err != nil {
				if p.Debug {
					p.debugLog("Plugin %s error: %v", pl.Name(), err)
				}
				return
			}

			if result != nil {
				// 设置来源插件名称
				result.FromPlugin = pl.Name()
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}
		}(plugin)
	}

	wg.Wait()
	if p.Debug {
		p.debugLog("dispatchPlugins: returning %d results", len(results))
	}
	return results
}

// processResults 处理插件返回结果
func (p *Pipeline) processResults(ctx context.Context, results []*Result, sourceURL string) {
	if p.Debug {
		p.debugLog("processResults CALLED: source=%s, results count=%d", sourceURL, len(results))
	}
	for _, r := range results {
		// Debug: 打印插件返回结果
		if p.Debug && (len(r.URLs) > 0 || len(r.ProbeTargets) > 0 || len(r.InlineScripts) > 0) {
			p.debugLog("processResults: source=%s, URLs=%d, ProbeTargets=%d, InlineScripts=%d, from=%s",
				sourceURL, len(r.URLs), len(r.ProbeTargets), len(r.InlineScripts), r.FromPlugin)
			if len(r.ProbeTargets) > 0 {
				p.debugLog("processResults: first ProbeTarget: %s", r.ProbeTargets[0].URL)
			}
		}

		// 更新知识库
		p.knowledge.AddPublicPath(r.PublicPaths...)
		if len(r.PrependURLs) > 0 {
			p.knowledge.AddPrependURL(r.PrependURLs...)
			if p.Debug {
				p.debugLog("PrependURLs from %s at %s: %v", sourceURL, time.Now().Format("15:04:05.000"), r.PrependURLs)
			}
		}

		// 处理完整 URL（直接加入下载队列）
		enqueuedCount := 0
		for i := range r.URLs {
			discovered := &r.URLs[i]
			normalizedURL := NormalizeURL(discovered.URL)
			if !p.knowledge.IsSeenURL(normalizedURL) {
				// 如果是 data: URI（内联 source map），保存到缓存
				if strings.HasPrefix(normalizedURL, "data:") {
					p.saveDataURIToCache(sourceURL, discovered.URL)
					continue
				}
				discovered.FromPlugin = r.FromPlugin
				if p.tryEnqueue(normalizedURL, discovered) {
					enqueuedCount++
					// 注意：不在这里添加到 jsURLs
					// jsURLs 的添加在 processURL/processFragment 中进行，届时从 urlContext 获取上下文
				}
			}
		}
		if p.Debug && enqueuedCount > 0 {
			p.debugLog("Enqueued %d new URLs from %s", enqueuedCount, sourceURL)
		}

		// 处理路径片段：收集到待处理列表，不立即探测
		// 等所有插件运行完毕后，再统一探测
		for _, discovered := range r.ProbeTargets {
			if p.knowledge.IsSeenFragment(discovered.URL) {
				continue
			}
			p.knowledge.MarkSeenFragment(discovered.URL)

			// 如果已经是绝对 URL（包含 ://），直接入队
			if strings.Contains(discovered.URL, "://") {
				discovered.FromPlugin = r.FromPlugin
				normalizedURL := NormalizeURL(discovered.URL)
				if !p.knowledge.IsSeenURL(normalizedURL) {
					if p.Debug {
						p.debugLog("Fragment (absolute URL): %s", normalizedURL)
					}
					p.tryEnqueue(normalizedURL, &discovered)
				}
				continue
			}

			// 如果 fragment 没有路径（只是文件名如 "780.c5f8833f.js"）
			// 直接使用 source URL 的目录 + fragment 作为完整 URL
			if !strings.Contains(discovered.URL, "/") {
				// 优先使用 source URL 的目录
				fromDir := GetDirFromURL(discovered.FromURL)
				if fromDir != "" {
					fullURL := fromDir + discovered.URL
					normalizedURL := NormalizeURL(fullURL)
					if p.Debug {
						p.debugLog("Fragment (from dir): %s -> %s", discovered.URL, normalizedURL)
					}
					if !p.knowledge.IsSeenURL(normalizedURL) {
						p.tryEnqueue(normalizedURL, &DiscoveredJS{
							URL:        normalizedURL,
							FromURL:    discovered.FromURL,
							FromPlugin: r.FromPlugin,
						})
					}
				}

				// 同时尝试 PrependURLs（可能有不同的域名）
				for _, prependURL := range p.knowledge.GetPrependURLs() {
					prependURLStr := prependURL
					if !strings.HasSuffix(prependURLStr, "/") {
						prependURLStr += "/"
					}
					fullURL := prependURLStr + discovered.URL
					normalizedURL := NormalizeURL(fullURL)
					if p.Debug {
						p.debugLog("Fragment (from prepend): %s -> %s", discovered.URL, normalizedURL)
					}
					if !p.knowledge.IsSeenURL(normalizedURL) {
						p.tryEnqueue(normalizedURL, &DiscoveredJS{
							URL:        normalizedURL,
							FromURL:    discovered.FromURL,
							FromPlugin: r.FromPlugin,
						})
					}
				}
				continue
			}

			// 添加到 fragment slice 等待处理
			discovered.FromPlugin = r.FromPlugin
			p.fragmentMu.Lock()
			p.fragments = append(p.fragments, discovered)
			p.fragmentMu.Unlock()
		}

		// 处理中间资源
		for _, intermediate := range r.Intermediates {
			normalizedURL := NormalizeURL(intermediate.URL)
			if !p.knowledge.IsSeenURL(normalizedURL) {
				p.tryEnqueue(normalizedURL, nil) // Intermediates 是配置文件，不需要 JS 上下文
			}
		}

		// 处理内联脚本 - 直接分发给所有插件分析
		for _, inlineScript := range r.InlineScripts {
			// 使用原始 HTML 的 URL 作为 base URL（用于解析相对路径）
			baseURL := inlineScript.SourceURL

			if p.Debug {
				p.debugLog("processResults: found %d inline scripts, processing inline %d (len=%d)", len(r.InlineScripts), inlineScript.Index, len(inlineScript.Content))
			}

			// 检查是否已分析过（基于 baseURL + index）
			inlineKey := fmt.Sprintf("%s:inline:%d", baseURL, inlineScript.Index)
			if p.knowledge.IsSeenURL(inlineKey) {
				continue
			}
			p.knowledge.MarkSeenURL(inlineKey)

			// 构建 AnalyzeInput，SourceURL 指向原始 HTML（用于正确解析相对路径）
			inlineInput := &AnalyzeInput{
				SourceURL:   baseURL,
				ContentType: ContentTypeJS, // 内联脚本按 JS 处理
				Content:     inlineScript.Content,
			}

			// 分发给所有插件分析（只处理 URLs 和 ProbeTargets，不递归处理 InlineScripts）
			inlineResults := p.dispatchPlugins(ctx, inlineInput)

			if p.Debug {
				p.debugLog("processResults: inline script %d returned %d results", inlineScript.Index, len(inlineResults))
			}

			// 处理内联脚本的 URLs 和 ProbeTargets，但不再递归处理其 InlineScripts
			p.processInlineResults(inlineResults, baseURL)
		}
	}
}

// processInlineResults 处理内联脚本的分析结果（不递归处理 InlineScripts）
func (p *Pipeline) processInlineResults(results []*Result, sourceURL string) {
	if p.Debug {
		p.debugLog("processInlineResults ENTER: source=%s, results count=%d", sourceURL, len(results))
	}
	for i, r := range results {
		if p.Debug {
			p.debugLog("processInlineResults: i=%d, r=%p, ProbeTargets len=%d, from=%s", i, r, len(r.ProbeTargets), r.FromPlugin)
		}
		if p.Debug && len(r.ProbeTargets) > 0 {
			p.debugLog("processInlineResults: first=%s, total=%d", r.ProbeTargets[0].URL, len(r.ProbeTargets))
		}
		// 更新知识库
		p.knowledge.AddPublicPath(r.PublicPaths...)
		p.knowledge.AddPrependURL(r.PrependURLs...)

		// 处理完整 URL
		for i := range r.URLs {
			discovered := &r.URLs[i]
			discovered.FromPlugin = r.FromPlugin
			normalizedURL := NormalizeURL(discovered.URL)
			if !p.knowledge.IsSeenURL(normalizedURL) {
				p.tryEnqueue(normalizedURL, discovered)
			}
		}

		// 处理路径片段
		for _, discovered := range r.ProbeTargets {
			if p.knowledge.IsSeenFragment(discovered.URL) {
				continue
			}
			p.knowledge.MarkSeenFragment(discovered.URL)

			// 所有 fragment 都需要探测验证，不能直接加入 KnownPaths
			if p.Debug {
				p.debugLog("processInlineResults: probing fragment=%s from %s at %s", discovered.URL, sourceURL, time.Now().Format("15:04:05.000"))
			}
			candidateURLs := p.probeFragment(discovered.URL, sourceURL)
			if p.Debug {
				p.debugLog("processInlineResults: fragment=%s -> %d candidates at %s: %v", discovered.URL, len(candidateURLs), time.Now().Format("15:04:05.000"), candidateURLs)
			}
			for _, candidateURL := range candidateURLs {
				normalizedURL := NormalizeURL(candidateURL)
				if !p.knowledge.IsSeenURL(normalizedURL) {
					p.tryEnqueue(normalizedURL, &DiscoveredJS{
						URL:        normalizedURL,
						FromURL:    discovered.FromURL,
						FromPlugin: discovered.FromPlugin,
					})
				}
			}
		}

		// 处理中间资源
		for _, intermediate := range r.Intermediates {
			normalizedURL := NormalizeURL(intermediate.URL)
			if !p.knowledge.IsSeenURL(normalizedURL) {
				p.tryEnqueue(normalizedURL, nil) // Intermediates 是配置文件，不需要 JS 上下文
			}
		}

		// 不再递归处理 InlineScripts，避免迭代爆炸
	}
}

// probeFragment 基于已验证的真实 JS URL 来探测 fragment
// 核心思路：用 fragment 的路径部分去真实 JS URL 中匹配，匹配上后在后面加上文件名
func (p *Pipeline) probeFragment(fragment, sourceURL string) []string {
	var candidates []string

	if p.Debug {
		p.debugLog("probeFragment CALLED: fragment=%s, sourceURL=%s", fragment, sourceURL)
	}

	// 分析 fragment 结构
	// 提取路径部分（如 "static/js/async/"）和文件名（如 "test.js"）
	fragmentHasPath := strings.Contains(fragment, "/")
	var fragmentPath string
	var fragmentFile string
	if fragmentHasPath {
		lastSlash := strings.LastIndex(fragment, "/")
		fragmentPath = fragment[:lastSlash+1] // 包含前导 /
		fragmentFile = fragment[lastSlash+1:]
	} else {
		fragmentPath = ""
		fragmentFile = fragment
	}

	// 检查是否是 Webpack chunk ID 模式（数字 ID + 通配符，如 "209-*.js"）
	// 这种模式无法直接探测，因为 hash 在文件名中而不是 JS 内容里
	// 直接跳过，不生成任何候选
	isChunkIdPattern := strings.HasSuffix(fragmentFile, "-*.js") ||
		strings.HasSuffix(fragmentFile, "-*.css")
	if isChunkIdPattern {
		return nil
	}

	// 策略：用 fragment 的路径部分去已知 JS URL 中匹配
	// 如果匹配上，使用已知 JS URL 的目录（到 fragment 路径结尾）+ fragment 文件名
	if fragmentHasPath && fragmentFile != "" {
		for _, knownPath := range p.knowledge.GetKnownPaths() {
			knownDir := GetDirFromURL(knownPath)
			if knownDir == "" {
				continue
			}

			// 检查 knownDir 是否以 fragmentPath 结尾（去掉前导斜杠后匹配）
			// 例如: knownDir = ".../chat/static/js/async/", fragmentPath = "static/js/async/"
			knownDirTrimmed := strings.TrimSuffix(knownDir, "/")
			fragmentPathTrimmed := strings.TrimSuffix(fragmentPath, "/")

			// 检查 knownDir 是否以 fragmentPath 结尾
			if strings.HasSuffix(knownDirTrimmed, fragmentPathTrimmed) {
				// 匹配成功：knownDir + fragmentFile
				candidate := joinURLPath(knownDir, fragmentFile)
				candidates = append(candidates, candidate)
			}
		}
	}

	// 如果 fragment 只是文件名，使用 sourceURL 的目录直接拼接
	// 这是 rspack/webpack runtime 格式（如 ChatGLM 的 {id}.{hash}.js）的标准做法
	if !fragmentHasPath && fragmentFile != "" {
		// 使用 sourceURL 的目录（包含完整路径）
		fromDir := GetDirFromURL(sourceURL)
		if fromDir != "" {
			candidate := joinURLPath(fromDir, fragmentFile)
			candidates = append(candidates, candidate)
		}

		// 尝试使用已知的 PrependURLs
		for _, prependURL := range p.knowledge.GetPrependURLs() {
			// 确保 URL 格式正确（带末尾斜杠）
			prependURLStr := prependURL
			if !strings.HasSuffix(prependURLStr, "/") {
				prependURLStr += "/"
			}
			candidate := prependURLStr + fragmentFile
			candidates = append(candidates, candidate)
		}
	}

	// 如果 fragment 有路径但没有匹配的 knownPath，尝试用所有已知域名组合
	// 这处理 webpack runtime 生成的 chunk 路径（如 static/js/chunk-xxx.hash.js）
	if fragmentHasPath && len(candidates) == 0 {
		if p.Debug {
			p.debugLog("probeFragment: no knownPath match, trying all known domains, fragmentPath=%s, fragmentFile=%s", fragmentPath, fragmentFile)
		}

		// 收集所有已知域名
		domains := make(map[string]bool)
		for _, knownPath := range p.knowledge.GetKnownPaths() {
			if parsed, err := url.Parse(knownPath); err == nil && parsed.Host != "" {
				domains[parsed.Scheme+"://"+parsed.Host] = true
			}
		}
		for _, prependURL := range p.knowledge.GetPrependURLs() {
			if parsed, err := url.Parse(prependURL); err == nil && parsed.Host != "" {
				domains[parsed.Scheme+"://"+parsed.Host] = true
			}
		}
		// 添加 sourceURL 的域名
		if parsed, err := url.Parse(sourceURL); err == nil && parsed.Host != "" {
			domains[parsed.Scheme+"://"+parsed.Host] = true
		}

		// 直接用 PrependURLs 拼接（包含完整路径）
		fullPath := fragmentPath + fragmentFile
		for _, prependURL := range p.knowledge.GetPrependURLs() {
			candidate := joinURLPath(prependURL, fullPath)
			candidates = append(candidates, candidate)
		}

		// 使用 PublicPaths + sourceURL 的域名 + fullPath
		// 例如: publicPath="/ryqq/", fullPath="js/xxx.js", domain from sourceURL
		sourceDomain := ""
		if parsed, err := url.Parse(sourceURL); err == nil {
			sourceDomain = parsed.Scheme + "://" + parsed.Host
		}
		for _, publicPath := range p.knowledge.GetPublicPaths() {
			// 如果 publicPath 是完整 URL，提取其中的路径部分
			pathToUse := publicPath
			if publicPathURL, err := url.Parse(publicPath); err == nil && publicPathURL.Host != "" {
				// publicPath 是完整 URL，使用其路径部分
				pathToUse = publicPathURL.Path
				// 同时可以使用其域名
				if publicPathURL.Scheme != "" && publicPathURL.Host != "" {
					sourceDomain = publicPathURL.Scheme + "://" + publicPathURL.Host
				}
			}
			candidate := joinURLPath(sourceDomain+pathToUse, fullPath)
			candidates = append(candidates, candidate)
		}

		// 用所有已知域名组合 fragment
		for domain := range domains {
			candidate := joinURLPath(domain, fullPath)
			candidates = append(candidates, candidate)
		}

		// 备用：如果 fragment 是根相对路径（以 / 开头），直接与 sourceURL 域名拼接
		// 例如 /p__Login.94493893.async.js -> https://bigdata.xingyeai.com/p__Login.94493893.async.js
		if strings.HasPrefix(fragment, "/") {
			sourceDomain := ""
			if parsed, err := url.Parse(sourceURL); err == nil && parsed.Host != "" {
				sourceDomain = parsed.Scheme + "://" + parsed.Host
			}
			if sourceDomain != "" {
				candidate := sourceDomain + fragment
				candidates = append(candidates, candidate)
			}
		}
	}

	return unique(candidates)
}

// extractHost 从 baseURL 中提取 host 部分，用于生成扁平化缓存路径
// https://example.com/login -> example.com-login
// https://example.com:8080/aa/bb -> example.com_8080-aa-bb
func extractHost(baseURL string) string {
	host := baseURL
	// 去掉尾部斜杠
	host = strings.TrimSuffix(host, "/")
	// 去掉协议
	if strings.HasPrefix(host, "https://") {
		host = host[8:]
	} else if strings.HasPrefix(host, "http://") {
		host = host[7:]
	}
	// 替换冒号为下划线（保留端口）
	host = strings.ReplaceAll(host, ":", "_")
	// 将路径中的 / 替换为 -
	host = strings.ReplaceAll(host, "/", "-")
	return host
}

// unique 去重
func unique(ss []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}
	return unique
}

// joinURLPath 智能拼接 baseURL 和 path
// 自动处理斜杠：前后都有/就去掉一个，前后都没有/就加上一个
func joinURLPath(baseURL, path string) string {
	if path == "" {
		return baseURL
	}
	// 确保 baseURL 不以 / 结尾
	baseURL = strings.TrimSuffix(baseURL, "/")
	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

// isLikelyStaticResource 判断 URL 是否是静态资源（.js/.css 等）
func isLikelyStaticResource(urlStr string) bool {
	lowerURL := strings.ToLower(urlStr)
	return strings.HasSuffix(lowerURL, ".js") ||
		strings.HasSuffix(lowerURL, ".mjs") ||
		strings.HasSuffix(lowerURL, ".jsonp") ||
		strings.HasSuffix(lowerURL, ".css") ||
		strings.HasSuffix(lowerURL, ".woff") ||
		strings.HasSuffix(lowerURL, ".woff2") ||
		strings.HasSuffix(lowerURL, ".ttf") ||
		strings.HasSuffix(lowerURL, ".svg") ||
		strings.HasSuffix(lowerURL, ".png") ||
		strings.HasSuffix(lowerURL, ".jpg")
}


// detectContentTypeFromHeader 优先从 HTTP 响应头检测 Content-Type
func (p *Pipeline) detectContentTypeFromHeader(contentTypeHeader string, content []byte) ContentType {
	// 解析 Content-Type header
	lowerCT := strings.ToLower(contentTypeHeader)
	if strings.HasPrefix(lowerCT, "text/html") || strings.HasPrefix(lowerCT, "application/xhtml") {
		return ContentTypeHTML
	}
	if strings.HasPrefix(lowerCT, "application/json") || strings.HasPrefix(lowerCT, "text/json") {
		return ContentTypeJSON
	}
	if strings.HasPrefix(lowerCT, "application/javascript") ||
		strings.HasPrefix(lowerCT, "text/javascript") ||
		strings.HasPrefix(lowerCT, "application/x-javascript") {
		return ContentTypeJS
	}

	// Content-Type 不明确或为空时，只在有实际内容且内容像 JS 时才返回 JS
	// 不再仅根据 URL 后缀假设是 JS
	if len(content) > 0 {
		trimmed := strings.TrimSpace(string(content[:200]))
		if strings.HasPrefix(trimmed, "<!DOCTYPE") || strings.HasPrefix(trimmed, "<html") {
			return ContentTypeHTML
		}
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return ContentTypeJSON
		}
		// 内容看起来像 JS 代码
		if strings.Contains(trimmed, "function") || strings.Contains(trimmed, "=>") ||
			strings.Contains(trimmed, "require") || strings.Contains(trimmed, "export") {
			return ContentTypeJS
		}
	}

	return ContentTypeHTML // 默认不认为是 JS
}

// GetKnowledgeFromContext 从 context 中获取 KnowledgeBase
func GetKnowledgeFromContext(ctx context.Context) *KnowledgeBase {
	if kb, ok := ctx.Value(knowledgeKey).(*KnowledgeBase); ok {
		return kb
	}
	return nil
}

// GetKnowledge 获取知识库（用于测试）
func (p *Pipeline) GetKnowledge() *KnowledgeBase {
	return p.knowledge
}

// GetFoundCh 获取发现 JS URL 的 channel
func (p *Pipeline) GetFoundCh() <-chan string {
	return p.foundCh
}

// GetOutputResult 获取格式化输出结果
func (p *Pipeline) GetOutputResult() *OutputResult {
	// 提取 URL 字符串列表
	jsURLList := make([]string, 0, len(p.jsURLs))
	for _, js := range p.jsURLs {
		jsURLList = append(jsURLList, js.URL)
	}

	result := &OutputResult{
		Summary: Summary{
			JSCount:        len(p.jsURLs),
			SourceMapCount: 0,
		},
		JSURLs: jsURLList,
	}

	// 设置缓存目录
	if p.cacheConfig != nil && p.cacheConfig.Enable && p.baseURL != "" {
		// 使用与 saveJSToCache 相同的 host 提取逻辑
		host := extractHost(p.baseURL)
		result.CacheBase = filepath.Join(p.cacheConfig.BaseDir, "https_"+host)
		result.CacheDirs = &CacheDirs{
			JS: filepath.Join(result.CacheBase, "js"),
			HTML: filepath.Join(result.CacheBase, "html", "web.html"),
		}

		// 统计 source_map 目录下的文件数量
		smPath := filepath.Join(result.CacheBase, "source_map")
		result.Summary.SourceMapCount = countFilesInDir(smPath)
		if result.Summary.SourceMapCount > 0 {
			result.CacheDirs.SourceMap = smPath
		}
	}

	return result
}

// countFilesInDir 统计目录下文件数量
func countFilesInDir(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}

// buildSourceMapURL 为 JS URL 构建 source map URL
// 处理带查询参数的 JS URL，如：
// 输入: https://example.com/js/app.js?v=123
// 输出: https://example.com/js/app.js.map?v=123
func buildSourceMapURL(jsURL string) string {
	// 分离 path 和 query
	path, query, found := strings.Cut(jsURL, "?")
	if !found {
		query = ""
	}

	// 在 .js 后面插入 .map（如果有 .js 后缀）
	var mapPath string
	if strings.HasSuffix(path, ".js") {
		mapPath = path + ".map"
	} else if strings.HasSuffix(path, ".mjs") {
		mapPath = path + ".map"
	} else if strings.HasSuffix(path, ".css") {
		mapPath = path + ".map"
	} else {
		mapPath = path + ".map"
	}

	if query != "" {
		return mapPath + "?" + query
	}
	return mapPath
}
