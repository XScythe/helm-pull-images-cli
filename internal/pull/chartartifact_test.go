package pull

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestStageChartArchiveCopiesLocalArchivePreservingName(t *testing.T) {
	chrt := mustLoadChart(t, "demo", "1.2.3")
	srcDir := t.TempDir()
	srcArchive, err := chartutil.Save(chrt, srcDir)
	if err != nil {
		t.Fatalf("chartutil.Save() error = %v", err)
	}
	original := mustReadFile(t, srcArchive)

	outDir := t.TempDir()
	gotPath, err := stageChartArchive(loadedChart{Chart: chrt, LocalArchivePath: srcArchive}, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if filepath.Base(gotPath) != filepath.Base(srcArchive) {
		t.Fatalf("staged archive name = %q, want %q", filepath.Base(gotPath), filepath.Base(srcArchive))
	}
	if got := mustReadFile(t, gotPath); !bytes.Equal(got, original) {
		t.Fatal("staged archive content differs from source archive")
	}
}

func TestStageChartArchiveReturnsSamePathWhenLocalArchiveAlreadyInOutputDir(t *testing.T) {
	chrt := mustLoadChart(t, "demo", "1.2.3")
	outDir := t.TempDir()
	srcArchive, err := chartutil.Save(chrt, outDir)
	if err != nil {
		t.Fatalf("chartutil.Save() error = %v", err)
	}
	original := mustReadFile(t, srcArchive)

	gotPath, err := stageChartArchive(loadedChart{Chart: chrt, LocalArchivePath: srcArchive}, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if gotPath != srcArchive {
		t.Fatalf("stageChartArchive() path = %q, want %q", gotPath, srcArchive)
	}
	if got := mustReadFile(t, gotPath); !bytes.Equal(got, original) {
		t.Fatal("staged archive content changed unexpectedly")
	}
}

func TestStageChartArchiveHandlesRelativePathToArchiveInOutputDir(t *testing.T) {
	chrt := mustLoadChart(t, "demo", "1.2.3")
	outDir := t.TempDir()
	srcArchive, err := chartutil.Save(chrt, outDir)
	if err != nil {
		t.Fatalf("chartutil.Save() error = %v", err)
	}
	original := mustReadFile(t, srcArchive)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() { _ = os.Chdir(prevWD) }()
	if err := os.Chdir(outDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	relPath := filepath.Base(srcArchive)
	gotPath, err := stageChartArchive(loadedChart{Chart: chrt, LocalArchivePath: relPath}, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if gotPath != srcArchive {
		t.Fatalf("stageChartArchive() path = %q, want %q", gotPath, srcArchive)
	}
	if got := mustReadFile(t, gotPath); !bytes.Equal(got, original) {
		t.Fatal("staged archive content changed unexpectedly")
	}
}

func TestStageChartArchivePackagesUnpackedChart(t *testing.T) {
	chartRoot := t.TempDir()
	writeChartMetadata(t, chartRoot, "unpacked", "0.4.0")
	loaded, err := localChartSource(context.Background(), Options{Chart: chartRoot})
	if err != nil {
		t.Fatalf("localChartSource() error = %v", err)
	}

	outDir := t.TempDir()
	gotPath, err := stageChartArchive(loaded, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if want := "unpacked-0.4.0.tgz"; filepath.Base(gotPath) != want {
		t.Fatalf("staged archive name = %q, want %q", filepath.Base(gotPath), want)
	}

	staged, err := loader.Load(gotPath)
	if err != nil {
		t.Fatalf("loader.Load() staged archive error = %v", err)
	}
	if got, want := staged.Metadata.Name, "unpacked"; got != want {
		t.Fatalf("staged chart name = %q, want %q", got, want)
	}
	if got, want := staged.Metadata.Version, "0.4.0"; got != want {
		t.Fatalf("staged chart version = %q, want %q", got, want)
	}
}

func TestStageChartArchiveWritesFetchedArchiveData(t *testing.T) {
	outDir := t.TempDir()
	data := []byte("chart-archive-data")
	gotPath, err := stageChartArchive(loadedChart{ArchiveData: data, ArchiveName: "remote-demo-3.0.1.tgz"}, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if got := filepath.Base(gotPath); got != "remote-demo-3.0.1.tgz" {
		t.Fatalf("staged archive name = %q, want %q", got, "remote-demo-3.0.1.tgz")
	}
	if got := mustReadFile(t, gotPath); !bytes.Equal(got, data) {
		t.Fatalf("staged archive bytes = %q, want %q", string(got), string(data))
	}
}

func TestStageChartArchiveUsesDefaultNameWhenFetchedNameMissing(t *testing.T) {
	chrt := mustLoadChart(t, "oci-demo", "9.1.0")
	archivePath, err := chartutil.Save(chrt, t.TempDir())
	if err != nil {
		t.Fatalf("chartutil.Save() error = %v", err)
	}

	outDir := t.TempDir()
	gotPath, err := stageChartArchive(loadedChart{Chart: chrt, ArchiveData: mustReadFile(t, archivePath)}, outDir)
	if err != nil {
		t.Fatalf("stageChartArchive() error = %v", err)
	}
	if got, want := filepath.Base(gotPath), "oci-demo-9.1.0.tgz"; got != want {
		t.Fatalf("staged archive name = %q, want %q", got, want)
	}
}

func TestStageChartArchiveErrorsWhenChartDataUnavailable(t *testing.T) {
	_, err := stageChartArchive(loadedChart{}, t.TempDir())
	if err == nil {
		t.Fatal("stageChartArchive() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "loaded chart is nil") {
		t.Fatalf("stageChartArchive() error = %v, want loaded chart nil error", err)
	}
}

func TestChartArchiveNameFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "tgz file", url: "https://charts.example.com/demo-1.2.3.tgz", want: "demo-1.2.3.tgz"},
		{name: "tar gz file", url: "https://charts.example.com/demo-1.2.3.tar.gz", want: "demo-1.2.3.tar.gz"},
		{name: "query retained", url: "https://charts.example.com/demo-1.2.3.tgz?token=abc", want: "demo-1.2.3.tgz"},
		{name: "non archive", url: "https://charts.example.com/demo", want: ""},
		{name: "invalid", url: "://bad-url", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chartArchiveNameFromURL(tt.url); got != tt.want {
				t.Fatalf("chartArchiveNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestIsChartArchivePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/charts/demo-1.2.3.tgz", want: true},
		{path: "/charts/demo-1.2.3.TAR.GZ", want: true},
		{path: "/charts/demo", want: false},
	}
	for _, tt := range tests {
		if got := isChartArchivePath(tt.path); got != tt.want {
			t.Fatalf("isChartArchivePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func mustLoadChart(t *testing.T, name, version string) *chart.Chart {
	t.Helper()
	chartRoot := t.TempDir()
	writeChartMetadata(t, chartRoot, name, version)
	chrt, err := loader.Load(chartRoot)
	if err != nil {
		t.Fatalf("loader.Load() error = %v", err)
	}
	return chrt
}

func writeChartMetadata(t *testing.T, chartRoot, name, version string) {
	t.Helper()
	chartYAML := fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", name, version)
	if err := os.WriteFile(filepath.Join(chartRoot, "Chart.yaml"), []byte(chartYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}
