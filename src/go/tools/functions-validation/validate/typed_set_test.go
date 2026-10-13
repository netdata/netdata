// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyTypedSetFixture(t *testing.T) {
	schemaBytes := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixture := readTestFile(t, filepath.Join("..", "fixtures", "topology-v1", "network-connections.json"))
	for name, tc := range map[string]struct {
		value    any
		nullable bool
		wantErr  string
	}{
		"typed set":          {value: []any{"alpha", "beta"}},
		"nullable member":    {value: []any{"alpha", nil}, nullable: true},
		"nonnullable member": {value: []any{"alpha", nil}, wantErr: "is null but column is not nullable"},
		"wrong member":       {value: []any{"alpha", 3}, wantErr: "is not a string"},
		"nested member":      {value: []any{"alpha", []any{"beta"}}, wantErr: "is not a string"},
	} {
		t.Run(name, func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, json.Unmarshal(fixture, &payload))
			actors := payload["data"].(map[string]any)["actors"].(map[string]any)
			rowCount := int(actors["rows"].(float64))
			require.Greater(t, rowCount, 1)
			values := make([]any, rowCount)
			for row := range values {
				values[row] = "scalar-key"
			}
			values[1] = tc.value
			actors["columns"] = append(actors["columns"].([]any), map[string]any{
				"id": "set_sample", "type": "string", "role": "merge_identity", "aggregation": "set", "nullable": tc.nullable,
			})
			actors["values"] = append(actors["values"].([]any), map[string]any{"codec": "values", "values": values})
			input, err := json.Marshal(payload)
			require.NoError(t, err)
			validated, err := validateJSON(schemaBytes, input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, "data.actors.set_sample[1][1] "+tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, payload, validated, "the CLI must preserve scalar and set cell representations")
		})
	}
}

func TestNetworkViewerProducerSetFixtures(t *testing.T) {
	schemaBytes := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixtureDir := filepath.Join("..", "..", "..", "..", "collectors", "network-viewer.plugin", "tests", "fixtures", "topology")
	// These synthetic producer fixtures also shipped in v2.11.0. Their cells
	// are scalar/null; the array mutation below models a Cloud aggregate.
	for _, fixture := range []string{"with-containers.json", "zero-containers.json", "mixed-containers.json", "whitelist-allow-all.json"} {
		t.Run(fixture, func(t *testing.T) {
			input := readTestFile(t, filepath.Join(fixtureDir, fixture))
			validated, err := validateJSON(schemaBytes, input)
			require.NoError(t, err)
			actors := validated.(map[string]any)["data"].(map[string]any)["actors"].(map[string]any)
			for i, rawColumn := range actors["columns"].([]any) {
				column := rawColumn.(map[string]any)
				if column["id"] != "orchestrator" {
					continue
				}
				require.Equal(t, "string", column["type"])
				require.Equal(t, "set", column["aggregation"])
				encoding := actors["values"].([]any)[i].(map[string]any)
				require.Equal(t, "values", encoding["codec"])
				values := encoding["values"].([]any)
				require.NotEmpty(t, values)
				values[0] = []any{"docker", "k8s"}
				aggregated, err := json.Marshal(validated)
				require.NoError(t, err)
				result, err := validateJSON(schemaBytes, aggregated)
				require.NoError(t, err)
				assert.Equal(t, validated, result, "aggregate validation must preserve producer metadata")
				return
			}
			t.Fatal("fixture has no orchestrator column")
		})
	}
}
