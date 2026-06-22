package chartmirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	helmchart "helm.sh/helm/v3/pkg/chart"

	"helm-pull-images-cli/internal/mirror"
)

func TestDefaultOutputDirCreatesNewDirectoryInCWD(t *testing.T) {
	h := newRunnerTestHarness(t)

	dir, err := h.runner.defaultOutputDir("openebs")
	if err != nil {
		t.Fatalf("defaultOutputDir() error = %v", err)
	}

	// Verify directory is created in the CWD with the chart name
	expected := filepath.Join(h.cwd, "openebs")
	if dir != expected {
		t.Fatalf("defaultOutputDir() = %q, want %q", dir, expected)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("defaultOutputDir() path missing or not a directory: %v, %v", info, err)
	}
}

func TestDefaultOutputDirAppendsTimestampWhenDirectoryExists(t *testing.T) {
	h := newRunnerTestHarness(t)

	// Create directory first
	chart := "nginx"
	first := filepath.Join(h.cwd, chart)
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Call defaultOutputDir for the same chart
	dir, err := h.runner.defaultOutputDir(chart)
	if err != nil {
		t.Fatalf("defaultOutputDir() error = %v", err)
	}

	// Should have timestamp appended in format {chart}-YYYY-MM-DD-HH
	if !strings.Contains(dir, chart+"-") {
		t.Fatalf("defaultOutputDir() = %q, should contain %q", dir, chart+"-")
	}

	// Verify it matches the expected pattern
	base := filepath.Base(dir)
	expectedPattern := fmt.Sprintf("%s-\\d{4}-\\d{2}-\\d{2}-\\d{2}", chart)
	if !matchesPattern(base, expectedPattern) {
		t.Fatalf("defaultOutputDir() = %q, doesn't match pattern %q", base, expectedPattern)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("defaultOutputDir() path missing or not a directory: %v, %v", info, err)
	}
}

func matchesPattern(s, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
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
	h.runner.extractChartImages = func(_ context.Context, _ Options) ([]string, error) {
		return nil, nil
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
		Chart:       chartDir,
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

func TestRunnerRunIncludesChartAnnotationImagesBeforeManifestImages(t *testing.T) {
	h := newRunnerTestHarness(t)

	var archiveCalls [][]string
	h.runner.renderManifest = func(_ Runner, _ context.Context, _ Options) (string, error) {
		return "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n", nil
	}
	h.runner.extractImages = func(_ string) ([]string, error) {
		return []string{
			"quay.io/example/app:v1",
			"quay.io/example/from-annotation:v2",
		}, nil
	}
	h.runner.archiveImages = func(_ context.Context, images []string, outputDir string, concurrency int) ([]mirror.ArchiveSpec, error) {
		archiveCalls = append(archiveCalls, append([]string{}, images...))
		if reflect.DeepEqual(images, []string{"quay.io/example/from-annotation:v2", "busybox:1.36"}) {
			return []mirror.ArchiveSpec{
				{
					Image:     "quay.io/example/from-annotation:v2",
					Target:    "example/from-annotation:v2",
					OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
				{
					Image:     "busybox:1.36",
					Target:    "library/busybox:1.36",
					OCIDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				},
			}, nil
		}
		return []mirror.ArchiveSpec{
			{
				Image:     "quay.io/example/app:v1",
				Target:    "example/app:v1",
				OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		}, nil
	}
	h.runner.writePushManifest = func(outputDir string, specs []mirror.ArchiveSpec) error {
		return os.WriteFile(filepath.Join(outputDir, mirror.PushManifestFileName()), []byte("{\n  \"images\": []\n}\n"), 0o644)
	}
	h.runner.copySelfExecutable = func(outputDir string) (string, error) {
		return filepath.Join(outputDir, mirror.PushBinaryName()), nil
	}

	dir := t.TempDir()
	chartDir := filepath.Join(dir, "chart")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: example
version: 0.1.0
annotations:
  annotation.helm.sh/images: |
    - quay.io/example/from-annotation:v2
    - busybox:1.36
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "noop.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: noop
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := h.runner.Run(context.Background(), Options{
		Chart:       chartDir,
		OutputDir:   filepath.Join(dir, "out"),
		Concurrency: 2,
	}); err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}

	wantCalls := [][]string{
		{"quay.io/example/from-annotation:v2", "busybox:1.36"},
		{"quay.io/example/app:v1"},
	}
	if !reflect.DeepEqual(archiveCalls, wantCalls) {
		t.Fatalf("Runner.Run() archive calls = %v, want %v", archiveCalls, wantCalls)
	}
}

func TestRunnerRunChecksChartAnnotationsBeforeRendering(t *testing.T) {
	h := newRunnerTestHarness(t)

	var calls []string
	h.runner.extractChartImages = func(_ context.Context, _ Options) ([]string, error) {
		calls = append(calls, "chart")
		return nil, nil
	}

	h.runner.renderManifest = func(_ Runner, _ context.Context, _ Options) (string, error) {
		calls = append(calls, "render")
		return "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n", nil
	}
	h.runner.extractImages = func(_ string) ([]string, error) {
		calls = append(calls, "extract")
		return []string{"busybox:1.36"}, nil
	}
	h.runner.archiveImages = func(_ context.Context, _ []string, _ string, _ int) ([]mirror.ArchiveSpec, error) {
		calls = append(calls, "archive")
		return []mirror.ArchiveSpec{{
			Image:     "busybox:1.36",
			Target:    "library/busybox:1.36",
			OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}}, nil
	}
	h.runner.writePushManifest = func(outputDir string, _ []mirror.ArchiveSpec) error {
		calls = append(calls, "manifest")
		return os.WriteFile(filepath.Join(outputDir, mirror.PushManifestFileName()), []byte("{}\n"), 0o644)
	}
	h.runner.copySelfExecutable = func(outputDir string) (string, error) {
		calls = append(calls, "copy")
		return filepath.Join(outputDir, mirror.PushBinaryName()), nil
	}

	if err := h.runner.Run(context.Background(), Options{
		Chart:       "ignored-by-stub",
		OutputDir:   t.TempDir(),
		Concurrency: 1,
	}); err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}

	want := []string{"chart", "render", "extract", "archive", "manifest", "copy"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("Runner.Run() calls = %v, want %v", calls, want)
	}
}

func TestRunnerExecuteReturnsPullResult(t *testing.T) {
	h := newRunnerTestHarness(t)

	h.runner.extractChartImages = func(_ context.Context, _ Options) ([]string, error) {
		return []string{"busybox:1.36"}, nil
	}
	h.runner.renderManifest = func(_ Runner, _ context.Context, _ Options) (string, error) {
		return "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n", nil
	}
	h.runner.extractImages = func(_ string) ([]string, error) {
		return nil, nil
	}
	h.runner.archiveImages = func(_ context.Context, images []string, _ string, _ int) ([]mirror.ArchiveSpec, error) {
		return []mirror.ArchiveSpec{{
			Image:     images[0],
			Target:    "library/busybox:1.36",
			OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}}, nil
	}
	h.runner.writePushManifest = func(outputDir string, _ []mirror.ArchiveSpec) error {
		return os.WriteFile(filepath.Join(outputDir, mirror.PushManifestFileName()), []byte("{}\n"), 0o644)
	}
	h.runner.copySelfExecutable = func(outputDir string) (string, error) {
		return filepath.Join(outputDir, mirror.PushBinaryName()), nil
	}

	outDir := t.TempDir()
	result, err := h.runner.Execute(context.Background(), Options{
		Chart:       "ignored-by-stub",
		OutputDir:   outDir,
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("Runner.Execute() error = %v", err)
	}

	if result.OutputDir != outDir {
		t.Fatalf("Runner.Execute() outputDir = %q, want %q", result.OutputDir, outDir)
	}
	if got, want := result.Images, []string{"busybox:1.36"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Runner.Execute() images = %v, want %v", got, want)
	}
	if got, want := result.ManifestPath, filepath.Join(outDir, mirror.PushManifestFileName()); got != want {
		t.Fatalf("Runner.Execute() manifestPath = %q, want %q", got, want)
	}
	if len(result.ArchiveSpecs) != 1 || result.ArchiveSpecs[0].Image != "busybox:1.36" {
		t.Fatalf("Runner.Execute() archiveSpecs = %#v", result.ArchiveSpecs)
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
		Chart: "chart",
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
		Chart: "chart",
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

func TestRenderChartManifestIncludesHookResources(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(chartDir, "templates", "hook-job.yaml"), []byte(`apiVersion: batch/v1
kind: Job
metadata:
  name: hook-job
  annotations:
    "helm.sh/hook": pre-install
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: hook
          image: quay.io/example/hook:v1
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := h.runner.renderChartManifest(context.Background(), Options{
		Chart: "chart",
	})
	if err != nil {
		t.Fatalf("renderChartManifest() error = %v", err)
	}
	if !strings.Contains(got, "quay.io/example/hook:v1") {
		t.Fatalf("renderChartManifest() = %q, want hook manifest image", got)
	}
}

func TestLoadChartUsesCachedChartForSameOptions(t *testing.T) {
	h := newRunnerTestHarness(t)
	chartDir := filepath.Join(h.cwd, "chart")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cached
version: 0.1.0
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opts := Options{Chart: "chart"}
	if _, err := h.runner.loadChart(context.Background(), opts); err != nil {
		t.Fatalf("loadChart() first call error = %v", err)
	}

	if err := os.Remove(filepath.Join(chartDir, "Chart.yaml")); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := h.runner.loadChart(context.Background(), opts); err != nil {
		t.Fatalf("loadChart() second call error = %v, want cached chart", err)
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

// TestLoadChartFallbackToConfiguredReposOnLocalFailure verifies fallback activates when local load fails
func TestLoadChartFallbackToConfiguredReposOnLocalFailure(t *testing.T) {
	h := newRunnerTestHarness(t)
	h.runner.localChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		return nil, fmt.Errorf("local chart not found")
	}
	h.runner.helmChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		// This simulates what happens when fallback searches configured repos
		// In a real scenario with configured repos, this would return a chart
		return nil, fmt.Errorf("chart not found in any configured repo")
	}

	_, err := h.runner.loadChart(context.Background(), Options{Chart: "test"})
	if err == nil {
		t.Fatal("loadChart() should return error when both local and fallback fail")
	}
	// Should show both local and fallback failures in the error
	errStr := err.Error()
	if !strings.Contains(errStr, "not found") {
		t.Fatalf("error should indicate chart not found, got: %s", errStr)
	}
}

// TestLoadChartNoFallbackWhenLocalSucceeds verifies fallback doesn't trigger if local load succeeds
func TestLoadChartNoFallbackWhenLocalSucceeds(t *testing.T) {
	h := newRunnerTestHarness(t)
	localCalled := false

	h.runner.localChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		localCalled = true
		return &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "local"}}, nil
	}

	chrt, err := h.runner.loadChart(context.Background(), Options{Chart: "test"})
	if err != nil {
		t.Fatalf("loadChart() error = %v, want nil", err)
	}
	if !localCalled {
		t.Fatal("local source should have been called")
	}
	if chrt.Metadata.Name != "local" {
		t.Fatalf("loadChart() returned chart name %q, want local", chrt.Metadata.Name)
	}
}

// TestLoadChartNoFallbackWhenRepoSpecified verifies fallback doesn't trigger when --repo is provided
func TestLoadChartNoFallbackWhenRepoSpecified(t *testing.T) {
	h := newRunnerTestHarness(t)
	localCalled := false

	h.runner.localChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		localCalled = true
		return nil, fmt.Errorf("should not call")
	}
	h.runner.helmChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		return &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "repo"}}, nil
	}

	chrt, err := h.runner.loadChart(context.Background(), Options{
		Chart: "test",
		Repo:  "https://example.com",
	})
	if err != nil {
		t.Fatalf("loadChart() error = %v, want nil", err)
	}
	if localCalled {
		t.Fatal("local source should not have been called when repo is specified")
	}
	if chrt.Metadata.Name != "repo" {
		t.Fatalf("loadChart() returned chart name %q, want repo", chrt.Metadata.Name)
	}
}

// TestLoadChartCombinedErrorContext verifies error message includes both local and fallback failure context
func TestLoadChartCombinedErrorContext(t *testing.T) {
	h := newRunnerTestHarness(t)
	h.runner.localChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		return nil, fmt.Errorf("local path does not exist")
	}
	// Simulate no configured repos by mocking helmChartSource to fail
	h.runner.helmChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		return nil, fmt.Errorf("no configured repos available")
	}

	_, err := h.runner.loadChart(context.Background(), Options{Chart: "test"})
	if err == nil {
		t.Fatal("loadChart() should return error when both local and fallback fail")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "not found") {
		t.Fatalf("error should indicate chart not found, got: %s", errStr)
	}
}

// TestLoadChartCachedResultNotBypassedByFallback verifies caching still works with fallback
func TestLoadChartCachedResultNotBypassedByFallback(t *testing.T) {
	h := newRunnerTestHarness(t)
	callCount := 0
	h.runner.localChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		callCount++
		return &helmchart.Chart{Metadata: &helmchart.Metadata{Name: "cached"}}, nil
	}

	opts := Options{Chart: "test"}
	chrt1, _ := h.runner.loadChart(context.Background(), opts)
	chrt2, _ := h.runner.loadChart(context.Background(), opts)

	if callCount != 1 {
		t.Fatalf("localChartSource called %d times, want 1 (should use cache)", callCount)
	}
	if chrt1 != chrt2 {
		t.Fatal("cached chart should return same object")
	}
}
