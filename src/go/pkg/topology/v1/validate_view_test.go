// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppliedViewFocus(t *testing.T) {
	zero, two := 0, 2
	for _, test := range []struct {
		name  string
		focus *ViewFocus
	}{
		{"absent", nil},
		{"unlimited", &ViewFocus{Roots: []int{0}}},
		{"zero depth", &ViewFocus{Roots: []int{0}, Depth: &zero}},
		{"finite depth", &ViewFocus{Roots: []int{0}, Depth: &two}},
		{"no surviving roots", &ViewFocus{Roots: []int{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := minimalValidationData(nil)
			data.View = &View{Focus: test.focus}
			encoded, err := json.Marshal(NewResponse(data))
			require.NoError(t, err)
			validateAgainstTopologySchema(t, encoded)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			require.NoError(t, ValidateDecodedResponse(payload))
			view := payload["data"].(map[string]any)["view"].(map[string]any)
			if test.focus == nil {
				require.NotContains(t, view, "focus")
			} else if test.focus.Depth == nil {
				require.NotContains(t, view["focus"], "depth")
			}
			viewJSON, err := json.Marshal(view)
			require.NoError(t, err)
			var decoded View
			require.NoError(t, json.Unmarshal(viewJSON, &decoded))
			require.Equal(t, test.focus, decoded.Focus)
		})
	}
}

func TestAppliedViewFocusRejectsInvalidReferencesAndDepth(t *testing.T) {
	for _, test := range []struct {
		name  string
		focus any
	}{
		{"null focus", nil},
		{"missing roots", map[string]any{}},
		{"null roots", map[string]any{"roots": nil}},
		{"negative root", map[string]any{"roots": []any{-1}}},
		{"out of bounds root", map[string]any{"roots": []any{1}}},
		{"fractional root", map[string]any{"roots": []any{0.5}}},
		{"string root", map[string]any{"roots": []any{"0"}}},
		{"null root", map[string]any{"roots": []any{nil}}},
		{"duplicate roots", map[string]any{"roots": []any{0, 0}}},
		{"negative depth", map[string]any{"roots": []any{0}, "depth": -1}},
		{"fractional depth", map[string]any{"roots": []any{0}, "depth": 0.5}},
		{"null depth", map[string]any{"roots": []any{0}, "depth": nil}},
		{"string depth", map[string]any{"roots": []any{0}, "depth": "2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := focusResponseFixture(t)
			data := payload["data"].(map[string]any)
			data["view"] = map[string]any{"focus": test.focus}
			require.ErrorContains(t, ValidateDecodedResponse(payload), "data.view.focus")
		})
	}
}
