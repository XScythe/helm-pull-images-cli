package chartmirror

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	Chart       string
	Repo        string
	Version     string
	OutputDir   string
	Concurrency int
}

type PullResult struct {
	OutputDir    string
	Images       []string
	ArchiveSpecs []mirror.ArchiveSpec
	ManifestPath string
}

type PullPipeline interface {
	Execute(ctx context.Context, opts Options) (PullResult, error)
}

type searchResult struct {
	Version string `yaml:"version"`
}

type chartSourceAdapter func(ctx context.Context, opts Options) (*helmchart.Chart, error)

type Runner struct {
	searchRepoVersions func(ctx context.Context, repo, chart string) ([]searchResult, error)
	renderManifest     func(r Runner, ctx context.Context, opts Options) (string, error)
	defaultOutputDir   func(chart string) (string, error)
	extractChartImages func(ctx context.Context, opts Options) ([]string, error)
	extractImages      func(manifest string) ([]string, error)
	archiveImages      func(ctx context.Context, images []string, outputDir string, concurrency int) ([]mirror.ArchiveSpec, error)
	writePushManifest  func(outputDir string, specs []mirror.ArchiveSpec) error
	copySelfExecutable func(outputDir string) (string, error)
	localChartSource   chartSourceAdapter
	helmChartSource    chartSourceAdapter
	chartCache         *loadedCharts
}

type loadedCharts struct {
	mu     sync.Mutex
	byOpts map[string]*helmchart.Chart
}

func Run(ctx context.Context, opts Options) error {
	return NewRunner().Run(ctx, opts)
}

func NewPullPipeline() PullPipeline {
	return NewRunner()
}

func NewRunner() Runner {
	r := Runner{
		searchRepoVersions: helmSearchRepoVersions,
		renderManifest: func(r Runner, ctx context.Context, opts Options) (string, error) {
			return r.renderChartManifest(ctx, opts)
		},
		defaultOutputDir:   defaultOutputDir,
		extractImages:      chartimages.ExtractImages,
		archiveImages:      mirror.ArchiveImages,
		writePushManifest:  mirror.WritePushManifest,
		copySelfExecutable: mirror.CopySelfExecutable,
		chartCache:         &loadedCharts{byOpts: make(map[string]*helmchart.Chart)},
	}
	r.extractChartImages = func(ctx context.Context, opts Options) ([]string, error) {
		return r.extractChartAnnotationImages(ctx, opts)
	}
	r.localChartSource = localChartSource
	r.helmChartSource = func(ctx context.Context, opts Options) (*helmchart.Chart, error) {
		return loadHelmRepoChart(ctx, opts, func(ctx context.Context, repoURL, chart string) (string, error) {
			return r.resolveChartVersion(ctx, repoURL, chart)
		})
	}
	return r
}

func (r Runner) Run(ctx context.Context, opts Options) error {
	_, err := r.Execute(ctx, opts)
	return err
}

func (r Runner) Execute(ctx context.Context, opts Options) (PullResult, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	outputDir := opts.OutputDir
	if outputDir == "" {
		dir, err := r.defaultOutputDir(opts.Chart)
		if err != nil {
			return PullResult{}, err
		}
		outputDir = dir
	} else if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return PullResult{}, fmt.Errorf("create output dir: %w", err)
	}

	chartImages, err := r.extractChartImages(runCtx, opts)
	if err != nil {
		return PullResult{}, err
	}

	type archiveResult struct {
		specs []mirror.ArchiveSpec
		err   error
	}
	var chartArchiveResult chan archiveResult
	fail := func(err error) (PullResult, error) {
		cancel()
		if chartArchiveResult != nil {
			<-chartArchiveResult
		}
		return PullResult{}, err
	}
	if len(chartImages) > 0 {
		chartArchiveResult = make(chan archiveResult, 1)
		go func(images []string) {
			specs, archiveErr := r.archiveImages(runCtx, images, outputDir, opts.Concurrency)
			chartArchiveResult <- archiveResult{specs: specs, err: archiveErr}
		}(append([]string{}, chartImages...))
	}

	manifest, err := r.renderManifest(r, runCtx, opts)
	if err != nil {
		return fail(err)
	}

	images, err := r.extractImages(manifest)
	if err != nil {
		return fail(err)
	}
	allImages := appendUnique(chartImages, images...)
	remainingImages := subtractImages(allImages, chartImages)

	var specs []mirror.ArchiveSpec
	if chartArchiveResult != nil {
		result := <-chartArchiveResult
		if result.err != nil {
			return PullResult{}, result.err
		}
		specs = append(specs, result.specs...)
	}

	if len(remainingImages) > 0 {
		remainingSpecs, archiveErr := r.archiveImages(runCtx, remainingImages, outputDir, opts.Concurrency)
		if archiveErr != nil {
			return PullResult{}, archiveErr
		}
		specs = append(specs, remainingSpecs...)
	}

	if err := r.writePushManifest(outputDir, specs); err != nil {
		return PullResult{}, err
	}

	if _, err := r.copySelfExecutable(outputDir); err != nil {
		return PullResult{}, err
	}

	return PullResult{
		OutputDir:    outputDir,
		Images:       allImages,
		ArchiveSpecs: specs,
		ManifestPath: filepath.Join(outputDir, mirror.PushManifestFileName()),
	}, nil
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
	// Hardcoded release name and namespace: these don't affect image extraction
	// (which is the CLI's sole purpose) as they only influence template rendering
	// metadata. Users cannot customize these values.
	renderValues, err := chartutil.ToRenderValuesWithSchemaValidation(
		chrt,
		map[string]interface{}{},
		chartutil.ReleaseOptions{
			Name:      "mirror",
			Namespace: "default",
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

	removeNotesTemplates(renderedFiles)

	hooks, manifests, err := releaseutil.SortManifests(renderedFiles, nil, releaseutil.InstallOrder)
	if err != nil {
		return renderDebugManifest(renderedFiles), fmt.Errorf("sort manifests: %w", err)
	}

	var out bytes.Buffer
	for _, crd := range chrt.CRDObjects() {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", crd.Filename, string(crd.File.Data))
	}
	for _, hook := range hooks {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
	}
	for _, manifest := range manifests {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", manifest.Name, manifest.Content)
	}
	return out.String(), nil
}

func removeNotesTemplates(renderedFiles map[string]string) {
	for name := range renderedFiles {
		if path.Base(name) == "NOTES.txt" {
			delete(renderedFiles, name)
		}
	}
}

func (r Runner) extractChartAnnotationImages(ctx context.Context, opts Options) ([]string, error) {
	if opts.Chart == "" {
		return nil, nil
	}

	chrt, err := r.loadChart(ctx, opts)
	if err != nil {
		return nil, err
	}
	return chartimages.ExtractChartAnnotationImages(chrt.Metadata.Annotations)
}

func (r Runner) loadChart(ctx context.Context, opts Options) (*helmchart.Chart, error) {
	if cached := r.getCachedChart(opts); cached != nil {
		return cached, nil
	}

	if opts.Repo == "" {
		chrt, localErr := r.localChartSource(ctx, opts)
		if localErr == nil {
			r.setCachedChart(opts, chrt)
			return chrt, nil
		}

		// Fallback to configured repos if local load fails
		chrt, fallbackErr := r.loadChartFromConfiguredRepos(ctx, opts)
		if fallbackErr == nil {
			r.setCachedChart(opts, chrt)
			return chrt, nil
		}

		// Return combined error context
		return nil, fmt.Errorf("chart %q not found: local path failed: %w; configured repos failed: %w",
			opts.Chart, localErr, fallbackErr)
	}
	chrt, err := r.helmChartSource(ctx, opts)
	if err != nil {
		return nil, err
	}
	r.setCachedChart(opts, chrt)
	return chrt, nil
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
	defer resp.Body.Close() //nolint:errcheck

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

func appendUnique(current []string, values ...string) []string {
	seen := make(map[string]struct{}, len(current)+len(values))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		current = append(current, value)
	}
	return current
}

func subtractImages(images []string, remove []string) []string {
	if len(remove) == 0 {
		return append([]string{}, images...)
	}
	excluded := make(map[string]struct{}, len(remove))
	for _, image := range remove {
		excluded[image] = struct{}{}
	}

	out := make([]string, 0, len(images))
	for _, image := range images {
		if _, ok := excluded[image]; ok {
			continue
		}
		out = append(out, image)
	}
	return out
}

func (r Runner) getCachedChart(opts Options) *helmchart.Chart {
	if r.chartCache == nil {
		return nil
	}
	key := cacheKeyForOptions(opts)
	r.chartCache.mu.Lock()
	defer r.chartCache.mu.Unlock()
	return r.chartCache.byOpts[key]
}

func (r Runner) setCachedChart(opts Options, chrt *helmchart.Chart) {
	if r.chartCache == nil || chrt == nil {
		return
	}
	key := cacheKeyForOptions(opts)
	r.chartCache.mu.Lock()
	r.chartCache.byOpts[key] = chrt
	r.chartCache.mu.Unlock()
}

func cacheKeyForOptions(opts Options) string {
	return strings.Join([]string{opts.Repo, opts.Chart, opts.Version}, "\x00")
}

func localChartSource(_ context.Context, opts Options) (*helmchart.Chart, error) {
	return loader.Load(opts.Chart)
}

func loadHelmRepoChart(ctx context.Context, opts Options, resolveVersion func(ctx context.Context, repoURL, chart string) (string, error)) (*helmchart.Chart, error) {
	version := opts.Version
	if version == "" {
		var err error
		version, err = resolveVersion(ctx, opts.Repo, opts.Chart)
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
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch chart archive: %s: %s", resp.Status, string(body))
	}

	return loader.LoadArchive(resp.Body)
}

func (r Runner) loadChartFromConfiguredRepos(ctx context.Context, opts Options) (*helmchart.Chart, error) {
	configRepos, err := loadConfiguredRepos()
	if err != nil {
		return nil, fmt.Errorf("read helm repositories config: %w", err)
	}

	if len(configRepos) == 0 {
		return nil, fmt.Errorf("no configured helm repositories found")
	}

	var repoErrors []string
	for _, configRepo := range configRepos {
		chrt, err := loadHelmRepoChart(ctx, Options{
			Chart:   opts.Chart,
			Repo:    configRepo.URL,
			Version: opts.Version,
		}, func(ctx context.Context, repoURL, chart string) (string, error) {
			return r.resolveChartVersion(ctx, repoURL, chart)
		})

		if err == nil {
			return chrt, nil
		}
		repoErrors = append(repoErrors, fmt.Sprintf("%s: %v", configRepo.Name, err))
	}

	return nil, fmt.Errorf("chart %q not found in configured repos: %s",
		opts.Chart, strings.Join(repoErrors, "; "))
}

func loadConfiguredRepos() ([]*repo.Entry, error) {
	settings := cli.New()
	repoFile := settings.RepositoryConfig

	repoIndex, err := repo.LoadFile(repoFile)
	if err != nil {
		// Return actual error so caller can distinguish between config issues
		// and legitimately no configured repos
		return nil, err
	}

	if len(repoIndex.Repositories) == 0 {
		// No configured repos is not an error condition
		return nil, nil
	}

	return repoIndex.Repositories, nil
}
