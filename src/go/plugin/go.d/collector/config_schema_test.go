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
