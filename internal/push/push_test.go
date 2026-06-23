package push

import (
	"bytes"
	"context"
	"fmt"
	"helm-deep-pack/internal/pushspec"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestPushImagesUsesManifestDigests(t *testing.T) {
	original := copyImageToRegistry
	originalLoadLayout := loadOCILayout
	originalResolveExec := resolveExecutablePath
	defer func() {
		copyImageToRegistry = original
		loadOCILayout = originalLoadLayout
		resolveExecutablePath = originalResolveExec
	}()

	var calls []string
	copyImageToRegistry = func(_ context.Context, registry string, _ layout.Path, sourceImage, target, ociDigest string) error {
		calls = append(calls, registry+"|"+sourceImage+"|"+target+"|"+ociDigest)
		return nil
	}
	loadOCILayout = func(path string) (layout.Path, error) {
		return layout.Path(path), nil
	}
	resolveExecutablePath = func() (string, error) {
		return "/unused/helper", nil
	}

	dir := t.TempDir()
	manifest, err := pushspec.GeneratePushManifest([]pushspec.ArchiveSpec{
		{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			Image:     "busybox:1.36",
			Target:    "library/busybox:1.36",
			OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if err != nil {
		t.Fatalf("pushspec.GeneratePushManifest() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, pushspec.PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PushImages(context.Background(), "registry.local:5000", dir, 4); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("PushImages() calls = %d, want 2", len(calls))
	}
	want := map[string]struct{}{
		"registry.local:5000|quay.io/example/api:v1|example/api:v1|sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {},
		"registry.local:5000|busybox:1.36|library/busybox:1.36|sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb":     {},
	}
	for _, call := range calls {
		if _, ok := want[call]; !ok {
			t.Fatalf("PushImages() unexpected call = %q", call)
		}
		delete(want, call)
	}
	if len(want) != 0 {
		t.Fatalf("PushImages() missing calls: %v", want)
	}
}

func TestPushImagesResolvesDefaultInputDir(t *testing.T) {
	original := copyImageToRegistry
	originalLoadLayout := loadOCILayout
	originalResolveExec := resolveExecutablePath
	defer func() {
		copyImageToRegistry = original
		loadOCILayout = originalLoadLayout
		resolveExecutablePath = originalResolveExec
	}()

	copyImageToRegistry = func(_ context.Context, _ string, _ layout.Path, _, _, _ string) error {
		return nil
	}

	baseDir := t.TempDir()
	helperDir := filepath.Join(baseDir, "helper")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	resolveExecutablePath = func() (string, error) {
		return filepath.Join(helperDir, "push_images"), nil
	}

	writtenLayoutPath := ""
	loadOCILayout = func(path string) (layout.Path, error) {
		writtenLayoutPath = path
		return layout.Path(path), nil
	}

	manifest, err := pushspec.GeneratePushManifest([]pushspec.ArchiveSpec{{
		Image:     "busybox:1.36",
		Target:    "library/busybox:1.36",
		OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}})
	if err != nil {
		t.Fatalf("pushspec.GeneratePushManifest() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, pushspec.PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PushImages(context.Background(), "registry.local:5000", "", 2); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	wantLayoutPath := filepath.Join(helperDir, pushspec.OCILayoutDirName())
	if writtenLayoutPath != wantLayoutPath {
		t.Fatalf("layout path = %q, want %q", writtenLayoutPath, wantLayoutPath)
	}
}

func TestCopyImageToRegistrySupportsDigestReferences(t *testing.T) {
	originalLoad := loadLayoutImage
	originalWrite := writeRemoteImage
	defer func() {
		loadLayoutImage = originalLoad
		writeRemoteImage = originalWrite
	}()

	var gotHash v1.Hash
	var gotDestRef name.Reference
	loadLayoutImage = func(_ layout.Path, hash v1.Hash) (v1.Image, error) {
		gotHash = hash
		return nil, nil
	}

	writeRemoteImage = func(ref name.Reference, _ v1.Image, _ ...remote.Option) error {
		gotDestRef = ref
		return nil
	}

	if err := copyImageToRegistryUsingGoContainerRegistry(
		context.Background(),
		"registry.local:5000",
		layout.Path("/tmp/layout"),
		"quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	); err != nil {
		t.Fatalf("copyImageToRegistryUsingGoContainerRegistry() error = %v", err)
	}

	if gotHash.String() != "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("layout hash = %v", gotHash)
	}
	if gotDestRef == nil || gotDestRef.String() != "registry.local:5000/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("destination reference = %v", gotDestRef)
	}
}

func TestCopyImageToRegistryReportsWebsiteLikeRegistryErrors(t *testing.T) {
	originalLoad := loadLayoutImage
	originalWrite := writeRemoteImage
	defer func() {
		loadLayoutImage = originalLoad
		writeRemoteImage = originalWrite
	}()

	loadLayoutImage = func(_ layout.Path, _ v1.Hash) (v1.Image, error) {
		return nil, nil
	}
	writeRemoteImage = func(_ name.Reference, _ v1.Image, _ ...remote.Option) error {
		return fmt.Errorf(`unexpected media type "text/html"`)
	}

	err := copyImageToRegistryUsingGoContainerRegistry(
		context.Background(),
		"example.com",
		layout.Path("/tmp/layout"),
		"quay.io/example/api:v1",
		"example/api:v1",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	if err == nil {
		t.Fatal("copyImageToRegistryUsingGoContainerRegistry() error = nil, want website error")
	}
	if !strings.Contains(err.Error(), "does not look like a container registry") {
		t.Fatalf("error = %v", err)
	}
}

func TestCopyImageToRegistryPreservesRegistry404Errors(t *testing.T) {
	originalLoad := loadLayoutImage
	originalWrite := writeRemoteImage
	defer func() {
		loadLayoutImage = originalLoad
		writeRemoteImage = originalWrite
	}()

	loadLayoutImage = func(_ layout.Path, _ v1.Hash) (v1.Image, error) {
		return nil, nil
	}
	writeRemoteImage = func(_ name.Reference, _ v1.Image, _ ...remote.Option) error {
		return fmt.Errorf("unexpected status code 404 Not Found")
	}

	err := copyImageToRegistryUsingGoContainerRegistry(
		context.Background(),
		"registry.local:5000",
		layout.Path("/tmp/layout"),
		"quay.io/example/api:v1",
		"example/api:v1",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	if err == nil {
		t.Fatal("copyImageToRegistryUsingGoContainerRegistry() error = nil, want registry 404 error")
	}
	if strings.Contains(err.Error(), "does not look like a container registry") {
		t.Fatalf("registry 404 was misclassified as website: %v", err)
	}
	if !strings.Contains(err.Error(), "push image") {
		t.Fatalf("expected wrapped push error, got: %v", err)
	}
}

func TestPushImagesReportsProgress(t *testing.T) {
	original := copyImageToRegistry
	originalLoadLayout := loadOCILayout
	originalResolveExec := resolveExecutablePath
	defer func() {
		copyImageToRegistry = original
		loadOCILayout = originalLoadLayout
		resolveExecutablePath = originalResolveExec
	}()

	copyImageToRegistry = func(_ context.Context, _ string, _ layout.Path, _, _, _ string) error {
		return nil
	}
	loadOCILayout = func(path string) (layout.Path, error) {
		return layout.Path(path), nil
	}
	resolveExecutablePath = func() (string, error) {
		return "/unused/helper", nil
	}

	dir := t.TempDir()
	manifest, err := pushspec.GeneratePushManifest([]pushspec.ArchiveSpec{{
		Image:     "busybox:1.36",
		Target:    "library/busybox:1.36",
		OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}})
	if err != nil {
		t.Fatalf("pushspec.GeneratePushManifest() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pushspec.PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	status := new(bytes.Buffer)
	if err := PushImages(context.Background(), "registry.local:5000", dir, 1, status); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	got := status.String()
	if strings.Contains(got, "+1 more") {
		t.Fatalf("PushImages() status unexpectedly summarized images: %q", got)
	}
	if strings.Contains(got, "\x1b[") || strings.Contains(got, "\r") {
		t.Fatalf("PushImages() status unexpectedly used terminal control codes: %q", got)
	}
	if !strings.Contains(got, "pushing 1/1: busybox:1.36") {
		t.Fatalf("PushImages() status = %q", got)
	}
}
