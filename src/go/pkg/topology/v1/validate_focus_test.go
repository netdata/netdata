// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func focusResponseFixture(t *testing.T) map[string]any {
	t.Helper()
	data := minimalValidationData(nil)
	columns := []Column{NewColumn("choice", "string"), NewColumn("entity", "actor_ref")}
	data.Types.TableTypes = map[string]TableType{"selection_refs": {
		Role: "actor_detail", Owner: "actor", Aggregation: "append", Columns: columns,
		Presentation: &TableTypePresentation{DefaultVisibility: "hidden"},
	}}
	data.Tables = &DetailTables{Actor: map[string]DetailTable{"choices": {
		Type: "selection_refs", Table: MustTable(2, columns, []ColumnEncoding{Values("a", "b"), Const(0)}),
	}}}
	response := NewResponse(data)
	response.RequiredParams = []any{
		(funcapi.ParamConfig{ID: "near", Selection: funcapi.ParamMultiSelect,
			Options: []funcapi.ParamOption{{ID: "everything"}, {ID: "a"}, {ID: "b"}, {ID: "unresolved"}},
			Focus: &funcapi.FocusConfig{Role: funcapi.FocusRoleRoots, UnfocusedOption: "everything",
				Bindings: &funcapi.FocusBindings{Table: "choices", OptionColumn: "choice", ActorColumn: "entity"},
				Traversal: &funcapi.FocusTraversal{ConnectorTypes: []string{"node"}, ConnectRoots: true,
					PruneLeafTypes: []string{"node"}, ProtectedMemberships: []funcapi.FocusProtectedMembership{{ActorType: "node", LinkType: "dependency"}}},
			}}).RequiredParam(),
		(funcapi.ParamConfig{ID: "radius", Options: []funcapi.ParamOption{{ID: "unlimited"}, {ID: "0"}, {ID: "2"}},
			Focus: &funcapi.FocusConfig{Role: funcapi.FocusRoleDepth, UnlimitedOption: "unlimited"}}).RequiredParam(),
	}
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	return payload
}

func focusFixtureParts(payload map[string]any) (map[string]any, map[string]any, map[string]any, map[string]any) {
	param := payload["required_params"].([]any)[0].(map[string]any)
	focus := param["focus"].(map[string]any)
	data := payload["data"].(map[string]any)
	entry := data["tables"].(map[string]any)["actor"].(map[string]any)["choices"].(map[string]any)
	return param, focus, entry["table"].(map[string]any), data
}

func TestFocusResponseSchemaAndSemantics(t *testing.T) {
	for _, codec := range []string{"values", "const", "dict", "string_ref"} {
		t.Run(codec, func(t *testing.T) {
			payload := focusResponseFixture(t)
			_, _, table, data := focusFixtureParts(payload)
			encoding := map[string]any{"codec": codec}
			switch codec {
			case "values":
				encoding["values"] = []any{"a", "b"}
			case "const":
				encoding["value"] = "a"
			case "dict":
				encoding["values"], encoding["indexes"] = []any{"a", "b"}, []any{0, 1}
			case "string_ref":
				// The ordinary string_ref column codec uses a shared dictionary.
				column := table["columns"].([]any)[0].(map[string]any)
				column["type"], column["dictionary"] = "string_ref", "focus_options"
				data["dictionaries"].(map[string]any)["focus_options"] = []any{"a", "b"}
				encoding = map[string]any{"codec": "values", "values": []any{0, 1}}
			}
			table["values"].([]any)[0] = encoding
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			validateAgainstTopologySchema(t, encoded)
			require.NoError(t, ValidateDecodedResponse(payload))
		})
	}
	t.Run("empty bindings retain unresolved choices", func(t *testing.T) {
		payload := focusResponseFixture(t)
		_, _, table, _ := focusFixtureParts(payload)
		table["rows"] = float64(0)
		table["values"] = []any{map[string]any{"codec": "values", "values": []any{}}, map[string]any{"codec": "values", "values": []any{}}}
		require.NoError(t, ValidateDecodedResponse(payload))
	})
	t.Run("one choice maps to multiple actors", func(t *testing.T) {
		payload := focusResponseFixture(t)
		_, _, table, data := focusFixtureParts(payload)
		actors := data["actors"].(map[string]any)
		actors["rows"] = float64(2)
		actors["values"].([]any)[0] = map[string]any{"codec": "values", "values": []any{"node-a", "node-b"}}
		table["values"] = []any{map[string]any{"codec": "const", "value": "a"}, map[string]any{"codec": "values", "values": []any{0, 1}}}
		require.NoError(t, ValidateDecodedResponse(payload))
	})
}

func TestFocusResponseRejectsInvalidReferences(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(map[string]any, map[string]any, map[string]any, map[string]any)
		want   string
	}{
		"missing actor table": {change: func(_, f, _, _ map[string]any) { f["bindings"].(map[string]any)["table"] = "absent" }, want: "missing actor table"},
		"wrong option column": {change: func(_, f, _, _ map[string]any) { f["bindings"].(map[string]any)["option_column"] = "entity" }, want: "must be distinct"},
		"wrong actor column":  {change: func(_, f, _, _ map[string]any) { f["bindings"].(map[string]any)["actor_column"] = "absent" }, want: "must be actor_ref"},
		"wrong option type": {change: func(_, _, table, _ map[string]any) {
			table["columns"].([]any)[0].(map[string]any)["type"] = "uint"
			table["values"].([]any)[0] = map[string]any{"codec": "const", "value": 1}
		}, want: "must be string or string_ref"},
		"unknown actor type":    {change: func(_, f, _, _ map[string]any) { f["traversal"].(map[string]any)["connector_types"] = []any{"absent"} }, want: "unknown actor type"},
		"unknown boundary type": {change: func(_, f, _, _ map[string]any) { f["traversal"].(map[string]any)["boundary_types"] = []any{"absent"} }, want: "unknown actor type"},
		"unknown prune type":    {change: func(_, f, _, _ map[string]any) { f["traversal"].(map[string]any)["prune_leaf_types"] = []any{"absent"} }, want: "unknown actor type"},
		"unknown membership": {change: func(_, f, _, _ map[string]any) {
			f["traversal"].(map[string]any)["protected_memberships"] = []any{map[string]any{"actor_type": "node", "link_type": "absent"}}
		}, want: "unknown link type"},
		"unknown option": {change: func(_, _, table, _ map[string]any) {
			table["values"].([]any)[0] = map[string]any{"codec": "const", "value": "alias-or-absent"}
		}, want: "unknown or unfocused option"},
		"unfocused binding": {change: func(_, _, table, _ map[string]any) {
			table["values"].([]any)[0] = map[string]any{"codec": "const", "value": "everything"}
		}, want: "unknown or unfocused option"},
		"null option": {change: func(_, _, table, _ map[string]any) {
			table["columns"].([]any)[0].(map[string]any)["nullable"] = true
			table["values"].([]any)[0] = map[string]any{"codec": "const", "value": nil}
		}, want: "must resolve to a focus option"},
		"null actor": {change: func(_, _, table, _ map[string]any) {
			table["columns"].([]any)[1].(map[string]any)["nullable"] = true
			table["values"].([]any)[1] = map[string]any{"codec": "const", "value": nil}
		}, want: "must reference an actor"},
		"actor out of bounds": {change: func(_, _, table, _ map[string]any) {
			table["values"].([]any)[1] = map[string]any{"codec": "const", "value": 5}
		}, want: "actor reference out of bounds"},
		"wrong binding owner": {change: func(_, _, _, data map[string]any) {
			data["types"].(map[string]any)["table_types"].(map[string]any)["selection_refs"].(map[string]any)["owner"] = "relationship"
		}, want: "actor-owned"},
		"wrong binding aggregation": {change: func(_, _, _, data map[string]any) {
			data["types"].(map[string]any)["table_types"].(map[string]any)["selection_refs"].(map[string]any)["aggregation"] = "set"
		}, want: "actor-owned"},
		"missing sentinel":       {change: func(_, f, _, _ map[string]any) { f["unfocused_option"] = "absent" }, want: "not declared"},
		"invalid metadata":       {change: func(p, _, _, _ map[string]any) { p["focus"] = nil }, want: "not an object"},
		"invalid selection mode": {change: func(p, _, _, _ map[string]any) { p["type"] = "arbitrary" }, want: "select or multiselect"},
	} {
		t.Run(name, func(t *testing.T) {
			payload := focusResponseFixture(t)
			p, f, table, data := focusFixtureParts(payload)
			tc.change(p, f, table, data)
			require.ErrorContains(t, ValidateDecodedResponse(payload), tc.want)
		})
	}
}

func TestFocusSchemasStayEquivalent(t *testing.T) {
	topology := loadTopologySchema(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "plugins.d", "FUNCTION_UI_SCHEMA.json"))
	require.NoError(t, err)
	var ui map[string]any
	require.NoError(t, json.Unmarshal(raw, &ui))
	for _, name := range []string{"focus", "focus_bindings", "focus_traversal", "focus_membership"} {
		left, err := json.Marshal(topology["$defs"].(map[string]any)[name])
		require.NoError(t, err)
		right, err := json.Marshal(ui["definitions"].(map[string]any)[name])
		require.NoError(t, err)
		assert.JSONEq(t, strings.ReplaceAll(string(left), "#/$defs/", "#/definitions/"), string(right), name)
	}
}
