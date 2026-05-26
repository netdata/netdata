// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
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

// ValidateDecodedResponse validates a full topology v1 Function response.
// Metadata-only Function info responses intentionally omit data and must not be
// passed here.
func ValidateDecodedResponse(payload any) error {
	obj, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("response is not an object")
	}
	data, ok := obj["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("response.data is not an object")
	}
	if data["schema_version"] != SchemaVersion {
		return fmt.Errorf("response.data.schema_version is not %q", SchemaVersion)
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
	if err := validateCorrelation(data["correlation"], shape, ctx); err != nil {
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

func GraphRowsFromDecodedData(raw any) (int, error) {
	data, ok := raw.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("data is not an object")
	}
	actorRows, err := decodedTableRows(data["actors"])
	if err != nil {
		return 0, fmt.Errorf("data.actors: %w", err)
	}
	linkRows, err := decodedTableRows(data["links"])
	if err != nil {
		return 0, fmt.Errorf("data.links: %w", err)
	}
	return max(actorRows, linkRows), nil
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
		section, ok := rawSection.(map[string]any)
		if !ok {
			return fmt.Errorf("data.evidence.%s is not an object", name)
		}
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

	seenColumns := make(map[string]struct{}, len(columns))
	for i := range columns {
		column, ok := columns[i].(map[string]any)
		if !ok {
			return 0, fmt.Errorf("%s.columns[%d] is not an object", path, i)
		}
		columnID, _ := column["id"].(string)
		if columnID == "" {
			return 0, fmt.Errorf("%s.columns[%d].id is required", path, i)
		}
		if _, ok := seenColumns[columnID]; ok {
			return 0, fmt.Errorf("%s.columns[%d].id duplicates column %q", path, i, columnID)
		}
		seenColumns[columnID] = struct{}{}
		columnType, _ := column["type"].(string)
		if columnType == "" {
			return 0, fmt.Errorf("%s.columns[%d].type is required", path, i)
		}
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
		case "bool":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s[%d] is not a bool", path, i)
			}
		case "int":
			if _, ok := integerValue(value); !ok {
				return fmt.Errorf("%s[%d] is not an integer", path, i)
			}
		case "uint":
			n, ok := integerValue(value)
			if !ok || n < 0 {
				return fmt.Errorf("%s[%d] is not a non-negative integer", path, i)
			}
		case "float", "duration":
			if _, ok := numberValue(value); !ok {
				return fmt.Errorf("%s[%d] is not a number", path, i)
			}
		case "string", "ip", "mac", "timestamp":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s[%d] is not a string", path, i)
			}
		case "json":
			// Any decoded JSON value is valid for a json column.
		default:
			return fmt.Errorf("%s has unsupported column type %q", path, columnType)
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

func validateStringColumnValuesInSet(path string, raw any, columnID string, known map[string]struct{}, typeName string, dictionaries map[string]any) error {
	_, err := collectStringColumnValuesInSet(path, raw, columnID, known, typeName, dictionaries)
	return err
}

func collectStringColumnValuesInSet(path string, raw any, columnID string, known map[string]struct{}, typeName string, dictionaries map[string]any) (map[string]struct{}, error) {
	table, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	rows, err := decodedTableRows(table)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	columns, ok := table["columns"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s.columns is not an array", path)
	}
	values, ok := table["values"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s.values is not an array", path)
	}
	for i, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.columns[%d] is not an object", path, i)
		}
		if id, _ := column["id"].(string); id != columnID {
			continue
		}
		columnType, _ := column["type"].(string)
		dictionary, _ := column["dictionary"].(string)
		decoded, err := decodeColumn(path, i, rows, values[i])
		if err != nil {
			return nil, err
		}
		found := make(map[string]struct{}, len(decoded))
		for row, value := range decoded {
			id, ok := resolveStringValue(value, columnType, dictionary, dictionaries)
			if !ok || id == "" {
				return nil, fmt.Errorf("%s.%s[%d] is not a non-empty string", path, columnID, row)
			}
			if _, ok := known[id]; !ok {
				return nil, fmt.Errorf("%s.%s[%d] references unknown %s %q", path, columnID, row, typeName, id)
			}
			found[id] = struct{}{}
		}
		return found, nil
	}
	return nil, fmt.Errorf("%s is missing required %s column", path, columnID)
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
	if err := validateTableTypePresentation(types["table_types"], shape); err != nil {
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
		if _, ok := presentation["label"]; ok {
			if err := validateRequiredString(path+".label", presentation["label"]); err != nil {
				return err
			}
		}
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
		if err := validateActorLayoutPresentation(path+".layout", presentation["layout"]); err != nil {
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
		if err := validateModalPresentation(path+".modal", presentation["modal"], shape); err != nil {
			return err
		}
		if err := validateActorSearchPolicy("data.types.actor_types."+typeID+".search", actorType["search"], shape.actorColumns); err != nil {
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
		if err := optionalEnum("data.types.link_types."+typeID+".semantic_role", linkType["semantic_role"], linkSemanticRoleTokens...); err != nil {
			return err
		}
		presentation, ok := linkType["presentation"].(map[string]any)
		if !ok {
			continue
		}
		path := "data.types.link_types." + typeID + ".presentation"
		if _, ok := presentation["label"]; ok {
			if err := validateRequiredString(path+".label", presentation["label"]); err != nil {
				return err
			}
		}
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
		if err := validateLinkLayoutPresentation(path+".layout", presentation["layout"]); err != nil {
			return err
		}
		if err := validateModalPresentation(path+".modal", presentation["modal"], shape); err != nil {
			return err
		}
	}
	return nil
}

func validateLinkLayoutPresentation(path string, raw any) error {
	if raw == nil {
		return nil
	}
	layout, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if err := optionalEnum(path+".strength", layout["strength"], layoutStrengthTokens...); err != nil {
		return err
	}
	return optionalEnum(path+".distance", layout["distance"], layoutDistanceTokens...)
}

func validateActorLayoutPresentation(path string, raw any) error {
	if raw == nil {
		return nil
	}
	layout, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	return optionalEnum(path+".repulsion", layout["repulsion"], layoutStrengthTokens...)
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
		if _, ok := presentation["label"]; ok {
			if err := validateRequiredString(path+".label", presentation["label"]); err != nil {
				return err
			}
		}
		if err := optionalEnum(path+".color_slot", presentation["color_slot"], colorSlotTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+".opacity", presentation["opacity"], opacityTokens...); err != nil {
			return err
		}
	}
	return nil
}

func validateTableTypePresentation(raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	tableTypes, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.types.table_types is not an object")
	}
	for typeID, rawType := range tableTypes {
		tableType, ok := rawType.(map[string]any)
		if !ok {
			return fmt.Errorf("data.types.table_types.%s is not an object", typeID)
		}
		presentation, ok := tableType["presentation"].(map[string]any)
		if !ok {
			continue
		}
		path := "data.types.table_types." + typeID + ".presentation"
		if _, ok := presentation["label"]; ok {
			if err := validateRequiredString(path+".label", presentation["label"]); err != nil {
				return err
			}
		}
		if err := optionalEnum(path+".default_visibility", presentation["default_visibility"], "table", "expanded", "hidden", "debug"); err != nil {
			return err
		}
		columns := shape.tableTypes[typeID]
		if err := validateModalColumns(path+".columns", presentation["columns"], columns); err != nil {
			return err
		}
	}
	return nil
}

func validateModalPresentation(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	modal, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if err := validateModalLabelsPresentation(path+".labels", modal["labels"], shape); err != nil {
		return err
	}
	if err := validateModalMiniTopologyPresentation(path+".mini_topology", modal["mini_topology"], shape); err != nil {
		return err
	}
	rawSections, hasSections := modal["sections"]
	if !hasSections || rawSections == nil {
		return nil
	}
	sections, ok := rawSections.([]any)
	if !ok {
		return fmt.Errorf("%s.sections is not an array", path)
	}
	seenIDs := make(map[string]struct{}, len(sections))
	for i, rawSection := range sections {
		section, ok := rawSection.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.sections[%d] is not an object", path, i)
		}
		id, _ := section["id"].(string)
		if id != "" {
			if _, ok := seenIDs[id]; ok {
				return fmt.Errorf("%s.sections[%d].id duplicates modal section id %q", path, i, id)
			}
			seenIDs[id] = struct{}{}
		}
		if err := validateModalSection(fmt.Sprintf("%s.sections[%d]", path, i), section, shape); err != nil {
			return err
		}
	}
	return nil
}

func validateModalLabelsPresentation(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	labels, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	table, _ := labels["table"].(string)
	if table == "" {
		table = "actor_labels"
	}
	columns := shape.actorTables[table]
	if columns == nil {
		columns = shape.tableTypes[table]
	}
	if columns == nil {
		return fmt.Errorf("%s.table references unknown actor labels table %q", path, table)
	}
	for field, defaultColumn := range map[string]string{
		"actor_column":       "actor",
		"key_column":         "key",
		"value_column":       "value",
		"source_column":      "source",
		"kind_column":        "kind",
		"value_index_column": "value_index",
	} {
		_, hasField := labels[field]
		column, _ := labels[field].(string)
		if column == "" {
			column = defaultColumn
		}
		columnType, ok := columns[column]
		if !ok {
			if (field == "source_column" || field == "kind_column" || field == "value_index_column") && !hasField {
				continue
			}
			return fmt.Errorf("%s.%s references unknown column %q", path, field, column)
		}
		if field == "actor_column" && columnType != "actor_ref" {
			return fmt.Errorf("%s.%s references non-actor_ref column %q (%s)", path, field, column, columnType)
		}
	}
	if err := validateModalLabelIdentification(path+".identification", labels["identification"]); err != nil {
		return err
	}
	return nil
}

func validateModalLabelIdentification(path string, raw any) error {
	if raw == nil {
		return nil
	}
	identification, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if rawFields := identification["fields"]; rawFields != nil {
		fields, ok := rawFields.([]any)
		if !ok {
			return fmt.Errorf("%s.fields is not an array", path)
		}
		for i, rawField := range fields {
			field, ok := rawField.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.fields[%d] is not an object", path, i)
			}
			key, ok := field["key"].(string)
			if !ok || key == "" {
				return fmt.Errorf("%s.fields[%d].key is required", path, i)
			}
			label, ok := field["label"].(string)
			if !ok || label == "" {
				return fmt.Errorf("%s.fields[%d].label is required", path, i)
			}
			if rawMaxValues, ok := field["max_values"]; ok {
				maxValues, ok := integerValue(rawMaxValues)
				if !ok || maxValues < 1 {
					return fmt.Errorf("%s.fields[%d].max_values must be a positive integer", path, i)
				}
			}
		}
	}
	return nil
}

func validateModalMiniTopologyPresentation(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	mini, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if _, present := mini["depth"]; present {
		depth, ok := integerValue(mini["depth"])
		if !ok {
			return fmt.Errorf("%s.depth is not an integer", path)
		}
		if depth != 1 {
			return fmt.Errorf("%s.depth must be 1", path)
		}
	}
	if err := validateKnownLinkTypeList(path+".include_link_types", mini["include_link_types"], shape); err != nil {
		return err
	}
	return validateKnownLinkTypeList(path+".exclude_link_types", mini["exclude_link_types"], shape)
}

func validateKnownLinkTypeList(path string, raw any, shape topologyShape) error {
	if raw == nil {
		return nil
	}
	values, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", path)
	}
	for i, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || value == "" {
			return fmt.Errorf("%s[%d] is not a non-empty string", path, i)
		}
		if _, ok := shape.linkTypes[value]; !ok {
			return fmt.Errorf("%s[%d] references unknown link type %q", path, i, value)
		}
	}
	return nil
}

func validateModalSection(path string, section map[string]any, shape topologyShape) error {
	if err := validateRequiredString(path+".id", section["id"]); err != nil {
		return err
	}
	if err := validateRequiredString(path+".label", section["label"]); err != nil {
		return err
	}
	columns, err := validateModalSource(path+".source", section["source"], shape)
	if err != nil {
		return err
	}
	if err := validateModalOwnerFilter(path+".owner_filter", section["owner_filter"], columns); err != nil {
		return err
	}
	if err := validateModalRowFilters(path+".row_filters", section["row_filters"], columns); err != nil {
		return err
	}
	rawColumns, ok := section["columns"].([]any)
	if !ok {
		return fmt.Errorf("%s.columns is not an array", path)
	}
	if len(rawColumns) == 0 {
		return fmt.Errorf("%s.columns must not be empty", path)
	}
	if err := validateModalColumns(path+".columns", section["columns"], columns); err != nil {
		return err
	}
	return validateModalSort(path+".sort", section["sort"], section["columns"])
}

func validateModalSource(path string, raw any, shape topologyShape) (map[string]string, error) {
	source, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", path)
	}
	kind, err := requiredEnum(path+".kind", source["kind"], "actors", "links", "evidence", "actor_table", "relationship_table")
	if err != nil {
		return nil, err
	}
	switch kind {
	case "actors":
		return shape.actorColumns, nil
	case "links":
		return shape.linkColumns, nil
	case "evidence":
		evidence, _ := source["evidence"].(string)
		if evidence == "" {
			return nil, fmt.Errorf("%s.evidence is required when kind is evidence", path)
		}
		columns := shape.evidenceTypes[evidence]
		if columns == nil {
			return nil, fmt.Errorf("%s.evidence references unknown evidence type %q", path, evidence)
		}
		return columns, nil
	case "actor_table":
		table, _ := source["table"].(string)
		if table == "" {
			return nil, fmt.Errorf("%s.table is required when kind is actor_table", path)
		}
		columns := shape.actorTables[table]
		if columns == nil {
			columns = shape.tableTypes[table]
		}
		if columns == nil {
			return nil, fmt.Errorf("%s.table references unknown actor table %q", path, table)
		}
		return columns, nil
	case "relationship_table":
		table, _ := source["table"].(string)
		if table == "" {
			return nil, fmt.Errorf("%s.table is required when kind is relationship_table", path)
		}
		columns := shape.relationshipTables[table]
		if columns == nil {
			columns = shape.tableTypes[table]
		}
		if columns == nil {
			return nil, fmt.Errorf("%s.table references unknown relationship table %q", path, table)
		}
		return columns, nil
	default:
		return nil, fmt.Errorf("%s.kind has unsupported value %q", path, kind)
	}
}

func validateModalOwnerFilter(path string, raw any, columns map[string]string) error {
	if raw == nil {
		return nil
	}
	filter, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	mode, err := requiredEnum(path+".mode", filter["mode"], "none", "actor_column", "link_column", "incident_link", "incident_evidence", "selected_link")
	if err != nil {
		return err
	}
	switch mode {
	case "actor_column":
		return validateModalColumnRef(path+".actor_column", filter["actor_column"], columns, "actor_ref", false)
	case "link_column", "selected_link":
		return validateModalColumnRef(path+".link_column", filter["link_column"], columns, "link_ref", false)
	case "incident_link", "incident_evidence":
		if err := validateModalColumnRef(path+".src_actor_column", filter["src_actor_column"], columns, "actor_ref", false); err != nil {
			return err
		}
		return validateModalColumnRef(path+".dst_actor_column", filter["dst_actor_column"], columns, "actor_ref", false)
	default:
		return nil
	}
}

func validateModalRowFilters(path string, raw any, columns map[string]string) error {
	if raw == nil {
		return nil
	}
	filters, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", path)
	}
	for i, rawFilter := range filters {
		filter, ok := rawFilter.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] is not an object", path, i)
		}
		filterPath := fmt.Sprintf("%s[%d]", path, i)
		if err := validateModalColumnRef(filterPath+".column", filter["column"], columns, "", false); err != nil {
			return err
		}
		op, err := requiredEnum(filterPath+".op", filter["op"], "eq", "ne", "in", "not_in", "exists", "missing")
		if err != nil {
			return err
		}
		switch op {
		case "eq", "ne":
			if _, ok := filter["value"]; !ok {
				return fmt.Errorf("%s.value is required when op is %s", filterPath, op)
			}
		case "in", "not_in":
			values, ok := filter["values"].([]any)
			if !ok {
				return fmt.Errorf("%s.values is required when op is %s", filterPath, op)
			}
			if len(values) == 0 {
				return fmt.Errorf("%s.values must not be empty when op is %s", filterPath, op)
			}
		}
	}
	return nil
}

func validateModalColumns(path string, raw any, sourceColumns map[string]string) error {
	if raw == nil {
		return nil
	}
	columns, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", path)
	}
	seenIDs := make(map[string]struct{}, len(columns))
	for i, rawColumn := range columns {
		column, ok := rawColumn.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] is not an object", path, i)
		}
		columnPath := fmt.Sprintf("%s[%d]", path, i)
		if err := validateRequiredString(columnPath+".id", column["id"]); err != nil {
			return err
		}
		id := column["id"].(string)
		if _, ok := seenIDs[id]; ok {
			return fmt.Errorf("%s[%d].id duplicates modal column id %q", path, i, id)
		}
		seenIDs[id] = struct{}{}
		if err := validateRequiredString(columnPath+".label", column["label"]); err != nil {
			return err
		}
		if err := optionalEnum(columnPath+".cell", column["cell"], "text", "number", "badge", "actor_link", "timestamp", "duration", "endpoint", "array_count", "debug_json"); err != nil {
			return err
		}
		if err := optionalEnum(columnPath+".visibility", column["visibility"], "table", "expanded", "hidden", "debug"); err != nil {
			return err
		}
		if err := optionalEnum(columnPath+".align", column["align"], "left", "center", "right"); err != nil {
			return err
		}
		if err := validateModalProjection(columnPath+".projection", column["projection"], sourceColumns); err != nil {
			return err
		}
		if err := validateModalBadgeMap(columnPath+".badge_map", column["badge_map"]); err != nil {
			return err
		}
	}
	return nil
}

func validateModalProjection(path string, raw any, columns map[string]string) error {
	projection, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	kind, err := requiredEnum(path+".kind", projection["kind"],
		"direct", "actor_ref_label", "opposite_actor", "formatted_endpoint", "label_lookup",
		"json_path", "const", "coalesce", "selected_side_endpoint")
	if err != nil {
		return err
	}
	switch kind {
	case "direct":
		return validateModalColumnRef(path+".column", projection["column"], columns, "", false)
	case "actor_ref_label":
		return validateModalColumnRef(path+".actor_column", projection["actor_column"], columns, "actor_ref", false)
	case "opposite_actor":
		if err := validateModalColumnRef(path+".src_actor_column", projection["src_actor_column"], columns, "actor_ref", false); err != nil {
			return err
		}
		return validateModalColumnRef(path+".dst_actor_column", projection["dst_actor_column"], columns, "actor_ref", false)
	case "formatted_endpoint":
		if stringValue(projection["ip_column"]) == "" && stringValue(projection["port_column"]) == "" {
			return fmt.Errorf("%s requires ip_column or port_column when kind is formatted_endpoint", path)
		}
		if err := validateModalColumnRef(path+".ip_column", projection["ip_column"], columns, "", true); err != nil {
			return err
		}
		if err := validateModalColumnRef(path+".port_column", projection["port_column"], columns, "", true); err != nil {
			return err
		}
		return validateModalColumnRef(path+".protocol_column", projection["protocol_column"], columns, "", true)
	case "label_lookup":
		if err := validateModalColumnRef(path+".actor_column", projection["actor_column"], columns, "actor_ref", true); err != nil {
			return err
		}
		labelKey, _ := projection["label_key"].(string)
		if labelKey == "" {
			return fmt.Errorf("%s.label_key is required when kind is label_lookup", path)
		}
		return nil
	case "json_path":
		if err := validateModalColumnRef(path+".column", projection["column"], columns, "json", false); err != nil {
			return err
		}
		pathValue, _ := projection["path"].(string)
		if pathValue == "" {
			return fmt.Errorf("%s.path is required when kind is json_path", path)
		}
		return nil
	case "const":
		if _, ok := projection["value"]; !ok {
			return fmt.Errorf("%s.value is required when kind is const", path)
		}
		return nil
	case "coalesce":
		rawColumns, ok := projection["columns"].([]any)
		if !ok || len(rawColumns) == 0 {
			return fmt.Errorf("%s.columns is required when kind is coalesce", path)
		}
		for i, rawColumn := range rawColumns {
			if err := validateModalColumnRef(fmt.Sprintf("%s.columns[%d]", path, i), rawColumn, columns, "", false); err != nil {
				return err
			}
		}
		return nil
	case "selected_side_endpoint":
		if err := validateModalColumnRef(path+".src_actor_column", projection["src_actor_column"], columns, "actor_ref", false); err != nil {
			return err
		}
		if err := validateModalColumnRef(path+".dst_actor_column", projection["dst_actor_column"], columns, "actor_ref", false); err != nil {
			return err
		}
		if stringValue(projection["local_ip_column"]) == "" && stringValue(projection["local_port_column"]) == "" {
			return fmt.Errorf("%s requires local_ip_column or local_port_column when kind is selected_side_endpoint", path)
		}
		if stringValue(projection["remote_ip_column"]) == "" && stringValue(projection["remote_port_column"]) == "" {
			return fmt.Errorf("%s requires remote_ip_column or remote_port_column when kind is selected_side_endpoint", path)
		}
		for _, field := range []string{"local_ip_column", "local_port_column", "remote_ip_column", "remote_port_column", "protocol_column"} {
			if err := validateModalColumnRef(path+"."+field, projection[field], columns, "", true); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s.kind has unsupported value %q", path, kind)
	}
}

func validateModalBadgeMap(path string, raw any) error {
	if raw == nil {
		return nil
	}
	badgeMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	for key, rawBadge := range badgeMap {
		badge, ok := rawBadge.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%s is not an object", path, key)
		}
		if err := optionalEnum(path+"."+key+".color_slot", badge["color_slot"], colorSlotTokens...); err != nil {
			return err
		}
		if err := optionalEnum(path+"."+key+".opacity", badge["opacity"], opacityTokens...); err != nil {
			return err
		}
	}
	return nil
}

func validateModalSort(path string, raw any, rawColumns any) error {
	if raw == nil {
		return nil
	}
	sortSpec, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	column, _ := sortSpec["column"].(string)
	if column == "" {
		return fmt.Errorf("%s.column is empty", path)
	}
	if err := optionalEnum(path+".direction", sortSpec["direction"], "asc", "desc"); err != nil {
		return err
	}
	columns, _ := rawColumns.([]any)
	for _, rawColumn := range columns {
		columnSpec, ok := rawColumn.(map[string]any)
		if !ok {
			continue
		}
		id, _ := columnSpec["id"].(string)
		if id == column {
			return nil
		}
	}
	return fmt.Errorf("%s.column references unknown modal column %q", path, column)
}

func validateModalColumnRef(path string, raw any, columns map[string]string, expectedType string, optional bool) error {
	column, _ := raw.(string)
	if column == "" {
		if optional {
			return nil
		}
		return fmt.Errorf("%s is required", path)
	}
	columnType, ok := columns[column]
	if !ok {
		return fmt.Errorf("%s references unknown source column %q", path, column)
	}
	if expectedType != "" && columnType != expectedType {
		return fmt.Errorf("%s references non-%s source column %q (%s)", path, expectedType, column, columnType)
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
	return optionalEnum(path+".scale", size["scale"], actorSizeScaleTokens...)
}

func validateActorSearchPolicy(path string, raw any, actorColumns map[string]string) error {
	if raw == nil {
		return nil
	}
	search, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	if enabled, ok := search["enabled"]; ok {
		if _, ok := enabled.(bool); !ok {
			return fmt.Errorf("%s.enabled is not a boolean", path)
		}
	}
	for _, field := range []string{"columns", "label_keys"} {
		rawList, ok := search[field]
		if !ok {
			continue
		}
		values, ok := rawList.([]any)
		if !ok {
			return fmt.Errorf("%s.%s is not an array", path, field)
		}
		seen := make(map[string]struct{}, len(values))
		for i, rawValue := range values {
			value, ok := rawValue.(string)
			if !ok || value == "" {
				return fmt.Errorf("%s.%s[%d] is not a non-empty string", path, field, i)
			}
			if _, ok := seen[value]; ok {
				return fmt.Errorf("%s.%s[%d] duplicates %q", path, field, i, value)
			}
			seen[value] = struct{}{}
			if field != "columns" {
				continue
			}
			columnType, ok := actorColumns[value]
			if !ok {
				return fmt.Errorf("%s.columns[%d] references unknown actor column %q", path, i, value)
			}
			if !isDisplayColumnType(columnType) {
				return fmt.Errorf("%s.columns[%d] references non-display actor column %q (%s)", path, i, value, columnType)
			}
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
	valueColumn, _ := source["value_column"].(string)
	if valueColumn != "" {
		columnType, ok := columns[valueColumn]
		if !ok {
			return fmt.Errorf("%s.value_column references unknown source column %q", path, valueColumn)
		}
		if !isNumericColumnType(columnType) {
			return fmt.Errorf("%s.value_column references non-numeric source column %q (%s)", path, valueColumn, columnType)
		}
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

func validateCorrelation(raw any, shape topologyShape, ctx validationContext) error {
	if raw == nil {
		return nil
	}
	correlation, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("data.correlation is not an object")
	}
	rules, ok := correlation["rules"].(map[string]any)
	if !ok || len(rules) == 0 {
		return fmt.Errorf("data.correlation.rules is empty")
	}

	ruleIDs := make(map[string]struct{}, len(rules))
	requiredColumnsByRule := make(map[string]map[string]struct{}, len(rules))
	for ruleID, rawRule := range rules {
		ruleIDs[ruleID] = struct{}{}
		requiredColumns := make(map[string]struct{})
		requiredColumnsByRule[ruleID] = requiredColumns
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return fmt.Errorf("data.correlation.rules.%s is not an object", ruleID)
		}
		if _, err := requiredEnum("data.correlation.rules."+ruleID+".action", rule["action"], "absorb", "link"); err != nil {
			return err
		}
		if err := optionalEnum("data.correlation.rules."+ruleID+".class", rule["class"], "resolve_loose_side", "replace_actor", "merge_enrich_actor"); err != nil {
			return err
		}
		if _, ok := integerValue(rule["priority"]); !ok {
			return fmt.Errorf("data.correlation.rules.%s.priority is not an integer", ruleID)
		}
		outputLinkType, ok := rule["output_link_type"].(string)
		if !ok || outputLinkType == "" {
			return fmt.Errorf("data.correlation.rules.%s.output_link_type is empty", ruleID)
		}
		if _, ok := shape.linkTypes[outputLinkType]; !ok {
			return fmt.Errorf("data.correlation.rules.%s.output_link_type references unknown link type %q", ruleID, outputLinkType)
		}
		if err := validateIDArrayRefs("data.correlation.rules."+ruleID+".point_actor_types", rule["point_actor_types"], shape.actorTypes, "actor type"); err != nil {
			return err
		}
		if err := validateOptionalIDArrayRefs("data.correlation.rules."+ruleID+".claim_actor_types", rule["claim_actor_types"], shape.actorTypes, "actor type"); err != nil {
			return err
		}
		if err := validateOptionalIDArrayRefs("data.correlation.rules."+ruleID+".correlation_link_types", rule["correlation_link_types"], shape.linkTypes, "link type"); err != nil {
			return err
		}
		key, ok := rule["key"].([]any)
		if !ok || len(key) == 0 {
			return fmt.Errorf("data.correlation.rules.%s.key is empty", ruleID)
		}
		for i, rawPart := range key {
			part, ok := rawPart.(map[string]any)
			if !ok {
				return fmt.Errorf("data.correlation.rules.%s.key[%d] is not an object", ruleID, i)
			}
			if column, ok := part["column"].(string); ok && column != "" {
				requiredColumns[column] = struct{}{}
				continue
			}
			if literal, ok := part["literal"].(string); ok && literal != "" {
				continue
			}
			return fmt.Errorf("data.correlation.rules.%s.key[%d] must define column or literal", ruleID, i)
		}
	}

	if err := validateCorrelationTable("data.correlation.points", correlation["points"], ctx, ruleIDs, requiredColumnsByRule); err != nil {
		return err
	}
	return validateCorrelationTable("data.correlation.claims", correlation["claims"], ctx, ruleIDs, requiredColumnsByRule)
}

func validateCorrelationTable(path string, raw any, ctx validationContext, ruleIDs map[string]struct{}, requiredColumnsByRule map[string]map[string]struct{}) error {
	if raw == nil {
		return nil
	}
	if _, err := validateCompactTable(path, raw, ctx); err != nil {
		return err
	}
	columns, err := columnTypesFromTable(raw, path)
	if err != nil {
		return err
	}
	if columnType := columns["actor"]; columnType != "actor_ref" {
		return fmt.Errorf("%s.actor must be actor_ref, got %q", path, columnType)
	}
	ruleColumnType := columns["rule"]
	if ruleColumnType != "string" && ruleColumnType != "string_ref" {
		return fmt.Errorf("%s.rule must be string or string_ref, got %q", path, ruleColumnType)
	}
	referencedRules, err := collectStringColumnValuesInSet(path, raw, "rule", ruleIDs, "correlation rule", ctx.dictionaries)
	if err != nil {
		return err
	}
	for ruleID := range referencedRules {
		for column := range requiredColumnsByRule[ruleID] {
			if _, ok := columns[column]; !ok {
				return fmt.Errorf("%s is missing correlation key column %q for rule %q", path, column, ruleID)
			}
		}
	}
	return nil
}

func validateIDArrayRefs(path string, raw any, known map[string]struct{}, kind string) error {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	for i, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || value == "" {
			return fmt.Errorf("%s[%d] is not a non-empty string", path, i)
		}
		if _, ok := known[value]; !ok {
			return fmt.Errorf("%s[%d] references unknown %s %q", path, i, kind, value)
		}
	}
	return nil
}

func validateOptionalIDArrayRefs(path string, raw any, known map[string]struct{}, kind string) error {
	if raw == nil {
		return nil
	}
	values, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s is not an array", path)
	}
	for i, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || value == "" {
			return fmt.Errorf("%s[%d] is not a non-empty string", path, i)
		}
		if _, ok := known[value]; !ok {
			return fmt.Errorf("%s[%d] references unknown %s %q", path, i, kind, value)
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

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func validateRequiredString(path string, raw any) error {
	value, ok := raw.(string)
	if !ok || value == "" {
		return fmt.Errorf("%s is required", path)
	}
	return nil
}

func stringInSet(value string, allowed []string) bool {
	return slices.Contains(allowed, value)
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
	opacityTokens          = []string{"normal", "muted", "faded"}
	widthTokens            = []string{"thin", "normal", "thick", "emphasis"}
	layoutStrengthTokens   = []string{"weakest", "weaker", "normal", "stronger", "strongest"}
	layoutDistanceTokens   = []string{"closest", "closer", "normal", "farther", "farthest"}
	actorSizeScaleTokens   = []string{"compact", "normal", "emphasized"}
	linkSemanticRoleTokens = []string{"normal", "discovery", "ownership", "traffic", "correlation", "control"}
	iconTokens             = []string{
		"router", "switch", "firewall", "access_point", "server", "storage", "load_balancer",
		"printer", "phone", "ups", "camera", "process", "agent", "netdata-agent", "parent",
		"remote-endpoint", "local-endpoint", "segment", "self", "ip", "cloud", "container",
		"vm", "database", "service", "datacenter", "cluster", "host", "network", "datastore",
		"datastore_cluster", "resource_pool", "device", "endpoint", "correlation", "interface",
		"group", "unknown",
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
	case int64:
		return int(value), true
	case uint64:
		if value > uint64(maxInt()) {
			return 0, false
		}
		return int(value), true
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

func numberValue(raw any) (float64, bool) {
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	case float64:
		return value, true
	case json.Number:
		n, err := value.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
