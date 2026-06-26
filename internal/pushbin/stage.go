package pushbin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"helm-deep-pack/internal/push"
)

const placeholderPayload = "PLACEHOLDER"

var copySelfExecutable = push.CopySelfExecutable
var readEmbeddedBinary = func() ([]byte, error) {
	return embeddedPushBinary, nil
}

func Stage(outputDir string) (string, error) {
	embeddedBinary, err := readEmbeddedBinary()
	if err != nil {
		return copySelfExecutable(outputDir)
	}
	if !hasEmbeddedPayload(embeddedBinary) {
		return copySelfExecutable(outputDir)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	dst := filepath.Join(outputDir, push.PushBinaryName())
	if err := os.WriteFile(dst, embeddedBinary, 0o755); err != nil {
		return "", fmt.Errorf("write embedded push binary: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dst, 0o755); err != nil {
			return "", fmt.Errorf("chmod embedded push binary: %w", err)
		}
	}
	return dst, nil
}

func hasEmbeddedPayload(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return !bytes.Equal(bytes.TrimSpace(data), []byte(placeholderPayload))
}
