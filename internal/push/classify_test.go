package push

import (
	"context"
	"fmt"
	"helm-deep-pack/internal/pushspec"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func mustHash(t *testing.T, s string) v1.Hash {
	hash, err := v1.NewHash(s)
	if err != nil {
		t.Fatalf("v1.NewHash(%q) error = %v", s, err)
	}
	return hash
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "404 in message",
			err:  fmt.Errorf("some error with 404 in it"),
			want: true,
		},
		{
			name: "not found in message",
			err:  fmt.Errorf("manifest not found"),
			want: true,
		},
		{
			name: "manifest_unknown in message",
			err:  fmt.Errorf("manifest_unknown: error"),
			want: true,
		},
		{
			name: "connection refused error",
			err:  fmt.Errorf("dial tcp: connection refused"),
			want: false,
		},
		{
			name: "generic error",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFoundError(tt.err)
			if got != tt.want {
				t.Fatalf("isNotFoundError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestClassifyImagesMirrored(t *testing.T) {
	specDigest := "sha256:" + strings.Repeat("a", 64)
	headFn := func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
		// Return descriptor with matching digest
		return &v1.Descriptor{
			Digest: mustHash(t, specDigest),
		}, nil
	}

	specs := []pushspec.ArchiveSpec{
		{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: specDigest,
		},
	}

	result := classifyImagesWithHead(context.Background(), "registry.local:5000", specs, headFn)

	if len(result) != 1 {
		t.Fatalf("classifyImages() returned %d images, want 1", len(result))
	}

	if result[0].Status != statusMirrored {
		t.Fatalf("Status = %v, want %v", result[0].Status, statusMirrored)
	}

	if result[0].RemoteDigest != specDigest {
		t.Fatalf("RemoteDigest = %q, want %q", result[0].RemoteDigest, specDigest)
	}

	if result[0].ProbeErr != nil {
		t.Fatalf("ProbeErr = %v, want nil", result[0].ProbeErr)
	}
}

func TestClassifyImagesConflict(t *testing.T) {
	specDigest := "sha256:" + strings.Repeat("a", 64)
	remoteDigest := "sha256:" + strings.Repeat("b", 64)
	headFn := func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
		// Return descriptor with different digest
		return &v1.Descriptor{
			Digest: mustHash(t, remoteDigest),
		}, nil
	}

	specs := []pushspec.ArchiveSpec{
		{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: specDigest,
		},
	}

	result := classifyImagesWithHead(context.Background(), "registry.local:5000", specs, headFn)

	if len(result) != 1 {
		t.Fatalf("classifyImages() returned %d images, want 1", len(result))
	}

	if result[0].Status != statusConflict {
		t.Fatalf("Status = %v, want %v", result[0].Status, statusConflict)
	}

	if result[0].RemoteDigest != remoteDigest {
		t.Fatalf("RemoteDigest = %q, want %q", result[0].RemoteDigest, remoteDigest)
	}
}

func TestClassifyImagesPushable(t *testing.T) {
	specDigest := "sha256:" + strings.Repeat("a", 64)
	headFn := func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
		// Return not found error
		return nil, fmt.Errorf("manifest unknown: manifest_unknown")
	}

	specs := []pushspec.ArchiveSpec{
		{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: specDigest,
		},
	}

	result := classifyImagesWithHead(context.Background(), "registry.local:5000", specs, headFn)

	if len(result) != 1 {
		t.Fatalf("classifyImages() returned %d images, want 1", len(result))
	}

	if result[0].Status != statusPushable {
		t.Fatalf("Status = %v, want %v", result[0].Status, statusPushable)
	}

	if result[0].RemoteDigest != "" {
		t.Fatalf("RemoteDigest = %q, want empty", result[0].RemoteDigest)
	}

	if result[0].ProbeErr != nil {
		t.Fatalf("ProbeErr = %v, want nil", result[0].ProbeErr)
	}
}

func TestClassifyImagesUnknown(t *testing.T) {
	specDigest := "sha256:" + strings.Repeat("a", 64)
	headFn := func(ref name.Reference, opts ...remote.Option) (*v1.Descriptor, error) {
		// Return a generic error (not a not-found error)
		return nil, fmt.Errorf("dial tcp: connection refused")
	}

	specs := []pushspec.ArchiveSpec{
		{
			Image:     "quay.io/example/api:v1",
			Target:    "example/api:v1",
			OCIDigest: specDigest,
		},
	}

	result := classifyImagesWithHead(context.Background(), "registry.local:5000", specs, headFn)

	if len(result) != 1 {
		t.Fatalf("classifyImages() returned %d images, want 1", len(result))
	}

	if result[0].Status != statusUnknown {
		t.Fatalf("Status = %v, want %v", result[0].Status, statusUnknown)
	}

	if result[0].RemoteDigest != "" {
		t.Fatalf("RemoteDigest = %q, want empty", result[0].RemoteDigest)
	}

	if result[0].ProbeErr == nil {
		t.Fatalf("ProbeErr = nil, want non-nil error")
	}
}
