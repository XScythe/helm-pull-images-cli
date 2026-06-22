package chartimages

import (
	"strings"

	"gopkg.in/yaml.v3"
)

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
		containsImageNameHint := mappingContainsImageNameHint(node)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				collectImages(value, seen, images)
				continue
			}

			keyValue := strings.ToLower(key.Value)
			switch {
			case value.Kind == yaml.ScalarNode && keyValue == "image":
				addParsedCandidates(images, seen, []string{value.Value}, true)
			case value.Kind == yaml.ScalarNode && strings.Contains(keyValue, "image"):
				addParsedCandidates(images, seen, []string{value.Value}, true)
			case value.Kind == yaml.ScalarNode && keyValue == "value" && containsImageNameHint:
				addParsedCandidates(images, seen, []string{value.Value}, true)
			case value.Kind == yaml.SequenceNode && (keyValue == "args" || keyValue == "command"):
				for i, child := range value.Content {
					if child.Kind != yaml.ScalarNode {
						collectImages(child, seen, images)
						continue
					}
					nextValue := ""
					hasNext := false
					if i+1 < len(value.Content) && value.Content[i+1].Kind == yaml.ScalarNode {
						nextValue = value.Content[i+1].Value
						hasNext = true
					}
					addParsedCandidates(images, seen, collectArgumentImageCandidates(child.Value, nextValue, hasNext), true)
				}
			default:
				collectImages(value, seen, images)
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			collectImages(child, seen, images)
		}
	}
}

func mappingContainsImageNameHint(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode {
			continue
		}
		if strings.ToLower(key.Value) != "name" {
			continue
		}
		if strings.Contains(strings.ToLower(value.Value), "image") {
			return true
		}
	}
	return false
}
