package push

import (
	"context"
	"errors"
	"fmt"
	"helm-pull-images-cli/internal/pushspec"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/sync/errgroup"
)

var fetchRemoteImage = remote.Image
var writeLayout = layout.Write
var fromLayoutPath = layout.FromPath
var appendLayoutImage = func(path layout.Path, img v1.Image) error {
	return path.AppendImage(img)
}

func ArchiveImages(ctx context.Context, images []string, outputDir string, concurrency int, status ...io.Writer) ([]pushspec.ArchiveSpec, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	specs, err := pushspec.BuildSpecs(images)
	if err != nil {
		return nil, fmt.Errorf("build archive specs: %w", err)
	}
	progress := newTransferProgress(statusWriter(status...), "pulling", len(specs))
	defer progress.Finish()

	layoutRoot := filepath.Join(outputDir, pushspec.OCILayoutDirName())
	layoutPath, createdLayout, err := openOrCreateLayout(layoutRoot)
	if err != nil {
		return nil, fmt.Errorf("open or create oci layout: %w", err)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(normalizeConcurrency(concurrency))

	var writeMu sync.Mutex
	for i := range specs {
		i := i
		group.Go(func() error {
			progress.Begin(specs[i].Image)
			defer progress.End(specs[i].Image)

			digest, copyErr := copyImageToLayoutUsingGoContainerRegistry(groupCtx, specs[i].Image, layoutPath, &writeMu)
			if copyErr != nil {
				return fmt.Errorf("archive %s: %w", specs[i].Image, copyErr)
			}
			specs[i].OCIDigest = digest
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		if createdLayout {
			_ = os.RemoveAll(layoutRoot)
		}
		return nil, fmt.Errorf("archive images: %w", err)
	}

	return specs, nil
}

func openOrCreateLayout(layoutRoot string) (layout.Path, bool, error) {
	info, err := os.Stat(layoutRoot)
	if err == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("open oci image layout: %s is not a directory", layoutRoot)
		}
		path, openErr := fromLayoutPath(layoutRoot)
		if openErr != nil {
			return "", false, fmt.Errorf("open oci image layout: %w", openErr)
		}
		return path, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat oci image layout: %w", err)
	}

	path, createErr := writeLayout(layoutRoot, empty.Index)
	if createErr != nil {
		return "", false, fmt.Errorf("create oci image layout: %w", createErr)
	}
	return path, true, nil
}

func copyImageToLayoutUsingGoContainerRegistry(ctx context.Context, image string, layoutPath layout.Path, writeMu *sync.Mutex) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("parse source image %q: %w", image, err)
	}

	img, err := fetchRemoteImage(ref, remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("fetch source image %q: %w", image, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("resolve digest for image %q: %w", image, err)
	}

	writeMu.Lock()
	defer writeMu.Unlock()
	if err := appendLayoutImage(layoutPath, img); err != nil {
		return "", fmt.Errorf("append image %q to oci layout: %w", image, err)
	}

	return digest.String(), nil
}

func normalizeConcurrency(value int) int {
	if value < 1 {
		return 1
	}
	return value
}
