package converter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	c := New(
		WithGhostscriptPath("/custom/gs"),
		WithPageDimensions(800, 600),
		WithCompatibilityLevel("1.7"),
		WithImageSearchDir("/custom/images"),
		WithKeepTempFiles(true),
	)

	if c.gsPath != "/custom/gs" {
		t.Errorf("expected gsPath /custom/gs, got %s", c.gsPath)
	}
	if c.pageWidthPoints != 800 || c.pageHeightPoints != 600 {
		t.Errorf("expected dimensions 800x600, got %fx%f", c.pageWidthPoints, c.pageHeightPoints)
	}
	if c.compatibilityLevel != "1.7" {
		t.Errorf("expected compatibilityLevel 1.7, got %s", c.compatibilityLevel)
	}
	if c.imageSearchDir != "/custom/images" {
		t.Errorf("expected imageSearchDir /custom/images, got %s", c.imageSearchDir)
	}
	if !c.keepTempFiles {
		t.Errorf("expected keepTempFiles true")
	}
}

func TestNormalizeFileName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Test File (1).jpg", "testfile(1).jpg"},
		{"测试【文件】..png", "测试[文件].png"},
		{"a — b.tif", "a-b.tif"},
		{"  Hello   World  ", "helloworld"},
	}

	for _, c := range cases {
		got := NormalizeFileName(c.input)
		if got != c.expected {
			t.Errorf("NormalizeFileName(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestParsePSString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{`hello\nworld`, "hello\nworld"},
		{`test\r\t`, "test\r\t"},
		{`\(brackets\)`, "(brackets)"},
		{`\\slash`, `\slash`},
		{`\101\102`, "AB"},
	}

	for _, c := range cases {
		got := string(ParsePSString([]byte(c.input)))
		if got != c.expected {
			t.Errorf("ParsePSString(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestMakeToUnicodeCMap(t *testing.T) {
	charMap := map[byte]string{
		0x20: "中",
		0x21: "国",
	}
	cmap := MakeToUnicodeCMap(charMap)
	cmapStr := string(cmap)

	if !strings.Contains(cmapStr, "begincmap") {
		t.Errorf("missing begincmap in CMap")
	}
	if !strings.Contains(cmapStr, "<20> <4E2D>") {
		t.Errorf("missing expected mapping for 0x20 ('中' -> 4E2D)")
	}
	if !strings.Contains(cmapStr, "<21> <56FD>") {
		t.Errorf("missing expected mapping for 0x21 ('国' -> 56FD)")
	}
}

func TestResolveAndPatchImagePaths(t *testing.T) {
	tmpDir := t.TempDir()
	imgName := "test_photo.jpg"
	imgPath := filepath.Join(tmpDir, imgName)
	if err := os.WriteFile(imgPath, []byte("fake image"), 0644); err != nil {
		t.Fatal(err)
	}

	psInput := []byte("/psdefine { some check } bd\n (C:\\\\images\\\\test_photo.jpg) run\n")
	patched, report := ResolveAndPatchImagePaths(psInput, tmpDir)

	if !report.PsdefinePatched {
		t.Errorf("expected psdefine to be patched")
	}
	if !bytes.Contains(patched, []byte("/psdefine { } bd")) {
		t.Errorf("psdefine patch content not found")
	}
	if len(report.Replacements) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(report.Replacements))
	}
	if report.Replacements[0].RealFilePath != imgPath {
		t.Errorf("expected replacement %s, got %s", imgPath, report.Replacements[0].RealFilePath)
	}
	if !bytes.Contains(patched, []byte(imgPath)) {
		t.Errorf("patched content does not contain real image path")
	}
}

func TestPrepareCmdPlatform(t *testing.T) {
	cmd := exec.Command("echo", "test")
	prepareCmdPlatform(cmd)
	// Verification that prepareCmdPlatform executes without panic
}

func TestFindGS(t *testing.T) {
	gs := FindGS()
	if gs == "" {
		t.Errorf("expected non-empty Ghostscript path/command")
	}
}
