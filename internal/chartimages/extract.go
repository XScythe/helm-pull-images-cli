package chartimages

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

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

func collectImages(node *yaml.Node, seen map[string]struct{}, images *[]string) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectImages(child, seen, images)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind == yaml.ScalarNode && key.Value == "image" && value.Kind == yaml.ScalarNode {
				if _, ok := seen[value.Value]; !ok {
					seen[value.Value] = struct{}{}
					*images = append(*images, value.Value)
				}
			}
			collectImages(value, seen, images)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			collectImages(child, seen, images)
		}
	}
}
