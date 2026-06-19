package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestPushImagesUsesManifestArchives(t *testing.T) {
	original := copyImageToRegistry
	defer func() { copyImageToRegistry = original }()

	var calls []string
	copyImageToRegistry = func(_ context.Context, registry, archivePath, sourceImage, target string) error {
		calls = append(calls, registry+"|"+filepath.Base(archivePath)+"|"+sourceImage+"|"+target)
		return nil
	}

	dir := t.TempDir()
	manifest, err := GeneratePushManifest([]string{
		"quay.io/example/api:v1",
		"busybox:1.36",
	})
	if err != nil {
		t.Fatalf("GeneratePushManifest() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PushImages(context.Background(), "registry.local:5000", dir); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("PushImages() calls = %d, want 2", len(calls))
	}
	if got, want := calls[0], "registry.local:5000|quay.io_example_api_v1.tar|quay.io/example/api:v1|example/api:v1"; got != want {
		t.Fatalf("PushImages()[0] = %q, want %q", got, want)
	}
}

func TestCopyImageToRegistrySupportsDigestReferences(t *testing.T) {
	originalLoad := loadTarballImage
	originalWrite := writeRemoteImage
	defer func() {
		loadTarballImage = originalLoad
		writeRemoteImage = originalWrite
	}()

	var gotSourceTag *name.Tag
	var gotDestRef name.Reference
	loadTarballImage = func(_ string, tag *name.Tag) (v1.Image, error) {
		gotSourceTag = tag
		return nil, nil
	}
	writeRemoteImage = func(ref name.Reference, _ v1.Image, _ ...remote.Option) error {
		gotDestRef = ref
		return nil
	}

	dir := t.TempDir()
	manifest, err := GeneratePushManifest([]string{
		"quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("GeneratePushManifest() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PushImages(context.Background(), "registry.local:5000", dir); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	if gotSourceTag != nil {
		t.Fatalf("loadTarballImage() tag = %v, want nil for digest reference", gotSourceTag)
	}
	if gotDestRef == nil || gotDestRef.String() != "registry.local:5000/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("destination reference = %v", gotDestRef)
	}
}
