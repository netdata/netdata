// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v2"
)

// Settings owns JSON-shaped values, including nested objects decoded from YAML.
// Decode errors deliberately omit values, which may contain credentials.
type Settings map[string]any

func (s *Settings) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return fmt.Errorf("config must be a JSON-compatible object")
	}
	value, err := jsonValue(value)
	if err != nil {
		return err
	}
	if value == nil {
		*s = nil
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("config must be an object")
	}
	*s = object
	return nil
}

func jsonValue(value any) (any, error) {
	switch v := value.(type) {
	case map[any]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("config object keys must be strings")
			}
			normalized, err := jsonValue(item)
			if err != nil {
				return nil, err
			}
			result[name] = normalized
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			normalized, err := jsonValue(item)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			normalized, err := jsonValue(item)
			if err != nil {
				return nil, err
			}
			result[i] = normalized
		}
		return result, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("config values must be JSON-compatible")
		}
		var result any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("config values must be JSON-compatible")
		}
		return result, nil
	}
}

type packageConfig struct {
	schema   *jsonschema.Schema
	document map[string]any
	ui       map[string]any
}

func loadPackageConfig(dir, name string) (*packageConfig, error) {
	if name == "" {
		return nil, nil
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(dir, name)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read package config schema: %w", err)
	}
	// The same strict JSON token checks used for protocol objects reject duplicate keys.
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	if !utf8.Valid(data) || validateJSONFields(tokens, "schema") != nil {
		return nil, fmt.Errorf("invalid package config schema JSON")
	}
	if _, err := tokens.Token(); err != io.EOF {
		return nil, fmt.Errorf("config schema must contain one JSON object")
	}
	var form struct {
		Schema map[string]any `json:"jsonSchema"`
		UI     map[string]any `json:"uiSchema"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&form) != nil || form.Schema == nil || form.UI == nil {
		return nil, fmt.Errorf("config schema requires jsonSchema and uiSchema objects")
	}
	if form.Schema["$schema"] != "http://json-schema.org/draft-07/schema#" || form.Schema["type"] != "object" {
		return nil, fmt.Errorf("config schema must be a draft-07 object")
	}
	positions := make(map[string]bool)
	var references []string
	if err := walkSchema(form.Schema, "", true, func(value any, pointer string, defaults bool) error {
		positions[pointer] = true
		node, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		if dialect, exists := node["$schema"]; exists && dialect != "http://json-schema.org/draft-07/schema#" {
			return fmt.Errorf("config schema dialects must be draft-07")
		}
		if _, exists := node["$id"]; exists {
			return fmt.Errorf("config schema IDs are not supported; use local references")
		}
		if ref, ok := node["$ref"].(string); ok {
			if ref != "#" && !strings.HasPrefix(ref, "#/") {
				return fmt.Errorf("config schema references must be local")
			}
			references = append(references, ref)
		}
		if value, exists := node["default"]; exists {
			if !defaults {
				return fmt.Errorf("config defaults must be on direct properties or homogeneous array items")
			}
			if referenceDefault(value) {
				return fmt.Errorf("config defaults must not contain secret references; put references in job config")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// JSON Pointer refs can otherwise turn arbitrary annotation data into schemas,
	// bypassing both validation above and reference rebasing in the job form.
	for _, ref := range references {
		pointer, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
		if err != nil || !positions[pointer] {
			return nil, fmt.Errorf("config schema references must target standard schema locations")
		}
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil) // Only this document and the compiler's built-in metaschemas.
	compiler.DefaultDraft(jsonschema.Draft7)
	const resource = "urn:netdata:native:config"
	if compiler.AddResource(resource, form.Schema) != nil {
		return nil, fmt.Errorf("invalid package config schema")
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid package config schema")
	}
	if err := validateConfigDefaults(form.Schema, compiler, resource, ""); err != nil {
		return nil, err
	}
	return &packageConfig{
		schema:   compiled,
		document: form.Schema,
		ui:       form.UI,
	}, nil
}

// Validate each authored default at its own schema location, after applying its
// child defaults. Required fields elsewhere in the package do not affect it.
func validateConfigDefaults(node map[string]any, compiler *jsonschema.Compiler, resource, pointer string) error {
	if value, exists := node["default"]; exists {
		owned, err := jsonValue(value)
		if err != nil {
			return fmt.Errorf("invalid config default")
		}
		schema, err := compiler.Compile(resource + "#" + pointer)
		if err != nil || schema.Validate(applyConfigDefaults(owned, true, node)) != nil {
			return fmt.Errorf("config default does not satisfy its schema")
		}
	}
	properties, _ := node["properties"].(map[string]any)
	for name, value := range properties {
		if child, ok := value.(map[string]any); ok {
			key := url.PathEscape(strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1"))
			if err := validateConfigDefaults(child, compiler, resource, pointer+"/properties/"+key); err != nil {
				return err
			}
		}
	}
	if child, ok := node["items"].(map[string]any); ok {
		return validateConfigDefaults(child, compiler, resource, pointer+"/items")
	}
	return nil
}

// Visit only Draft 7 schema positions, never arbitrary annotation/default data.
func walkSchema(value any, pointer string, defaults bool, visit func(any, string, bool) error) error {
	node, object := value.(map[string]any)
	if _, boolean := value.(bool); !object && !boolean {
		return nil
	}
	if err := visit(value, pointer, defaults); err != nil {
		return err
	}
	for _, key := range []string{"properties", "patternProperties", "definitions", "dependencies"} {
		children, _ := node[key].(map[string]any)
		for name, child := range children {
			path := pointer + "/" + key + "/" + strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
			if err := walkSchema(child, path, defaults && key == "properties", visit); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"items", "additionalItems", "additionalProperties", "contains", "propertyNames", "if", "then", "else", "not", "allOf", "anyOf", "oneOf"} {
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

func referenceDefault(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, "${")
	case map[string]any:
		for _, child := range v {
			if referenceDefault(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if referenceDefault(child) {
				return true
			}
		}
	}
	return false
}

func applyConfigDefaults(value any, present bool, schema map[string]any) any {
	if !present {
		if fallback, ok := schema["default"]; ok {
			value, _ = jsonValue(fallback)
			present = true
		}
	}
	if !present {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		for name, raw := range props {
			child, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			current, exists := v[name]
			if !exists {
				if _, hasDefault := child["default"]; !hasDefault {
					continue
				}
			}
			v[name] = applyConfigDefaults(current, exists, child)
		}
	case []any:
		if item, ok := schema["items"].(map[string]any); ok {
			for i := range v {
				v[i] = applyConfigDefaults(v[i], true, item)
			}
		}
	}
	return value
}

func (p *packageConfig) effective(input Settings) Settings {
	if p == nil {
		return input
	}
	value, _ := jsonValue(map[string]any(input))
	// Omitted configuration uses a root default, otherwise an empty object.
	if input == nil {
		value = applyConfigDefaults(nil, false, p.document)
		if value == nil {
			value = map[string]any{}
		}
	}
	value = applyConfigDefaults(value, true, p.document)
	result, _ := value.(map[string]any)
	return result
}

func (c *Collector) configurationEnvelope() ([]byte, error) {
	if c.definition.config == nil {
		if len(c.ScriptConfig) != 0 {
			return nil, fmt.Errorf("package does not declare config_schema")
		}
		return nil, nil
	}
	effective := c.definition.config.effective(c.ScriptConfig)
	if err := c.definition.config.schema.Validate(map[string]any(effective)); err != nil {
		return nil, fmt.Errorf("config does not satisfy package schema")
	}
	data, err := json.Marshal(struct {
		Version string   `json:"version"`
		Config  Settings `json:"config"`
	}{"v1", effective})
	if err != nil {
		return nil, fmt.Errorf("config cannot be encoded")
	}
	if len(data)+1 > maxResponseBytes {
		return nil, fmt.Errorf("config envelope exceeds 1 MiB")
	}
	return append(data, '\n'), nil
}

// Keep generic YAML jobs using the same decoder as registered package jobs.
var _ yaml.Unmarshaler = (*Settings)(nil)
