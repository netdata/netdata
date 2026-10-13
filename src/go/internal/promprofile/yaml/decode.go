// SPDX-License-Identifier: GPL-3.0-or-later

// Package promyaml provides strict YAML decoding for Prometheus profile proof contracts.
package promyaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// DecodeFile decodes one strict YAML mapping document from path.
func DecodeFile[T any](path string, requiredTopLevelKeys ...string) (T, error) {
	var zero T
	content, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read %q: %w", path, err)
	}
	return Decode[T](path, content, requiredTopLevelKeys...)
}

// Decode decodes one strict YAML mapping document.
func Decode[T any](name string, content []byte, requiredTopLevelKeys ...string) (T, error) {
	var value T
	root, err := decodeNode(name, content)
	if err != nil {
		return value, err
	}
	if err := validateNode(root); err != nil {
		return value, fmt.Errorf("decode %q: %w", name, err)
	}
	if err := requireTopLevelKeys(root, requiredTopLevelKeys); err != nil {
		return value, fmt.Errorf("decode %q: %w", name, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode %q: %w", name, err)
	}
	return value, nil
}

// DecodeNode decodes one strict embedded YAML mapping node.
func DecodeNode[T any](name string, node *yaml.Node, requiredKeys ...string) (T, error) {
	var zero T
	if node == nil {
		return zero, fmt.Errorf("decode %q: node is nil", name)
	}
	var content bytes.Buffer
	encoder := yaml.NewEncoder(&content)
	if err := encoder.Encode(node); err != nil {
		return zero, fmt.Errorf("encode %q: %w", name, err)
	}
	if err := encoder.Close(); err != nil {
		return zero, fmt.Errorf("encode %q: %w", name, err)
	}
	return Decode[T](name, content.Bytes(), requiredKeys...)
}

func decodeNode(name string, content []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode %q: %w", name, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("decode %q: document must be a YAML mapping", name)
	}

	var trailing yaml.Node
	err := decoder.Decode(&trailing)
	switch {
	case err == nil:
		return nil, fmt.Errorf("decode %q: must contain exactly one YAML document", name)
	case !errors.Is(err, io.EOF):
		return nil, fmt.Errorf("decode trailing content in %q: %w", name, err)
	}
	return document.Content[0], nil
}

func validateNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Value == "<<" {
		return fmt.Errorf("YAML anchors, aliases, and merge keys are not allowed (line %d)", node.Line)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return fmt.Errorf("YAML null values are not allowed (line %d)", node.Line)
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]int, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode {
				return fmt.Errorf("mapping key at line %d must be a scalar", key.Line)
			}
			if previous := seen[key.Value]; previous != 0 {
				return fmt.Errorf("duplicate mapping key %q at line %d (previously line %d)", key.Value, key.Line, previous)
			}
			seen[key.Value] = key.Line
		}
	}
	for _, child := range node.Content {
		if err := validateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func requireTopLevelKeys(root *yaml.Node, required []string) error {
	present := make(map[string]struct{}, len(root.Content)/2)
	for index := 0; index < len(root.Content); index += 2 {
		present[root.Content[index].Value] = struct{}{}
	}
	for _, key := range required {
		if _, ok := present[key]; !ok {
			return fmt.Errorf("required field %s is missing", key)
		}
	}
	return nil
}
