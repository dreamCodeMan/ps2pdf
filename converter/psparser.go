package converter

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// ImageReplacement 记录图片替换详情
type ImageReplacement struct {
	RawBytes     []byte
	RealFilePath string
	FileName     string
	Count        int
}

// PatchReport 记录修补报告
type PatchReport struct {
	PsdefinePatched bool
	Replacements    []ImageReplacement
	MissingImages   []string
}

// NormalizeFileName 归一化文件名（全半角标点、连续点、空格与小写），用于智能模糊匹配
func NormalizeFileName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "（", "(")
	name = strings.ReplaceAll(name, "）", ")")
	name = strings.ReplaceAll(name, "【", "[")
	name = strings.ReplaceAll(name, "】", "]")
	name = strings.ReplaceAll(name, "—", "-")
	name = strings.ReplaceAll(name, "..", ".")
	var sb strings.Builder
	for _, r := range name {
		if !unicode.IsSpace(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// DecodeGBKOrLatin1 解码 GBK，如失败退回 Latin-1
func DecodeGBKOrLatin1(data []byte) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err == nil {
		return string(decoded)
	}
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}

// ResolveAndPatchImagePaths 自动扫描 PS 中的图片引用，在本地智能匹配并直接替换为真实绝对路径（无需任何软链接）
func ResolveAndPatchImagePaths(psData []byte, searchDir string) ([]byte, PatchReport) {
	var report PatchReport

	// 1. 禁用 /psdefine 反盗版检查
	idx1 := bytes.Index(psData, []byte("/psdefine"))
	if idx1 != -1 {
		idx2 := bytes.Index(psData[idx1:], []byte("} bd"))
		if idx2 != -1 {
			idx2End := idx1 + idx2 + 4
			newPS := make([]byte, 0, len(psData))
			newPS = append(newPS, psData[:idx1]...)
			newPS = append(newPS, []byte("/psdefine { } bd")...)
			newPS = append(newPS, psData[idx2End:]...)
			psData = newPS
			report.PsdefinePatched = true
		}
	}

	// 2. 建立本地文件索引 (支持当前工作区及子目录检索)
	exactMap := make(map[string]string)
	lowerMap := make(map[string]string)
	normMap := make(map[string]string)
	var allFiles []string

	_ = filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != searchDir {
				return filepath.SkipDir
			}
			return nil
		}
		// 忽略临时文件和符号链接
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		name := d.Name()
		allFiles = append(allFiles, path)
		if _, exists := exactMap[name]; !exists {
			exactMap[name] = path
			lowerMap[strings.ToLower(name)] = path
			normMap[NormalizeFileName(name)] = path
		}
		return nil
	})

	// 3. 扫描 PS 文件中所有引用的图片路径
	reImgPath := regexp.MustCompile(`\(([^()\r\n]+?\.(?i:jpg|jpeg|png|tif|tiff|eps|bmp))\)`)
	matches := reImgPath.FindAllSubmatch(psData, -1)

	var replacements []ImageReplacement
	seenRaw := make(map[string]bool)

	for _, m := range matches {
		rawPath := m[1]
		if seenRaw[string(rawPath)] {
			continue
		}
		seenRaw[string(rawPath)] = true

		// 截取路径中的文件名部分
		lastSlash := bytes.LastIndex(rawPath, []byte(`\\`))
		if slash2 := bytes.LastIndex(rawPath, []byte(`\`)); slash2 > lastSlash {
			lastSlash = slash2
		}
		if slash3 := bytes.LastIndex(rawPath, []byte(`/`)); slash3 > lastSlash {
			lastSlash = slash3
		}

		var rawFilename []byte
		if lastSlash != -1 {
			rawFilename = rawPath[lastSlash+1:]
		} else {
			rawFilename = rawPath
		}
		rawFilename = bytes.TrimLeft(rawFilename, `\/`)

		targetName := DecodeGBKOrLatin1(rawFilename)

		// 多级智能匹配真实本地文件
		var realFilePath string
		if p, ok := exactMap[targetName]; ok {
			realFilePath = p
		} else if p, ok := lowerMap[strings.ToLower(targetName)]; ok {
			realFilePath = p
		} else if p, ok := normMap[NormalizeFileName(targetName)]; ok {
			realFilePath = p
		} else {
			targetNorm := NormalizeFileName(targetName)
			for _, p := range allFiles {
				base := filepath.Base(p)
				if NormalizeFileName(base) == targetNorm {
					realFilePath = p
					break
				}
			}
		}

		if realFilePath == "" {
			report.MissingImages = append(report.MissingImages, targetName)
			continue
		}

		replacements = append(replacements, ImageReplacement{
			RawBytes:     rawPath,
			RealFilePath: realFilePath,
			FileName:     filepath.Base(realFilePath),
		})
	}

	// 4. 直接替换为真实本地绝对路径
	for i := range replacements {
		cnt := bytes.Count(psData, replacements[i].RawBytes)
		psData = bytes.ReplaceAll(psData, replacements[i].RawBytes, []byte(replacements[i].RealFilePath))
		replacements[i].Count = cnt
	}

	report.Replacements = replacements
	return psData, report
}

// ParsePSString 解析 PostScript 字符串转义
func ParsePSString(s []byte) []byte {
	var res []byte
	n := len(s)
	i := 0
	for i < n {
		b := s[i]
		if b == '\\' {
			i++
			if i >= n {
				break
			}
			c := s[i]
			if c >= '0' && c <= '7' {
				octalStr := []byte{c}
				if i+1 < n && s[i+1] >= '0' && s[i+1] <= '7' {
					i++
					octalStr = append(octalStr, s[i])
					if i+1 < n && s[i+1] >= '0' && s[i+1] <= '7' {
						i++
						octalStr = append(octalStr, s[i])
					}
				}
				val, _ := strconv.ParseInt(string(octalStr), 8, 32)
				res = append(res, byte(val))
			} else {
				switch c {
				case 'n':
					res = append(res, 10)
				case 'r':
					res = append(res, 13)
				case 't':
					res = append(res, 9)
				case '(':
					res = append(res, '(')
				case ')':
					res = append(res, ')')
				case '\\':
					res = append(res, '\\')
				default:
					res = append(res, c)
				}
			}
		} else {
			res = append(res, b)
		}
		i++
	}
	return res
}

// BuildCharMapping 从 PS 文件中解析 DownLoadCode 及字符编码映射
func BuildCharMapping(psPath string) (map[string]map[byte]string, error) {
	file, err := os.Open(psPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return BuildCharMappingFromReader(file)
}

// BuildCharMappingFromReader 从 io.Reader 中解析字符映射
func BuildCharMappingFromReader(r io.Reader) (map[string]map[byte]string, error) {
	var currentFont string
	var lastDlCode string
	hasLastDlCode := false
	mapping := make(map[string]map[byte]string)

	reFont := regexp.MustCompile(`(DLF-[0-9]+-[0-9]+-[0-9]+)`)
	reMatch := regexp.MustCompile(`\((.*?)\)\s*\[[^\]]*\]\s*\d+\s*(?:fxs|fys|VT)`)

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		if bytes.Contains(line, []byte("setfont")) || bytes.Contains(line, []byte("selectfont")) {
			m := reFont.FindSubmatch(line)
			if len(m) > 1 {
				currentFont = string(m[1])
			}
		}

		if bytes.HasPrefix(line, []byte("%%DownLoadCode")) {
			val := bytes.TrimRight(line[14:], "\r\n")
			if bytes.HasPrefix(val, []byte(" ")) {
				val = val[1:]
			}
			if len(val) == 0 {
				lastDlCode = " "
			} else {
				lastDlCode = DecodeGBKOrLatin1(val)
			}
			hasLastDlCode = true
		}

		mMatch := reMatch.FindSubmatch(line)
		if len(mMatch) > 1 && currentFont != "" && hasLastDlCode {
			raw := mMatch[1]
			parsed := ParsePSString(raw)
			if len(parsed) == 1 {
				code := parsed[0]
				if _, ok := mapping[currentFont]; !ok {
					mapping[currentFont] = make(map[byte]string)
				}
				mapping[currentFont][code] = lastDlCode
			}
			hasLastDlCode = false
			lastDlCode = ""
		}
	}

	return mapping, scanner.Err()
}
