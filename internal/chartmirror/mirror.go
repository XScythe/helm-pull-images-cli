package chartmirror

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/repo"

	"helm-pull-images-cli/internal/chartimages"
	"helm-pull-images-cli/internal/mirror"
)

type Options struct {
	ReleaseName string
	Chart       string
	Repo        string
	Version     string
	Namespace   string
	OutputDir   string
	Concurrency int
}

type searchResult struct {
	Version string `yaml:"version"`
}

type Runner struct {
	searchRepoVersions func(ctx context.Context, repo, chart string) ([]searchResult, error)
	renderManifest     func(r Runner, ctx context.Context, opts Options) (string, error)
	defaultOutputDir   func(chart string) (string, error)
	extractImages      func(manifest string) ([]string, error)
	archiveImages      func(ctx context.Context, images []string, outputDir string, concurrency int) ([]mirror.ArchiveSpec, error)
	writePushManifest  func(outputDir string, specs []mirror.ArchiveSpec) error
	copySelfExecutable func(outputDir string) (string, error)
}

func Run(ctx context.Context, opts Options) error {
	return NewRunner().Run(ctx, opts)
}

func NewRunner() Runner {
	return Runner{
		searchRepoVersions: helmSearchRepoVersions,
		renderManifest: func(r Runner, ctx context.Context, opts Options) (string, error) {
			return r.renderChartManifest(ctx, opts)
		},
		defaultOutputDir:   defaultOutputDir,
		extractImages:      chartimages.ExtractImages,
		archiveImages:      mirror.ArchiveImages,
		writePushManifest:  mirror.WritePushManifest,
		copySelfExecutable: mirror.CopySelfExecutable,
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

	manifest, err := r.renderManifest(r, ctx, opts)
	if err != nil {
		return err
	}

	images, err := r.extractImages(manifest)
	if err != nil {
		return err
	}

	specs, err := r.archiveImages(ctx, images, outputDir, opts.Concurrency)
	if err != nil {
		return err
	}

	if err := r.writePushManifest(outputDir, specs); err != nil {
		return err
	}

	if _, err := r.copySelfExecutable(outputDir); err != nil {
		return err
	}

	return nil
}

func (r Runner) renderChartManifest(ctx context.Context, opts Options) (string, error) {
	chrt, err := r.loadChart(ctx, opts)
	if err != nil {
		return "", err
	}

	if err := chartutil.ProcessDependenciesWithMerge(chrt, chartutil.Values{}); err != nil {
		return "", err
	}

	caps := chartutil.DefaultCapabilities.Copy()
	renderValues, err := chartutil.ToRenderValuesWithSchemaValidation(
		chrt,
		map[string]interface{}{},
		chartutil.ReleaseOptions{
			Name:      opts.ReleaseName,
			Namespace: opts.Namespace,
			Revision:  1,
			IsInstall: true,
		},
		caps,
		false,
	)
	if err != nil {
		return "", err
	}

	renderedFiles, err := engine.Render(chrt, renderValues)
	if err != nil {
		return "", err
	}

	delete(renderedFiles, "NOTES.txt")

	_, manifests, err := releaseutil.SortManifests(renderedFiles, nil, releaseutil.InstallOrder)
	if err != nil {
		return renderDebugManifest(renderedFiles), fmt.Errorf("sort manifests: %w", err)
	}

	var out bytes.Buffer
	for _, crd := range chrt.CRDObjects() {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", crd.Filename, string(crd.File.Data))
	}
	for _, manifest := range manifests {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", manifest.Name, manifest.Content)
	}
	return out.String(), nil
}

func (r Runner) loadChart(ctx context.Context, opts Options) (*helmchart.Chart, error) {
	if opts.Repo == "" {
		return loader.Load(opts.Chart)
	}

	version := opts.Version
	if version == "" {
		var err error
		version, err = r.resolveChartVersion(ctx, opts.Repo, opts.Chart)
		if err != nil {
			return nil, err
		}
	}

	chartURL, err := repo.FindChartInRepoURL(opts.Repo, opts.Chart, version, "", "", "", getter.All(cli.New()))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chartURL, nil)
	if err != nil {
		return nil, fmt.Errorf("prepare chart archive request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch chart archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch chart archive: %s: %s", resp.Status, string(body))
	}

	return loader.LoadArchive(resp.Body)
}

func helmSearchRepoVersions(ctx context.Context, repoURL, chart string) ([]searchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(repoURL, "/")+"/index.yaml", nil)
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
	if err := yaml.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, err
	}
	return index.Entries[chart], nil
}

func renderDebugManifest(files map[string]string) string {
	var b bytes.Buffer
	for name, content := range files {
		if strings.TrimSpace(content) == "" {
			continue
		}
		fmt.Fprintf(&b, "---\n# Source: %s\n%s\n", name, content)
	}
	return b.String()
}

func (r Runner) resolveChartVersion(ctx context.Context, repoURL, chart string) (string, error) {
	search := r.searchRepoVersions
	if search == nil {
		search = helmSearchRepoVersions
	}

	results, err := search(ctx, repoURL, chart)
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if isStableVersion(result.Version) {
			return result.Version, nil
		}
	}

	return "", fmt.Errorf("no stable version found for %s/%s", repoURL, chart)
}

func isStableVersion(version string) bool {
	return version != "" && !strings.Contains(version, "-")
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
