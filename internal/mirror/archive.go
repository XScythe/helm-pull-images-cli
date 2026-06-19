package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/sync/errgroup"
)

var fetchRemoteImage = remote.Image
var writeLayout = layout.Write
var appendLayoutImage = func(path layout.Path, img v1.Image) error {
	return path.AppendImage(img)
}

func ArchiveImages(ctx context.Context, images []string, outputDir string, concurrency int) ([]ArchiveSpec, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	specs, err := BuildSpecs(images)
	if err != nil {
		return nil, err
	}

	layoutPath, err := writeLayout(filepath.Join(outputDir, OCILayoutDirName()), empty.Index)
	if err != nil {
		return nil, fmt.Errorf("create oci image layout: %w", err)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(normalizeConcurrency(concurrency))

	var writeMu sync.Mutex
	for i := range specs {
		i := i
		group.Go(func() error {
			digest, copyErr := copyImageToLayoutUsingGoContainerRegistry(groupCtx, specs[i].Image, layoutPath, &writeMu)
			if copyErr != nil {
				return fmt.Errorf("archive %s: %w", specs[i].Image, copyErr)
			}
			specs[i].OCIDigest = digest
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		_ = os.RemoveAll(string(layoutPath))
		return nil, err
	}

	return specs, nil
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
