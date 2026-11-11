// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyFocusValidationPipeline(t *testing.T) {
	schema := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixture := readTestFile(t, filepath.Join("..", "fixtures", "topology-v1", "snmp-l2.json"))
	for _, test := range []struct{ choice, want string }{{"device-a", ""}, {"not-declared", "unknown or unfocused option"}} {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(fixture, &payload))
		data := payload["data"].(map[string]any)
		columns := []any{map[string]any{"id": "option", "type": "string"}, map[string]any{"id": "actor", "type": "actor_ref"}}
		types := data["types"].(map[string]any)
		if types["table_types"] == nil {
			types["table_types"] = map[string]any{}
		}
		types["table_types"].(map[string]any)["choice_bindings"] = map[string]any{
			"role": "actor_detail", "owner": "actor", "aggregation": "append", "columns": columns,
		}
		if data["tables"] == nil {
			data["tables"] = map[string]any{}
		}
		tables := data["tables"].(map[string]any)
		if tables["actor"] == nil {
			tables["actor"] = map[string]any{}
		}
		tables["actor"].(map[string]any)["choices"] = map[string]any{"type": "choice_bindings", "table": map[string]any{
			"rows": 1, "columns": columns, "values": []any{
				map[string]any{"codec": "const", "value": test.choice}, map[string]any{"codec": "const", "value": 0},
			},
		}}
		payload["required_params"] = []any{map[string]any{"id": "near", "type": "multiselect",
			"options": []any{map[string]any{"id": "all"}, map[string]any{"id": "device-a"}},
			"focus": map[string]any{"role": "roots", "unfocused_option": "all",
				"bindings":  map[string]any{"table": "choices", "option_column": "option", "actor_column": "actor"},
				"traversal": map[string]any{"connect_roots": true},
			},
		}}
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		got, err := validateJSON(schema, encoded)
		if test.want != "" {
			require.ErrorContains(t, err, test.want)
		} else {
			require.NoError(t, err)
			output, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, string(encoded), string(output))
		}
	}
}

func TestAppliedFocusValidationPipeline(t *testing.T) {
	schema := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixture := readTestFile(t, filepath.Join("..", "fixtures", "topology-v1", "snmp-l2.json"))
	for _, test := range []struct {
		name  string
		focus map[string]any
		valid bool
	}{
		{"unlimited", map[string]any{"roots": []int{0}}, true},
		{"zero depth", map[string]any{"roots": []int{0}, "depth": 0}, true},
		{"empty roots", map[string]any{"roots": []int{}}, true},
		{"out of bounds", map[string]any{"roots": []int{1000000}}, false},
		{"negative depth", map[string]any{"roots": []int{0}, "depth": -1}, false},
		{"missing roots", map[string]any{"depth": 1}, false},
		{"null roots", map[string]any{"roots": nil}, false},
		{"unknown member", map[string]any{"roots": []int{0}, "unknown": true}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, json.Unmarshal(fixture, &payload))
			payload["data"].(map[string]any)["view"] = map[string]any{"focus": test.focus}
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			got, err := validateJSON(schema, encoded)
			if !test.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			output, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, string(encoded), string(output))
		})
	}
}
