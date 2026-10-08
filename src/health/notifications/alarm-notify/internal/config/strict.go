// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

var nodeType = reflect.TypeFor[yaml.Node]()
var unmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

type nodeSchema struct {
	node   *yaml.Node
	schema reflect.Type
}

// Decode on the original graph preserves aliases across destination boundaries.
// Node.Decode checks types, duplicate keys and alias limits, but not known fields.
func decodeStrict(node *yaml.Node, target any) error {
	if err := node.Decode(target); err != nil {
		return err
	}
	return checkFields(node, reflect.TypeOf(target), make(map[nodeSchema]bool))
}

func checkFields(node *yaml.Node, schema reflect.Type, seen map[nodeSchema]bool) error {
	for schema.Kind() == reflect.Pointer {
		schema = schema.Elem()
	}
	key := nodeSchema{node, schema}
	if seen[key] {
		return nil
	}
	seen[key] = true
	// Explicit unmarshallers own their field validation. Node is a deferred subtree.
	if schema == nodeType || reflect.PointerTo(schema).Implements(unmarshalerType) {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 0 {
			return checkFields(node.Content[0], schema, seen)
		}
		return nil
	case yaml.AliasNode:
		return checkFields(node.Alias, schema, seen)
	}
	if node.Tag == "!!null" {
		return nil
	}
	switch schema.Kind() {
	case reflect.Struct:
		fields := make(map[string]reflect.Type)
		collectFields(schema, fields)
		for i := 0; i+1 < len(node.Content); i += 2 {
			name, value := node.Content[i], node.Content[i+1]
			if isMergeKey(name) {
				if err := checkMerge(value, schema, seen); err != nil {
					return err
				}
				continue
			}
			var fieldName string
			if err := name.Decode(&fieldName); err != nil {
				return err
			}
			child, ok := fields[fieldName]
			if !ok {
				return errors.New("unknown configuration field")
			}
			if err := checkFields(value, child, seen); err != nil {
				return err
			}
		}
	case reflect.Map:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if isMergeKey(node.Content[i]) {
				if err := checkMerge(node.Content[i+1], schema, seen); err != nil {
					return err
				}
			} else if err := checkFields(node.Content[i+1], schema.Elem(), seen); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for _, child := range node.Content {
			if err := checkFields(child, schema.Elem(), seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkMerge(node *yaml.Node, schema reflect.Type, seen map[nodeSchema]bool) error {
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if err := checkFields(child, schema, seen); err != nil {
				return err
			}
		}
		return nil
	}
	return checkFields(node, schema, seen)
}

func collectFields(schema reflect.Type, fields map[string]reflect.Type) {
	for i := 0; i < schema.NumField(); i++ {
		field := schema.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")
		if tag[0] == "-" {
			continue
		}
		inline := false
		for _, option := range tag[1:] {
			if option == "inline" {
				inline = true
			}
		}
		if inline {
			nested := field.Type
			for nested.Kind() == reflect.Pointer {
				nested = nested.Elem()
			}
			collectFields(nested, fields)
			continue
		}
		name := tag[0]
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		fields[name] = field.Type
	}
}

// Match yaml.v3's merge recognition: an explicit merge tag alone is not a merge.
func isMergeKey(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Value == "<<" &&
		(node.Tag == "" || node.Tag == "!" || node.ShortTag() == "!!merge")
}
