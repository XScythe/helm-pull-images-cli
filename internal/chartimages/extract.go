// Package chartimages extracts container image references from rendered Helm
// manifests and from chart-level image annotations.
//
// The package exposes two entrypoints, ExtractImages and
// ExtractChartAnnotationImages. Their supporting logic is split by concern:
//
//   - extract.go     public entrypoints and the annotation key constant
//   - manifest.go    YAML manifest traversal for image references
//   - annotations.go parsing of the chart image annotation payload
//   - args.go        image candidates embedded in container args/commands
//   - reference.go   image reference normalization and validation
package chartimages

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const chartImageAnnotationKey = "annotation.helm.sh/images"

func ExtractImages(manifest string) ([]string, error) {
	decoder := yaml.NewDecoder(bytes.NewBufferString(manifest))
	seen := make(map[string]struct{})
	var images []string

	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				return images, nil
			}
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		collectImages(&doc, seen, &images)
	}
}

func ExtractChartAnnotationImages(annotations map[string]string) ([]string, error) {
	if len(annotations) == 0 {
		return nil, nil
	}

	raw := strings.TrimSpace(annotations[chartImageAnnotationKey])
	if raw == "" {
		return nil, nil
	}

	images, err := parseImageAnnotation(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s annotation: %w", chartImageAnnotationKey, err)
	}
	return images, nil
}
