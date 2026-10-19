// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Ordinary metadata stays open; only declared focus controls add graph obligations.
func validateFocusMetadata(raw any, data map[string]any) error {
	var params []funcapi.ParamConfig
	for index, value := range decodedArray(raw) {
		param, ok := value.(map[string]any)
		if !ok {
			continue
		}
		focus, declared := param["focus"]
		if !declared {
			continue
		}
		path := fmt.Sprintf("response.required_params[%d]", index)
		config, err := decodedFocusParam(path, param, focus)
		if err != nil {
			return err
		}
		params = append(params, config)
	}
	if len(params) == 0 {
		return nil
	}
	if err := funcapi.ValidateFocusParams(params); err != nil {
		return fmt.Errorf("response.required_params focus: %w", err)
	}
	shape, err := collectTopologyShape(data)
	if err != nil {
		return err
	}
	for _, param := range params {
		if param.Focus.Role != funcapi.FocusRoleRoots {
			continue
		}
		if err := validateFocusBindings(param, data); err != nil {
			return fmt.Errorf("parameter %q focus: %w", param.ID, err)
		}
		if err := validateFocusTraversal(*param.Focus.Traversal, shape); err != nil {
			return fmt.Errorf("parameter %q focus: %w", param.ID, err)
		}
	}
	return nil
}

func decodedArray(raw any) []any {
	values, _ := raw.([]any)
	return values
}

func decodedFocusParam(path string, param map[string]any, rawFocus any) (funcapi.ParamConfig, error) {
	config := funcapi.ParamConfig{}
	config.ID, _ = param["id"].(string)
	if config.ID == "" {
		return config, fmt.Errorf("%s.id is required for focus", path)
	}
	switch param["type"] {
	case "select":
		config.Selection = funcapi.ParamSelect
	case "multiselect":
		config.Selection = funcapi.ParamMultiSelect
	default:
		return config, fmt.Errorf("%s.type must be select or multiselect for focus", path)
	}
	for _, rawOption := range decodedArray(param["options"]) {
		option, _ := rawOption.(map[string]any)
		id, _ := option["id"].(string)
		selected, _ := option["defaultSelected"].(bool)
		config.Options = append(config.Options, funcapi.ParamOption{ID: id, Default: selected})
	}
	if _, ok := rawFocus.(map[string]any); !ok {
		return config, fmt.Errorf("%s.focus is not an object", path)
	}
	encoded, err := json.Marshal(rawFocus)
	if err == nil {
		err = json.Unmarshal(encoded, &config.Focus)
	}
	if err != nil {
		return config, fmt.Errorf("%s.focus: %w", path, err)
	}
	return config, nil
}

func validateFocusBindings(param funcapi.ParamConfig, data map[string]any) error {
	bindings := param.Focus.Bindings
	tables, _ := data["tables"].(map[string]any)
	actorTables, _ := tables["actor"].(map[string]any)
	table, ok := actorTables[bindings.Table].(map[string]any)
	if !ok {
		return fmt.Errorf("bindings reference missing actor table %q", bindings.Table)
	}
	types, _ := data["types"].(map[string]any)
	tableTypes, _ := types["table_types"].(map[string]any)
	typeID, _ := table["type"].(string)
	tableType, _ := tableTypes[typeID].(map[string]any)
	if tableType["owner"] != "actor" || tableType["role"] != "actor_detail" || tableType["aggregation"] != "append" {
		return fmt.Errorf("bindings table %q must declare an actor-owned actor_detail append type", bindings.Table)
	}
	path := "data.tables.actor." + bindings.Table + ".table"
	_, columns, err := decodedColumnsFromCompactTable(path, table["table"])
	if err != nil {
		return err
	}
	option := columns[bindings.OptionColumn]
	if option.columnType != "string" && option.columnType != "string_ref" {
		return fmt.Errorf("bindings option column %q must be string or string_ref", bindings.OptionColumn)
	}
	actor := columns[bindings.ActorColumn]
	if actor.columnType != "actor_ref" {
		return fmt.Errorf("bindings actor column %q must be actor_ref", bindings.ActorColumn)
	}
	options := make(map[string]struct{}, len(param.Options))
	for _, option := range param.Options {
		options[option.ID] = struct{}{}
	}
	dictionaries, _ := data["dictionaries"].(map[string]any)
	for row, value := range option.values {
		id, ok := resolveStringValue(value, option.columnType, option.dictionary, dictionaries)
		if !ok {
			return fmt.Errorf("%s.%s[%d] must resolve to a focus option", path, bindings.OptionColumn, row)
		}
		if _, exists := options[id]; !exists || id == param.Focus.UnfocusedOption {
			return fmt.Errorf("%s.%s[%d] references unknown or unfocused option %q", path, bindings.OptionColumn, row, id)
		}
		if actor.values[row] == nil {
			return fmt.Errorf("%s.%s[%d] must reference an actor", path, bindings.ActorColumn, row)
		}
	}
	return nil
}

func validateFocusTraversal(traversal funcapi.FocusTraversal, shape topologyShape) error {
	for name, types := range map[string][]string{
		"connector_types": traversal.ConnectorTypes, "boundary_types": traversal.BoundaryTypes,
		"prune_leaf_types": traversal.PruneLeafTypes,
	} {
		for _, id := range types {
			if _, exists := shape.actorTypes[id]; !exists {
				return fmt.Errorf("traversal.%s references unknown actor type %q", name, id)
			}
		}
	}
	for _, membership := range traversal.ProtectedMemberships {
		if _, exists := shape.actorTypes[membership.ActorType]; !exists {
			return fmt.Errorf("protected membership references unknown actor type %q", membership.ActorType)
		}
		if _, exists := shape.linkTypes[membership.LinkType]; !exists {
			return fmt.Errorf("protected membership references unknown link type %q", membership.LinkType)
		}
	}
	return nil
}
