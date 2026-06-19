package chartimages

import "testing"

func TestExtractImages(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: quay.io/example/api:v1
        - name: sidecar
          image: busybox:1.36
      initContainers:
        - name: init
          image: busybox:1.36
---
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
spec:
  template:
    spec:
      containers:
        - name: migrate
          image: quay.io/example/migrate:v2
`

	got, err := ExtractImages(manifest)
	if err != nil {
		t.Fatalf("ExtractImages() error = %v", err)
	}

	want := []string{
		"quay.io/example/api:v1",
		"busybox:1.36",
		"quay.io/example/migrate:v2",
	}

	if len(got) != len(want) {
		t.Fatalf("ExtractImages() len = %d, want %d (%v)", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractImages()[%d] = %q, want %q (full = %v)", i, got[i], want[i], got)
		}
	}
}
