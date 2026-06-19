package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

var copyImageToArchive = copyImageToArchiveUsingGoContainerRegistry
var fetchRemoteImage = remote.Image
var writeTarball = tarball.Write

func ArchiveImages(ctx context.Context, images []string, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	archives := make([]string, 0, len(images))
	for _, image := range images {
		archivePath := filepath.Join(outputDir, ArchiveFileName(image))
		if err := copyImageToArchive(ctx, image, archivePath); err != nil {
			return nil, fmt.Errorf("archive %s: %w", image, err)
		}
		archives = append(archives, archivePath)
	}

	return archives, nil
}

func copyImageToArchiveUsingGoContainerRegistry(ctx context.Context, image, archivePath string) error {
	ref, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("parse source image %q: %w", image, err)
	}

	img, err := fetchRemoteImage(ref, remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("fetch source image %q: %w", image, err)
	}

	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive %q: %w", archivePath, err)
	}
	defer out.Close()

	if err := writeTarball(ref, img, out); err != nil {
		_ = out.Close()
		_ = os.Remove(archivePath)
		return fmt.Errorf("write archive %q: %w", archivePath, err)
	}

	return nil
}
