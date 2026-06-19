package mirror

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

type ArchiveSpec struct {
	Image     string `json:"image"`
	Target    string `json:"target"`
	OCIDigest string `json:"ociDigest"`
}

type PushManifest struct {
	LayoutDir string        `json:"layoutDir"`
	Images    []ArchiveSpec `json:"images"`
}

func GeneratePushManifest(specs []ArchiveSpec) (string, error) {
	for _, spec := range specs {
		if spec.OCIDigest == "" {
			return "", fmt.Errorf("missing oci digest for image %q", spec.Image)
		}
	}

	data, err := json.MarshalIndent(PushManifest{
		LayoutDir: OCILayoutDirName(),
		Images:    specs,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal push manifest: %w", err)
	}
	return string(data) + "\n", nil
}

func PushManifestFileName() string {
	return "push_images.json"
}

func WritePushManifest(outputDir string, specs []ArchiveSpec) error {
	manifest, err := GeneratePushManifest(specs)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(outputDir, PushManifestFileName())
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("write push manifest: %w", err)
	}

	return nil
}

func OCILayoutDirName() string {
	return "oci-layout"
}

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
