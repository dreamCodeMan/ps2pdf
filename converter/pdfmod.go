package converter

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reTrailer  = regexp.MustCompile(`(?s)trailer\s*<<(.*?)>>\s*startxref`)
	reObjStart = regexp.MustCompile(`(?m)^(\d+)\s+(\d+)\s+obj\b`)
	reBaseFont = regexp.MustCompile(`/BaseFont\s*/([A-Za-z0-9_\-+]+)`)
	reWidths   = regexp.MustCompile(`(?s)/Widths\s*\[(.*?)\]`)
	reDecode   = regexp.MustCompile(`(?s)/Decode\s*\[.*?\]`)
	reSize     = regexp.MustCompile(`/Size\s+\d+`)
)

// PDFObject 表示解析后的 PDF 对象
type PDFObject struct {
	ID   int
	Gen  int
	Body []byte // 包括 << ... >> 以及可能的 stream ... endstream
}

// PostProcessResult 保存 PDF 后处理的统计结果
type PostProcessResult struct {
	InjectedFonts int
	FixedCMYK     int
}

// ParsePDFObjectsSafe 流安全的对象解析器，防止流二进制数据中偶然出现的 endobj 引起误截断
func ParsePDFObjectsSafe(data []byte) ([]*PDFObject, string, int, error) {
	mTrailer := reTrailer.FindSubmatch(data)
	if mTrailer == nil {
		return nil, "", 0, fmt.Errorf("valid PDF trailer not found")
	}
	trailerBody := string(mTrailer[1])

	locs := reObjStart.FindAllSubmatchIndex(data, -1)
	var objs []*PDFObject
	maxID := 0

	for _, loc := range locs {
		id, _ := strconv.Atoi(string(data[loc[2]:loc[3]]))
		gen, _ := strconv.Atoi(string(data[loc[4]:loc[5]]))
		if id > maxID {
			maxID = id
		}
		objStart := loc[1]

		nextStream := bytes.Index(data[objStart:], []byte("stream"))
		nextEndobj := bytes.Index(data[objStart:], []byte("endobj"))

		var bodyEnd int
		if nextStream != -1 && (nextEndobj == -1 || nextStream < nextEndobj) {
			// 含有 stream，跳过 endstream 之后再寻找 endobj
			endStreamIdx := bytes.Index(data[objStart+nextStream:], []byte("endstream"))
			if endStreamIdx != -1 {
				realEndStream := objStart + nextStream + endStreamIdx + 9
				endObjIdx := bytes.Index(data[realEndStream:], []byte("endobj"))
				if endObjIdx != -1 {
					bodyEnd = realEndStream + endObjIdx
				}
			}
		} else if nextEndobj != -1 {
			bodyEnd = objStart + nextEndobj
		}

		if bodyEnd > objStart {
			body := bytes.TrimSpace(data[objStart:bodyEnd])
			objs = append(objs, &PDFObject{
				ID:   id,
				Gen:  gen,
				Body: body,
			})
		}
	}

	return objs, trailerBody, maxID, nil
}

// PostProcessPDFBytes 在内存中对 base PDF 数据进行后处理：
// 1. 注入 ToUnicode CMap 流，使字体支持文本复制/搜索
// 2. 修正 DLF-32769 标点字体的字宽为 1000
// 3. 修正 /DeviceCMYK 图片的 /Decode 数组
// 4. 重建交叉引用表 (xref) 与 trailer 并返回最终 PDF 二进制
func PostProcessPDFBytes(pdfData []byte, mapping map[string]map[byte]string) ([]byte, *PostProcessResult, error) {
	objs, trailerBody, maxID, err := ParsePDFObjectsSafe(pdfData)
	if err != nil {
		return nil, nil, err
	}

	result := &PostProcessResult{}
	var newObjs []*PDFObject

	for _, obj := range objs {
		bodyStr := string(obj.Body)

		// 1. 检查是否是 Font 对象
		if strings.Contains(bodyStr, "/Font") || strings.Contains(bodyStr, "/Type1") {
			mBF := reBaseFont.FindStringSubmatch(bodyStr)
			if len(mBF) > 1 {
				fullFont := mBF[1]
				parts := strings.Split(fullFont, "+")
				cleanName := strings.TrimPrefix(parts[len(parts)-1], "/")

				// ToUnicode 注入
				if charMap, ok := mapping[cleanName]; ok {
					cmapBytes := MakeToUnicodeCMap(charMap)
					maxID++
					toUnicodeID := maxID

					var cmapBody bytes.Buffer
					cmapBody.WriteString(fmt.Sprintf("<< /Length %d >>\nstream\n", len(cmapBytes)))
					cmapBody.Write(cmapBytes)
					cmapBody.WriteString("\nendstream")

					newObjs = append(newObjs, &PDFObject{
						ID:   toUnicodeID,
						Gen:  0,
						Body: cmapBody.Bytes(),
					})

					// 插入 /ToUnicode <toUnicodeID> 0 R 到 Font 字典闭合 >> 之前
					lastDictEnd := bytes.LastIndex(obj.Body, []byte(">>"))
					if lastDictEnd != -1 {
						var newBody []byte
						newBody = append(newBody, obj.Body[:lastDictEnd]...)
						newBody = append(newBody, []byte(fmt.Sprintf(" /ToUnicode %d 0 R ", toUnicodeID))...)
						newBody = append(newBody, obj.Body[lastDictEnd:]...)
						obj.Body = newBody
						result.InjectedFonts++
					}
				}

				// 修正 DLF-32769 字宽 (消除标点后多余空格)
				if strings.Contains(cleanName, "DLF-32769") {
					mW := reWidths.FindSubmatchIndex(obj.Body)
					if len(mW) > 3 {
						inner := string(obj.Body[mW[2]:mW[3]])
						nums := strings.Fields(inner)
						var fixedWidths []string
						for range nums {
							fixedWidths = append(fixedWidths, "1000")
						}
						rep := []byte("/Widths[ " + strings.Join(fixedWidths, " ") + "]")
						var newBody []byte
						newBody = append(newBody, obj.Body[:mW[0]]...)
						newBody = append(newBody, rep...)
						newBody = append(newBody, obj.Body[mW[1]:]...)
						obj.Body = newBody
					}
				}
			}
		}

		// 2. 检查是否是 CMYK XObject 图片
		if strings.Contains(bodyStr, "/DeviceCMYK") {
			streamIdx := bytes.Index(obj.Body, []byte("stream"))
			dictPart := obj.Body
			streamPart := []byte(nil)
			if streamIdx != -1 {
				dictPart = obj.Body[:streamIdx]
				streamPart = obj.Body[streamIdx:]
			}

			// 如果字典中已有 /Decode 则替换，否则插入
			if reDecode.Match(dictPart) {
				dictPart = reDecode.ReplaceAll(dictPart, []byte("/Decode [ 1 0 1 0 1 0 1 0 ]"))
				obj.Body = append(dictPart, streamPart...)
				result.FixedCMYK++
			} else {
				lastDictEnd := bytes.LastIndex(dictPart, []byte(">>"))
				if lastDictEnd != -1 {
					var newDict []byte
					newDict = append(newDict, dictPart[:lastDictEnd]...)
					newDict = append(newDict, []byte(" /Decode [ 1 0 1 0 1 0 1 0 ] ")...)
					newDict = append(newDict, dictPart[lastDictEnd:]...)
					obj.Body = append(newDict, streamPart...)
					result.FixedCMYK++
				}
			}
		}
	}

	allObjs := append(objs, newObjs...)
	sort.Slice(allObjs, func(i, j int) bool {
		return allObjs[i].ID < allObjs[j].ID
	})

	// 3. 重建 PDF 1.4 文件
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make(map[int]int)
	for _, obj := range allObjs {
		offsets[obj.ID] = out.Len()
		out.WriteString(fmt.Sprintf("%d %d obj\n", obj.ID, obj.Gen))
		out.Write(obj.Body)
		out.WriteString("\nendobj\n")
	}

	xrefOffset := out.Len()
	totalSize := maxID + 1
	out.WriteString(fmt.Sprintf("xref\n0 %d\n", totalSize))
	out.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxID; i++ {
		if off, ok := offsets[i]; ok {
			out.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
		} else {
			out.WriteString("0000000000 65535 f \n")
		}
	}

	// 规范化 trailer 的 /Size
	if reSize.MatchString(trailerBody) {
		trailerBody = reSize.ReplaceAllString(trailerBody, fmt.Sprintf("/Size %d", totalSize))
	} else {
		trailerBody = fmt.Sprintf("/Size %d ", totalSize) + trailerBody
	}

	out.WriteString("trailer\n<<")
	out.WriteString(trailerBody)
	out.WriteString(">>\nstartxref\n")
	out.WriteString(fmt.Sprintf("%d\n%%%%EOF\n", xrefOffset))

	return out.Bytes(), result, nil
}

// PostProcessPDF 对 Ghostscript 生成的 base PDF 文件进行后处理并写出到 finalPDFPath
func PostProcessPDF(basePDFPath, finalPDFPath string, mapping map[string]map[byte]string) (*PostProcessResult, error) {
	pdfData, err := os.ReadFile(basePDFPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base PDF: %w", err)
	}

	finalData, result, err := PostProcessPDFBytes(pdfData, mapping)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(finalPDFPath, finalData, 0644); err != nil {
		return nil, fmt.Errorf("failed to save final PDF: %w", err)
	}

	return result, nil
}
