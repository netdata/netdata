// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func focusTestParam() ParamConfig {
	return ParamConfig{
		ID: "near", Name: "Near", Selection: ParamMultiSelect,
		Options: []ParamOption{{ID: "everything", Name: "Everything", Default: true}, {ID: "opaque,choice-a", Name: "A"}},
		Focus: &FocusConfig{
			Role: FocusRoleRoots, UnfocusedOption: "everything",
			Bindings: &FocusBindings{Table: "choices", OptionColumn: "choice", ActorColumn: "entity"},
			Traversal: &FocusTraversal{
				ConnectorTypes: []string{"connector"}, BoundaryTypes: []string{"boundary"}, ConnectRoots: true,
				PruneLeafTypes:       []string{"connector"},
				ProtectedMemberships: []FocusProtectedMembership{{ActorType: "boundary", LinkType: "membership"}},
			},
		},
	}
}

func TestFocusMetadataSerializationAndOwnership(t *testing.T) {
	param := focusTestParam()
	require.NoError(t, param.ValidateFocus())
	wire := param.RequiredParam()
	raw, err := json.Marshal(wire["focus"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"roots","unfocused_option":"everything",
		"bindings":{"table":"choices","option_column":"choice","actor_column":"entity"},
		"traversal":{"connector_types":["connector"],"boundary_types":["boundary"],"connect_roots":true,
		"prune_leaf_types":["connector"],"protected_memberships":[{"actor_type":"boundary","link_type":"membership"}]}}`, string(raw))
	copy := wire["focus"].(*FocusConfig)
	copy.Bindings.Table = "changed"
	copy.Traversal.ConnectorTypes[0] = "changed"
	copy.Traversal.BoundaryTypes[0] = "changed"
	copy.Traversal.PruneLeafTypes[0] = "changed"
	copy.Traversal.ProtectedMemberships[0].LinkType = "changed"
	assert.Equal(t, focusTestParam().Focus, param.Focus)
	assert.NotContains(t, (ParamConfig{ID: "ordinary"}).RequiredParam(), "focus")
}

func TestFocusMetadataDepth(t *testing.T) {
	param := ParamConfig{ID: "radius", Name: "Radius", Focus: &FocusConfig{Role: FocusRoleDepth, UnlimitedOption: "unlimited"},
		Options: []ParamOption{{ID: "unlimited"}, {ID: "0"}, {ID: "2"}}}
	require.NoError(t, param.ValidateFocus())
	raw, err := json.Marshal(param.RequiredParam()["focus"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"depth","unlimited_option":"unlimited"}`, string(raw))
	param.Options = append(param.Options, ParamOption{ID: "opaque"})
	require.ErrorContains(t, param.ValidateFocus(), "non-negative integer")
}

func TestFocusMetadataValidation(t *testing.T) {
	for name, change := range map[string]func(*ParamConfig){
		"unknown role":             func(p *ParamConfig) { p.Focus.Role = "ordinary" },
		"missing bindings":         func(p *ParamConfig) { p.Focus.Bindings = nil },
		"empty table":              func(p *ParamConfig) { p.Focus.Bindings.Table = "" },
		"empty option column":      func(p *ParamConfig) { p.Focus.Bindings.OptionColumn = "" },
		"empty actor column":       func(p *ParamConfig) { p.Focus.Bindings.ActorColumn = "" },
		"missing traversal":        func(p *ParamConfig) { p.Focus.Traversal = nil },
		"missing unfocused option": func(p *ParamConfig) { p.Focus.UnfocusedOption = "missing" },
		"wrong depth field":        func(p *ParamConfig) { p.Focus.UnlimitedOption = "everything" },
		"focused default":          func(p *ParamConfig) { p.Options[0].Default = false; p.Options[1].Default = true },
		"duplicate connector":      func(p *ParamConfig) { p.Focus.Traversal.ConnectorTypes = []string{"connector", "connector"} },
		"overlapping actor roles":  func(p *ParamConfig) { p.Focus.Traversal.BoundaryTypes = []string{"connector"} },
		"empty prune type":         func(p *ParamConfig) { p.Focus.Traversal.PruneLeafTypes = []string{""} },
		"empty protected link":     func(p *ParamConfig) { p.Focus.Traversal.ProtectedMemberships[0].LinkType = "" },
	} {
		t.Run(name, func(t *testing.T) {
			param := focusTestParam()
			change(&param)
			require.Error(t, param.ValidateFocus())
		})
	}
}

func TestFocusMetadataRoleRelationshipsAndDefaults(t *testing.T) {
	root := focusTestParam()
	depth := ParamConfig{ID: "radius", Options: []ParamOption{{ID: "all"}, {ID: "0"}},
		Focus: &FocusConfig{Role: FocusRoleDepth, UnlimitedOption: "all"}}
	require.NoError(t, ValidateFocusParams([]ParamConfig{root, depth}))
	require.ErrorContains(t, ValidateFocusParams([]ParamConfig{depth}), "requires a roots")
	require.ErrorContains(t, ValidateFocusParams([]ParamConfig{root, root}), "multiple parameters")
	require.ErrorContains(t, ValidateFocusParams([]ParamConfig{root, depth, depth}), "multiple parameters")
	root.Options[0], root.Options[1] = root.Options[1], root.Options[0]
	require.NoError(t, root.ValidateFocus(), "explicit unfocused default need not be first")
	root.Options[1].Default = false
	require.ErrorContains(t, root.ValidateFocus(), "default to its unfocused")
	root.Options[0], root.Options[1] = root.Options[1], root.Options[0]
	require.NoError(t, root.ValidateFocus(), "first option is the implicit default")
	depth.Selection = ParamMultiSelect
	require.ErrorContains(t, depth.ValidateFocus(), "single selection")
	depth.Selection = ParamSelect
	depth.Options[1].ID = "-1"
	require.ErrorContains(t, depth.ValidateFocus(), "non-negative integer")
}

func TestFocusValidationUsesEffectiveDefaults(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection ParamSelection
		options   []ParamOption
		valid     bool
	}{
		{"select first default focused", ParamSelect, []ParamOption{{ID: "device", Default: true}, {ID: "everything", Default: true}}, false},
		{"select first default unfocused", ParamSelect, []ParamOption{{ID: "everything", Default: true}, {ID: "device", Default: true}}, true},
		{"select later explicit default", ParamSelect, []ParamOption{{ID: "device"}, {ID: "everything", Default: true}}, true},
		{"select implicit focused", ParamSelect, []ParamOption{{ID: "device"}, {ID: "everything"}}, false},
		{"select implicit unfocused", ParamSelect, []ParamOption{{ID: "everything"}, {ID: "device"}}, true},
		{"multiselect includes unfocused", ParamMultiSelect, []ParamOption{{ID: "device", Default: true}, {ID: "everything", Default: true}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			param := focusTestParam()
			param.Selection, param.Options = test.selection, test.options
			if test.valid {
				require.NoError(t, param.ValidateFocus())
			} else {
				require.ErrorContains(t, param.ValidateFocus(), "default to its unfocused")
			}
		})
	}
}
