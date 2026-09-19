package converter

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

// utf16BEHex 将字符串转为 UTF-16BE 的十六进制大写表示 (例如 "\u4e2d" -> "4E2D")
func utf16BEHex(s string) string {
	u16 := utf16.Encode([]rune(s))
	var sb strings.Builder
	for _, val := range u16 {
		sb.WriteString(fmt.Sprintf("%04X", val))
	}
	return sb.String()
}

// MakeToUnicodeCMap 生成符合 Adobe 规范的 ToUnicode CMap
func MakeToUnicodeCMap(codeToChar map[byte]string) []byte {
	var lines []string
	lines = append(lines,
		"/CIDInit /ProcSet findresource begin",
		"12 dict begin", "begincmap",
		"/CIDSystemInfo <<",
		"  /Registry (Adobe)", "  /Ordering (UCS)", "  /Supplement 0",
		">> def",
		"/CMapName /Founder-ToUnicode def",
		"/CMapType 2 def",
		"1 begincodespacerange", "  <00> <FF>", "endcodespacerange",
	)

	type entry struct {
		code byte
		ch   string
	}
	items := make([]entry, 0, len(codeToChar))
	for k, v := range codeToChar {
		items = append(items, entry{code: k, ch: v})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].code < items[j].code
	})

	for i := 0; i < len(items); i += 100 {
		end := i + 100
		if end > len(items) {
			end = len(items)
		}
		chunk := items[i:end]
		lines = append(lines, fmt.Sprintf("%d beginbfchar", len(chunk)))
		for _, item := range chunk {
			lines = append(lines, fmt.Sprintf("<%02X> <%s>", item.code, utf16BEHex(item.ch)))
		}
		lines = append(lines, "endbfchar")
	}

	lines = append(lines, "endcmap", "CMapName currentdict /CMap defineresource pop", "end", "end")
	return []byte(strings.Join(lines, "\n"))
}
