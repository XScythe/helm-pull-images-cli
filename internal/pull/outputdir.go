package pull

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func defaultOutputDir(chart string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current working directory: %w", err)
	}

	dirName := sanitizeName(chart)
	if dirName == "" {
		dirName = "chart"
	}

	// Try the base name first
	outputDir := filepath.Join(cwd, dirName)
	if _, err := os.Stat(outputDir); err == nil {
		// Directory exists, append date and hour
		timestamp := time.Now().Format("2006-01-02-15")
		outputDir = filepath.Join(cwd, dirName+"-"+timestamp)
	}

	// Create the directory
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	return outputDir, nil
}

func sanitizeName(value string) string {
	value = filepath.Base(value)
	value = strings.ReplaceAll(value, string(os.PathSeparator), "-")
	value = strings.NewReplacer(" ", "-", ":", "-", "@", "-", ".", "-").Replace(value)
	if value == "" {
		return "chart"
	}
	return value
}
