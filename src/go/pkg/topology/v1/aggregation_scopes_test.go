// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorAggregationScopeReferences(t *testing.T) {
	tests := map[string]struct {
		scopes   any
		registry bool
		valid    bool
	}{
		"scope-less actor":       {scopes: []any{}, valid: true},
		"registered scope":       {scopes: []any{"node"}, registry: true, valid: true},
		"undefined endpoint":     {scopes: []any{"endpoint"}, registry: true},
		"missing scope registry": {scopes: []any{"node"}},
		"one undefined scope":    {scopes: []any{"node", "endpoint"}, registry: true},
		"duplicate scope":        {scopes: []any{"node", "node"}, registry: true},
		"null scope":             {scopes: []any{nil}, registry: true},
		"empty scope":            {scopes: []any{""}, registry: true},
		"non-list scopes":        {scopes: "node", registry: true},
		"null scopes":            {scopes: nil, registry: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			data := minimalValidationData(nil)
			if test.registry {
				data.Types.AggregationScopes = map[string]AggregationScope{"node": {Columns: []string{"id"}}}
			}
			encoded, err := json.Marshal(NewResponse(data))
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			types := payload["data"].(map[string]any)["types"].(map[string]any)
			actorType := types["actor_types"].(map[string]any)["node"].(map[string]any)
			actorType["aggregation_scopes"] = test.scopes
			err = ValidateDecodedResponse(payload)
			if test.valid {
				require.NoError(t, err)
				encoded, err = json.Marshal(payload)
				require.NoError(t, err)
				validateAgainstTopologySchema(t, encoded)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "data.types.actor_types.node.aggregation_scopes")
			}
		})
	}
}
