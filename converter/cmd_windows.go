//go:build windows

package converter

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// prepareCmdPlatform 在 Windows 下设置 HideWindow 和 CREATE_NO_WINDOW，防止执行 Ghostscript 时弹出黑窗口
func prepareCmdPlatform(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	// CREATE_NO_WINDOW = 0x08000000，确保子进程完全不创建控制台窗口
	cmd.SysProcAttr.CreationFlags |= 0x08000000
}

// findPlatformGS 在 Windows 环境下优先寻找 gswin64c.exe / gswin32c.exe / gs.exe
func findPlatformGS() string {
	// 优先在 PATH 中查找控制台版本的 Ghostscript (带有 c 后缀的控制台版，避免使用有独立界面的 gswin64.exe)
	names := []string{"gswin64c", "gswin32c", "gs"}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}

	// 尝试常见的 Windows 安装目录，优先使用控制台版 gswin64c.exe / gswin32c.exe
	programFiles := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		`C:\Program Files`,
		`C:\Program Files (x86)`,
	}
	for _, pf := range programFiles {
		if pf == "" {
			continue
		}
		// 搜索例如 C:\Program Files\gs\gs*\bin\gswin*c.exe
		matches, _ := filepath.Glob(filepath.Join(pf, "gs", "gs*", "bin", "gswin*c.exe"))
		if len(matches) > 0 {
			// 优先使用最新匹配到的版本
			return matches[len(matches)-1]
		}
	}

	return "gswin64c"
}
