// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

type NormalizedData struct {
	SchemaVersion string                    `json:"schema_version"`
	Producer      topologyapi.Producer      `json:"producer"`
	CollectedAt   string                    `json:"collected_at"`
	ValidAfter    string                    `json:"valid_after,omitempty"`
	ValidUntil    string                    `json:"valid_until,omitempty"`
	View          *topologyapi.View         `json:"view,omitempty"`
	Types         topologyapi.TypeRegistry  `json:"types"`
	Presentation  *topologyapi.Presentation `json:"presentation,omitempty"`
	Correlation   any                       `json:"correlation,omitempty"`
	Actors        NormalizedTable           `json:"actors"`
	Links         NormalizedTable           `json:"links"`
	Evidence      map[string]NormalizedRef  `json:"evidence,omitempty"`
	Tables        *NormalizedDetailTables   `json:"tables,omitempty"`
	Overlays      any                       `json:"overlays,omitempty"`
	Stats         any                       `json:"stats,omitempty"`
	Extensions    any                       `json:"extensions,omitempty"`
}

type NormalizedDetailTables struct {
	Actor map[string]NormalizedRef `json:"actor,omitempty"`
}

type NormalizedRef struct {
	Type  string          `json:"type"`
	Table NormalizedTable `json:"table"`
}

type NormalizedTable struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

type topologyV1GoldenRefs struct {
	actors []string
	links  []string
}

func NormalizeData(t testing.TB, data topologyapi.Data) NormalizedData {
	t.Helper()

	refs := topologyV1GoldenRefs{}
	actorRows := tableRows(t, data, data.Actors, refs)
	refs.actors = actorRefs(t, actorRows)
	linkRows := tableRows(t, data, data.Links, refs)
	refs.links = linkRefs(linkRows)

	normalized := NormalizedData{
		SchemaVersion: data.SchemaVersion,
		Producer:      data.Producer,
		CollectedAt:   data.CollectedAt.UTC().Format(time.RFC3339Nano),
		ValidAfter:    timeString(data.ValidAfter),
		ValidUntil:    timeString(data.ValidUntil),
		View:          data.View,
		Types:         data.Types,
		Presentation:  data.Presentation,
		Actors: NormalizedTable{
			Columns: columnSummaries(data.Actors.Columns),
			Rows:    sortedRows(actorRows),
		},
		Links: NormalizedTable{
			Columns: columnSummaries(data.Links.Columns),
			Rows:    sortedRows(linkRows),
		},
		Stats: normalizeValue(t, data.Stats),
	}
	if data.Correlation != nil {
		normalized.Correlation = normalizeValue(t, data.Correlation)
	}
	if data.Overlays != nil {
		normalized.Overlays = normalizeValue(t, data.Overlays)
	}
	if len(data.Extensions) > 0 {
		normalized.Extensions = normalizeValue(t, data.Extensions)
	}

	if len(data.Evidence) > 0 {
		normalized.Evidence = make(map[string]NormalizedRef, len(data.Evidence))
		for key, section := range data.Evidence {
			normalized.Evidence[key] = NormalizedRef{
				Type:  section.Type,
				Table: normalizeTable(t, data, section.Table, refs),
			}
		}
	}

	if data.Tables != nil && len(data.Tables.Actor) > 0 {
		normalized.Tables = &NormalizedDetailTables{
			Actor: make(map[string]NormalizedRef, len(data.Tables.Actor)),
		}
		for key, detail := range data.Tables.Actor {
			normalized.Tables.Actor[key] = NormalizedRef{
				Type:  detail.Type,
				Table: normalizeTable(t, data, detail.Table, refs),
			}
		}
	}

	return normalized
}

func ValidateData(data topologyapi.Data) error {
	return ValidateDataWithSchemaPath(data, topologySchemaPath())
}

func ValidateDataWithSchemaPath(data topologyapi.Data, schemaPath string) error {
	bs, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err := json.Unmarshal(bs, &decoded); err != nil {
		return err
	}
	if err := topologyapi.ValidateDecodedData(decoded); err != nil {
		return err
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return err
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	var response any
	if err := json.Unmarshal([]byte(`{"status":200,"type":"topology","data":`+string(bs)+`}`), &response); err != nil {
		return err
	}
	return schema.Validate(response)
}

func DecodeColumnValues(t testing.TB, table topologyapi.Table, columnIndex int) []any {
	t.Helper()

	switch encoding := table.Values[columnIndex].(type) {
	case topologyapi.ValuesEncoding:
		return encoding.Values
	case *topologyapi.ValuesEncoding:
		require.NotNil(t, encoding)
		return encoding.Values
	case topologyapi.ConstEncoding:
		values := make([]any, table.Rows)
		for i := range values {
			values[i] = encoding.Value
		}
		return values
	case *topologyapi.ConstEncoding:
		require.NotNil(t, encoding)
		values := make([]any, table.Rows)
		for i := range values {
			values[i] = encoding.Value
		}
		return values
	case topologyapi.DictEncoding:
		values := make([]any, 0, len(encoding.Indexes))
		for _, index := range encoding.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(encoding.Values))
			values = append(values, encoding.Values[index])
		}
		return values
	case *topologyapi.DictEncoding:
		require.NotNil(t, encoding)
		values := make([]any, 0, len(encoding.Indexes))
		for _, index := range encoding.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(encoding.Values))
			values = append(values, encoding.Values[index])
		}
		return values
	default:
		require.Failf(t, "unsupported encoding", "column %d has unsupported encoding %T", columnIndex, encoding)
		return nil
	}
}

func CanonicalJSON(t testing.TB, value any) string {
	t.Helper()

	bs, err := json.Marshal(value)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(bs, &normalized))

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(normalized))
	return out.String()
}

func normalizeTable(
	t testing.TB,
	data topologyapi.Data,
	table topologyapi.Table,
	refs topologyV1GoldenRefs,
) NormalizedTable {
	t.Helper()

	return NormalizedTable{
		Columns: columnSummaries(table.Columns),
		Rows:    sortedRows(tableRows(t, data, table, refs)),
	}
}

func tableRows(
	t testing.TB,
	data topologyapi.Data,
	table topologyapi.Table,
	refs topologyV1GoldenRefs,
) []map[string]any {
	t.Helper()

	rows := make([]map[string]any, table.Rows)
	for row := range rows {
		rows[row] = make(map[string]any, len(table.Columns))
	}
	for columnIndex, column := range table.Columns {
		values := DecodeColumnValues(t, table, columnIndex)
		require.Len(t, values, table.Rows)
		for rowIndex, value := range values {
			decoded := decodeValue(t, data, column, value, refs)
			if decoded != nil {
				rows[rowIndex][column.ID] = decoded
			}
		}
	}
	return rows
}

func columnSummaries(columns []topologyapi.Column) []string {
	out := make([]string, len(columns))
	for i, column := range columns {
		summary := column.ID + ":" + column.Type
		if column.Dictionary != "" {
			summary += ":dict=" + column.Dictionary
		}
		if column.Nullable {
			summary += ":nullable"
		}
		if column.Role != "" {
			summary += ":role=" + column.Role
		}
		if column.Aggregation != "" {
			summary += ":aggregation=" + column.Aggregation
		}
		if column.Unit != "" {
			summary += ":unit=" + column.Unit
		}
		out[i] = summary
	}
	return out
}

func decodeValue(
	t testing.TB,
	data topologyapi.Data,
	column topologyapi.Column,
	value any,
	refs topologyV1GoldenRefs,
) any {
	t.Helper()

	if value == nil {
		return nil
	}

	switch column.Type {
	case "string_ref", "ip_ref", "mac_ref":
		return dictionaryValue(t, data, column, value)
	case "actor_ref":
		return refValue(t, refs.actors, value, "actor")
	case "link_ref":
		return refValue(t, refs.links, value, "link")
	default:
		return normalizeValue(t, value)
	}
}

func dictionaryValue(t testing.TB, data topologyapi.Data, column topologyapi.Column, value any) any {
	t.Helper()

	ref := intValue(t, value)
	dict := data.Dictionaries[column.Dictionary]
	require.NotNilf(t, dict, "missing dictionary %q for column %q", column.Dictionary, column.ID)
	require.GreaterOrEqual(t, ref, 0)
	require.Less(t, ref, len(dict))
	return normalizeValue(t, dict[ref])
}

func refValue(t testing.TB, values []string, value any, kind string) any {
	t.Helper()

	ref := intValue(t, value)
	require.GreaterOrEqual(t, ref, 0)
	require.Lessf(t, ref, len(values), "%s ref %d out of bounds", kind, ref)
	return values[ref]
}

func intValue(t testing.TB, value any) int {
	t.Helper()

	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	default:
		require.Failf(t, "invalid reference value", "got %T", value)
		return 0
	}
}

func actorRefs(t testing.TB, rows []map[string]any) []string {
	t.Helper()

	refs := make([]string, len(rows))
	for i, row := range rows {
		id, ok := row["id"].(string)
		require.Truef(t, ok && id != "", "actor row %d has invalid id %v", i, row["id"])
		refs[i] = id
	}
	return refs
}

func linkRefs(rows []map[string]any) []string {
	refs := make([]string, len(rows))
	seen := make(map[string]int, len(rows))
	for i, row := range rows {
		key := fmt.Sprintf("%v -> %v | %v | %v | %v | %v | %v",
			row["src_actor"],
			row["dst_actor"],
			row["type"],
			row["protocol"],
			row["state"],
			row["src_port_name"],
			row["dst_port_name"],
		)
		seen[key]++
		if seen[key] > 1 {
			key = fmt.Sprintf("%s #%d", key, seen[key])
		}
		refs[i] = key
	}
	return refs
}

func sortedRows(rows []map[string]any) []map[string]any {
	out := append([]map[string]any(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		return sortKey(out[i]) < sortKey(out[j])
	})
	return out
}

func sortKey(row map[string]any) string {
	bs, err := json.Marshal(row)
	if err != nil {
		panic(err)
	}
	return string(bs)
}

func normalizeValue(t testing.TB, value any) any {
	t.Helper()

	if value == nil {
		return nil
	}
	bs, err := json.Marshal(value)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(bs, &normalized))
	return normalized
}

func timeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func topologySchemaPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json"))
}
