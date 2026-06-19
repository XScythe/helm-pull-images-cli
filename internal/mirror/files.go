package mirror

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func PushBinaryName() string {
	if runtime.GOOS == "windows" {
		return "push_images.exe"
	}
	return "push_images"
}

func CopySelfExecutable(outputDir string) (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}

	dst := filepath.Join(outputDir, PushBinaryName())
	if filepath.Clean(src) == filepath.Clean(dst) {
		return dst, nil
	}

	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("open executable: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("create helper binary: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", fmt.Errorf("copy helper binary: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close helper binary: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dst, 0o755); err != nil {
			return "", fmt.Errorf("chmod helper binary: %w", err)
		}
	}
	return dst, nil
}
