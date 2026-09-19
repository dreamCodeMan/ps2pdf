# ps2pdf

一个高性能、纯净的 Go 语言工具库，用于将**方正 PS (PostScript)** 排版文件转换为可检索、可复制文本的现代化标准 PDF。

不仅提供开箱即用的命令行工具（CLI），更封装成了高度解耦、面向对象的 Go 方法库（`converter` 包），方便被其他服务与系统直接调用。

---

## 特性亮点

- **智能图片重定向**：通用自动探测 PS 中引用的全部插图，支持全半角标点、空格、连续点等智能模糊匹配，直接使用本地真实路径替换，无需生成任何软链接。
- **跨平台路径兼容**：自动修复 PS 中残留的 Windows 绝对路径，在 macOS / Linux / Windows 环境下均可无缝转换。
- **反盗版检查绕过**：自动修补 PS 中的 `/psdefine` 校验逻辑，确保渲染流程顺畅。
- **文本可检索与复制**：自动提取 PS 中 DownLoadCode 字符映射，动态生成符合 Adobe 规范的 ToUnicode CMap 并注入 PDF 字体流，彻底解决生成 PDF 文字乱码、无法复制、无法搜索的问题。
- **排版字宽智能修正**：自动修正 `DLF-32769` 标点字体的字宽为 1000，消除排版中中文标点后的多余空格。
- **CMYK 图片反相修复**：自动识别并修正 `/DeviceCMYK` 格式图片的 `/Decode` 颜色数组，修复报头徽标等反相偏色问题。
- **零临时文件残留**：转换过程中的中间文件全在系统临时目录中流转并自动清理，保持工作区绝对整洁。
- **纯净库设计**：
  - 核心库 `converter` 完全静默，不污染调用方的标准输出。
  - 所有错误均遵循 Go 规范使用标准英文返回。
  - 完整返回结构化统计数据（耗时、文件大小、修补图片数、字符映射数等）。

---

## 运行环境与依赖

本程序依赖 **Ghostscript (`gs`)** 负责 PostScript 底层光栅化与基础 PDF 生成。

### 安装 Ghostscript

- **macOS** (Homebrew):
  ```bash
  brew install ghostscript
  ```
- **Ubuntu / Debian**:
  ```bash
  sudo apt-get update && sudo apt-get install -y ghostscript
  ```
- **CentOS / RHEL**:
  ```bash
  sudo yum install -y ghostscript
  ```
- **Windows**:
  从 [Ghostscript 官方网站](https://ghostscript.com/releases/) 下载安装，并确保 `gs` 或 `gswin64c` 在系统 `PATH` 环境变量中，或在代码中通过配置项显式指定路径。

---

## 快速开始 (CLI 命令行)

### 编译

```bash
# 进入项目目录
cd ps2pdf

# 编译为可执行文件
go build -o ps2pdf .
```

### 命令行使用

```bash
# 格式:
./ps2pdf <input.ps> [output.pdf]

# 示例 1: 自动在同目录下生成 3390-1.pdf
./ps2pdf 3390-1.ps

# 示例 2: 指定输出 PDF 路径
./ps2pdf 3390-1.ps /path/to/output.pdf
```

---

## 作为 Go 库调用 (Library Usage)

你可以在任意 Go 项目中导入 `ps2pdf/converter` 使用。

### 1. 面向对象调用（推荐）

通过 `converter.New` 创建实例，支持函数式选项（Option 模式）自由配置：

```go
package main

import (
	"context"
	"fmt"
	"log"

	"ps2pdf/converter"
)

func main() {
	// 创建转换器实例
	c := converter.New(
		// 可选：自定义 Ghostscript 可执行文件路径（默认自动探测）
		converter.WithGhostscriptPath("/usr/local/bin/gs"),

		// 可选：自定义输出页面宽高 (points，默认 1114.02 x 1545.93)
		converter.WithPageDimensions(1114.02, 1545.93),

		// 可选：设置 PDF 兼容级别 (默认 "1.4")
		converter.WithCompatibilityLevel("1.4"),

		// 可选：自定义图片资源检索目录（默认使用输入 PS 所在的同级目录）
		converter.WithImageSearchDir("/path/to/images"),

		// 可选：监听转换进度回调 (当前步骤, 总步骤, 描述)
		converter.WithProgress(func(step, total int, msg string) {
			fmt.Printf("[%d/%d] %s\n", step, total, msg)
		}),
	)

	// 执行转换 (支持 context 超时和取消控制)
	ctx := context.Background()
	result, err := c.Convert(ctx, "sample.ps", "output.pdf")
	if err != nil {
		log.Fatalf("conversion failed: %v", err)
	}

	// 读取结构化统计信息
	fmt.Printf("转换成功!\n")
	fmt.Printf("输出文件: %s\n", result.OutputPDFPath)
	fmt.Printf("文件大小: %d KB\n", result.OutputSizeBytes/1024)
	fmt.Printf("修补图片: %d 张\n", result.PatchedImages)
	fmt.Printf("字体数量: %d 种\n", result.FontCount)
	fmt.Printf("字符映射: %d 个\n", result.CharCount)
	fmt.Printf("总计耗时: %s\n", result.Duration)
}
```

### 2. 包级快捷调用（开箱即用）

如果无需复杂定制，可直接使用包级函数单行完成转换：

```go
package main

import (
	"log"

	"ps2pdf/converter"
)

func main() {
	result, err := converter.ConvertFile("input.ps", "output.pdf")
	if err != nil {
		log.Fatalf("conversion failed: %v", err)
	}
	log.Println("PDF 生成成功:", result.OutputPDFPath)
}
```

### 3. 分步底层 API（高级用法）

库将各阶段核心能力进行了导出，支持按需组合或基于内存流转：

- **图片修补与反盗版处理**：
  ```go
  patchedPSData, report := converter.ResolveAndPatchImagePaths(rawPSBytes, searchDir)
  ```
- **PS 字符与 DownLoadCode 映射解析**：
  ```go
  mapping, err := converter.BuildCharMapping("sample.ps")
  // 或从 io.Reader 解析：
  mapping, err := converter.BuildCharMappingFromReader(reader)
  ```
- **ToUnicode CMap 生成**：
  ```go
  cmapBytes := converter.MakeToUnicodeCMap(charMap)
  ```
- **PDF 结构后处理（注入 CMap / 字宽 / CMYK 修复）**：
  ```go
  // 文件到文件后处理
  result, err := converter.PostProcessPDF(basePDFPath, finalPDFPath, mapping)
  // 或内存流到内存流
  finalPDFBytes, result, err := converter.PostProcessPDFBytes(rawPDFBytes, mapping)
  ```

---

## 目录结构

```text
ps2pdf/
├── converter/              # 核心类库 (package converter)
│   ├── converter.go        # Converter 结构体、Option 模式、调度主流程
│   ├── cmd_windows.go      # Windows 专属平台实现（隐藏 Ghostscript 黑窗口、探测 gswin64c）
│   ├── cmd_other.go        # Unix/macOS/Linux 平台实现
│   ├── psparser.go         # PS 解析、图片路径智能模糊匹配、DownLoadCode 提取
│   ├── pdfmod.go           # 流安全 PDF 解析、ToUnicode 注入、字宽与 CMYK 修复
│   ├── cmap.go             # 符合 Adobe 规范的 ToUnicode CMap 生成器
│   └── converter_test.go   # 完整单元测试集
├── main.go                 # 命令行工具 (CLI 入口)
├── go.mod                  # Go 模块定义
├── go.sum                  # 依赖校验
└── README.md               # 项目使用说明文档
```

### Windows 编译与运行说明

- **标准命令行版（默认）**：
  ```bash
  go build -o ps2pdf.exe .
  ```
  在终端中正常输出日志。执行 Ghostscript 渲染时已内置设置 `CREATE_NO_WINDOW` 与 `HideWindow`，**不会额外弹窗或闪烁 CMD 黑窗口**。
- **完全静默无窗口版（适合 GUI/自动化后台调用）**：
  ```bash
  go build -ldflags="-H windowsgui" -o ps2pdf.exe .
  ```
  双击运行时完全不会出现任何 CMD 控制台黑窗口。

---

## 测试

运行内置单元测试验证各项核心解析逻辑与选项配置：

```bash
go test -v ./...
```
