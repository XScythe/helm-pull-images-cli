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
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

var copyImageToRegistry = copyImageToRegistryUsingGoContainerRegistry
var loadTarballImage = tarball.ImageFromPath
var writeRemoteImage = remote.Write

func PushImages(ctx context.Context, registry, inputDir string) error {
	specs, err := ReadPushManifest(inputDir)
	if err != nil {
		return err
	}

	for _, spec := range specs.Images {
		archivePath := filepath.Join(inputDir, spec.Archive)
		if err := copyImageToRegistry(ctx, registry, archivePath, spec.Image, spec.Target); err != nil {
			return fmt.Errorf("push %s: %w", spec.Image, err)
		}
	}

	return nil
}

func copyImageToRegistryUsingGoContainerRegistry(ctx context.Context, registry, archivePath, sourceImage, target string) error {
	sourceRef, err := name.ParseReference(sourceImage)
	if err != nil {
		return fmt.Errorf("parse source image %q: %w", sourceImage, err)
	}

	registry = strings.TrimRight(registry, "/")
	var sourceTag *name.Tag
	if tag, ok := sourceRef.(name.Tag); ok {
		sourceTag = &tag
	}

	img, err := loadTarballImage(archivePath, sourceTag)
	if err != nil {
		return fmt.Errorf("load archive %q: %w", archivePath, err)
	}

	destRef, err := name.ParseReference(fmt.Sprintf("%s/%s", registry, target))
	if err != nil {
		return fmt.Errorf("parse destination reference %q: %w", registry+"/"+target, err)
	}

	if err := writeRemoteImage(destRef, img, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return fmt.Errorf("push archive %q to registry %q: %w", archivePath, registry, err)
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
