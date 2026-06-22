package push

import "testing"

func TestNormalizeDisplayImageStripsDockerHubRegistry(t *testing.T) {
	tests := map[string]string{
		"docker.io/library/busybox:1.36":    "library/busybox:1.36",
		"index.docker.io/library/nginx:1.0": "library/nginx:1.0",
		"quay.io/example/app:v1":            "quay.io/example/app:v1",
	}

	for input, want := range tests {
		if got := normalizeDisplayImage(input); got != want {
			t.Fatalf("normalizeDisplayImage(%q) = %q, want %q", input, got, want)
		}
	}
}
