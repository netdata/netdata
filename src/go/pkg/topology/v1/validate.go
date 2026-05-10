// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"fmt"
	"math"
)

type validationContext struct {
	dictionaries       map[string]any
	actorRows          int
	linkRows           int
	evidenceRowsByType map[string]int
	evidenceRows       int
}

type topologyShape struct {
	actorColumns       map[string]string
	linkColumns        map[string]string
	actorTypes         map[string]struct{}
	linkTypes          map[string]struct{}
	portTypes          map[string]struct{}
	evidenceTypes      map[string]map[string]string
	tableTypes         map[string]map[string]string
	tableTypeOwners    map[string]string
	actorTables        map[string]map[string]string
	relationshipTables map[string]map[string]string
	scaleKeys          map[string]struct{}
}

func ValidateDecodedResponse(payload any) error {
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	data, ok := obj["data"].(map[string]any)
	if !ok || data["schema_version"] != SchemaVersion {
		return nil
	}

	return ValidateDecodedData(data)
}

func ValidateDecodedData(data map[string]any) error {
	dictionaries, ok := data["dictionaries"].(map[string]any)
	if !ok {
		return fmt.Errorf("data.dictionaries is not an object")
	}

	actorRows, err := decodedTableRows(data["actors"])
	if err != nil {
		return fmt.Errorf("data.actors: %w", err)
	}
	linkRows, err := decodedTableRows(data["links"])
	if err != nil {
		return fmt.Errorf("data.links: %w", err)
	}

	evidenceRowsByType, err := collectEvidenceRows(data["evidence"])
	if err != nil {
		return err
	}
	tableEvidenceSources := collectTableEvidenceSources(data["types"])

	ctx := validationContext{
		dictionaries:       dictionaries,
		actorRows:          actorRows,
		linkRows:           linkRows,
		evidenceRowsByType: evidenceRowsByType,
		evidenceRows:       -1,
	}

	if _, err := validateCompactTable("data.actors", data["actors"], ctx); err != nil {
		return err
	}
	if _, err := validateCompactTable("data.links", data["links"], ctx); err != nil {
		return err
	}
	if err := validateEvidenceSections(data["evidence"], ctx); err != nil {
		return err
	}
	if err := validateDetailTables(data["tables"], ctx, tableEvidenceSources); err != nil {
		return err
	}
	if err := validateOverlayRefs(data["overlays"], ctx); err != nil {
		return err
	}
	shape, err := collectTopologyShape(data)
	if err != nil {
		return err
	}
	if err := validateTypeColumns(data, shape); err != nil {
		return err
	}
	if err := validatePresentation(data, shape); err != nil {
		return err
	}

	return nil
}

func IsDecodedData(raw any) bool {
	data, ok := raw.(map[string]any)
	return ok && data["schema_version"] == SchemaVersion
}

func LinkRowsFromDecodedData(raw any) (int, error) {
	data, ok := raw.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("data is not an object")
	}
	rows, err := decodedTableRows(data["links"])
	if err != nil {
		return 0, fmt.Errorf("data.links: %w", err)
	}
	return rows, nil
}

func collectEvidenceRows(raw any) (map[string]int, error) {
	rowsByType := make(map[string]int)
	if raw == nil {
		return rowsByType, nil
	}
	sections, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.evidence is not an object")
	}
	for name, rawSection := range sections {
		section, ok := rawSection.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("data.evidence.%s is not an object", name)
		}
		typ, ok := section["type"].(string)
		if !ok || typ == "" {
			return nil, fmt.Errorf("data.evidence.%s.type is empty", name)
		}
		rows, err := decodedTableRows(section["table"])
		if err != nil {
			return nil, fmt.Errorf("data.evidence.%s.table: %w", name, err)
		}
		rowsByType[name] = rows
		rowsByType[typ] = rows
	}
	return rowsByType, nil
}

func collectTableEvidenceSources(rawTypes any) map[string]string {
	sources := make(map[string]string)
	types, ok := rawTypes.(map[string]any)
	if !ok {
		return sources
	}
	tableTypes, ok := types["table_types"].(map[string]any)
	if !ok {
		return sources
	}
	for name, rawTableType := range tableTypes {
		tableType, ok := rawTableType.(map[string]any)
		if !ok {
			continue
		}
		source, ok := tableType["source_evidence"].(string)
		if ok && source != "" {
			sources[name] = source
		}
	}
	return sources
}

func validateEvidenceSections(raw any, ctx validationContext) error {
	if raw == nil {
		return nil
	}
	sections, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.evidence is not an object")
	}
	for name, rawSection := range sections {
		section := rawSection.(map[string]any)
		if _, err := validateCompactTable("data.evidence."+name+".table", section["table"], ctx); err != nil {
			return err
		}
	}
	return nil
}

func validateDetailTables(raw any, ctx validationContext, tableEvidenceSources map[string]string) error {
	if raw == nil {
		return nil
	}
	tables, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.tables is not an object")
	}
	for _, groupName := range []string{"actor", "relationship"} {
		groupRaw, ok := tables[groupName]
		if !ok {
			continue
		}
		group, ok := groupRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("data.tables.%s is not an object", groupName)
		}
		for name, rawTable := range group {
			detail, ok := rawTable.(map[string]any)
			if !ok {
				return fmt.Errorf("data.tables.%s.%s is not an object", groupName, name)
			}
			tableCtx := ctx
			tableType, _ := detail["type"].(string)
			if sourceEvidence := tableEvidenceSources[tableType]; sourceEvidence != "" {
				rows, ok := ctx.evidenceRowsByType[sourceEvidence]
				if !ok {
					return fmt.Errorf("data.tables.%s.%s references unknown source_evidence %q", groupName, name, sourceEvidence)
				}
				tableCtx.evidenceRows = rows
			}
			if _, err := validateCompactTable("data.tables."+groupName+"."+name+".table", detail["table"], tableCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOverlayRefs(raw any, ctx validationContext) error {
	if raw == nil {
		return nil
	}
	overlays, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.overlays is not an object")
	}
	refs, ok := overlays["refs"]
	if !ok {
		return nil
	}
	if _, err := validateCompactTable("data.overlays.refs", refs, ctx); err != nil {
		return err
	}
	return nil
}

func validateCompactTable(path string, raw any, ctx validationContext) (int, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("%s is not an object", path)
	}
	rows, err := decodedTableRows(table)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	columns, ok := table["columns"].([]any)
	if !ok {
		return 0, fmt.Errorf("%s.columns is not an array", path)
	}
	values, ok := table["values"].([]any)
	if !ok {
		return 0, fmt.Errorf("%s.values is not an array", path)
	}
	if len(columns) != len(values) {
		return 0, fmt.Errorf("%s columns/values length mismatch: %d columns, %d values", path, len(columns), len(values))
	}

	for i := range columns {
		column, ok := columns[i].(map[string]any)
		if !ok {
			return 0, fmt.Errorf("%s.columns[%d] is not an object", path, i)
		}
		columnID, _ := column["id"].(string)
		columnType, _ := column["type"].(string)
		decoded, err := decodeColumn(path, i, rows, values[i])
		if err != nil {
			return 0, err
		}
		if err := validateColumnValues(fmt.Sprintf("%s.%s", path, columnID), column, columnType, decoded, ctx); err != nil {
			return 0, err
		}
	}

	return rows, nil
}

func decodeColumn(path string, columnIndex int, rows int, raw any) ([]any, error) {
	encoding, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.values[%d] is not an object", path, columnIndex)
	}
	codec, _ := encoding["codec"].(string)
	switch codec {
	case "const":
		value, ok := encoding["value"]
		if !ok {
			return nil, fmt.Errorf("%s.values[%d] const encoding missing value", path, columnIndex)
		}
		values := make([]any, rows)
		for i := range values {
			values[i] = value
		}
		return values, nil
	case "values":
		values, ok := encoding["values"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s.values[%d] values encoding missing values array", path, columnIndex)
		}
		if len(values) != rows {
			return nil, fmt.Errorf("%s.values[%d] decoded length mismatch: expected %d, got %d", path, columnIndex, rows, len(values))
		}
		return values, nil
	case "dict":
		dictValues, ok := encoding["values"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s.values[%d] dict encoding missing values array", path, columnIndex)
		}
		indexes, ok := encoding["indexes"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s.values[%d] dict encoding missing indexes array", path, columnIndex)
		}
		if len(indexes) != rows {
			return nil, fmt.Errorf("%s.values[%d] decoded length mismatch: expected %d, got %d", path, columnIndex, rows, len(indexes))
		}
		values := make([]any, rows)
		for i, rawIndex := range indexes {
			index, ok := integerValue(rawIndex)
			if !ok {
				return nil, fmt.Errorf("%s.values[%d].indexes[%d] is not an integer", path, columnIndex, i)
			}
			if index < 0 || index >= len(dictValues) {
				return nil, fmt.Errorf("%s.values[%d].indexes[%d] out of bounds: %d", path, columnIndex, i, index)
			}
			values[i] = dictValues[index]
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%s.values[%d] unsupported codec %q", path, columnIndex, codec)
	}
}

func validateColumnValues(path string, column map[string]any, columnType string, values []any, ctx validationContext) error {
	nullable, _ := column["nullable"].(bool)
	for i, value := range values {
		if value == nil {
			if nullable {
				continue
			}
			return fmt.Errorf("%s[%d] is null but column is not nullable", path, i)
		}

		switch columnType {
		case "string_ref", "ip_ref", "mac_ref":
			dictName, _ := column["dictionary"].(string)
			if dictName == "" {
				return fmt.Errorf("%s is %s without dictionary", path, columnType)
			}
			dict, ok := ctx.dictionaries[dictName].([]any)
			if !ok {
				return fmt.Errorf("%s references missing dictionary %q", path, dictName)
			}
			index, ok := integerValue(value)
			if !ok {
				return fmt.Errorf("%s[%d] is not an integer dictionary reference", path, i)
			}
			if index < 0 || index >= len(dict) {
				return fmt.Errorf("%s[%d] dictionary index out of bounds: %d", path, i, index)
			}
		case "actor_ref":
			if err := validateReference(path, i, value, ctx.actorRows, "actor"); err != nil {
				return err
			}
		case "link_ref":
			if err := validateReference(path, i, value, ctx.linkRows, "link"); err != nil {
				return err
			}
		case "evidence_ref":
			index, ok := integerValue(value)
			if !ok || index < 0 {
				return fmt.Errorf("%s[%d] is not a non-negative evidence reference", path, i)
			}
			if ctx.evidenceRows >= 0 && index >= ctx.evidenceRows {
				return fmt.Errorf("%s[%d] evidence reference out of bounds: %d", path, i, index)
			}
		case "array":
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("%s[%d] is not an array", path, i)
			}
		}
	}
	return nil
}

func collectTopologyShape(data map[string]any) (topologyShape, error) {
	types, ok := data["types"].(map[string]any)
	if !ok {
		return topologyShape{}, fmt.Errorf("data.types is not an object")
	}
	actorColumns, err := columnTypesFromTable(data["actors"], "data.actors")
	if err != nil {
		return topologyShape{}, err
	}
	linkColumns, err := columnTypesFromTable(data["links"], "data.links")
	if err != nil {
		return topologyShape{}, err
	}
	actorTypes, err := objectKeySet(types["actor_types"], "data.types.actor_types")
	if err != nil {
		return topologyShape{}, err
	}
	linkTypes, err := objectKeySet(types["link_types"], "data.types.link_types")
	if err != nil {
		return topologyShape{}, err
	}
	portTypes, err := optionalObjectKeySet(types["port_types"], "data.types.port_types")
	if err != nil {
		return topologyShape{}, err
	}

	evidenceTypes, err := columnTypesByRegistryObject(types["evidence_types"], "data.types.evidence_types")
	if err != nil {
		return topologyShape{}, err
	}
	tableTypes, err := columnTypesByRegistryObject(types["table_types"], "data.types.table_types")
	if err != nil {
		return topologyShape{}, err
	}
	tableTypeOwners, err := tableTypeOwners(types["table_types"], "data.types.table_types")
	if err != nil {
		return topologyShape{}, err
	}
	actorTables, relationshipTables, err := collectDetailTableColumnTypes(data["tables"])
	if err != nil {
		return topologyShape{}, err
	}
	scaleKeys, err := collectScaleKeys(data["presentation"])
	if err != nil {
		return topologyShape{}, err
	}

	return topologyShape{
		actorColumns:       actorColumns,
		linkColumns:        linkColumns,
		actorTypes:         actorTypes,
		linkTypes:          linkTypes,
		portTypes:          portTypes,
		evidenceTypes:      evidenceTypes,
		tableTypes:         tableTypes,
		tableTypeOwners:    tableTypeOwners,
		actorTables:        actorTables,
		relationshipTables: relationshipTables,
		scaleKeys:          scaleKeys,
	}, nil
}

func validateTypeColumns(data map[string]any, shape topologyShape) error {
	if _, ok := shape.actorColumns["type"]; !ok {
		return fmt.Errorf("data.actors is missing required type column")
	}
	if _, ok := shape.linkColumns["type"]; !ok {
		return fmt.Errorf("data.links is missing required type column")
	}
	dictionaries, _ := data["dictionaries"].(map[string]any)
	if err := validateTypeColumnValues("data.actors", data["actors"], shape.actorTypes, "actor", dictionaries); err != nil {
		return err
	}
	if err := validateTypeColumnValues("data.links", data["links"], shape.linkTypes, "link", dictionaries); err != nil {
		return err
	}
	return nil
}

func validateTypeColumnValues(path string, raw any, known map[string]struct{}, typeName string, dictionaries map[string]any) error {
	table, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	rows, err := decodedTableRows(table)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	columns, ok := table["columns"].([]any)
	if !ok {
		return fmt.Errorf("%s.columns is not an array", path)
	}
	values, ok := table["values"].([]any)
	if !ok {
		return fmt.Errorf("%s.values is not an array", path)
	}
	for i, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.columns[%d] is not an object", path, i)
		}
		if id, _ := column["id"].(string); id != "type" {
			continue
		}
		columnType, _ := column["type"].(string)
		dictionary, _ := column["dictionary"].(string)
		decoded, err := decodeColumn(path, i, rows, values[i])
		if err != nil {
			return err
		}
		for row, value := range decoded {
			id, ok := resolveStringValue(value, columnType, dictionary, dictionaries)
			if !ok || id == "" {
				return fmt.Errorf("%s.type[%d] is not a non-empty string", path, row)
			}
			if _, ok := known[id]; !ok {
				return fmt.Errorf("%s.type[%d] references unknown %s type %q", path, row, typeName, id)
			}
		}
		return nil
	}
	return fmt.Errorf("%s is missing required type column", path)
}

func resolveStringValue(value any, columnType, dictionary string, dictionaries map[string]any) (string, bool) {
	if text, ok := value.(string); ok {
		return text, true
	}
	if columnType != "string_ref" && columnType != "ip_ref" && columnType != "mac_ref" {
		return "", false
	}
	index, ok := integerValue(value)
	if !ok || dictionary == "" {
		return "", false
	}
	values, ok := dictionaries[dictionary].([]any)
	if !ok || index < 0 || index >= len(values) {
		return "", false
	}
	text, ok := values[index].(string)
	return text, ok
}

func validatePresentation(data map[string]any, shape topologyShape) error {
	types, _ := data["types"].(map[string]any)
	if err := validateActorTypePresentation(types["actor_types"], shape); err != nil {
		return err
	}
	if err := validateLinkTypePresentation(types["link_types"], shape); err != nil {
		return err
	}
	if err := validatePortTypePresentation(types["port_types"]); err != nil {
		return err
	}
	if err := validateGraphPresentation(data["presentation"], shape); err != nil {
		return err
	}
	return nil
}

func validateActorTypePresentation(raw any, shape topologyShape) error {
	actorTypes, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.types.actor_types is not an object")
	}
	for typeID, rawType := range actorTypes {
		actorType, ok := rawType.(map[string]any)
		if !ok {
			return fmt.Errorf("data.types.actor_types.%s is not an object", typeID)
		}
		presentation, ok := actorType["presentation"].(map[string]any)
		if !ok {
			continue
		}
		path := "data.types.actor_types." + typeID + ".presentation"
		if err := optionalEnum(path+".role", presentation["role"], "actor", "endpoint", "group"); err != nil {
			return err
		}
		if err := optionalEnum(path+".icon", presentation["icon"], iconTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".color_slot", presentation["color_slot"], colorSlotTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".opacity", presentation["opacity"], opacityTokens...); err != nil {
			return err
		}
		if err := validateBorderPresentation(path+".border", presentation["border"]); err != nil {
			return err
		}
		if err := validateAnnotationPresentation(path+".annotation", presentation["annotation"]); err != nil {
			return err
		}
		if err := validateActorSizePresentation(path+".size", presentation["size"], shape.actorColumns); err != nil {
			return err
		}
		if err := validateLabelPolicy(path+".label_policy", presentation["label_policy"], shape.actorColumns); err != nil {
			return err
		}
		if err := validateActorPortsPresentation(path+".ports", presentation["ports"], shape); err != nil {
			return err
		}
		if err := validateHoverPresentation(path+".hover", presentation["hover"], shape.actorColumns); err != nil {
			return err
		}
	}
	return nil
}

func validateLinkTypePresentation(raw any, shape topologyShape) error {
	linkTypes, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.types.link_types is not an object")
	}
	for typeID, rawType := range linkTypes {
		linkType, ok := rawType.(map[string]any)
		if !ok {
			return fmt.Errorf("data.types.link_types.%s is not an object", typeID)
		}
		presentation, ok := linkType["presentation"].(map[string]any)
		if !ok {
			continue
		}
		path := "data.types.link_types." + typeID + ".presentation"
		if err := optionalEnum(path+".color_slot", presentation["color_slot"], colorSlotTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".opacity", presentation["opacity"], opacityTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".line_style", presentation["line_style"], "solid", "dashed", "dotted"); err != nil {
			return err
		}
		if err := optionalEnum(path+".width", presentation["width"], widthTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".curve", presentation["curve"], "straight", "clockwise", "counter_clockwise", "auto"); err != nil {
			return err
		}
		if err := optionalEnum(path+".arrow", presentation["arrow"], "none", "forward", "reverse", "both", "auto"); err != nil {
			return err
		}
		if err := validateLinkVariablePresentation(path+".variable", presentation["variable"], shape); err != nil {
			return err
		}
		if err := validateHoverPresentation(path+".hover", presentation["hover"], shape.linkColumns); err != nil {
			return err
		}
	}
	return nil
}

func validatePortTypePresentation(raw any) error {
	if raw == nil {
		return nil
	}
	portTypes, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.types.port_types is not an object")
	}
	for typeID, rawType := range portTypes {
		portType, ok := rawType.(map[string]any)
		if !ok {
			return fmt.Errorf("data.types.port_types.%s is not an object", typeID)
		}
		presentation, ok := portType["presentation"].(map[string]any)
		if !ok {
			continue
		}
		path := "data.types.port_types." + typeID + ".presentation"
		if err := optionalEnum(path+".color_slot", presentation["color_slot"], colorSlotTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".opacity", presentation["opacity"], opacityTokens...); err != nil {
			return err
		}
	}
	return nil
}

func validateGraphPresentation(raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	presentation, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.presentation is not an object")
	}
	if err := validateSelectionPresentation(presentation["selection"], shape); err != nil {
		return err
	}
	if err := validateLegendPresentation(presentation["legend"], shape); err != nil {
		return err
	}
	return nil
}

func validateBorderPresentation(path string, raw any) error {
	if raw == nil {
		return nil
	}
	border, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if err := optionalEnum(path+".color_slot", border["color_slot"], colorSlotTokens...); err != nil {
		return err
	}
	return optionalEnum(path+".style", border["style"], "solid", "dashed", "dotted")
}

func validateAnnotationPresentation(path string, raw any) error {
	if raw == nil {
		return nil
	}
	annotation, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if err := optionalEnum(path+".color_slot", annotation["color_slot"], colorSlotTokens...); err != nil {
		return err
	}
	return optionalEnum(path+".style", annotation["style"], "ring", "dot", "none")
}

func validateActorSizePresentation(path string, raw any, actorColumns map[string]string) error {
	if raw == nil {
		return nil
	}
	size, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	mode, err := requiredEnum(path+".mode", size["mode"], "fixed", "link_count", "metric")
	if err != nil {
		return err
	}
	if mode == "metric" {
		column, _ := size["metric_column"].(string)
		if column == "" {
			return fmt.Errorf("%s.metric_column is required when mode is metric", path)
		}
		columnType, ok := actorColumns[column]
		if !ok {
			return fmt.Errorf("%s.metric_column references unknown actor column %q", path, column)
		}
		if !isNumericColumnType(columnType) {
			return fmt.Errorf("%s.metric_column references non-numeric actor column %q (%s)", path, column, columnType)
		}
	}
	return nil
}

func validateLabelPolicy(path string, raw any, actorColumns map[string]string) error {
	if raw == nil {
		return nil
	}
	policy, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if err := optionalEnum(path+".fallback", policy["fallback"], "type_label", "row_index", "none"); err != nil {
		return err
	}
	if err := optionalEnum(path+".array", policy["array"], "reject", "first", "summarize"); err != nil {
		return err
	}
	columns, ok := policy["columns"].([]any)
	if !ok {
		return nil
	}
	for i, rawColumn := range columns {
		column, ok := rawColumn.(string)
		if !ok || column == "" {
			return fmt.Errorf("%s.columns[%d] is not a non-empty string", path, i)
		}
		columnType, ok := actorColumns[column]
		if !ok {
			return fmt.Errorf("%s.columns[%d] references unknown actor column %q", path, i, column)
		}
		if !isDisplayColumnType(columnType) {
			return fmt.Errorf("%s.columns[%d] references non-display actor column %q (%s)", path, i, column, columnType)
		}
	}
	return nil
}

func validateActorPortsPresentation(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	ports, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	showBullets, _ := ports["show_bullets"].(bool)
	sources, ok := ports["sources"].([]any)
	if showBullets && (!ok || len(sources) == 0) {
		return fmt.Errorf("%s.sources is required when show_bullets is true", path)
	}
	if !ok {
		return nil
	}
	for i, rawSource := range sources {
		source, ok := rawSource.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.sources[%d] is not an object", path, i)
		}
		if err := validatePortSourcePresentation(fmt.Sprintf("%s.sources[%d]", path, i), source, shape); err != nil {
			return err
		}
	}
	return nil
}

func validatePortSourcePresentation(path string, source map[string]any, shape topologyShape) error {
	sourceKind, err := requiredEnum(path+".source", source["source"], "links", "evidence", "actor_table")
	if err != nil {
		return err
	}
	defaultType, _ := source["default_type"].(string)
	if defaultType != "" {
		if _, ok := shape.portTypes[defaultType]; !ok {
			return fmt.Errorf("%s.default_type references unknown port type %q", path, defaultType)
		}
	}

	var columns map[string]string
	switch sourceKind {
	case "links":
		columns = shape.linkColumns
	case "evidence":
		evidence, _ := source["evidence"].(string)
		if evidence == "" {
			return fmt.Errorf("%s.evidence is required when source is evidence", path)
		}
		columns = shape.evidenceTypes[evidence]
		if columns == nil {
			return fmt.Errorf("%s.evidence references unknown evidence type %q", path, evidence)
		}
	case "actor_table":
		table, _ := source["table"].(string)
		if table == "" {
			return fmt.Errorf("%s.table is required when source is actor_table", path)
		}
		columns = shape.actorTables[table]
		if columns == nil {
			columns = shape.tableTypes[table]
		}
		if columns == nil {
			return fmt.Errorf("%s.table references unknown actor table %q", path, table)
		}
	}

	for _, field := range []string{"actor_column", "name_column"} {
		column, _ := source[field].(string)
		if column == "" {
			return fmt.Errorf("%s.%s is required", path, field)
		}
		if _, ok := columns[column]; !ok {
			return fmt.Errorf("%s.%s references unknown source column %q", path, field, column)
		}
	}
	actorColumn, _ := source["actor_column"].(string)
	if columnType := columns[actorColumn]; columnType != "actor_ref" {
		return fmt.Errorf("%s.actor_column references non-actor_ref source column %q (%s)", path, actorColumn, columnType)
	}
	nameColumn, _ := source["name_column"].(string)
	if columnType := columns[nameColumn]; !isDisplayColumnType(columnType) {
		return fmt.Errorf("%s.name_column references non-display source column %q (%s)", path, nameColumn, columnType)
	}
	for _, field := range []string{"type_column", "status_column", "mode_column", "role_column", "sources_column"} {
		column, _ := source[field].(string)
		if column == "" {
			continue
		}
		if _, ok := columns[column]; !ok {
			if sourceKind == "actor_table" {
				continue
			}
			return fmt.Errorf("%s.%s references unknown source column %q", path, field, column)
		}
	}
	return nil
}

func validateLinkVariablePresentation(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	variable, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	channel, err := requiredEnum(path+".channel", variable["channel"], "width", "opacity")
	if err != nil {
		return err
	}
	scaleKey, _ := variable["scale_key"].(string)
	if scaleKey == "" {
		return fmt.Errorf("%s.scale_key is required", path)
	}
	if _, ok := shape.scaleKeys[scaleKey]; !ok {
		return fmt.Errorf("%s.scale_key references unknown presentation scale key %q", path, scaleKey)
	}
	valueColumn, _ := variable["value_column"].(string)
	if valueColumn == "" {
		return fmt.Errorf("%s.value_column is required", path)
	}
	columnType, ok := shape.linkColumns[valueColumn]
	if !ok {
		return fmt.Errorf("%s.value_column references unknown link column %q", path, valueColumn)
	}
	if !isNumericColumnType(columnType) {
		return fmt.Errorf("%s.value_column references non-numeric link column %q (%s)", path, valueColumn, columnType)
	}
	allowed := widthTokens
	if channel == "opacity" {
		allowed = opacityTokens
	}
	if err := optionalEnum(path+".min", variable["min"], allowed...); err != nil {
		return err
	}
	return optionalEnum(path+".max", variable["max"], allowed...)
}

func validateHoverPresentation(path string, raw any, columns map[string]string) error {
	if raw == nil {
		return nil
	}
	hover, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	fields, ok := hover["fields"].([]any)
	if !ok {
		return nil
	}
	for i, rawField := range fields {
		field, ok := rawField.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.fields[%d] is not an object", path, i)
		}
		key, _ := field["key"].(string)
		if key == "" {
			return fmt.Errorf("%s.fields[%d].key is empty", path, i)
		}
		columnType, ok := columns[key]
		if !ok {
			return fmt.Errorf("%s.fields[%d].key references unknown column %q", path, i, key)
		}
		if !isDisplayColumnType(columnType) {
			return fmt.Errorf("%s.fields[%d].key references non-display column %q (%s)", path, i, key, columnType)
		}
	}
	return nil
}

func validateSelectionPresentation(raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	selection, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.presentation.selection is not an object")
	}
	rawActorClick := selection["actor_click"]
	if rawActorClick == nil {
		return nil
	}
	actorClick, ok := rawActorClick.(map[string]any)
	if !ok {
		return fmt.Errorf("data.presentation.selection.actor_click is not an object")
	}
	mode, err := requiredEnum("data.presentation.selection.actor_click.mode", actorClick["mode"], "none", "highlight_connections", "highlight_path")
	if err != nil {
		return err
	}
	if mode != "highlight_path" {
		return nil
	}
	table, _ := actorClick["path_table"].(string)
	if table == "" {
		return fmt.Errorf("data.presentation.selection.actor_click.path_table is required when mode is highlight_path")
	}
	columns := shape.actorTables[table]
	if columns == nil {
		columns = shape.tableTypes[table]
		if columns != nil && shape.tableTypeOwners[table] != "actor" {
			return fmt.Errorf("data.presentation.selection.actor_click.path_table references non-actor table %q", table)
		}
	}
	if columns == nil {
		return fmt.Errorf("data.presentation.selection.actor_click.path_table references unknown actor table %q", table)
	}
	for _, field := range []string{"path_actor_column", "path_order_column"} {
		column, _ := actorClick[field].(string)
		if column == "" {
			return fmt.Errorf("data.presentation.selection.actor_click.%s is required when mode is highlight_path", field)
		}
		if _, ok := columns[column]; !ok {
			return fmt.Errorf("data.presentation.selection.actor_click.%s references unknown path table column %q", field, column)
		}
	}
	actorColumn, _ := actorClick["path_actor_column"].(string)
	if columnType := columns[actorColumn]; columnType != "actor_ref" {
		return fmt.Errorf("data.presentation.selection.actor_click.path_actor_column references non-actor_ref path table column %q (%s)", actorColumn, columnType)
	}
	ownerColumn, _ := actorClick["path_owner_column"].(string)
	if ownerColumn != "" {
		columnType, ok := columns[ownerColumn]
		if !ok {
			return fmt.Errorf("data.presentation.selection.actor_click.path_owner_column references unknown path table column %q", ownerColumn)
		}
		if columnType != "actor_ref" {
			return fmt.Errorf("data.presentation.selection.actor_click.path_owner_column references non-actor_ref path table column %q (%s)", ownerColumn, columnType)
		}
	}
	orderColumn, _ := actorClick["path_order_column"].(string)
	if columnType := columns[orderColumn]; !isNumericColumnType(columnType) {
		return fmt.Errorf("data.presentation.selection.actor_click.path_order_column references non-numeric path table column %q (%s)", orderColumn, columnType)
	}
	return nil
}

func validateLegendPresentation(raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	legend, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.presentation.legend is not an object")
	}
	if err := validateLegendEntries("data.presentation.legend.actors", legend["actors"], shape.actorTypes); err != nil {
		return err
	}
	if err := validateLegendEntries("data.presentation.legend.links", legend["links"], shape.linkTypes); err != nil {
		return err
	}
	return validateLegendEntries("data.presentation.legend.ports", legend["ports"], shape.portTypes)
}

func validateLegendEntries(path string, raw any, known map[string]struct{}) error {
	if raw == nil {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", path)
	}
	for i, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] is not an object", path, i)
		}
		typeID, _ := entry["type"].(string)
		if typeID == "" {
			return fmt.Errorf("%s[%d].type is empty", path, i)
		}
		if _, ok := known[typeID]; !ok {
			return fmt.Errorf("%s[%d].type references unknown type %q", path, i, typeID)
		}
	}
	return nil
}

func validateReference(path string, row int, value any, maxRows int, name string) error {
	index, ok := integerValue(value)
	if !ok {
		return fmt.Errorf("%s[%d] is not an integer %s reference", path, row, name)
	}
	if index < 0 || index >= maxRows {
		return fmt.Errorf("%s[%d] %s reference out of bounds: %d", path, row, name, index)
	}
	return nil
}

func columnTypesFromTable(raw any, path string) (map[string]string, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	return columnTypesFromRawColumns(table["columns"], path+".columns")
}

func columnTypesFromRawColumns(raw any, path string) (map[string]string, error) {
	columns, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an array", path)
	}
	types := make(map[string]string, len(columns))
	for i, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] is not an object", path, i)
		}
		id, _ := column["id"].(string)
		columnType, _ := column["type"].(string)
		if id == "" {
			return nil, fmt.Errorf("%s[%d].id is empty", path, i)
		}
		if columnType == "" {
			return nil, fmt.Errorf("%s[%d].type is empty", path, i)
		}
		types[id] = columnType
	}
	return types, nil
}

func objectKeySet(raw any, path string) (map[string]struct{}, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	keys := make(map[string]struct{}, len(obj))
	for key := range obj {
		keys[key] = struct{}{}
	}
	return keys, nil
}

func optionalObjectKeySet(raw any, path string) (map[string]struct{}, error) {
	if raw == nil {
		return map[string]struct{}{}, nil
	}
	return objectKeySet(raw, path)
}

func columnTypesByRegistryObject(raw any, path string) (map[string]map[string]string, error) {
	out := make(map[string]map[string]string)
	if raw == nil {
		return out, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	for id, rawType := range obj {
		typeObj, ok := rawType.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not an object", path, id)
		}
		columns, err := columnTypesFromRawColumns(typeObj["columns"], path+"."+id+".columns")
		if err != nil {
			return nil, err
		}
		out[id] = columns
	}
	return out, nil
}

func tableTypeOwners(raw any, path string) (map[string]string, error) {
	out := make(map[string]string)
	if raw == nil {
		return out, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	for id, rawType := range obj {
		typeObj, ok := rawType.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not an object", path, id)
		}
		owner, _ := typeObj["owner"].(string)
		if owner != "" {
			out[id] = owner
		}
	}
	return out, nil
}

func collectDetailTableColumnTypes(raw any) (map[string]map[string]string, map[string]map[string]string, error) {
	actorTables := make(map[string]map[string]string)
	relationshipTables := make(map[string]map[string]string)
	if raw == nil {
		return actorTables, relationshipTables, nil
	}
	tables, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("data.tables is not an object")
	}
	if err := collectDetailTableGroupColumnTypes(tables["actor"], "data.tables.actor", actorTables); err != nil {
		return nil, nil, err
	}
	if err := collectDetailTableGroupColumnTypes(tables["relationship"], "data.tables.relationship", relationshipTables); err != nil {
		return nil, nil, err
	}
	return actorTables, relationshipTables, nil
}

func collectDetailTableGroupColumnTypes(raw any, path string, out map[string]map[string]string) error {
	if raw == nil {
		return nil
	}
	group, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	for name, rawDetail := range group {
		detail, ok := rawDetail.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%s is not an object", path, name)
		}
		columns, err := columnTypesFromTable(detail["table"], path+"."+name+".table")
		if err != nil {
			return err
		}
		out[name] = columns
	}
	return nil
}

func collectScaleKeys(raw any) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	if raw == nil {
		return keys, nil
	}
	presentation, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.presentation is not an object")
	}
	scaleKeys, ok := presentation["scale_keys"].(map[string]any)
	if !ok {
		return keys, nil
	}
	for key := range scaleKeys {
		keys[key] = struct{}{}
	}
	return keys, nil
}

func requiredEnum(path string, raw any, allowed ...string) (string, error) {
	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is not a non-empty string", path)
	}
	if !stringInSet(value, allowed) {
		return "", fmt.Errorf("%s has unsupported value %q", path, value)
	}
	return value, nil
}

func optionalEnum(path string, raw any, allowed ...string) error {
	if raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return fmt.Errorf("%s is not a non-empty string", path)
	}
	if !stringInSet(value, allowed) {
		return fmt.Errorf("%s has unsupported value %q", path, value)
	}
	return nil
}

func stringInSet(value string, allowed []string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func isNumericColumnType(columnType string) bool {
	return columnType == "int" || columnType == "uint" || columnType == "float" || columnType == "duration"
}

func isDisplayColumnType(columnType string) bool {
	switch columnType {
	case "bool", "int", "uint", "float", "string", "string_ref", "timestamp", "duration", "ip", "ip_ref", "mac", "mac_ref":
		return true
	default:
		return false
	}
}

var (
	colorSlotTokens = []string{
		"primary", "secondary", "accent", "self", "neutral", "muted", "dim", "derived",
		"info", "structural", "warning", "success", "danger", "blue", "green", "orange",
		"purple", "cyan", "yellow", "teal", "gray",
	}
	opacityTokens = []string{"normal", "muted", "faded"}
	widthTokens   = []string{"thin", "normal", "thick", "emphasis"}
	iconTokens    = []string{
		"router", "switch", "firewall", "access_point", "server", "storage", "load_balancer",
		"printer", "phone", "ups", "camera", "process", "agent", "netdata-agent", "parent",
		"remote-endpoint", "local-endpoint", "segment", "self", "ip", "cloud", "container",
		"vm", "database", "service", "datacenter", "cluster", "host", "network", "datastore",
		"datastore_cluster", "resource_pool",
	}
)

func decodedTableRows(raw any) (int, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("table is not an object")
	}
	rows, ok := integerValue(table["rows"])
	if !ok || rows < 0 {
		return 0, fmt.Errorf("rows is not a non-negative integer")
	}
	return rows, nil
}

func integerValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case float64:
		if math.Trunc(value) != value {
			return 0, false
		}
		return int(value), true
	case json.Number:
		n, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}
