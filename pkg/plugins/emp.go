package plugins

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ejfkdev/dj/pkg/extractor"
)

// EmpPlugin 提取 EMP（@efox/emp）微前端宿主侧的资源清单。
//
// EMP host 的 JS bundle 里存在框架标记（window.__EMP__ /
// __MFE_REMOTE__ 等），且 EMP 部署目录下通常有一份 emp.json
// 联邦模块清单，内容形如：
//
//	{"federatedModules":[{"remote":"host","entry":"emp.js","exposes":{
//	    "./hostEntry":[{"chunks":["js/src_hostEntry_ts.c300e890.js"],...}]
//	}}]}
//
// 其中 entry 是 EMP 运行库 JS 的入口，exposes 列出各模块的全部 chunk
// 路径。这些名字只在 emp.json 中出现，主 bundle 无字面量引用——
// 先探测 emp.json，再从其内容提取 entry 与 exposes chunks。
type EmpPlugin struct{}

// NewEmpPlugin 创建插件
func NewEmpPlugin() *EmpPlugin {
	return &EmpPlugin{}
}

func (p *EmpPlugin) Name() string {
	return "EmpPlugin"
}

func (p *EmpPlugin) Precheck(ctx context.Context, input *extractor.AnalyzeInput) bool {
	switch input.ContentType {
	case extractor.ContentTypeJS, extractor.ContentTypeHTML:
		content := string(input.Content)
		return strings.Contains(content, "__MFE_REMOTE__") ||
			strings.Contains(content, "emp.json") ||
			strings.Contains(content, "__EMP__")
	case extractor.ContentTypeJSON:
		return strings.Contains(string(input.Content), `"federatedModules"`)
	}
	return false
}

// empConfig EMP emp.json 结构（只取需要的字段）
type empConfig struct {
	FederatedModules []struct {
		Remote  string                       `json:"remote"`
		Entry   string                       `json:"entry"`
		Exposes map[string][]empExposeChunks `json:"exposes"`
	} `json:"federatedModules"`
}

type empExposeChunks struct {
	Chunks []string `json:"chunks"`
}

func (p *EmpPlugin) Analyze(ctx context.Context, input *extractor.AnalyzeInput) (*extractor.Result, error) {
	result := &extractor.Result{}

	// JSON: 解析 emp.json，提取 entry 与 exposes chunks
	if input.ContentType == extractor.ContentTypeJSON {
		var cfg empConfig
		if err := json.Unmarshal(input.Content, &cfg); err != nil {
			return result, nil
		}
		for _, fed := range cfg.FederatedModules {
			// entry 是相对部署目录的 JS 路径（如 emp.js）
			resolveOne := func(path string) {
				if path == "" {
					return
				}
				absoluteURL := extractor.ResolveRelativePath(input.SourceURL, path)
				absoluteURL = extractor.NormalizeURL(absoluteURL)
				if !extractor.IsAbsoluteURL(absoluteURL) {
					return
				}
				result.URLs = append(result.URLs, extractor.DiscoveredJS{
					URL:      absoluteURL,
					FromURL:  input.SourceURL,
					IsInline: false,
				})
			}
			resolveOne(fed.Entry)
			for _, exposeList := range fed.Exposes {
				for _, expose := range exposeList {
					for _, chunk := range expose.Chunks {
						resolveOne(chunk)
					}
				}
			}
		}
		return result, nil
	}

	// JS/HTML: 探测同目录 EMP 联邦清单（不同版本文件名不同：emp.json / emp-stats.json）
	base := extractor.GetBaseURL(input.SourceURL)
	if base != "" {
		for _, name := range []string{"emp.json", "emp-stats.json"} {
			result.Intermediates = append(result.Intermediates, extractor.Intermediate{
				URL:     base + "/" + name,
				Type:    extractor.ContentTypeJSON,
				FromURL: input.SourceURL,
			})
		}
	}
	return result, nil
}
