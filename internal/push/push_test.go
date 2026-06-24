package push

import (
	"bytes"
	"context"
	"encoding/json"
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

	if err := PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: dir, Concurrency: 4, All: true}); err != nil {
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
	t.Chdir(baseDir)
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

	if err := PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: "", Concurrency: 2, All: true}); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	wantLayoutPath := filepath.Join(helperDir, pushspec.OCILayoutDirName())
	if writtenLayoutPath != wantLayoutPath {
		t.Fatalf("layout path = %q, want %q", writtenLayoutPath, wantLayoutPath)
	}
}

func TestPushImagesFallsBackToWorkingDirWhenExecutableDirHasNoManifest(t *testing.T) {
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
	workingDir := filepath.Join(baseDir, "work")
	helperDir := filepath.Join(baseDir, "helper")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() working dir error = %v", err)
	}
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() helper dir error = %v", err)
	}
	t.Chdir(workingDir)

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
	if err := os.WriteFile(filepath.Join(workingDir, pushspec.PushManifestFileName()), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: "", Concurrency: 2, All: true}); err != nil {
		t.Fatalf("PushImages() error = %v", err)
	}

	wantLayoutPath := filepath.Join(workingDir, pushspec.OCILayoutDirName())
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
	if err := PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: dir, Concurrency: 1, All: true}, status); err != nil {
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

func TestPushImagesInteractiveRequiresTerminal(t *testing.T) {
	original := copyImageToRegistry
	originalLoadLayout := loadOCILayout
	originalResolveExec := resolveExecutablePath
	defer func() {
		copyImageToRegistry = original
		loadOCILayout = originalLoadLayout
		resolveExecutablePath = originalResolveExec
	}()

	// Mock network operations so they don't run
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
	manifest, err := pushspec.GeneratePushManifest([]pushspec.ArchiveSpec{
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

	// Use strings.NewReader which is not a terminal
	err = PushImages(context.Background(), Options{
		Registry:    "registry.local:5000",
		InputDir:    dir,
		Concurrency: 1,
		All:         false,
		In:          strings.NewReader(""),
	})

	if err == nil {
		t.Fatalf("PushImages() error = nil, want non-nil error for non-terminal input")
	}

	if !strings.Contains(err.Error(), "requires terminal input and output") {
		t.Fatalf("PushImages() error = %v, want message containing 'requires terminal input and output'", err)
	}
}

func TestPushImagesRejectsManifestLayoutDirTraversal(t *testing.T) {
	originalResolveExec := resolveExecutablePath
	defer func() {
		resolveExecutablePath = originalResolveExec
	}()
	resolveExecutablePath = func() (string, error) {
		return "/unused/helper", nil
	}

	dir := t.TempDir()
	manifest := pushspec.PushManifest{
		LayoutDir: "../outside-layout",
		Images: []pushspec.ArchiveSpec{
			{
				Image:     "busybox:1.36",
				Target:    "library/busybox:1.36",
				OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pushspec.PushManifestFileName()), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: dir, Concurrency: 1, All: true})
	if err == nil {
		t.Fatal("PushImages() error = nil, want layoutDir traversal error")
	}
	if !strings.Contains(err.Error(), "layoutDir") {
		t.Fatalf("PushImages() error = %v", err)
	}
}

func TestPushImagesRejectsManifestLayoutDirAbsolutePath(t *testing.T) {
	originalResolveExec := resolveExecutablePath
	defer func() {
		resolveExecutablePath = originalResolveExec
	}()
	resolveExecutablePath = func() (string, error) {
		return "/unused/helper", nil
	}

	dir := t.TempDir()
	manifest := pushspec.PushManifest{
		LayoutDir: filepath.Join(string(filepath.Separator), "tmp", "layout"),
		Images: []pushspec.ArchiveSpec{
			{
				Image:     "busybox:1.36",
				Target:    "library/busybox:1.36",
				OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, pushspec.PushManifestFileName()), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err = PushImages(context.Background(), Options{Registry: "registry.local:5000", InputDir: dir, Concurrency: 1, All: true})
	if err == nil {
		t.Fatal("PushImages() error = nil, want absolute layoutDir error")
	}
	if !strings.Contains(err.Error(), "layoutDir") {
		t.Fatalf("PushImages() error = %v", err)
	}
}

func TestSelectedConflicts(t *testing.T) {
	selected := []pushspec.ArchiveSpec{
		{Image: "a:v1", Target: "a:v1", OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Image: "b:v1", Target: "b:v1", OCIDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}
	classified := []classifiedImage{
		{Spec: selected[0], Status: statusConflict, RemoteDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},
		{Spec: selected[1], Status: statusMirrored},
		{Spec: pushspec.ArchiveSpec{Image: "c:v1", Target: "c:v1", OCIDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}, Status: statusConflict},
	}

	got := selectedConflicts(selected, classified)
	if len(got) != 1 {
		t.Fatalf("selectedConflicts() len = %d, want 1", len(got))
	}
	if got[0].Spec.Target != "a:v1" {
		t.Fatalf("selectedConflicts()[0].Spec.Target = %q, want %q", got[0].Spec.Target, "a:v1")
	}
}

func TestConfirmConflictSelection(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		out := new(bytes.Buffer)
		confirmed, err := confirmConflictSelection(strings.NewReader("yes\n"), out, []classifiedImage{
			{
				Spec:         pushspec.ArchiveSpec{Target: "a:v1", OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				Status:       statusConflict,
				RemoteDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		})
		if err != nil {
			t.Fatalf("confirmConflictSelection() error = %v", err)
		}
		if !confirmed {
			t.Fatal("confirmConflictSelection() confirmed = false, want true")
		}
		if !strings.Contains(out.String(), "WARNING:") {
			t.Fatalf("warning output missing, got: %q", out.String())
		}
		if !strings.Contains(out.String(), "current=sha256:bbbb") || !strings.Contains(out.String(), "staged=sha256:aaaa") {
			t.Fatalf("digest details missing in warning output: %q", out.String())
		}
	})

	t.Run("no", func(t *testing.T) {
		out := new(bytes.Buffer)
		confirmed, err := confirmConflictSelection(strings.NewReader("no\n"), out, []classifiedImage{
			{Spec: pushspec.ArchiveSpec{Target: "a:v1", OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Status: statusConflict, RemoteDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		})
		if err != nil {
			t.Fatalf("confirmConflictSelection() error = %v", err)
		}
		if confirmed {
			t.Fatal("confirmConflictSelection() confirmed = true, want false")
		}
	})

	t.Run("invalid then yes", func(t *testing.T) {
		out := new(bytes.Buffer)
		confirmed, err := confirmConflictSelection(strings.NewReader("maybe\ny\n"), out, []classifiedImage{
			{Spec: pushspec.ArchiveSpec{Target: "a:v1", OCIDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Status: statusConflict, RemoteDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		})
		if err != nil {
			t.Fatalf("confirmConflictSelection() error = %v", err)
		}
		if !confirmed {
			t.Fatal("confirmConflictSelection() confirmed = false, want true")
		}
		if !strings.Contains(out.String(), "Please type yes or no.") {
			t.Fatalf("expected retry hint, got: %q", out.String())
		}
	})
}
