package converter

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	customDimensions   bool
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
		c.customDimensions = true
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

// PSPageDimensions 页面尺寸 (pts) 与连版裁剪偏移
type PSPageDimensions struct {
	WidthPts  float64
	HeightPts float64
	OffsetX   float64
	OffsetY   float64
}

// FindCJKFont 寻找系统或 Ghostscript 自带的中文字体文件 (TTF/TTC)
func FindCJKFont(gsPath string) string {
	candidates := []string{
		// 1. Ghostscript 自带的 DroidSansFallback.ttf (最稳定，字库全，无嵌入版权限制)
		filepath.Join(filepath.Dir(filepath.Dir(gsPath)), "share", "ghostscript", "Resource", "CIDFSubst", "DroidSansFallback.ttf"),
		filepath.Join(filepath.Dir(filepath.Dir(gsPath)), "share", "ghostscript", "*", "Resource", "CIDFSubst", "DroidSansFallback.ttf"),
		filepath.Join(filepath.Dir(filepath.Dir(gsPath)), "Resource", "CIDFSubst", "DroidSansFallback.ttf"),
		"/opt/homebrew/share/ghostscript/Resource/CIDFSubst/DroidSansFallback.ttf",
		"/opt/homebrew/share/ghostscript/*/Resource/CIDFSubst/DroidSansFallback.ttf",
		"/usr/local/share/ghostscript/Resource/CIDFSubst/DroidSansFallback.ttf",
		"/usr/share/ghostscript/Resource/CIDFSubst/DroidSansFallback.ttf",
		// 2. Windows 常见中文字体
		"C:\\Windows\\Fonts\\msyh.ttc",
		"C:\\Windows\\Fonts\\simsun.ttc",
		"C:\\Windows\\Fonts\\simhei.ttf",
	}

	// 检查当前用户 Home 目录下的 Fonts
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Library", "Fonts", "msyh.ttf"))
		candidates = append(candidates, filepath.Join(home, "Library", "Fonts", "Microsoft", "msyh.ttf"))
	}

	// 3. 其它系统字体作为保底
	candidates = append(candidates,
		"/Library/Fonts/Microsoft/msyh.ttf",
		"/System/Library/Fonts/Supplemental/Songti.ttc",
		"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	)

	for _, pattern := range candidates {
		if strings.Contains(pattern, "*") {
			matches, _ := filepath.Glob(pattern)
			for _, m := range matches {
				if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
					return m
				}
			}
		} else {
			if fi, err := os.Stat(pattern); err == nil && !fi.IsDir() {
				return pattern
			}
		}
	}
	return ""
}

// GenerateCidfmap 为 Ghostscript 生成 CIDFont 映射表
func GenerateCidfmap(tmpDir string, psData []byte, cjkFont string) error {
	if cjkFont == "" {
		return nil
	}
	reFounderFont := regexp.MustCompile(`/([A-Za-z0-9_\-]+)--(GBK1-0|GB-EUC-H)`)
	matches := reFounderFont.FindAllSubmatch(psData, -1)
	prefixes := make(map[string]bool)
	for _, m := range matches {
		prefixes[string(m[1])] = true
	}
	prefixes["FZBSK"] = true
	prefixes["FZHTK"] = true
	prefixes["FZSSK"] = true
	prefixes["FZKTK"] = true
	prefixes["FZFSK"] = true

	var buf bytes.Buffer
	buf.WriteString("%! cidfmap for Founder CJK fonts\n")
	cleanPath := filepath.ToSlash(cjkFont)
	for prefix := range prefixes {
		fmt.Fprintf(&buf, "/%s << /FileType /TrueType /Path (%s) /SubfontID 0 /CSI [(GB1) 5] >> ;\n", prefix, cleanPath)
	}
	return os.WriteFile(filepath.Join(tmpDir, "cidfmap"), buf.Bytes(), 0644)
}

// DetectPSPageDimensions 探测 PS 文件的页面尺寸 (pts) 与连版裁剪偏移
func DetectPSPageDimensions(psData []byte) PSPageDimensions {
	defaultDims := PSPageDimensions{
		WidthPts:  DefaultPageWidthPoints,
		HeightPts: DefaultPageHeightPoints,
		OffsetX:   0,
		OffsetY:   0,
	}

	// 1. 探测缩放比例 (飞腾常为 72/742)
	scale := 72.0 / 742.0
	reScale := regexp.MustCompile(`(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s+div\s+dup\s+neg\s+scale`)
	if m := reScale.FindSubmatch(psData); len(m) > 2 {
		num, _ := strconv.ParseFloat(string(m[1]), 64)
		denom, _ := strconv.ParseFloat(string(m[2]), 64)
		if denom > 0 {
			scale = num / denom
		}
	}

	// 2. 探测 Rect clip (裁剪区域，精准识别连版中的单版位置)
	reClips := regexp.MustCompile(`(?m)^\s*([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s+Rect[\s\r\n]+(?:gs[\s\r\n]+)?clip`)
	clips := reClips.FindAllSubmatch(psData, -1)
	if len(clips) >= 2 {
		x1, _ := strconv.ParseFloat(string(clips[1][1]), 64)
		y1, _ := strconv.ParseFloat(string(clips[1][2]), 64)
		x2, _ := strconv.ParseFloat(string(clips[1][3]), 64)
		y2, _ := strconv.ParseFloat(string(clips[1][4]), 64)
		w := math.Abs(x2-x1) * scale
		h := math.Abs(y2-y1) * scale
		ox := math.Min(x1, x2) * scale
		oy := math.Min(y1, y2) * scale
		if w > 100 && h > 100 {
			return PSPageDimensions{
				WidthPts:  w,
				HeightPts: h,
				OffsetX:   ox,
				OffsetY:   oy,
			}
		}
	} else if len(clips) == 1 {
		x1, _ := strconv.ParseFloat(string(clips[0][1]), 64)
		y1, _ := strconv.ParseFloat(string(clips[0][2]), 64)
		x2, _ := strconv.ParseFloat(string(clips[0][3]), 64)
		y2, _ := strconv.ParseFloat(string(clips[0][4]), 64)
		w := math.Abs(x2-x1) * scale
		h := math.Abs(y2-y1) * scale
		ox := math.Min(x1, x2) * scale
		oy := math.Min(y1, y2) * scale
		if w > 100 && h > 100 {
			return PSPageDimensions{
				WidthPts:  w,
				HeightPts: h,
				OffsetX:   ox,
				OffsetY:   oy,
			}
		}
	}

	// 3. 回退到 BoundingBox
	reBBox := regexp.MustCompile(`(?m)^%%(?:HiRes|Page)?BoundingBox:\s*(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)`)
	matches := reBBox.FindAllSubmatch(psData, -1)
	for _, m := range matches {
		x1, _ := strconv.ParseFloat(string(m[1]), 64)
		y1, _ := strconv.ParseFloat(string(m[2]), 64)
		x2, _ := strconv.ParseFloat(string(m[3]), 64)
		y2, _ := strconv.ParseFloat(string(m[4]), 64)
		w := math.Abs(x2 - x1)
		h := math.Abs(y2 - y1)
		if w > 100 && h > 100 {
			return PSPageDimensions{
				WidthPts:  w,
				HeightPts: h,
				OffsetX:   0,
				OffsetY:   0,
			}
		}
	}
	return defaultDims
}

// PatchPS 读取 PS 字节流，修补图片引用路径、禁用 /psdefine 并注入方正发排环境兼容垫片
func (c *Converter) PatchPS(psData []byte, searchDir string) ([]byte, PatchReport, error) {
	patchedData, report := ResolveAndPatchImagePaths(psData, searchDir)
	patchedData = InjectFounderPolyfill(patchedData)
	return patchedData, report, nil
}

// RenderBasePDFWithOptions 调用 Ghostscript 将修补后的 PS 渲染为基础 PDF（支持自定义临时目录与页面几何参数）
func (c *Converter) RenderBasePDFWithOptions(ctx context.Context, fixedPSPath, basePDFPath, tmpDir string, dims PSPageDimensions) error {
	width := dims.WidthPts
	if width <= 0 {
		width = c.pageWidthPoints
	}
	height := dims.HeightPts
	if height <= 0 {
		height = c.pageHeightPoints
	}

	gsArgs := []string{
		"-dBATCH", "-dNOPAUSE", "-dNOSAFER",
		"-sDEVICE=pdfwrite",
		fmt.Sprintf("-sOutputFile=%s", basePDFPath),
		fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.2f", width),
		fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.2f", height),
		"-dFIXEDMEDIA",
		fmt.Sprintf("-dCompatibilityLevel=%s", c.compatibilityLevel),
	}

	if tmpDir != "" {
		gsArgs = append([]string{fmt.Sprintf("-I%s", tmpDir)}, gsArgs...)
	}

	if dims.OffsetX != 0 || dims.OffsetY != 0 {
		gsArgs = append(gsArgs, "-c", fmt.Sprintf("<< /PageOffset [-%.2f -%.2f] >> setpagedevice", dims.OffsetX, dims.OffsetY), "-f", fixedPSPath)
	} else {
		gsArgs = append(gsArgs, fixedPSPath)
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

// RenderBasePDF 调用 Ghostscript 将修补后的 PS 渲染为基础 PDF
func (c *Converter) RenderBasePDF(ctx context.Context, fixedPSPath, basePDFPath string) error {
	return c.RenderBasePDFWithOptions(ctx, fixedPSPath, basePDFPath, "", PSPageDimensions{
		WidthPts:  c.pageWidthPoints,
		HeightPts: c.pageHeightPoints,
	})
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

	// [1/4] 扫描图片引用并修补 PS，注入方正发排环境垫片与 CJK 字体映射
	c.reportProgress(1, 4, "自动扫描图片并直接重定向到本地真实路径，注入发排环境垫片...")
	psData, err := os.ReadFile(absPS)
	if err != nil {
		return nil, fmt.Errorf("failed to read PS file: %w", err)
	}

	dims := DetectPSPageDimensions(psData)
	if c.customDimensions {
		dims.WidthPts = c.pageWidthPoints
		dims.HeightPts = c.pageHeightPoints
	}

	cjkFont := FindCJKFont(c.gsPath)
	_ = GenerateCidfmap(tmpDir, psData, cjkFont)

	patchedData, patchReport, err := c.PatchPS(psData, searchDir)
	if err != nil {
		return nil, fmt.Errorf("failed to patch PS image paths: %w", err)
	}
	if err := os.WriteFile(fixedPS, patchedData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write patched PS file: %w", err)
	}

	// [2/4] Ghostscript 渲染基础 PDF
	c.reportProgress(2, 4, "Ghostscript 渲染基础 PDF...")
	if err := c.RenderBasePDFWithOptions(ctx, fixedPS, basePDF, tmpDir, dims); err != nil {
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
