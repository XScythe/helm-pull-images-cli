package chartmirror

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"helm-pull-images-cli/internal/mirror"
)

func TestDefaultOutputDirCreatesNewDirectoryInCWD(t *testing.T) {
	h := newRunnerTestHarness(t)

	dir, err := h.runner.defaultOutputDir("openebs")
	if err != nil {
		t.Fatalf("defaultOutputDir() error = %v", err)
	}

	if !strings.HasPrefix(dir, h.cwd+string(filepath.Separator)) {
		t.Fatalf("defaultOutputDir() = %q, want inside %q", dir, h.cwd)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("defaultOutputDir() path missing or not a directory: %v, %v", info, err)
	}
}

func TestResolveChartVersionSkipsPrereleases(t *testing.T) {
	h := newRunnerTestHarness(t)
	h.runner.searchRepoVersions = func(context.Context, string, string) ([]searchResult, error) {
		return []searchResult{
			{Version: "4.0.0-rc.1"},
			{Version: "3.10.0"},
			{Version: "3.9.0"},
		}, nil
	}

	got, err := h.runner.resolveChartVersion(context.Background(), "https://example.invalid", "openebs")
	if err != nil {
		t.Fatalf("resolveChartVersion() error = %v", err)
	}

	if got != "3.10.0" {
		t.Fatalf("resolveChartVersion() = %q, want %q", got, "3.10.0")
	}
}

func TestRunnerRunOrchestratesDependencies(t *testing.T) {
	h := newRunnerTestHarness(t)

	var calls []string
	h.runner.renderManifest = func(_ Runner, _ context.Context, opts Options) (string, error) {
		calls = append(calls, "render:"+opts.Chart)
		return "kind: ConfigMap\nmetadata:\n  name: demo\n  annotations:\n    image: quay.io/example/api:v1\n", nil
	}
	h.runner.extractImages = func(manifest string) ([]string, error) {
		calls = append(calls, "extract")
		if !strings.Contains(manifest, "demo") {
			t.Fatalf("extractImages() saw unexpected manifest: %q", manifest)
		}
		return []string{"quay.io/example/api:v1"}, nil
	}
	h.runner.archiveImages = func(_ context.Context, images []string, outputDir string, concurrency int) ([]mirror.ArchiveSpec, error) {
		calls = append(calls, "archive")
		if got, want := images, []string{"quay.io/example/api:v1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("archiveImages() images = %v, want %v", got, want)
		}
		if outputDir == "" {
			t.Fatal("archiveImages() got empty outputDir")
		}
		if concurrency != 4 {
			t.Fatalf("archiveImages() concurrency = %d, want 4", concurrency)
		}
		return []mirror.ArchiveSpec{{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}}, nil
	}
	h.runner.writePushManifest = func(outputDir string, specs []mirror.ArchiveSpec) error {
		calls = append(calls, "manifest")
		want := []mirror.ArchiveSpec{{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}}
		if got := specs; !reflect.DeepEqual(got, want) {
			t.Fatalf("writePushManifest() specs = %v, want %v", got, want)
		}
		manifestPath := filepath.Join(outputDir, mirror.PushManifestFileName())
		return os.WriteFile(manifestPath, []byte("{\n  \"images\": []\n}\n"), 0o644)
	}
	h.runner.copySelfExecutable = func(outputDir string) (string, error) {
		calls = append(calls, "copy")
		if outputDir == "" {
			t.Fatal("copySelfExecutable() got empty outputDir")
		}
		return filepath.Join(outputDir, mirror.PushBinaryName()), nil
	}

	dir := t.TempDir()
	chartDir := filepath.Join(dir, "chart")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := h.runner.Run(context.Background(), Options{
		ReleaseName: "mirror",
		Chart:       chartDir,
		Namespace:   "default",
		OutputDir:   filepath.Join(dir, "out"),
		Concurrency: 4,
	}); err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}

	if got, want := calls, []string{"render:" + chartDir, "extract", "archive", "manifest", "copy"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Runner.Run() calls = %v, want %v", got, want)
	}

	if _, err := os.Stat(filepath.Join(dir, "out", mirror.PushManifestFileName())); err != nil {
		t.Fatalf("Runner.Run() did not write push manifest: %v", err)
	}
}

func TestRenderChartManifestRendersLocalChart(t *testing.T) {
	h := newRunnerTestHarness(t)
	chartDir := filepath.Join(h.cwd, "chart")
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: example
version: 0.1.0
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "deployment.yaml"), []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: example
spec:
  template:
    spec:
      containers:
        - name: app
          image: quay.io/example/app:v1
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := h.runner.renderChartManifest(context.Background(), Options{
		ReleaseName: "mirror",
		Chart:       "chart",
		Namespace:   "default",
	})
	if err != nil {
		t.Fatalf("renderChartManifest() error = %v", err)
	}
	if !strings.Contains(got, "quay.io/example/app:v1") {
		t.Fatalf("renderChartManifest() = %q, want rendered image", got)
	}
}

func TestRenderChartManifestIgnoresNestedNotesTemplates(t *testing.T) {
	h := newRunnerTestHarness(t)
	chartDir := filepath.Join(h.cwd, "chart")
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: openebs
version: 0.1.0
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "configmap.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: openebs
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	subchartDir := filepath.Join(chartDir, "charts", "alloy")
	if err := os.MkdirAll(filepath.Join(subchartDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(subchartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: alloy
version: 0.1.0
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(subchartDir, "templates", "NOTES.txt"), []byte(`This is plain text from NOTES.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := h.runner.renderChartManifest(context.Background(), Options{
		ReleaseName: "mirror",
		Chart:       "chart",
		Namespace:   "default",
	})
	if err != nil {
		t.Fatalf("renderChartManifest() error = %v", err)
	}
	if !strings.Contains(got, "kind: ConfigMap") {
		t.Fatalf("renderChartManifest() = %q, want rendered yaml manifest", got)
	}
	if strings.Contains(got, "This is plain text from NOTES.") {
		t.Fatalf("renderChartManifest() unexpectedly included NOTES.txt content: %q", got)
	}
}

func TestResolveChartVersionHonorsContextCancellation(t *testing.T) {
	h := newRunnerTestHarness(t)
	h.runner.searchRepoVersions = func(ctx context.Context, _, _ string) ([]searchResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := h.runner.resolveChartVersion(ctx, "https://example.invalid", "openebs"); err == nil {
		t.Fatal("resolveChartVersion() error = nil, want cancellation error")
	}
}

type runnerTestHarness struct {
	t      *testing.T
	cwd    string
	runner Runner
}

func newRunnerTestHarness(t *testing.T) *runnerTestHarness {
	t.Helper()

	cwd := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	h := &runnerTestHarness{t: t, cwd: cwd, runner: NewRunner()}
	return h
}
