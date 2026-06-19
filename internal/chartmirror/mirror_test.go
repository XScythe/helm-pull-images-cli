package chartmirror

import (
	"context"
	"fmt"
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

func TestRenderChartArgsUsesLocalChartWithoutRepo(t *testing.T) {
	testCases := []struct {
		name     string
		opts     Options
		wantOmit []string
	}{
		{
			name: "local path",
			opts: Options{
				ReleaseName: "mirror",
				Chart:       t.TempDir(),
				Namespace:   "default",
				Version:     "ignored",
			},
			wantOmit: []string{"--repo", "--version"},
		},
		{
			name: "explicit local flag",
			opts: Options{
				ReleaseName: "mirror",
				Chart:       "nginx",
				Local:       true,
				Namespace:   "default",
			},
			wantOmit: []string{"--repo", "--version"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := NewRunner().renderChartArgs(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("renderChartArgs() error = %v", err)
			}
			for _, arg := range tc.wantOmit {
				if containsArg(args, arg) {
					t.Fatalf("renderChartArgs() unexpectedly contained %q: %v", arg, args)
				}
			}
		})
	}
}

func TestRenderChartArgsWarnsOnBareChartAndUsesRemote(t *testing.T) {
	h := newRunnerTestHarness(t)

	args, err := h.runner.renderChartArgs(context.Background(), Options{
		ReleaseName: "mirror",
		Chart:       "nginx",
		Repo:        "https://example.invalid",
		Version:     "1.2.3",
		Namespace:   "default",
	})
	if err != nil {
		t.Fatalf("renderChartArgs() error = %v", err)
	}

	if h.warning == "" || !strings.Contains(h.warning, "defaulting to remote") {
		t.Fatalf("renderChartArgs() warning = %q, want remote warning", h.warning)
	}

	for _, arg := range []string{"--repo", "--version"} {
		if !containsArg(args, arg) {
			t.Fatalf("renderChartArgs() missing %q: %v", arg, args)
		}
	}
}

func TestRunnerRunOrchestratesDependencies(t *testing.T) {
	h := newRunnerTestHarness(t)

	var calls []string
	h.runner.renderChartCommand = func(_ context.Context, args []string) (string, error) {
		calls = append(calls, "render:"+strings.Join(args, " "))
		return "kind: ConfigMap\nmetadata:\n  name: demo\n  annotations:\n    image: quay.io/example/api:v1\n", nil
	}
	h.runner.extractImages = func(manifest string) ([]string, error) {
		calls = append(calls, "extract")
		if !strings.Contains(manifest, "demo") {
			t.Fatalf("extractImages() saw unexpected manifest: %q", manifest)
		}
		return []string{"quay.io/example/api:v1"}, nil
	}
	h.runner.archiveImages = func(_ context.Context, images []string, outputDir string) ([]string, error) {
		calls = append(calls, "archive")
		if got, want := images, []string{"quay.io/example/api:v1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("archiveImages() images = %v, want %v", got, want)
		}
		if outputDir == "" {
			t.Fatal("archiveImages() got empty outputDir")
		}
		return []string{filepath.Join(outputDir, "quay.io_example_api_v1.tar")}, nil
	}
	h.runner.generatePushManifest = func(images []string) (string, error) {
		calls = append(calls, "manifest")
		if got, want := images, []string{"quay.io/example/api:v1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("generatePushManifest() images = %v, want %v", got, want)
		}
		return "{\n  \"images\": []\n}\n", nil
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
	}); err != nil {
		t.Fatalf("Runner.Run() error = %v", err)
	}

	if got, want := calls, []string{"render:template mirror " + chartDir + " --namespace default --include-crds", "extract", "archive", "manifest", "copy"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Runner.Run() calls = %v, want %v", got, want)
	}

	if _, err := os.Stat(filepath.Join(dir, "out", mirror.PushManifestFileName())); err != nil {
		t.Fatalf("Runner.Run() did not write push manifest: %v", err)
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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

type runnerTestHarness struct {
	t       *testing.T
	cwd     string
	runner  Runner
	warning string
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
	h.runner.warnf = func(format string, args ...any) {
		h.warning = fmt.Sprintf(format, args...)
	}
	return h
}
