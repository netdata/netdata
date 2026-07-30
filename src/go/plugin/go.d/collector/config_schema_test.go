// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigSchemasDoNotMaterializeOptionalArrays forbids one authoring mistake in every
// collector's DynCfg schema instead of describing what any single schema currently contains.
//
// The DynCfg form materializes an array that declares minItems even when the operator never
// opened that section, fills its required leaves with nulls, and then refuses to delete the item
// it created — leaving a job the collector rejects and the operator cannot repair. minItems is
// therefore valid only on an array its parent requires, or one carrying a default the form can
// fill. "Non-empty when present" belongs in the collector's own validation, which is the only
// layer that runs: nothing validates a job config against these schemas.
func TestConfigSchemasDoNotMaterializeOptionalArrays(t *testing.T) {
	files, err := filepath.Glob("*/config_schema.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err, file)
		var doc struct {
			JSONSchema map[string]any `json:"jsonSchema"`
		}
		require.NoError(t, json.Unmarshal(data, &doc), file)

		// operatorAdded marks a node reached through additionalProperties: a map value exists
		// only once the operator adds its key, so the form cannot materialize it unasked.
		var walk func(node any, path, name string, required map[string]bool, operatorAdded bool)
		walk = func(node any, path, name string, required map[string]bool, operatorAdded bool) {
			obj, ok := node.(map[string]any)
			if !ok {
				return
			}
			if _, declared := obj["minItems"]; declared && schemaAllowsArray(obj["type"]) {
				_, hasDefault := obj["default"]
				assert.Truef(t, required[name] || hasDefault || operatorAdded,
					"%s: %s is optional and declares minItems; the DynCfg form would materialize an item the operator cannot delete",
					file, path)
			}

			childRequired := make(map[string]bool)
			if list, ok := obj["required"].([]any); ok {
				for _, item := range list {
					if key, ok := item.(string); ok {
						childRequired[key] = true
					}
				}
			}
			if properties, ok := obj["properties"].(map[string]any); ok {
				for key, child := range properties {
					walk(child, path+"."+key, key, childRequired, false)
				}
			}
			walk(obj["items"], path+"[]", name, childRequired, false)
			walk(obj["additionalProperties"], path+"{}", name, childRequired, true)
			for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
				branches, ok := obj[keyword].([]any)
				if !ok {
					continue
				}
				for i, branch := range branches {
					walk(branch, fmt.Sprintf("%s.%s[%d]", path, keyword, i), name, childRequired, false)
				}
			}
			if dependencies, ok := obj["dependencies"].(map[string]any); ok {
				for key, child := range dependencies {
					walk(child, fmt.Sprintf("%s.dependencies.%s", path, key), name, childRequired, false)
				}
			}
		}
		walk(doc.JSONSchema, "", "", nil, false)
	}
}

// TestConfigSchemasDoNotDeclareBranchKeysAsProperties forbids the second way a schema can make the
// DynCfg form show something the operator did not ask for.
//
// A `dependencies` entry reveals extra fields only for the selected value of its discriminator --
// `discovery.mode: filters` reveals `mode_filters`, and so on. Declaring one of those revealed keys
// as a plain sibling property as well makes the form render it unconditionally, for every mode, and
// the runtime then has to decide what a field belonging to an unselected branch means.
func TestConfigSchemasDoNotDeclareBranchKeysAsProperties(t *testing.T) {
	files, err := filepath.Glob("*/config_schema.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err, file)
		var doc struct {
			JSONSchema map[string]any `json:"jsonSchema"`
		}
		require.NoError(t, json.Unmarshal(data, &doc), file)

		var walk func(node any, path string)
		walk = func(node any, path string) {
			obj, ok := node.(map[string]any)
			if !ok {
				return
			}
			properties, _ := obj["properties"].(map[string]any)
			for discriminator, raw := range mapField(obj["dependencies"]) {
				for _, branch := range schemaBranches(raw) {
					for key := range mapField(branch["properties"]) {
						if key == discriminator {
							continue // the discriminator itself is a real property
						}
						assert.NotContainsf(t, properties, key,
							"%s: %s declares %q both as a %q-dependent branch field and as a plain property; the form would always show it",
							file, path, key, discriminator)
					}
				}
			}
			for key, child := range properties {
				walk(child, path+"."+key)
			}
			walk(obj["items"], path+"[]")
		}
		walk(doc.JSONSchema, "")
	}
}

// mapField returns node as an object, or nil when it is absent or another type.
func mapField(node any) map[string]any {
	obj, _ := node.(map[string]any)
	return obj
}

// schemaBranches returns a dependency's oneOf/anyOf alternatives, or the dependency itself when it
// applies unconditionally.
func schemaBranches(node any) []map[string]any {
	obj := mapField(node)
	if obj == nil {
		return nil
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		raw, ok := obj[keyword].([]any)
		if !ok {
			continue
		}
		branches := make([]map[string]any, 0, len(raw))
		for _, branch := range raw {
			if b := mapField(branch); b != nil {
				branches = append(branches, b)
			}
		}
		return branches
	}
	return []map[string]any{obj}
}

func schemaAllowsArray(schemaType any) bool {
	switch value := schemaType.(type) {
	case string:
		return value == "array"
	case []any:
		for _, item := range value {
			if item == "array" {
				return true
			}
		}
	}
	return false
}
