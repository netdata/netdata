// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"math"
	"strings"
)

type topologyV1DynamicRow struct {
	actorRef int
	values   map[string]any
}

func buildSNMPTopologyV1DynamicTable(rows []topologyV1DynamicRow, stringsDict *topologyv1.StringDictionary) (topologyv1.Table, error) {
	keysSet := make(map[string]struct{})
	normalizedRows := make([]map[string]any, len(rows))
	for i, row := range rows {
		normalizedRows[i] = normalizeTopologyV1DynamicRowValues(row.values)
		for key := range normalizedRows[i] {
			keysSet[key] = struct{}{}
		}
	}
	keys := sortedMapKeys(keysSet)

	columns := make([]topologyv1.Column, 0, len(keys)+1)
	values := make([]topologyv1.ColumnEncoding, 0, len(keys)+1)
	actorRefs := make([]any, len(rows))
	for i, row := range rows {
		actorRefs[i] = row.actorRef
	}
	columns = append(columns, topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")))
	values = append(values, topologyv1.Values(actorRefs...))

	for _, key := range keys {
		columnValues := make([]any, len(rows))
		for i, values := range normalizedRows {
			if value, ok := values[key]; ok {
				columnValues[i] = value
			}
		}
		columnType := inferTopologyV1ColumnType(columnValues)
		columnID := topologyID(key, "field")
		column := topologyv1.NewColumn(columnID, columnType, topologyv1.WithNullable())
		if columnType == "string_ref" {
			column = topologyv1.NewColumn(columnID, columnType, topologyv1.WithNullable(), topologyv1.WithDictionary("strings"))
			for i, value := range columnValues {
				if value == nil {
					continue
				}
				columnValues[i] = stringsDict.Ref(fmt.Sprint(value))
			}
		}
		columns = append(columns, column)
		values = append(values, topologyv1.Values(columnValues...))
	}

	return topologyv1.NewTable(len(rows), columns, values)
}

func normalizeTopologyV1DynamicRowValues(values map[string]any) map[string]any {
	normalized := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			normalized[key] = value
		}
	}
	return normalized
}

func inferTopologyV1ColumnType(values []any) string {
	typ := ""
	for _, value := range values {
		if value == nil {
			continue
		}
		valueType := topologyV1ValueType(value)
		if typ == "" {
			typ = valueType
			continue
		}
		if typ != valueType {
			return "json"
		}
	}
	if typ == "" {
		return "json"
	}
	return typ
}

func topologyV1ValueType(value any) string {
	switch typed := value.(type) {
	case bool:
		return "bool"
	case int, int8, int16, int32, int64:
		return "int"
	case uint, uint8, uint16, uint32, uint64:
		return "uint"
	case float32:
		if math.Trunc(float64(typed)) == float64(typed) {
			return "int"
		}
		return "float"
	case float64:
		if math.Trunc(typed) == typed {
			return "int"
		}
		return "float"
	case string:
		return "string_ref"
	case []string, []int, []int64, []uint, []uint64, []float64, []bool:
		return "array"
	case []any:
		if scalarArray(typed) {
			return "array"
		}
		return "json"
	default:
		return "json"
	}
}

func scalarArray(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string:
		default:
			return false
		}
	}
	return true
}
