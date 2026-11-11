// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func functionFocusTestParams() []funcapi.ParamConfig {
	return []funcapi.ParamConfig{
		{ID: "near", Name: "Near", Selection: funcapi.ParamMultiSelect,
			Options: []funcapi.ParamOption{{ID: "everything", Name: "Everything"}, {ID: "opaque-choice", Name: "Device"}},
			Focus: &funcapi.FocusConfig{Role: funcapi.FocusRoleRoots, UnfocusedOption: "everything",
				Bindings: &funcapi.FocusBindings{Table: "choices", OptionColumn: "option", ActorColumn: "actor"},
				Traversal: &funcapi.FocusTraversal{ConnectorTypes: []string{"connector"}, BoundaryTypes: []string{"boundary"}, ConnectRoots: true,
					PruneLeafTypes: []string{"connector"}, ProtectedMemberships: []funcapi.FocusProtectedMembership{{ActorType: "boundary", LinkType: "member"}}},
			}},
		{ID: "radius", Name: "Radius", Options: []funcapi.ParamOption{{ID: "unlimited", Name: "All"}, {ID: "0", Name: "0"}},
			Focus: &funcapi.FocusConfig{Role: funcapi.FocusRoleDepth, UnlimitedOption: "unlimited"}},
	}
}

func TestFunctionFocusMetadataInfoAndDataOverride(t *testing.T) {
	method := funcapi.FunctionConfig{ID: "events", DynamicInfo: true, RequiredParams: functionFocusTestParams()}
	h := newFunctionInfoHarness(t, method, "instance", func(collectorapi.RuntimeJob) funcapi.MethodHandler {
		return &structuredInfoHandler{t: t,
			params: func(context.Context, string) ([]funcapi.ParamConfig, error) { return functionFocusTestParams(), nil },
			handle: func(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
				params := functionFocusTestParams()
				params[0].Focus.Traversal.PruneLeafTypes = nil
				params[0].Focus.Traversal.ProtectedMemberships = nil
				params[0].Options[1].ID = "snapshot-choice"
				return &funcapi.FunctionResponse{RequiredParams: params, Data: []any{}}
			},
		}
	})
	info := h.call(t, []string{"info"}, nil, 200)
	assertFunctionInfoSchema(t, info, "info_response")
	assert.Contains(t, string(info), `"prune_leaf_types":["connector"]`)
	assert.Contains(t, string(info), `"opaque-choice"`)
	data := h.call(t, nil, nil, 200)
	assert.NotContains(t, string(data), "prune_leaf_types")
	assert.NotContains(t, string(data), "protected_memberships")
	assert.NotContains(t, string(data), "opaque-choice")
	assert.Contains(t, string(data), "snapshot-choice")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(info, &decoded))
	param := decoded["required_params"].([]any)[0].(map[string]any)
	focus := param["focus"].(map[string]any)
	for name, change := range map[string]func(){
		"unknown field":    func() { focus["arbitrary"] = true },
		"missing bindings": func() { delete(focus, "bindings") },
		"wrong traversal":  func() { focus["traversal"] = "opaque" },
	} {
		t.Run(name, func(t *testing.T) {
			// Each mutation starts from the real emitted info response.
			require.NoError(t, json.Unmarshal(info, &decoded))
			param = decoded["required_params"].([]any)[0].(map[string]any)
			focus = param["focus"].(map[string]any)
			change()
			require.Error(t, functionInfoSchema(t, "info_response").Validate(decoded))
		})
	}
}

func TestFunctionRejectsInvalidDynamicFocusMetadata(t *testing.T) {
	for _, data := range []bool{false, true} {
		h := newFunctionInfoHarness(t, funcapi.FunctionConfig{ID: "events", DynamicInfo: true}, "instance", func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			invalid := functionFocusTestParams()
			invalid[0].Focus.UnfocusedOption = "missing"
			return &structuredInfoHandler{t: t,
				params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
					if data {
						return nil, nil
					}
					return invalid, nil
				},
				handle: func(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
					return &funcapi.FunctionResponse{RequiredParams: invalid}
				},
			}
		})
		args := []string{"info"}
		if data {
			args = nil
		}
		got := h.call(t, args, nil, 500)
		assert.Contains(t, string(got), "invalid focus metadata")
	}
}

func TestControllerGroupSignatureFocusMetadata(t *testing.T) {
	for name, change := range map[string]func([]funcapi.ParamConfig){
		"role":           func(p []funcapi.ParamConfig) { p[0].Focus.Role = funcapi.FocusRoleDepth },
		"unfocused":      func(p []funcapi.ParamConfig) { p[0].Focus.UnfocusedOption = "other" },
		"unlimited":      func(p []funcapi.ParamConfig) { p[1].Focus.UnlimitedOption = "other" },
		"binding table":  func(p []funcapi.ParamConfig) { p[0].Focus.Bindings.Table = "other" },
		"binding column": func(p []funcapi.ParamConfig) { p[0].Focus.Bindings.ActorColumn = "other" },
		"connector":      func(p []funcapi.ParamConfig) { p[0].Focus.Traversal.ConnectorTypes[0] = "other" },
		"boundary":       func(p []funcapi.ParamConfig) { p[0].Focus.Traversal.BoundaryTypes[0] = "other" },
		"paths":          func(p []funcapi.ParamConfig) { p[0].Focus.Traversal.ConnectRoots = false },
		"pruning":        func(p []funcapi.ParamConfig) { p[0].Focus.Traversal.PruneLeafTypes = nil },
		"membership":     func(p []funcapi.ParamConfig) { p[0].Focus.Traversal.ProtectedMemberships[0].LinkType = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			method := funcapi.FunctionConfig{ID: "events", RequiredParams: functionFocusTestParams()}
			before, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
			require.NoError(t, err)
			change(method.RequiredParams)
			after, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
			require.NoError(t, err)
			assert.NotEqual(t, before, after, "a changed focus contract must replace the published generation")
		})
	}
}
