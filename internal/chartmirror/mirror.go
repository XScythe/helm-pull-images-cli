package chartmirror

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"helm-pull-images-cli/internal/chartimages"
	"helm-pull-images-cli/internal/mirror"
)

type Options struct {
	ReleaseName string
	Chart       string
	Local       bool
	Repo        string
	Version     string
	Namespace   string
	OutputDir   string
}

type searchResult struct {
	Version string `yaml:"version"`
}

type Runner struct {
	searchRepoVersions   func(ctx context.Context, repo, chart string) ([]searchResult, error)
	renderChartCommand   func(ctx context.Context, args []string) (string, error)
	defaultOutputDir     func(chart string) (string, error)
	warnf                func(format string, args ...any)
	extractImages        func(manifest string) ([]string, error)
	archiveImages        func(ctx context.Context, images []string, outputDir string) ([]string, error)
	generatePushManifest func(images []string) (string, error)
	copySelfExecutable   func(outputDir string) (string, error)
}

func Run(ctx context.Context, opts Options) error {
	return NewRunner().Run(ctx, opts)
}

func NewRunner() Runner {
	return Runner{
		searchRepoVersions: helmSearchRepoVersions,
		renderChartCommand: renderChartCommand,
		defaultOutputDir:   defaultOutputDir,
		warnf: func(format string, args ...any) {
			_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
		},
		extractImages:        chartimages.ExtractImages,
		archiveImages:        mirror.ArchiveImages,
		generatePushManifest: mirror.GeneratePushManifest,
		copySelfExecutable:   mirror.CopySelfExecutable,
	}
}

func (r Runner) Run(ctx context.Context, opts Options) error {
	outputDir := opts.OutputDir
	if outputDir == "" {
		dir, err := r.defaultOutputDir(opts.Chart)
		if err != nil {
			return err
		}
		outputDir = dir
	} else if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	manifest, err := r.renderChart(ctx, opts)
	if err != nil {
		return err
	}

	images, err := r.extractImages(manifest)
	if err != nil {
		return err
	}

	if _, err := r.archiveImages(ctx, images, outputDir); err != nil {
		return err
	}

	pushManifest, err := r.generatePushManifest(images)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(outputDir, mirror.PushManifestFileName())
	if err := os.WriteFile(manifestPath, []byte(pushManifest), 0o644); err != nil {
		return fmt.Errorf("write push manifest: %w", err)
	}

	if _, err := r.copySelfExecutable(outputDir); err != nil {
		return err
	}

	return nil
}

func (r Runner) renderChart(ctx context.Context, opts Options) (string, error) {
	args, err := r.renderChartArgs(ctx, opts)
	if err != nil {
		return "", err
	}

	return r.renderChartCommand(ctx, args)
}

func renderChartCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("render chart: %w: %s", err, string(output))
	}

	return string(output), nil
}

func (r Runner) renderChartArgs(ctx context.Context, opts Options) ([]string, error) {
	if ok, err := r.shouldTreatAsLocal(opts); err != nil {
		return nil, err
	} else if ok {
		return []string{
			"template",
			opts.ReleaseName,
			opts.Chart,
			"--namespace",
			opts.Namespace,
			"--include-crds",
		}, nil
	}

	if isBareChart(opts.Chart) {
		if r.warnf != nil {
			r.warnf("warning: chart %q looks like a bare name; defaulting to remote. Use an explicit path like ./%s or --local to treat it as local.", opts.Chart, opts.Chart)
		}
	}

	if opts.Repo == "" {
		return nil, fmt.Errorf("--repo is required for remote charts")
	}

	version := opts.Version
	if version == "" {
		var err error
		version, err = r.resolveChartVersion(ctx, opts.Repo, opts.Chart)
		if err != nil {
			return nil, err
		}
	}

	return []string{
		"template",
		opts.ReleaseName,
		opts.Chart,
		"--namespace",
		opts.Namespace,
		"--include-crds",
		"--repo", opts.Repo,
		"--version", version,
	}, nil
}

func (r Runner) shouldTreatAsLocal(opts Options) (bool, error) {
	if opts.Local {
		return true, nil
	}

	if !hasPathIndicator(opts.Chart) {
		return false, nil
	}

	info, err := os.Stat(opts.Chart)
	if err != nil {
		return false, fmt.Errorf("resolve local chart %q: %w", opts.Chart, err)
	}

	return info.IsDir() || info.Mode().IsRegular(), nil
}

func (r Runner) resolveChartVersion(ctx context.Context, repo, chart string) (string, error) {
	results, err := r.searchRepoVersions(ctx, repo, chart)
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if isStableVersion(result.Version) {
			return result.Version, nil
		}
	}

	return "", fmt.Errorf("no stable version found for %s/%s", repo, chart)
}

func helmSearchRepoVersions(ctx context.Context, repo, chart string) ([]searchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(repo, "/")+"/index.yaml", nil)
	if err != nil {
		return nil, fmt.Errorf("prepare chart index request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch chart index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch chart index: %s: %s", resp.Status, string(body))
	}

	var index struct {
		Entries map[string][]searchResult `yaml:"entries"`
	}
	decoder := yaml.NewDecoder(resp.Body)
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("parse chart index: %w", err)
	}
	return index.Entries[chart], nil
}

func isStableVersion(version string) bool {
	return version != "" && !strings.Contains(version, "-")
}

func hasPathIndicator(chart string) bool {
	return filepath.IsAbs(chart) ||
		strings.HasPrefix(chart, "."+string(filepath.Separator)) ||
		strings.HasPrefix(chart, ".."+string(filepath.Separator)) ||
		strings.ContainsRune(chart, filepath.Separator)
}

func isBareChart(chart string) bool {
	return chart != "" && !hasPathIndicator(chart)
}

func defaultOutputDir(chart string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current working directory: %w", err)
	}
	prefix := "helm-pull-images-"
	if chart != "" {
		prefix += sanitizeName(chart) + "-"
	}
	return os.MkdirTemp(cwd, prefix)
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
