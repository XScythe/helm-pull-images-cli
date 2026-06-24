//go:build windows

package upgrade

import (
	"os"
	"path/filepath"
	"strings"
)

func CleanupStale() {
	executablePath, err := resolveExecutablePath()
	if err != nil {
		return
	}
	realPath, err := evalSymlinks(executablePath)
	if err != nil {
		realPath = executablePath
	}
	_ = os.Remove(filepath.Clean(realPath) + ".old")
	_ = cleanupWindowsHelperCopies()
}

func cleanupWindowsHelperCopies() error {
	helperDir := filepath.Join(os.TempDir(), "helm-deep-pack-upgrade")
	entries, err := os.ReadDir(helperDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "helper-") || !strings.HasSuffix(strings.ToLower(name), ".exe") {
			continue
		}
		_ = os.Remove(filepath.Join(helperDir, name))
	}
	_ = os.Remove(helperDir)
	return nil
}
