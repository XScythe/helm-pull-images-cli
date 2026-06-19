package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
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

func PushImages(ctx context.Context, registry, inputDir string, concurrency int) error {
	manifest, err := ReadPushManifest(inputDir)
	if err != nil {
		return err
	}

	layoutDir := manifest.LayoutDir
	if layoutDir == "" {
		layoutDir = OCILayoutDirName()
	}
	layoutPath, err := loadOCILayout(filepath.Join(inputDir, layoutDir))
	if err != nil {
		return fmt.Errorf("load oci image layout: %w", err)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(normalizeConcurrency(concurrency))
	for _, spec := range manifest.Images {
		spec := spec
		group.Go(func() error {
			if err := copyImageToRegistry(groupCtx, registry, layoutPath, spec.Image, spec.Target, spec.OCIDigest); err != nil {
				return fmt.Errorf("push %s: %w", spec.Image, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}

	return nil
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
		return fmt.Errorf("push image %q to registry %q: %w", sourceImage, registry, err)
	}

	return nil
}

func ReadPushManifest(inputDir string) (*PushManifest, error) {
	path := filepath.Join(inputDir, PushManifestFileName())
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read push manifest: %w", err)
	}

	var manifest PushManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse push manifest: %w", err)
	}

	return &manifest, nil
}
