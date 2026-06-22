package chartimages

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseImageAnnotation(raw string) ([]string, error) {
	var stringList []string
	if err := yaml.Unmarshal([]byte(raw), &stringList); err == nil && len(stringList) > 0 {
		return uniqueValidImageReferences(stringList, true), nil
	}

	var objectList []struct {
		Image string `yaml:"image"`
	}
	if err := yaml.Unmarshal([]byte(raw), &objectList); err == nil && len(objectList) > 0 {
		images := make([]string, 0, len(objectList))
		for _, item := range objectList {
			if item.Image != "" {
				images = append(images, item.Image)
			}
		}
		if len(images) > 0 {
			return uniqueValidImageReferences(images, true), nil
		}
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	if len(parts) == 0 {
		return nil, fmt.Errorf("unsupported annotation format")
	}

	valid := uniqueValidImageReferences(parts, false)
	if len(valid) == 0 {
		return nil, fmt.Errorf("unsupported annotation format")
	}
	return valid, nil
}
