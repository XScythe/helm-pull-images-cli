package mirror

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeneratePushManifest(t *testing.T) {
	got, err := GeneratePushManifest([]string{
		"quay.io/example/api:v1",
		"busybox:1.36",
		"quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("GeneratePushManifest() error = %v", err)
	}

	if !strings.Contains(got, `"archive": "quay.io_example_api_v1.tar"`) {
		t.Fatalf("GeneratePushManifest() missing archive entry in:\n%s", got)
	}
	if !strings.Contains(got, `"target": "example/api:v1"`) {
		t.Fatalf("GeneratePushManifest() missing target entry in:\n%s", got)
	}
	if !strings.Contains(got, `"target": "example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("GeneratePushManifest() missing digest target entry in:\n%s", got)
	}

	var manifest PushManifest
	if err := json.Unmarshal([]byte(got), &manifest); err != nil {
		t.Fatalf("GeneratePushManifest() returned invalid JSON: %v", err)
	}

	if len(manifest.Images) != 3 {
		t.Fatalf("GeneratePushManifest() images len = %d, want 3", len(manifest.Images))
	}
	if manifest.Images[0].Archive != "quay.io_example_api_v1.tar" {
		t.Fatalf("GeneratePushManifest() archive = %q, want %q", manifest.Images[0].Archive, "quay.io_example_api_v1.tar")
	}
	if manifest.Images[2].Target != "example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("GeneratePushManifest() digest target = %q, want digest target", manifest.Images[2].Target)
	}
}

func TestBuildSpecsSupportsDigestReferences(t *testing.T) {
	specs, err := BuildSpecs([]string{
		"quay.io/example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("BuildSpecs() error = %v", err)
	}

	if len(specs) != 1 {
		t.Fatalf("BuildSpecs() len = %d, want 1", len(specs))
	}
	if specs[0].Archive != "quay.io_example_api_sha256_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.tar" {
		t.Fatalf("BuildSpecs() archive = %q", specs[0].Archive)
	}
	if specs[0].Target != "example/api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("BuildSpecs() target = %q", specs[0].Target)
	}
}
