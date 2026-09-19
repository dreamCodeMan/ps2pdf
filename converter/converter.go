package converter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ProgressHandler 进度报告回调函数 (step 当前步骤, total 总步骤, message 说明)
type ProgressHandler func(step, total int, message string)

// DefaultPageWidthPoints 默认页面宽度 (points)
const DefaultPageWidthPoints = 1114.02

// DefaultPageHeightPoints 默认页面高度 (points)
const DefaultPageHeightPoints = 1545.93

// DefaultCompatibilityLevel 默认 PDF 兼容版本
const DefaultCompatibilityLevel = "1.4"

// Converter 提供 PS 到 PDF 的转换服务
type Converter struct {
	gsPath             string
	pageWidthPoints    float64
	pageHeightPoints   float64
	compatibilityLevel string
	imageSearchDir     string
	progress           ProgressHandler
	keepTempFiles      bool
}

// Option 配置函数
type Option func(*Converter)

// WithGhostscriptPath 自定义 Ghostscript 可执行文件路径
func WithGhostscriptPath(path string) Option {
	return func(c *Converter) {
		c.gsPath = path
	}
}

// WithPageDimensions 自定义输出 PDF 的页面宽高点数
func WithPageDimensions(widthPoints, heightPoints float64) Option {
	return func(c *Converter) {
		c.pageWidthPoints = widthPoints
		c.pageHeightPoints = heightPoints
	}
}

// WithCompatibilityLevel 自定义 PDF 兼容级别 (如 "1.4")
func WithCompatibilityLevel(level string) Option {
	return func(c *Converter) {
		c.compatibilityLevel = level
	}
}

// WithImageSearchDir 自定义本地图片检索目录（默认使用输入 PS 文件所在目录）
func WithImageSearchDir(dir string) Option {
	return func(c *Converter) {
		c.imageSearchDir = dir
	}
}

// WithProgress 配置进度回调
func WithProgress(handler ProgressHandler) Option {
	return func(c *Converter) {
		c.progress = handler
	}
}

// WithKeepTempFiles 是否保留转换过程中的临时文件（调试用）
func WithKeepTempFiles(keep bool) Option {
	return func(c *Converter) {
		c.keepTempFiles = keep
	}
}

// FindGS 寻找系统中 Ghostscript 的路径
func FindGS() string {
	return findPlatformGS()
}

// New 创建一个新的 Converter 实例
func New(opts ...Option) *Converter {
	c := &Converter{
		gsPath:             FindGS(),
		pageWidthPoints:    DefaultPageWidthPoints,
		pageHeightPoints:   DefaultPageHeightPoints,
		compatibilityLevel: DefaultCompatibilityLevel,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ConvertResult 转换结果详情
type ConvertResult struct {
	InputPSPath     string
	OutputPDFPath   string
	OutputSizeBytes int64
	PatchedImages   int
	FontCount       int
	CharCount       int
	Duration        time.Duration
}

func (c *Converter) reportProgress(step, total int, msg string) {
	if c.progress != nil {
		c.progress(step, total, msg)
	}
}

// PatchPS 读取 PS 字节流，修补图片引用路径并禁用 /psdefine
func (c *Converter) PatchPS(psData []byte, searchDir string) ([]byte, PatchReport, error) {
	patchedData, report := ResolveAndPatchImagePaths(psData, searchDir)
	return patchedData, report, nil
}

// RenderBasePDF 调用 Ghostscript 将修补后的 PS 渲染为基础 PDF
func (c *Converter) RenderBasePDF(ctx context.Context, fixedPSPath, basePDFPath string) error {
	gsArgs := []string{
		"-dBATCH", "-dNOPAUSE", "-dNOSAFER",
		"-sDEVICE=pdfwrite",
		fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.2f", c.pageWidthPoints),
		fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.2f", c.pageHeightPoints),
		"-dFIXEDMEDIA",
		fmt.Sprintf("-dCompatibilityLevel=%s", c.compatibilityLevel),
		fmt.Sprintf("-sOutputFile=%s", basePDFPath),
		fixedPSPath,
	}

	cmd := exec.CommandContext(ctx, c.gsPath, gsArgs...)
	prepareCmdPlatform(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := stdout.String() + stderr.String()
		if len(output) > 1000 {
			output = output[len(output)-1000:]
		}
		return fmt.Errorf("ghostscript execution failed: %w\n%s", err, output)
	}

	return nil
}

// Convert 执行完整的 PS 转 PDF 流程
func (c *Converter) Convert(ctx context.Context, psPath, outputPDFPath string) (*ConvertResult, error) {
	startTime := time.Now()

	absPS, err := filepath.Abs(psPath)
	if err != nil {
		return nil, fmt.Errorf("invalid input PS file path: %w", err)
	}

	absOut, err := filepath.Abs(outputPDFPath)
	if err != nil {
		return nil, fmt.Errorf("invalid output PDF file path: %w", err)
	}

	// 确定检索图片的根目录
	searchDir := c.imageSearchDir
	if searchDir == "" {
		searchDir = filepath.Dir(absPS)
	}

	stem := strings.TrimSuffix(filepath.Base(absPS), filepath.Ext(absPS))

	// 创建临时工作区
	tmpDir, err := os.MkdirTemp("", "ps2pdf_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	if !c.keepTempFiles {
		defer os.RemoveAll(tmpDir)
	}

	fixedPS := filepath.Join(tmpDir, stem+"_fixed.ps")
	basePDF := filepath.Join(tmpDir, stem+"_base.pdf")

	// [1/4] 扫描图片引用并修补 PS
	c.reportProgress(1, 4, "自动扫描图片并直接重定向到本地真实路径...")
	psData, err := os.ReadFile(absPS)
	if err != nil {
		return nil, fmt.Errorf("failed to read PS file: %w", err)
	}

	patchedData, patchReport, err := c.PatchPS(psData, searchDir)
	if err != nil {
		return nil, fmt.Errorf("failed to patch PS image paths: %w", err)
	}
	if err := os.WriteFile(fixedPS, patchedData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write patched PS file: %w", err)
	}

	// [2/4] Ghostscript 渲染基础 PDF
	c.reportProgress(2, 4, "Ghostscript 渲染基础 PDF...")
	if err := c.RenderBasePDF(ctx, fixedPS, basePDF); err != nil {
		return nil, err
	}

	// [3/4] 解析字符映射
	c.reportProgress(3, 4, "解析字符映射...")
	mapping, err := BuildCharMapping(absPS)
	if err != nil {
		return nil, fmt.Errorf("failed to parse character mapping: %w", err)
	}
	totalChars := 0
	for _, v := range mapping {
		totalChars += len(v)
	}

	// [4/4] 注入 ToUnicode / 修正字宽 / 修正 CMYK
	c.reportProgress(4, 4, "注入 ToUnicode / 修正字宽 / 修正 CMYK 并生成最终文件...")
	if _, err := PostProcessPDF(basePDF, absOut, mapping); err != nil {
		return nil, fmt.Errorf("failed to post-process PDF: %w", err)
	}

	fi, err := os.Stat(absOut)
	if err != nil {
		return nil, fmt.Errorf("failed to stat final PDF file: %w", err)
	}

	result := &ConvertResult{
		InputPSPath:     absPS,
		OutputPDFPath:   absOut,
		OutputSizeBytes: fi.Size(),
		PatchedImages:   len(patchReport.Replacements),
		FontCount:       len(mapping),
		CharCount:       totalChars,
		Duration:        time.Since(startTime),
	}

	return result, nil
}

// ConvertFile 便捷方法，使用 context.Background() 执行文件转换
func (c *Converter) ConvertFile(psPath, outputPDFPath string) (*ConvertResult, error) {
	return c.Convert(context.Background(), psPath, outputPDFPath)
}

// Convert 包级快捷转换函数
func Convert(ctx context.Context, psPath, outputPDFPath string, opts ...Option) (*ConvertResult, error) {
	c := New(opts...)
	return c.Convert(ctx, psPath, outputPDFPath)
}

// ConvertFile 包级快捷转换函数 (默认 context)
func ConvertFile(psPath, outputPDFPath string, opts ...Option) (*ConvertResult, error) {
	c := New(opts...)
	return c.ConvertFile(psPath, outputPDFPath)
}
