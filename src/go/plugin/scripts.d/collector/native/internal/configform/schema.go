// SPDX-License-Identifier: GPL-3.0-or-later

package configform

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// checkSchema enforces the supported schema subset before compilation.
func checkSchema(document map[string]any) error {
	positions := make(map[string]bool)
	var references []string
	err := walkSchema(document, "", true, func(value any, pointer string, defaults bool) error {
		positions[pointer] = true
		node, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		if dialect, exists := node["$schema"]; exists && dialect != draft7 {
			return errors.New("config schema dialects must be draft-07")
		}
		if _, exists := node["$id"]; exists {
			return errors.New("config schema IDs are not supported; use local references")
		}
		if ref, ok := node["$ref"].(string); ok {
			if ref != "#" && !strings.HasPrefix(ref, "#/") {
				return errors.New("config schema references must be local")
			}
			references = append(references, ref)
		}
		if value, exists := node["default"]; exists {
			if !defaults {
				return errors.New("config defaults must be on direct properties or homogeneous array items")
			}
			if hasSecretReference(value) {
				return errors.New("config defaults must not contain secret references; put references in job config")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// JSON Pointer refs can otherwise turn arbitrary annotation data into schemas,
	// bypassing both validation above and reference rebasing in the job form.
	for _, ref := range references {
		pointer, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
		if err != nil || !positions[pointer] {
			return errors.New("config schema references must target standard schema locations")
		}
	}
	return nil
}

// checkDefaults validates each authored default at its own schema location,
// after applying its child defaults. Required fields elsewhere in the package
// do not affect it.
func checkDefaults(node map[string]any, compiler *jsonschema.Compiler, resource, pointer string) error {
	if value, exists := node["default"]; exists {
		owned, err := Normalize(value)
		if err != nil {
			return errors.New("invalid config default")
		}
		schema, err := compiler.Compile(resource + "#" + pointer)
		if err != nil || schema.Validate(applyDefaults(owned, true, node)) != nil {
			return errors.New("config default does not satisfy its schema")
		}
	}
	properties, _ := node["properties"].(map[string]any)
	for name, value := range properties {
		if child, ok := value.(map[string]any); ok {
			key := url.PathEscape(escapePointerToken(name))
			if err := checkDefaults(child, compiler, resource, pointer+"/properties/"+key); err != nil {
				return err
			}
		}
	}
	if child, ok := node["items"].(map[string]any); ok {
		return checkDefaults(child, compiler, resource, pointer+"/items")
	}
	return nil
}

var (
	// Keywords whose values map names to subschemas.
	schemaMapKeywords = []string{"properties", "patternProperties", "definitions", "dependencies"}
	// Keywords whose values are a subschema or an array of subschemas.
	subschemaKeywords = []string{
		"items", "additionalItems", "additionalProperties", "contains", "propertyNames",
		"if", "then", "else", "not", "allOf", "anyOf", "oneOf",
	}
)

// walkSchema visits only Draft 7 schema positions, never arbitrary annotation
// or default data. defaults reports whether a default is allowed at the
// position: on direct properties and homogeneous array items only.
func walkSchema(
	value any,
	pointer string,
	defaults bool,
	visit func(value any, pointer string, defaults bool) error,
) error {
	node, object := value.(map[string]any)
	if _, boolean := value.(bool); !object && !boolean {
		return nil
	}
	if err := visit(value, pointer, defaults); err != nil {
		return err
	}
	for _, key := range schemaMapKeywords {
		children, _ := node[key].(map[string]any)
		for name, child := range children {
			path := pointer + "/" + key + "/" + escapePointerToken(name)
			if err := walkSchema(child, path, defaults && key == "properties", visit); err != nil {
				return err
			}
		}
	}
	for _, key := range subschemaKeywords {
		if children, ok := node[key].([]any); ok {
			for i, child := range children {
				if err := walkSchema(child, fmt.Sprintf("%s/%s/%d", pointer, key, i), false, visit); err != nil {
					return err
				}
			}
		} else if err := walkSchema(node[key], pointer+"/"+key, defaults && key == "items", visit); err != nil {
			return err
		}
	}
	return nil
}

// escapePointerToken escapes one JSON Pointer reference token (RFC 6901).
func escapePointerToken(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
}

func hasSecretReference(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, "${")
	case map[string]any:
		for _, child := range v {
			if hasSecretReference(child) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, hasSecretReference)
	}
	return false
}
