package sourcemap

// Base64 VLQ 编码使用的字符表（source map 标准）
// 参考：https://sourcemaps.info/spec.html#h.qz3o9nc69um5
const base64VLQChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// base64VLQDecodeMap 字符 -> 值 的反查表，初始化时构建
var base64VLQDecodeMap [128]int8

func init() {
	for i := range base64VLQDecodeMap {
		base64VLQDecodeMap[i] = -1
	}
	for i := 0; i < len(base64VLQChars); i++ {
		base64VLQDecodeMap[base64VLQChars[i]] = int8(i)
	}
}

// VLQ (Variable-Length Quantity) 解码常量
// source map 使用 5-bit 一组的 VLQ 编码，第 6 位（0x20）是 continuation bit
const (
	vlqBaseShift = 5
	vlqBase      = 1 << vlqBaseShift // 32
	vlqBaseMask  = vlqBase - 1       // 0b11111
	vlqSignBit   = 1                 // 最低位是符号位
)

// decodeVLQSegment 解码单个 VLQ 段（从字符串 s 的位置 start 开始）
// 返回解码出的整数值和下一个字符的位置
// 如果遇到非法字符返回 ok=false
func decodeVLQSegment(s string, start int) (value int, next int, ok bool) {
	var result int
	var shift uint
	for i := start; i < len(s); i++ {
		c := s[i]
		if int(c) >= len(base64VLQDecodeMap) {
			return 0, i, false
		}
		digit := base64VLQDecodeMap[c]
		if digit < 0 {
			return 0, i, false
		}

		// continuation bit 是第 6 位（0x20），低 5 位是数据
		continuation := (digit >> vlqBaseShift) & 1
		dataBits := digit & vlqBaseMask
		result |= int(dataBits) << shift
		shift += vlqBaseShift

		// continuation bit 为 0 表示结束
		if continuation == 0 {
			// 处理符号位（最低位）
			negative := result&vlqSignBit != 0
			value := result >> 1
			if negative {
				value = -value
			}
			return value, i + 1, true
		}
	}
	return 0, start, false
}

// Mapping 表示 source map mappings 中的单条映射
type Mapping struct {
	// GeneratedLine 生成代码（压缩后）的行号，0-based
	GeneratedLine int
	// GeneratedColumn 生成代码的列号，0-based
	GeneratedColumn int
	// SourceIndex sources 数组的索引（-1 表示无）
	SourceIndex int
	// SourceLine 原始源码的行号，0-based（-1 表示无）
	SourceLine int
	// SourceColumn 原始源码的列号，0-based（-1 表示无）
	SourceColumn int
	// NameIndex names 数组的索引（-1 表示无）
	NameIndex int
}

// ParseMappings 解析 source map 的 mappings 字符串
// mappings 字符串格式：行之间用 ; 分隔，段之间用 , 分隔
// 每个段是 1、4 或 5 个 VLQ 编码的值：
//   - 1 个值：GeneratedColumn（与前一段同 source）
//   - 4 个值：GeneratedColumn, SourceIndex, SourceLine, SourceColumn
//   - 5 个值：GeneratedColumn, SourceIndex, SourceLine, SourceColumn, NameIndex
//
// 除 GeneratedColumn 外，所有值都是相对前一段的增量（跨行也保持增量）
func ParseMappings(mappings string) []Mapping {
	if mappings == "" {
		return nil
	}

	var result []Mapping

	// 跨行保持的累加器（增量编码）
	var genCol, srcIdx, srcLine, srcCol, nameIdx int

	lines := splitMappingsBySemicolon(mappings)
	for lineIdx, line := range lines {
		// 每行开始时，GeneratedColumn 重置为 0
		genCol = 0
		if line == "" {
			continue
		}

		segments := splitMappingsByComma(line)
		for _, seg := range segments {
			if seg == "" {
				continue
			}

			pos := 0
			// 1. GeneratedColumn（必有）
			val, next, ok := decodeVLQSegment(seg, pos)
			if !ok {
				break
			}
			pos = next
			genCol += val

			m := Mapping{
				GeneratedLine:   lineIdx,
				GeneratedColumn: genCol,
				SourceIndex:     -1,
				SourceLine:      -1,
				SourceColumn:    -1,
				NameIndex:       -1,
			}

			// 2. SourceIndex（可选）
			if pos < len(seg) {
				val, next, ok = decodeVLQSegment(seg, pos)
				if !ok {
					break
				}
				pos = next
				srcIdx += val
				m.SourceIndex = srcIdx

				// 3. SourceLine
				val, next, ok = decodeVLQSegment(seg, pos)
				if !ok {
					break
				}
				pos = next
				srcLine += val
				m.SourceLine = srcLine

				// 4. SourceColumn
				val, next, ok = decodeVLQSegment(seg, pos)
				if !ok {
					break
				}
				pos = next
				srcCol += val
				m.SourceColumn = srcCol

				// 5. NameIndex（可选）
				if pos < len(seg) {
					val, next, ok = decodeVLQSegment(seg, pos)
					if !ok {
						break
					}
					pos = next
					nameIdx += val
					m.NameIndex = nameIdx
				}
			}

			result = append(result, m)
		}
	}

	return result
}

// splitMappingsBySemicolon 按 ; 分割字符串（保留空段，用于行号对齐）
func splitMappingsBySemicolon(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// splitMappingsByComma 按 , 分割字符串，跳过空段
func splitMappingsByComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
