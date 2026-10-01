// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologyNumericCellFixture(t *testing.T) {
	schema := readTestFile(t, filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	fixture := readTestFile(t, filepath.Join("..", "fixtures", "topology-v1", "network-connections.json"))
	for name, tc := range map[string]struct {
		typ     string
		value   json.Number
		wantErr string
	}{
		"unsigned maximum":         {typ: "uint", value: "18446744073709551615"},
		"integer unsigned maximum": {typ: "int", value: "18446744073709551615"},
		"integer minimum":          {typ: "int", value: "-9223372036854775808"},
		"unsigned negative":        {typ: "uint", value: "-1", wantErr: "is not a non-negative integer"},
		"unsigned fraction":        {typ: "uint", value: "1.5", wantErr: "is not a non-negative integer"},
		"integer fraction":         {typ: "int", value: "1.5", wantErr: "is not an integer"},
	} {
		for _, aggregation := range []string{"none", "set"} {
			t.Run(name+"/"+aggregation, func(t *testing.T) {
				var payload map[string]any
				require.NoError(t, json.Unmarshal(fixture, &payload))
				actors := payload["data"].(map[string]any)["actors"].(map[string]any)
				var cell any = tc.value
				if aggregation == "set" {
					cell = []any{1, tc.value}
				}
				actors["columns"] = append(actors["columns"].([]any), map[string]any{
					"id": "numeric_sample", "type": tc.typ, "aggregation": aggregation,
				})
				actors["values"] = append(actors["values"].([]any), map[string]any{"codec": "const", "value": cell})
				input, err := json.Marshal(payload)
				require.NoError(t, err)
				result, err := validateJSON(schema, input)
				if tc.wantErr != "" {
					path := "data.actors.numeric_sample[0]"
					if aggregation == "set" {
						path += "[1]"
					}
					require.ErrorContains(t, err, path+" "+tc.wantErr)
					return
				}
				require.NoError(t, err)
				var expected any
				require.NoError(t, json.Unmarshal(input, &expected))
				assert.Equal(t, expected, result, "validation preserves the default decoder's numeric representation")
			})
		}
	}
}
