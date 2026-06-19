package chartimages

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
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

func collectArgumentImageCandidates(value, nextValue string, hasNext bool) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var candidates []string
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == ';'
	})
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if idx := strings.LastIndex(token, "="); idx >= 0 && idx+1 < len(token) {
			lhs := strings.ToLower(token[:idx])
			if strings.Contains(lhs, "image") {
				candidates = append(candidates, token[idx+1:])
			}
			continue
		}
		lowerToken := strings.ToLower(token)
		if (lowerToken == "--set" || lowerToken == "--set-string") && i+1 < len(tokens) {
			candidates = append(candidates, collectSetAssignmentCandidates(tokens[i+1])...)
			i++
			continue
		}
		if strings.HasPrefix(token, "--") && strings.Contains(lowerToken, "image") && i+1 < len(tokens) {
			candidates = appendImageFlagCandidate(candidates, token, tokens[i+1])
			i++
			continue
		}
	}

	if len(tokens) == 1 && hasNext {
		token := strings.TrimSpace(tokens[0])
		lowerToken := strings.ToLower(token)
		if lowerToken == "--set" || lowerToken == "--set-string" {
			candidates = append(candidates, collectSetAssignmentCandidates(nextValue)...)
		} else if strings.HasPrefix(token, "--") && strings.Contains(lowerToken, "image") {
			candidates = appendImageFlagCandidate(candidates, token, nextValue)
		}
	}

	return candidates
}

func collectSetAssignmentCandidates(value string) []string {
	var candidates []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		idx := strings.Index(item, "=")
		if idx <= 0 || idx+1 >= len(item) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item[:idx]))
		if !strings.Contains(key, "image") {
			continue
		}
		candidates = append(candidates, strings.TrimSpace(item[idx+1:]))
	}
	return candidates
}

func appendImageFlagCandidate(candidates []string, token, value string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return candidates
	}
	if strings.Contains(strings.ToLower(token), "image") {
		return append(candidates, strings.TrimSpace(value))
	}
	return candidates
}

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
