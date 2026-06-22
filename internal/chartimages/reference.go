package chartimages

import (
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

func addParsedCandidates(images *[]string, seen map[string]struct{}, candidates []string, allowBare bool) {
	for _, candidate := range uniqueValidImageReferences(candidates, allowBare) {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		*images = append(*images, candidate)
	}
}

func uniqueValidImageReferences(values []string, allowBare bool) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'()[]{},`)
		if value == "" || strings.HasPrefix(value, "-") {
			continue
		}
		if !allowBare && !strings.ContainsAny(value, "/:@") {
			continue
		}
		if _, err := name.ParseReference(value, name.WeakValidation); err != nil {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
