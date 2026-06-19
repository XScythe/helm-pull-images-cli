package mirror

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func TestArchiveImagesUsesArchiveFilenames(t *testing.T) {
	original := copyImageToArchive
	defer func() { copyImageToArchive = original }()

	var calls []string
	copyImageToArchive = func(_ context.Context, image, archivePath string) error {
		calls = append(calls, image+"=>"+archivePath)
		return nil
	}

	archives, err := ArchiveImages(context.Background(), []string{
		"quay.io/example/api:v1",
		"busybox:1.36",
	}, t.TempDir())
	if err != nil {
		t.Fatalf("ArchiveImages() error = %v", err)
	}

	if len(archives) != 2 {
		t.Fatalf("ArchiveImages() len = %d, want 2", len(archives))
	}

	if got, want := filepath.Base(archives[0]), "quay.io_example_api_v1.tar"; got != want {
		t.Fatalf("ArchiveImages()[0] = %q, want %q", got, want)
	}
	if got, want := filepath.Base(archives[1]), "busybox_1.36.tar"; got != want {
		t.Fatalf("ArchiveImages()[1] = %q, want %q", got, want)
	}

	if len(calls) != 2 {
		t.Fatalf("copy calls = %d, want 2", len(calls))
	}
}

func TestCopyImageToArchiveRemovesPartialFileOnWriteError(t *testing.T) {
	originalFetch := fetchRemoteImage
	originalWrite := writeTarball
	defer func() {
		fetchRemoteImage = originalFetch
		writeTarball = originalWrite
	}()

	fetchRemoteImage = func(_ name.Reference, _ ...remote.Option) (v1.Image, error) {
		return nil, nil
	}
	writeTarball = func(_ name.Reference, _ v1.Image, _ io.Writer, _ ...tarball.WriteOption) error {
		return errors.New("boom")
	}

	archivePath := filepath.Join(t.TempDir(), "image.tar")
	err := copyImageToArchiveUsingGoContainerRegistry(context.Background(), "busybox:1.36", archivePath)
	if err == nil {
		t.Fatal("copyImageToArchiveUsingGoContainerRegistry() error = nil, want error")
	}

	if _, statErr := os.Stat(archivePath); !os.IsNotExist(statErr) {
		t.Fatalf("archive path still exists after failure: %v", statErr)
	}
}

func TestCopyImageToArchiveSupportsDigestReferences(t *testing.T) {
	originalFetch := fetchRemoteImage
	originalWrite := writeTarball
	defer func() {
		fetchRemoteImage = originalFetch
		writeTarball = originalWrite
	}()

	var gotRef name.Reference
	fetchRemoteImage = func(ref name.Reference, _ ...remote.Option) (v1.Image, error) {
		gotRef = ref
		return nil, nil
	}
	writeTarball = func(_ name.Reference, _ v1.Image, _ io.Writer, _ ...tarball.WriteOption) error {
		return nil
	}

	archivePath := filepath.Join(t.TempDir(), "image.tar")
	if err := copyImageToArchiveUsingGoContainerRegistry(context.Background(), "quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", archivePath); err != nil {
		t.Fatalf("copyImageToArchiveUsingGoContainerRegistry() error = %v", err)
	}

	if gotRef == nil || gotRef.String() != "quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("fetchRemoteImage() ref = %v", gotRef)
	}
}
