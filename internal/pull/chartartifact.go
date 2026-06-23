package pull

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

func stageChartArchive(loaded loadedChart, outputDir string) (string, error) {
	if loaded.LocalArchivePath != "" {
		return copyChartArchive(loaded.LocalArchivePath, outputDir)
	}
	if len(loaded.ArchiveData) > 0 {
		name := loaded.ArchiveName
		if name == "" {
			name = defaultChartArchiveName(loaded.Chart)
		}
		return writeChartArchive(loaded.ArchiveData, name, outputDir)
	}
	if loaded.Chart == nil {
		return "", fmt.Errorf("package chart: loaded chart is nil")
	}
	archivePath, err := chartutil.Save(loaded.Chart, outputDir)
	if err != nil {
		return "", fmt.Errorf("package chart: %w", err)
	}
	return archivePath, nil
}

func copyChartArchive(srcPath, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	dstPath := filepath.Join(outputDir, filepath.Base(srcPath))

	if absSrc, err := filepath.Abs(srcPath); err == nil {
		if absDst, dstErr := filepath.Abs(dstPath); dstErr == nil && absSrc == absDst {
			return dstPath, nil
		}
	}
	if srcInfo, err := os.Stat(srcPath); err == nil {
		if dstInfo, dstErr := os.Stat(dstPath); dstErr == nil && os.SameFile(srcInfo, dstInfo) {
			return dstPath, nil
		}
	}

	if filepath.Clean(srcPath) == filepath.Clean(dstPath) {
		return dstPath, nil
	}

	in, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open chart archive: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("create staged chart archive: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", fmt.Errorf("copy chart archive: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close staged chart archive: %w", err)
	}

	return dstPath, nil
}

func writeChartArchive(data []byte, archiveName, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	if archiveName == "" {
		archiveName = "chart.tgz"
	}
	archivePath := filepath.Join(outputDir, archiveName)
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write chart archive: %w", err)
	}
	return archivePath, nil
}

func defaultChartArchiveName(chart *helmchart.Chart) string {
	name := "chart"
	version := "0.0.0"
	if chart != nil && chart.Metadata != nil {
		if chart.Metadata.Name != "" {
			name = chart.Metadata.Name
		}
		if chart.Metadata.Version != "" {
			version = chart.Metadata.Version
		}
	}
	return fmt.Sprintf("%s-%s.tgz", name, version)
}

func chartArchiveNameFromURL(chartURL string) string {
	u, err := url.Parse(chartURL)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz") {
		return name
	}
	return ""
}

func isChartArchivePath(chartPath string) bool {
	lower := strings.ToLower(chartPath)
	return strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz")
}
