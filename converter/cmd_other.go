//go:build !windows

package converter

import (
	"os"
	"os/exec"
)

// prepareCmdPlatform 非 Windows 系统无需额外处理子进程控制台窗口
func prepareCmdPlatform(cmd *exec.Cmd) {
}

// findPlatformGS 非 Windows 环境下查找 Ghostscript
func findPlatformGS() string {
	candidates := []string{
		"/opt/homebrew/bin/gs",
		"/usr/local/bin/gs",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("gs"); err == nil {
		return p
	}
	return "gs"
}
