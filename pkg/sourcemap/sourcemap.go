// Package sourcemap 提供 source map（v3）的解析与原始源码还原功能。
//
// 还原策略采用"sourcesContent 优先 + mappings 回退"两者结合：
//   - 优先路径：直接读取 source map 的 sourcesContent 字段（打包前的原始明文源码），
//     按 sources 路径写成分离的源文件。速度快、准确，但依赖打包工具保留了该字段。
//   - 回退路径：当 sourcesContent 缺失或对应元素为空时，解析 mappings（VLQ 编码），
//     把压缩后的 JS 按 source 索引切分重组为可读片段。注意：mappings 只能提供位置
//     映射，无法 100% 还原原文，回退产物会在文件头加注释标注"非完整源码"。
package sourcemap

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SourceMap 对应 source map v3 规范的 JSON 结构
// 参考：https://sourcemaps.info/spec.html
type SourceMap struct {
	// Version source map 规范版本（通常为 3）
	Version int `json:"version"`
	// File 生成文件（压缩后的 JS）的名称
	File string `json:"file,omitempty"`
	// SourceRoot 所有 sources 路径的前缀，用于解析相对路径
	SourceRoot string `json:"sourceRoot,omitempty"`
	// Sources 原始源文件路径列表
	Sources []string `json:"sources"`
	// SourcesContent 原始源文件内容列表，与 sources 一一对应；
	// 可能为 null、缺失、或某些元素为 null
	SourcesContent []string `json:"sourcesContent,omitempty"`
	// Names mappings 中引用的标识符名称列表
	Names []string `json:"names,omitempty"`
	// Mappings VLQ 编码的位置映射字符串
	Mappings string `json:"mappings"`
}

// RestoreMode 标记还原出的源文件来源
type RestoreMode int

const (
	// RestoreModeNone 未还原（无可用内容）
	RestoreModeNone RestoreMode = iota
	// RestoreModeSourcesContent 来自 sourcesContent 字段（完整原文）
	RestoreModeSourcesContent
	// RestoreModeMappings 来自 mappings 重组（非完整源码）
	RestoreModeMappings
)

// SourceFile 还原出的单个源文件
type SourceFile struct {
	// Path 规范化后的相对路径（如 src/App.jsx）
	Path string
	// Content 文件内容
	Content []byte
	// Mode 还原方式
	Mode RestoreMode
}

// Parse 解析 source map JSON 内容
func Parse(content []byte) (*SourceMap, error) {
	if len(content) == 0 {
		return nil, errors.New("empty source map content")
	}

	var sm SourceMap
	if err := json.Unmarshal(content, &sm); err != nil {
		return nil, fmt.Errorf("parse source map json: %w", err)
	}

	if sm.Version != 0 && sm.Version != 3 {
		// 版本不为 3 仍尝试解析，但记录下来（v3 是事实标准）
		// 不直接报错，避免对略有偏差的 map 过度严格
	}

	if len(sm.Sources) == 0 {
		return nil, errors.New("source map has no sources")
	}

	return &sm, nil
}

// HasSourcesContent 判断 source map 是否包含可用的 sourcesContent
// 返回 true 当且仅当至少有一个 sourcesContent 元素非空
func (sm *SourceMap) HasSourcesContent() bool {
	for _, c := range sm.SourcesContent {
		if c != "" {
			return true
		}
	}
	return false
}

// RestoreFiles 从 source map 还原出原始源文件列表。
//
// 还原顺序：
//  1. 优先用 sourcesContent（完整原文）
//  2. 若 sourcesContent 对应元素为空，且 minifiedContent 非空，则用 mappings
//     重组压缩 JS 片段作为回退
//
// 参数 minifiedContent 是压缩后的 JS 内容，仅在 sourcesContent 缺失时用于回退。
// 若 sourcesContent 完整，可传 nil。
func RestoreFiles(sm *SourceMap, minifiedContent []byte) ([]SourceFile, error) {
	if sm == nil {
		return nil, errors.New("nil source map")
	}
	if len(sm.Sources) == 0 {
		return nil, errors.New("no sources to restore")
	}

	var result []SourceFile

	// 按 source 索引收集 mappings（仅在有缺失 sourcesContent 时需要）
	var mappings []Mapping
	mappingsParsed := false

	for i, src := range sm.Sources {
		var content string
		mode := RestoreModeNone

		// 优先 sourcesContent
		if i < len(sm.SourcesContent) {
			content = sm.SourcesContent[i]
		}
		if content != "" {
			mode = RestoreModeSourcesContent
		}

		// 回退：mappings 重组
		if mode == RestoreModeNone && len(minifiedContent) > 0 {
			if !mappingsParsed {
				mappings = ParseMappings(sm.Mappings)
				mappingsParsed = true
			}
			reconstructed := reconstructFromMappings(mappings, minifiedContent, i)
			if reconstructed != "" {
				content = reconstructed
				mode = RestoreModeMappings
			}
		}

		if mode == RestoreModeNone {
			continue
		}

		normalizedPath := NormalizeSourcePath(src, sm.SourceRoot)
		if normalizedPath == "" {
			normalizedPath = fmt.Sprintf("unnamed_%d.js", i)
		}

		fileContent := []byte(content)
		// mappings 回退产物加注释标注，避免误导用户以为是完整原文
		if mode == RestoreModeMappings {
			header := fmt.Sprintf(
				"/* [dj] 还原自 source map mappings，非完整源码。\n"+
					"   原始路径: %s\n"+
					"   此文件由压缩 JS 按 source map 位置映射重组，可能不完整。 */\n",
				src,
			)
			fileContent = append([]byte(header), fileContent...)
		}

		result = append(result, SourceFile{
			Path:    normalizedPath,
			Content: fileContent,
			Mode:    mode,
		})
	}

	return result, nil
}

// reconstructFromMappings 从压缩 JS 内容中按指定 source 索引重组源码片段。
//
// 注意：source map 的 mappings 描述的是"压缩代码位置 -> 原始源码位置"的映射，
// 并不包含原始源码本身。因此本函数只能从压缩 JS 中提取出"映射到该 source 的代码片段"，
// 按生成代码的顺序拼接，尽量还原可读性，但无法恢复变量名、注释等被压缩丢失的信息。
//
// 这是 sourcesContent 缺失时的尽力而为方案。
func reconstructFromMappings(mappings []Mapping, minifiedContent []byte, sourceIndex int) string {
	if len(mappings) == 0 || len(minifiedContent) == 0 {
		return ""
	}

	// 按生成代码位置排序，确保重组后的片段顺序与压缩代码一致
	// （mappings 通常已排序，但保险起见）
	var relevant []Mapping
	for _, m := range mappings {
		if m.SourceIndex == sourceIndex {
			relevant = append(relevant, m)
		}
	}
	if len(relevant) == 0 {
		return ""
	}

	// 将 minifiedContent 按行切分，便于按 (line, column) 取字符
	lines := strings.Split(string(minifiedContent), "\n")

	var sb strings.Builder
	prevGenLine := -1
	prevGenCol := 0

	for _, m := range relevant {
		// 跳过超出范围的映射
		if m.GeneratedLine < 0 || m.GeneratedLine >= len(lines) {
			continue
		}
		line := lines[m.GeneratedLine]

		// 同一行内的连续片段：取从上一个片段结尾到当前片段开始位置的字符
		if m.GeneratedLine == prevGenLine {
			start := prevGenCol
			end := m.GeneratedColumn
			if start < 0 {
				start = 0
			}
			if end > len(line) {
				end = len(line)
			}
			if end > start {
				sb.WriteString(line[start:end])
			}
		} else {
			// 跨行：补上前一行的剩余部分
			if prevGenLine >= 0 && prevGenLine < len(lines) {
				prevLine := lines[prevGenLine]
				if prevGenCol < len(prevLine) {
					sb.WriteString(prevLine[prevGenCol:])
				}
				sb.WriteString("\n")
			}
			// 补上中间的空行
			for l := prevGenLine + 1; l < m.GeneratedLine; l++ {
				if l < len(lines) {
					sb.WriteString(lines[l])
					sb.WriteString("\n")
				}
			}
			// 当前行从开头到 column
			end := m.GeneratedColumn
			if end > len(line) {
				end = len(line)
			}
			if end > 0 {
				sb.WriteString(line[:end])
			}
		}
		prevGenLine = m.GeneratedLine
		prevGenCol = m.GeneratedColumn
	}

	// 补上最后一个片段到行尾的内容
	if prevGenLine >= 0 && prevGenLine < len(lines) {
		line := lines[prevGenLine]
		if prevGenCol < len(line) {
			sb.WriteString(line[prevGenCol:])
		}
	}

	return sb.String()
}

// String 实现 RestoreMode 的字符串表示，便于调试输出
func (m RestoreMode) String() string {
	switch m {
	case RestoreModeSourcesContent:
		return "sourcesContent"
	case RestoreModeMappings:
		return "mappings"
	default:
		return "none"
	}
}
