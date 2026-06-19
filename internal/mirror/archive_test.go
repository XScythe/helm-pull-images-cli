package mirror

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestArchiveImagesCreatesDigestSpecs(t *testing.T) {
	originalFetch := fetchRemoteImage
	originalWriteLayout := writeLayout
	originalAppend := appendLayoutImage
	defer func() {
		fetchRemoteImage = originalFetch
		writeLayout = originalWriteLayout
		appendLayoutImage = originalAppend
	}()

	fetchRemoteImage = func(ref name.Reference, _ ...remote.Option) (v1.Image, error) {
		return fakeImageWithDigest(t, map[string]string{
			"quay.io/example/api:v1": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"busybox:1.36":           "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}[ref.String()]), nil
	}

	var layoutPath string
	writeLayout = func(path string, _ v1.ImageIndex) (layout.Path, error) {
		layoutPath = path
		return layout.Path(path), nil
	}

	var appended int
	appendLayoutImage = func(_ layout.Path, _ v1.Image) error {
		appended++
		return nil
	}

	specs, err := ArchiveImages(context.Background(), []string{"quay.io/example/api:v1", "busybox:1.36"}, t.TempDir(), 4)
	if err != nil {
		t.Fatalf("ArchiveImages() error = %v", err)
	}

	if filepath.Base(layoutPath) != OCILayoutDirName() {
		t.Fatalf("ArchiveImages() layout path = %q, want suffix %q", layoutPath, OCILayoutDirName())
	}
	if appended != 2 {
		t.Fatalf("ArchiveImages() appended = %d, want 2", appended)
	}
	if len(specs) != 2 {
		t.Fatalf("ArchiveImages() specs len = %d, want 2", len(specs))
	}
	if specs[0].OCIDigest == "" || specs[1].OCIDigest == "" {
		t.Fatalf("ArchiveImages() missing digests: %#v", specs)
	}
}

func TestArchiveImagesFailsOnCopyError(t *testing.T) {
	originalFetch := fetchRemoteImage
	originalWriteLayout := writeLayout
	defer func() {
		fetchRemoteImage = originalFetch
		writeLayout = originalWriteLayout
	}()

	writeLayout = func(path string, _ v1.ImageIndex) (layout.Path, error) {
		return layout.Path(path), nil
	}

	fetchRemoteImage = func(_ name.Reference, _ ...remote.Option) (v1.Image, error) {
		return nil, errors.New("boom")
	}

	_, err := ArchiveImages(context.Background(), []string{"busybox:1.36"}, t.TempDir(), 1)
	if err == nil {
		t.Fatal("ArchiveImages() error = nil, want error")
	}
}

func TestArchiveImagesSupportsDigestReferences(t *testing.T) {
	originalFetch := fetchRemoteImage
	originalWriteLayout := writeLayout
	originalAppend := appendLayoutImage
	defer func() {
		fetchRemoteImage = originalFetch
		writeLayout = originalWriteLayout
		appendLayoutImage = originalAppend
	}()

	var gotRef name.Reference
	fetchRemoteImage = func(ref name.Reference, _ ...remote.Option) (v1.Image, error) {
		gotRef = ref
		return fakeImageWithDigest(t, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), nil
	}
	writeLayout = func(path string, _ v1.ImageIndex) (layout.Path, error) {
		return layout.Path(path), nil
	}
	appendLayoutImage = func(_ layout.Path, _ v1.Image) error {
		return nil
	}

	specs, err := ArchiveImages(context.Background(), []string{"quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, t.TempDir(), 2)
	if err != nil {
		t.Fatalf("ArchiveImages() error = %v", err)
	}

	if gotRef == nil || gotRef.String() != "quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("fetchRemoteImage() ref = %v", gotRef)
	}
	if specs[0].OCIDigest == "" {
		t.Fatal("ArchiveImages() digest is empty")
	}
}

type fakeImage struct {
	v1.Image
	digest v1.Hash
}

func (f fakeImage) Digest() (v1.Hash, error) {
	return f.digest, nil
}

func fakeImageWithDigest(t *testing.T, digest string) v1.Image {
	t.Helper()
	hash, err := v1.NewHash(digest)
	if err != nil {
		t.Fatalf("NewHash(%q): %v", digest, err)
	}
	return fakeImage{digest: hash}
}
