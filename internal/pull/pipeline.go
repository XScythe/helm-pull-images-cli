package pull

import (
	"context"
	"fmt"
	"helm-pull-images-cli/internal/pushspec"
	"io"
	"os"
	"path/filepath"
)

func (r Runner) Run(ctx context.Context, opts Options, status ...io.Writer) error {
	_, err := r.Execute(ctx, opts, status...)
	return err
}

func statusWriter(status ...io.Writer) io.Writer {
	if len(status) > 0 && status[0] != nil {
		return status[0]
	}
	return io.Discard
}

func (r Runner) Execute(ctx context.Context, opts Options, status ...io.Writer) (result PullResult, err error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	statusOut := statusWriter(status...)
	outputDir := opts.OutputDir
	defer func() {
		if err == nil || outputDir == "" {
			return
		}
		if entries, readErr := os.ReadDir(outputDir); readErr == nil && len(entries) == 0 {
			_ = os.Remove(outputDir)
		}
	}()

	if outputDir == "" {
		dir, err := defaultOutputDir(opts.Chart)
		if err != nil {
			return PullResult{}, err
		}
		outputDir = dir
	} else if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return PullResult{}, fmt.Errorf("create output dir: %w", err)
	}

	loaded, err := r.loadChart(runCtx, opts)
	if err != nil {
		return PullResult{}, err
	}
	fmt.Fprintf(statusOut, "chart: name=%s version=%s source=%s\n", loaded.Info.Name, loaded.Info.Version, loaded.Info.Source)

	chartImages, err := r.extractChartImages(runCtx, opts)
	if err != nil {
		return PullResult{}, err
	}

	manifest, err := r.renderManifest(r, runCtx, opts)
	if err != nil {
		cancel()
		return PullResult{}, err
	}

	images, err := r.extractImages(manifest)
	if err != nil {
		cancel()
		return PullResult{}, err
	}
	allImages := appendUnique(chartImages, images...)

	specs := make([]pushspec.ArchiveSpec, 0, len(allImages))
	if len(allImages) > 0 {
		archived, archiveErr := r.archiveImages(runCtx, allImages, outputDir, opts.Concurrency, statusOut)
		if archiveErr != nil {
			cancel()
			return PullResult{}, archiveErr
		}
		specs = append(specs, archived...)
	}

	if err := r.writePushManifest(outputDir, specs); err != nil {
		return PullResult{}, err
	}

	if _, err := r.copySelfExecutable(outputDir); err != nil {
		return PullResult{}, err
	}

	result = PullResult{
		OutputDir:    outputDir,
		Chart:        loaded.Info,
		Images:       allImages,
		ArchiveSpecs: specs,
		ManifestPath: filepath.Join(outputDir, pushspec.PushManifestFileName()),
	}
	return result, nil
}
