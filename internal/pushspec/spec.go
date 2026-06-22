package pushspec

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

func BuildSpecs(images []string) ([]ArchiveSpec, error) {
	specs := make([]ArchiveSpec, 0, len(images))
	for _, image := range images {
		target, err := mirrorReference(image)
		if err != nil {
			return nil, err
		}
		specs = append(specs, ArchiveSpec{
			Image:  image,
			Target: target,
		})
	}
	return specs, nil
}

func mirrorReference(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("parse image reference %q: %w", image, err)
	}

	switch ref := ref.(type) {
	case name.Tag:
		path := repositoryPath(ref.Context().Name())
		tag := ref.Identifier()
		if tag == "" {
			tag = "latest"
		}
		return fmt.Sprintf("%s:%s", path, tag), nil
	case name.Digest:
		path := repositoryPath(ref.Context().Name())
		return fmt.Sprintf("%s@%s", path, ref.Identifier()), nil
	default:
		return "", fmt.Errorf("unsupported image reference %q", image)
	}
}

func repositoryPath(image string) string {
	parts := strings.Split(image, "/")
	if len(parts) == 1 {
		return "library/" + image
	}
	if isRegistryHost(parts[0]) {
		parts = parts[1:]
	}
	if len(parts) == 1 {
		return "library/" + parts[0]
	}
	return strings.Join(parts, "/")
}

func isRegistryHost(part string) bool {
	return strings.Contains(part, ".") || strings.Contains(part, ":") || part == "localhost"
}
