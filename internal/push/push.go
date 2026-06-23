// Package push is the image transfer engine shared by both CLI phases. During
// pull it stages remote images into a local OCI layout (ArchiveImages) and
// copies the helper binary; during push it uploads that layout to a target
// registry (PushImages). The on-disk manifest contract lives in pushspec.
package push

import (
	"context"
	"fmt"
	"helm-deep-pack/internal/pushspec"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/sync/errgroup"
)

var copyImageToRegistry = copyImageToRegistryUsingGoContainerRegistry
var writeRemoteImage = remote.Write
var loadLayoutImage = func(layoutPath layout.Path, hash v1.Hash) (v1.Image, error) {
	return layoutPath.Image(hash)
}
var loadOCILayout = layout.FromPath
var resolveExecutablePath = os.Executable

func PushImages(ctx context.Context, registry, inputDir string, concurrency int, status ...io.Writer) error {
	resolvedInputDir, err := resolvePushInputDir(inputDir)
	if err != nil {
		return fmt.Errorf("resolve push input dir: %w", err)
	}

	manifest, err := pushspec.ReadPushManifest(resolvedInputDir)
	if err != nil {
		return fmt.Errorf("read push manifest: %w", err)
	}
	progress := newTransferProgress(statusWriter(status...), "pushing", len(manifest.Images))
	defer progress.Finish()

	layoutDir := manifest.LayoutDir
	if layoutDir == "" {
		layoutDir = pushspec.OCILayoutDirName()
	}
	layoutPath, err := loadOCILayout(filepath.Join(resolvedInputDir, layoutDir))
	if err != nil {
		return fmt.Errorf("load oci image layout: %w", err)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(normalizeConcurrency(concurrency))
	for _, spec := range manifest.Images {
		spec := spec
		group.Go(func() error {
			progress.Begin(spec.Image)
			defer progress.End(spec.Image)

			if err := copyImageToRegistry(groupCtx, registry, layoutPath, spec.Image, spec.Target, spec.OCIDigest); err != nil {
				return fmt.Errorf("push %s: %w", spec.Image, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("push images: %w", err)
	}

	return nil
}

func resolvePushInputDir(inputDir string) (string, error) {
	if inputDir != "" {
		return inputDir, nil
	}

	executable, err := resolveExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Dir(executable), nil
}

func copyImageToRegistryUsingGoContainerRegistry(ctx context.Context, registry string, layoutPath layout.Path, sourceImage, target, ociDigest string) error {
	if _, err := name.ParseReference(sourceImage); err != nil {
		return fmt.Errorf("parse source image %q: %w", sourceImage, err)
	}

	hash, err := v1.NewHash(ociDigest)
	if err != nil {
		return fmt.Errorf("parse oci digest %q: %w", ociDigest, err)
	}

	registry = strings.TrimRight(registry, "/")

	img, err := loadLayoutImage(layoutPath, hash)
	if err != nil {
		return fmt.Errorf("load image %q from oci layout: %w", sourceImage, err)
	}

	destRef, err := name.ParseReference(fmt.Sprintf("%s/%s", registry, target))
	if err != nil {
		return fmt.Errorf("parse destination reference %q: %w", registry+"/"+target, err)
	}

	if err := writeRemoteImage(destRef, img, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		if looksLikeWebsite(err) {
			return fmt.Errorf("registry %q does not look like a container registry", registry)
		}
		return fmt.Errorf("push image %q to registry %q: %w", sourceImage, registry, err)
	}

	return nil
}

func looksLikeWebsite(err error) bool {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "text/html"):
		return true
	case strings.Contains(msg, "invalid character '<'"):
		return true
	case strings.Contains(msg, "<!doctype"):
		return true
	case strings.Contains(msg, "<html"):
		return true
	default:
		return false
	}
}
