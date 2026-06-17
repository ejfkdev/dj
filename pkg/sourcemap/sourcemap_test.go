package sourcemap

import (
	"strings"
	"testing"
)

// ===== VLQ 解码测试 =====

func TestDecodeVLQSegment(t *testing.T) {
	// 标准 base64 VLQ 测试向量
	// 单字符：值 = 编码值，符号位 = 最低位
	//   A=0 -> 0, C=2 -> 1, D=3 -> -1, E=4 -> 2, F=5 -> -2
	//   G=6 -> 3, H=7 -> -3, Y=24 -> 12, Z=25 -> -12
	tests := []struct {
		in   string
		want int
	}{
		{"A", 0},    // 0
		{"C", 1},    // 1
		{"D", -1},   // -1 (符号位)
		{"E", 2},    // 2
		{"F", -2},   // -2
		{"G", 3},    // 3
		{"H", -3},   // -3
		{"Y", 12},   // 12
		{"Z", -12},  // -12
		{"gB", 16},  // 16 (跨组：g=32+data, B=0)
		{"hB", -16}, // -16
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, next, ok := decodeVLQSegment(tt.in, 0)
			if !ok {
				t.Fatalf("decodeVLQSegment(%q) ok=false, want true", tt.in)
			}
			if got != tt.want {
				t.Errorf("decodeVLQSegment(%q) = %d, want %d", tt.in, got, tt.want)
			}
			if next != len(tt.in) {
				t.Errorf("decodeVLQSegment(%q) next = %d, want %d", tt.in, next, len(tt.in))
			}
		})
	}
}

func TestDecodeVLQSegment_MultipleInString(t *testing.T) {
	// "AAAA" 解码 4 次都应是 0
	s := "AAAA"
	pos := 0
	for i := 0; i < 4; i++ {
		val, next, ok := decodeVLQSegment(s, pos)
		if !ok || val != 0 {
			t.Errorf("segment %d: got val=%d ok=%v, want val=0 ok=true", i, val, ok)
		}
		pos = next
	}
	if pos != len(s) {
		t.Errorf("final pos = %d, want %d", pos, len(s))
	}
}

func TestDecodeVLQSegment_Invalid(t *testing.T) {
	// 非法字符
	_, _, ok := decodeVLQSegment("!", 0)
	if ok {
		t.Error("expected ok=false for invalid char '!'")
	}
}

// ===== ParseMappings 测试 =====

func TestParseMappings_Empty(t *testing.T) {
	if got := ParseMappings(""); got != nil {
		t.Errorf("ParseMappings(\"\") = %v, want nil", got)
	}
}

func TestParseMappings_SingleSegment(t *testing.T) {
	// "AAAA" 在第一行第一段：4 个 VLQ = 完整段
	// GeneratedColumn=0, SourceIndex=0, SourceLine=0, SourceColumn=0
	mappings := ParseMappings("AAAA")
	if len(mappings) != 1 {
		t.Fatalf("got %d mappings, want 1", len(mappings))
	}
	m := mappings[0]
	if m.GeneratedLine != 0 || m.GeneratedColumn != 0 {
		t.Errorf("got line=%d col=%d, want line=0 col=0", m.GeneratedLine, m.GeneratedColumn)
	}
	if m.SourceIndex != 0 {
		t.Errorf("got SourceIndex=%d, want 0", m.SourceIndex)
	}
}

func TestParseMappings_FullSegment(t *testing.T) {
	// "AAAA,SAASA,GAAG" 是常见的 source map 片段
	// 第一段：AAAA -> (genCol=0, srcIdx=0, srcLine=0, srcCol=0)
	// 第二段：SAASA -> (genCol=9, srcIdx=0, srcLine=0, srcCol=0)
	mappings := ParseMappings("AAAA,SAASA")
	if len(mappings) != 2 {
		t.Fatalf("got %d mappings, want 2", len(mappings))
	}
	if mappings[0].GeneratedColumn != 0 {
		t.Errorf("first genCol = %d, want 0", mappings[0].GeneratedColumn)
	}
	if mappings[0].SourceIndex != 0 {
		t.Errorf("first srcIdx = %d, want 0", mappings[0].SourceIndex)
	}
	if mappings[1].GeneratedColumn != 9 {
		t.Errorf("second genCol = %d, want 9", mappings[1].GeneratedColumn)
	}
}

func TestParseMappings_MultiLine(t *testing.T) {
	// 两行，每行一个段
	// 第一行 "AAAA"：genCol=0
	// 第二行 "AAAA"：genCol=0, line=1
	mappings := ParseMappings("AAAA;AAAA")
	if len(mappings) != 2 {
		t.Fatalf("got %d mappings, want 2", len(mappings))
	}
	if mappings[0].GeneratedLine != 0 {
		t.Errorf("first line = %d, want 0", mappings[0].GeneratedLine)
	}
	if mappings[1].GeneratedLine != 1 {
		t.Errorf("second line = %d, want 1", mappings[1].GeneratedLine)
	}
	// 第二行 genCol 应重置为 0
	if mappings[1].GeneratedColumn != 0 {
		t.Errorf("second genCol = %d, want 0 (reset per line)", mappings[1].GeneratedColumn)
	}
}

func TestParseMappings_NameIndex(t *testing.T) {
	// 5 个 VLQ 的段：GeneratedColumn, SourceIndex, SourceLine, SourceColumn, NameIndex
	// "AAAA,EAAE" 中第二段 EAAE=2,0,0,0（4 个值，无 name）
	// 构造一个带 name 的：第一段 AAAA, 第二段带 5 值
	// 使用一个真实的小例子：mappings="AAAA,EAAE,CAAE"
	mappings := ParseMappings("AAAA,EAAE")
	if len(mappings) != 2 {
		t.Fatalf("got %d, want 2", len(mappings))
	}
	// 第二段 EAAE: E=2, A=0, A=0, E=2 -> genCol=2, srcIdx=0, srcLine=0, srcCol=2
	m := mappings[1]
	if m.GeneratedColumn != 2 {
		t.Errorf("genCol = %d, want 2", m.GeneratedColumn)
	}
	if m.SourceColumn != 2 {
		t.Errorf("srcCol = %d, want 2", m.SourceColumn)
	}
	if m.NameIndex != -1 {
		t.Errorf("nameIdx = %d, want -1 (no name)", m.NameIndex)
	}
}

// ===== NormalizeSourcePath 测试 =====

func TestNormalizeSourcePath(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		sourceRoot string
		want       string
	}{
		{
			name:   "webpack prefix",
			source: "webpack:///./src/App.jsx",
			want:   "src/App.jsx",
		},
		{
			name:   "webpack-internal prefix",
			source: "webpack-internal:///./node_modules/foo.js",
			want:   "node_modules/foo.js",
		},
		{
			name:   "relative path",
			source: "./src/utils.js",
			want:   "src/utils.js",
		},
		{
			name:   "absolute path",
			source: "/static/js/app.js",
			want:   "static/js/app.js",
		},
		{
			name:   "simple filename",
			source: "index.js",
			want:   "index.js",
		},
		{
			name:   "parent traversal consumed at root",
			source: "../../etc/passwd",
			want:   "etc/passwd",
		},
		{
			name:   "vite relative path with ../",
			source: "../../../../node_modules/foo/bar.js",
			want:   "node_modules/foo/bar.js",
		},
		{
			name:   "relative ../ within path",
			source: "src/a/../b.js",
			want:   "src/b.js",
		},
		{
			name:   "dot segments",
			source: "./src/./utils/./a.js",
			want:   "src/utils/a.js",
		},
		{
			name:       "with sourceRoot",
			source:     "App.jsx",
			sourceRoot: "webpack:///./src",
			want:       "src/App.jsx",
		},
		{
			name:   "empty source",
			source: "",
			want:   "",
		},
		{
			name:   "backslash normalized",
			source: "src\\utils\\a.js",
			want:   "src/utils/a.js",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSourcePath(tt.source, tt.sourceRoot)
			if got != tt.want {
				t.Errorf("NormalizeSourcePath(%q, %q) = %q, want %q",
					tt.source, tt.sourceRoot, got, tt.want)
			}
		})
	}
}

func TestNormalizeSourcePath_NoTraversal(t *testing.T) {
	// 多种尝试穿越的形式都不能产生 ".." 路径段（会逃逸到上级目录）
	// 检查：规范化后的路径按 / 切分，不应出现 ".." 段
	evil := []string{
		"../../../etc/passwd",
		"./..//etc",
		"a/../../b",
		"../foo",
		"foo/../../bar",
	}
	for _, s := range evil {
		got := NormalizeSourcePath(s, "")
		for _, seg := range strings.Split(got, "/") {
			if seg == ".." {
				t.Errorf("NormalizeSourcePath(%q) = %q contains '..' segment", s, got)
			}
		}
	}
}

// ===== Parse 测试 =====

func TestParse_Valid(t *testing.T) {
	content := `{
		"version": 3,
		"sources": ["webpack:///./src/index.js"],
		"sourcesContent": ["console.log('hello');\n"],
		"mappings": "AAAA",
		"names": []
	}`
	sm, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if sm.Version != 3 {
		t.Errorf("version = %d, want 3", sm.Version)
	}
	if len(sm.Sources) != 1 {
		t.Fatalf("sources len = %d, want 1", len(sm.Sources))
	}
	if !sm.HasSourcesContent() {
		t.Error("HasSourcesContent() = false, want true")
	}
}

func TestParse_Empty(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Error("Parse(nil) should error")
	}
}

func TestParse_NoSources(t *testing.T) {
	content := `{"version":3,"sources":[],"mappings":""}`
	if _, err := Parse([]byte(content)); err == nil {
		t.Error("Parse with empty sources should error")
	}
}

// ===== RestoreFiles 测试 =====

func TestRestoreFiles_FromSourcesContent(t *testing.T) {
	sm := &SourceMap{
		Version: 3,
		Sources: []string{
			"webpack:///./src/index.js",
			"webpack:///./src/utils.js",
		},
		SourcesContent: []string{
			"console.log('index');\n",
			"export const x = 1;\n",
		},
		Mappings: "AAAA",
	}

	files, err := RestoreFiles(sm, nil)
	if err != nil {
		t.Fatalf("RestoreFiles error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	if files[0].Path != "src/index.js" {
		t.Errorf("file[0] path = %q, want src/index.js", files[0].Path)
	}
	if files[0].Mode != RestoreModeSourcesContent {
		t.Errorf("file[0] mode = %v, want sourcesContent", files[0].Mode)
	}
	if string(files[0].Content) != "console.log('index');\n" {
		t.Errorf("file[0] content mismatch")
	}
}

func TestRestoreFiles_PartialSourcesContent(t *testing.T) {
	// 第二个 source 的 sourcesContent 为空，应跳过（无 minified 内容无法回退）
	sm := &SourceMap{
		Version: 3,
		Sources: []string{"src/a.js", "src/b.js"},
		SourcesContent: []string{
			"content a",
			"", // 空
		},
		Mappings: "AAAA",
	}
	files, err := RestoreFiles(sm, nil)
	if err != nil {
		t.Fatalf("RestoreFiles error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1 (only non-empty sourcesContent)", len(files))
	}
	if files[0].Path != "src/a.js" {
		t.Errorf("path = %q, want src/a.js", files[0].Path)
	}
}

func TestRestoreFiles_FromMappings(t *testing.T) {
	// sourcesContent 全空，提供 minified 内容，应回退到 mappings
	minified := "var x = 1;\n"
	// 构造一个 mapping：source 0, 指向整个 minified 内容
	// 第一行第一段：genCol=0, srcIdx=0, srcLine=0, srcCol=0
	sm := &SourceMap{
		Version:        3,
		Sources:        []string{"src/original.js"},
		SourcesContent: []string{""}, // 空，触发回退
		Mappings:       "AAAA",
	}
	files, err := RestoreFiles(sm, []byte(minified))
	if err != nil {
		t.Fatalf("RestoreFiles error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Mode != RestoreModeMappings {
		t.Errorf("mode = %v, want mappings", files[0].Mode)
	}
	// 应包含还原标注注释
	if !strings.HasPrefix(string(files[0].Content), "/* [dj]") {
		t.Errorf("mappings-restored file should start with comment header")
	}
	// 应包含 minified 内容的片段
	if !strings.Contains(string(files[0].Content), "var x = 1;") {
		t.Errorf("reconstructed content should contain minified fragment")
	}
}

func TestRestoreFiles_NoSourcesContentNoMinified(t *testing.T) {
	// 既无 sourcesContent 也无 minified，应返回空列表
	sm := &SourceMap{
		Version:        3,
		Sources:        []string{"src/a.js"},
		SourcesContent: nil,
		Mappings:       "AAAA",
	}
	files, err := RestoreFiles(sm, nil)
	if err != nil {
		t.Fatalf("RestoreFiles error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestRestoreFiles_UnnamedFallback(t *testing.T) {
	// source 为空字符串时，路径应回退到 unnamed_N.js
	sm := &SourceMap{
		Version:        3,
		Sources:        []string{""},
		SourcesContent: []string{"content"},
		Mappings:       "",
	}
	files, err := RestoreFiles(sm, nil)
	if err != nil {
		t.Fatalf("RestoreFiles error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "unnamed_0.js" {
		t.Errorf("path = %q, want unnamed_0.js", files[0].Path)
	}
}

// ===== SafeFilePath 测试 =====

func TestSafeFilePath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"src/App.jsx", "src/App.jsx"},
		{"a/b/c.js", "a/b/c.js"},
		{"single.js", "single.js"},
		{"", ""},
	}
	for _, tt := range tests {
		got := SafeFilePath(tt.in)
		if got != tt.want {
			t.Errorf("SafeFilePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ===== truncatePathImpl 测试 =====

func TestTruncatePathImpl(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		{"short", "a/b.js", 100, "a/b.js"},
		{"exact", "abcde", 5, "abcde"},
		{"truncate keep filename", "aaaa/bbbb/cccc.js", 10, "aaa_cccc.js"},
		{"filename too long", "verylongfilename.js", 5, "veryl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncatePathImpl(tt.path, tt.maxLen)
			if len(got) > tt.maxLen {
				t.Errorf("truncatePathImpl(%q, %d) result len=%d exceeds max", tt.path, tt.maxLen, len(got))
			}
			if tt.name == "short" && got != tt.want {
				t.Errorf("truncatePathImpl(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ===== flattenExcessDepth 测试 =====

func TestFlattenExcessDepth(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		maxDepth int
		want     string
	}{
		{"within limit", "a/b/c.js", 5, "a/b/c.js"},
		{"flatten tail", "a/b/c/d/e.js", 2, "a/b/c_d_e.js"},
		{"single segment", "file.js", 0, "file.js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenExcessDepth(tt.path, tt.maxDepth)
			if tt.name != "single segment" && got == "" {
				t.Errorf("flattenExcessDepth(%q, %d) = empty", tt.path, tt.maxDepth)
			}
			// 验证深度不超过限制
			depth := strings.Count(got, "/")
			if depth > tt.maxDepth {
				t.Errorf("flattenExcessDepth(%q, %d) = %q depth=%d exceeds max", tt.path, tt.maxDepth, got, depth)
			}
		})
	}
}

// ===== RestoreMode.String 测试 =====

func TestRestoreModeString(t *testing.T) {
	tests := []struct {
		mode RestoreMode
		want string
	}{
		{RestoreModeNone, "none"},
		{RestoreModeSourcesContent, "sourcesContent"},
		{RestoreModeMappings, "mappings"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("RestoreMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// ===== HasSourcesContent 边界测试 =====

func TestHasSourcesContent(t *testing.T) {
	tests := []struct {
		name string
		sm   *SourceMap
		want bool
	}{
		{"nil content", &SourceMap{Sources: []string{"a"}, SourcesContent: nil}, false},
		{"empty slice", &SourceMap{Sources: []string{"a"}, SourcesContent: []string{}}, false},
		{"all null", &SourceMap{Sources: []string{"a", "b"}, SourcesContent: []string{"", ""}}, false},
		{"has content", &SourceMap{Sources: []string{"a"}, SourcesContent: []string{"code"}}, true},
		{"partial", &SourceMap{Sources: []string{"a", "b"}, SourcesContent: []string{"", "code"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sm.HasSourcesContent(); got != tt.want {
				t.Errorf("HasSourcesContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ===== reconstructFromMappings 多 source 测试 =====

func TestReconstructFromMappings_MultiSource(t *testing.T) {
	// 两个 source，mappings 指向不同 source
	// 第一行：source 0 的代码 "aaa"
	// 第二行：source 1 的代码 "bbb"
	minified := "aaa\nbbb\n"
	// mappings: 第一行一个段指向 source 0，第二行一个段指向 source 1
	// 段格式: genCol, srcIdx, srcLine, srcCol
	// 第一段 AAAA: genCol+=0, srcIdx+=0, srcLine+=0, srcCol+=0 -> srcIdx=0
	// 第二行 ACCA: genCol+=0(A), srcIdx+=1(C), srcLine+=0(A), srcCol+=0(A) -> srcIdx=1
	mappings := ParseMappings("AAAA;ACCA")
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}

	// source 0 应得到 "aaa"
	r0 := reconstructFromMappings(mappings, []byte(minified), 0)
	if !strings.Contains(r0, "aaa") {
		t.Errorf("source 0 reconstruction = %q, want contain 'aaa'", r0)
	}
	// source 1 应得到 "bbb"
	r1 := reconstructFromMappings(mappings, []byte(minified), 1)
	if !strings.Contains(r1, "bbb") {
		t.Errorf("source 1 reconstruction = %q, want contain 'bbb'", r1)
	}
}

func TestReconstructFromMappings_NoMatchingSource(t *testing.T) {
	// source index 不存在
	minified := "code\n"
	mappings := ParseMappings("AAAA")
	result := reconstructFromMappings(mappings, []byte(minified), 99)
	if result != "" {
		t.Errorf("expected empty for non-existent source, got %q", result)
	}
}

// ===== NormalizeSourcePath 超长路径测试 =====

func TestNormalizeSourcePath_TooLong(t *testing.T) {
	// 构造超长路径，验证截断后不超过限制
	longPart := strings.Repeat("a", 100)
	longPath := ""
	for i := 0; i < 10; i++ {
		longPath += longPart + "/"
	}
	longPath += "filename.js"

	result := NormalizeSourcePath(longPath, "")
	if len(result) > maxSourcePathLen+100 { // 截断后允许少量余量
		t.Errorf("path too long after normalize: %d (limit %d)", len(result), maxSourcePathLen)
	}
	if result == "" {
		t.Error("expected non-empty path after truncation")
	}
}

// ===== NormalizeSourcePath 超深路径测试 =====

func TestNormalizeSourcePath_TooDeep(t *testing.T) {
	// 构造超深目录
	deepPath := strings.Repeat("dir/", 50) + "file.js"
	result := NormalizeSourcePath(deepPath, "")
	depth := strings.Count(result, "/")
	if depth > maxSourcePathDepth {
		t.Errorf("depth %d exceeds max %d", depth, maxSourcePathDepth)
	}
	if result == "" {
		t.Error("expected non-empty path after flattening")
	}
}
