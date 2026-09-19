package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ps2pdf/converter"
)

const docStr = `ps2pdf  --  将方正 PS 文件转换为可搜索/可复制的 PDF (Go 版本)

用法:
    ps2pdf <input.ps> [output.pdf]

示例:
    ps2pdf 3390-1.ps
    ps2pdf 3390-1.ps 3390-1.pdf

特性:
    - 通用自动探测 PS 中引用的全部图片，智能匹配并直接使用本地真实绝对路径（无软链接污染）
    - 自动修复 PS 中的 Windows 绝对路径，跨平台无缝适配
    - 禁用 /psdefine 反盗版检查
    - 注入 ToUnicode CMap，使 PDF 文字可正常搜索、复制
    - 修正 DLF-32769 标点字体宽度（消除标点后多余空格）
    - 修正 CMYK JPEG 颜色反相（报头徽标等）
    - 零临时文件残留：转换过程中的中间文件全在系统临时目录中自动流转并自动清理
    - 需要系统安装 Ghostscript (gs)
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Print(docStr)
		os.Exit(1)
	}

	psPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效的输入路径: %v\n", err)
		os.Exit(1)
	}

	workspace := filepath.Dir(psPath)
	baseName := filepath.Base(psPath)
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)

	var finalPDF string
	if len(os.Args) >= 3 {
		finalPDF, _ = filepath.Abs(os.Args[2])
	} else {
		finalPDF = filepath.Join(workspace, stem+".pdf")
	}

	fmt.Printf("\n=== 构建: %s -> %s ===\n\n", filepath.Base(psPath), filepath.Base(finalPDF))

	// 创建 Converter 实例，配置进度提示
	c := converter.New(
		converter.WithProgress(func(step, total int, msg string) {
			fmt.Printf("[%d/%d] %s\n", step, total, msg)
		}),
	)

	result, err := c.ConvertFile(psPath, finalPDF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[错误] 转换失败: %v\n", err)
		os.Exit(1)
	}

	sizeKB := result.OutputSizeBytes / 1024
	fmt.Printf("\n✓ 完成！%s  (%d KB, 耗时 %s)\n\n", result.OutputPDFPath, sizeKB, result.Duration.Round(100*1000*1000))
}
