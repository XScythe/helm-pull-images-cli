package chartimages

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkExtractImages(b *testing.B) {
	var manifest strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&manifest, `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-%d
spec:
  template:
    spec:
      containers:
        - name: app
          image: quay.io/example/app:%d
          args:
            - --executor-image=quay.io/example/executor:%d
          env:
            - name: DEFAULT_IMAGE
              value: quay.io/example/sidecar:%d
---
`, i, i, i, i)
	}
	data := manifest.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ExtractImages(data); err != nil {
			b.Fatalf("ExtractImages() error = %v", err)
		}
	}
}
