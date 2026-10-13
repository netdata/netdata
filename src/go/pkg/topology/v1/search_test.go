// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchLabelKeysSchemaAndSemanticValidation(t *testing.T) {
	tests := map[string]struct {
		keys  any
		valid bool
	}{
		"empty list":      {keys: []any{}, valid: true},
		"ordinary labels": {keys: []any{"hostname", "os"}, valid: true},
		"built-in labels": {keys: []any{"_hostname", "_os"}, valid: true},
		"qualified label": {keys: []any{"example.net/rack"}, valid: true},
		"unicode label":   {keys: []any{"περιοχή"}, valid: true},
		"space in label":  {keys: []any{"rack name"}, valid: true},
		"empty key":       {keys: []any{""}},
		"duplicate key":   {keys: []any{"_hostname", "_hostname"}},
		"null key":        {keys: []any{nil}},
		"numeric key":     {keys: []any{1}},
		"non-list":        {keys: "_hostname"},
		"null list":       {keys: nil},
	}
	for name, test := range tests {
		for variant, presentation := range map[string]bool{"without presentation": false, "with presentation": true} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				payload, actorType := searchTestResponse(t, presentation)
				actorType["search"] = map[string]any{"label_keys": test.keys}
				encoded, err := json.Marshal(payload)
				require.NoError(t, err)
				assert.Equal(t, test.valid, topologySchemaValidationError(t, encoded) == nil, "schema validity")
				assert.Equal(t, test.valid, ValidateDecodedResponse(payload) == nil, "semantic validity")
			})
		}
	}
}

func TestSearchLabelKeyRelaxationPreservesColumnIdentifiers(t *testing.T) {
	for _, id := range []string{"_hostname", "example.net/rack", "rack name", "περιοχή", ""} {
		t.Run(id, func(t *testing.T) {
			payload, actorType := searchTestResponse(t, true)
			actorType["search"] = map[string]any{"columns": []any{id}}
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			require.Error(t, topologySchemaValidationError(t, encoded))
			require.Error(t, ValidateDecodedResponse(payload))
		})
	}
}

func TestSearchColumnReferencesWithoutPresentation(t *testing.T) {
	for _, column := range []string{"id", "missing"} {
		t.Run(column, func(t *testing.T) {
			payload, actorType := searchTestResponse(t, false)
			actorType["search"] = map[string]any{"columns": []any{column}}
			assert.Equal(t, column == "id", ValidateDecodedResponse(payload) == nil)
		})
	}
}

func searchTestResponse(t *testing.T, withPresentation bool) (map[string]any, map[string]any) {
	t.Helper()
	var presentation *ActorPresentation
	if withPresentation {
		presentation = &ActorPresentation{Label: "Node"}
	}
	encoded, err := json.Marshal(NewResponse(minimalValidationData(presentation)))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	actorTypes := payload["data"].(map[string]any)["types"].(map[string]any)["actor_types"].(map[string]any)
	return payload, actorTypes["node"].(map[string]any)
}
