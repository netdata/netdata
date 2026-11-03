// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedSetScalarColumns(t *testing.T) {
	types := map[string]struct {
		first, second, invalid any
	}{
		"bool":      {true, false, "true"},
		"int":       {int64(-2), json.Number("3"), 1.5},
		"uint":      {uint64(0), uint64(3), -1},
		"float":     {1.25, json.Number("2.5"), "1.25"},
		"duration":  {1.25, int64(3), "1s"},
		"string":    {"alpha", "beta", true},
		"timestamp": {"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 123},
		"ip":        {"192.0.2.1", "2001:db8::1", 123},
		"mac":       {"02:00:00:00:00:01", "02:00:00:00:00:02", 123},
	}
	for typ, values := range types {
		t.Run(typ, func(t *testing.T) {
			cases := map[string]struct {
				value    any
				nullable bool
				wantErr  string
			}{
				"scalar":                 {value: values.first},
				"set":                    {value: []any{values.first, values.second}},
				"empty set":              {value: []any{}},
				"nullable cell":          {nullable: true},
				"nonnullable cell":       {wantErr: "is null but column is not nullable"},
				"nullable member":        {value: []any{values.first, nil}, nullable: true},
				"nonnullable member":     {value: []any{values.first, nil}, wantErr: "is null but column is not nullable"},
				"nullable nil slice":     {value: []any(nil), nullable: true},
				"nonnullable nil slice":  {value: []any(nil), wantErr: "is null but column is not nullable"},
				"wrong member":           {value: []any{values.first, values.invalid}, wantErr: "is not"},
				"nested member":          {value: []any{values.first, []any{values.second}}, wantErr: "is not"},
				"nested nullable member": {value: []any{values.first, []any{nil}}, nullable: true, wantErr: "is not"},
				"object member":          {value: []any{map[string]any{"value": values.first}}, wantErr: "is not"},
				"typed slice":            {value: []string{"alpha"}, wantErr: "is not"},
			}
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) {
					column := Column{ID: "sample", Type: typ, Nullable: tc.nullable, Role: "attribute", Aggregation: "set"}
					if name == "typed slice" {
						// Builders accept decoded []any arrays, not arbitrary Go slices.
						_, err := NewTable(1, []Column{column}, []ColumnEncoding{Const(tc.value)})
						require.ErrorContains(t, err, tc.wantErr)
						return
					}
					assertScalarSetCodecs(t, column, values.first, tc.value, tc.wantErr)
				})
			}
			for _, aggregation := range []string{"", "none", "first", "last", "sum", "min", "max", "avg", "count"} {
				t.Run("non-set/"+aggregation, func(t *testing.T) {
					column := Column{ID: "sample", Type: typ, Aggregation: aggregation}
					assertScalarSetCodecs(t, column, values.first, values.second, "")
					assertScalarSetCodecs(t, column, values.first, []any{values.first}, "is not")
				})
			}
		})
	}
}

func assertScalarSetCodecs(t *testing.T, column Column, scalar, value any, wantErr string) {
	t.Helper()
	encodings := map[string]struct {
		rows     int
		encoding ColumnEncoding
	}{
		"const":  {rows: 2, encoding: Const(value)},
		"values": {rows: 2, encoding: Values(scalar, value)},
		"dict":   {rows: 3, encoding: Dict([]any{scalar, value}, 1, 0, 1)},
	}
	for name, tc := range encodings {
		t.Run(name, func(t *testing.T) {
			columns := []Column{column}
			encodings := []ColumnEncoding{tc.encoding}
			before, err := json.Marshal(Table{Rows: tc.rows, Columns: columns, Values: encodings})
			require.NoError(t, err)
			table, builderErr := NewTable(tc.rows, columns, encodings)
			var decoded any
			require.NoError(t, json.Unmarshal(before, &decoded))
			_, decodedErr := validateCompactTable("table", decoded, validationContext{})
			if wantErr != "" {
				require.ErrorContains(t, builderErr, wantErr)
				require.ErrorContains(t, decodedErr, wantErr)
				return
			}
			require.NoError(t, builderErr)
			require.NoError(t, decodedErr)
			after, err := json.Marshal(table)
			require.NoError(t, err)
			assert.Equal(t, before, after, "validation must preserve metadata and cell representation")
		})
	}
}

func TestTypedSetReferenceColumnsKeepIndexValidation(t *testing.T) {
	for _, typ := range []string{"string_ref", "ip_ref", "mac_ref", "actor_ref", "link_ref", "evidence_ref"} {
		t.Run(typ, func(t *testing.T) {
			column := Column{ID: "sample", Type: typ, Aggregation: "set"}
			if typ == "string_ref" || typ == "ip_ref" || typ == "mac_ref" {
				column.Dictionary = "items"
			}
			ctx := validationContext{
				dictionaries: map[string]any{"items": []any{"scalar", []any{"alpha", "beta"}}},
				actorRows:    2, linkRows: 2, evidenceRows: 2,
			}
			for name, tc := range map[string]struct {
				value   any
				wantErr bool
			}{
				"scalar":        {value: 0},
				"second scalar": {value: 1},
				"array":         {value: []any{0, 1}, wantErr: true},
				"negative":      {value: -1, wantErr: true},
				"fractional":    {value: 0.5, wantErr: true},
			} {
				t.Run(name, func(t *testing.T) {
					_, builderErr := NewTable(1, []Column{column}, []ColumnEncoding{Const(tc.value)})
					wireColumn := map[string]any{"type": typ, "dictionary": column.Dictionary, "aggregation": "set"}
					decodedErr := validateColumnValues("table.sample", wireColumn, typ, []any{tc.value}, ctx)
					if tc.wantErr {
						require.Error(t, builderErr)
						require.Error(t, decodedErr)
					} else {
						require.NoError(t, builderErr)
						require.NoError(t, decodedErr)
					}
				})
			}
			wireColumn := map[string]any{"type": typ, "dictionary": column.Dictionary, "aggregation": "set"}
			require.ErrorContains(t, validateColumnValues("table.sample", wireColumn, typ, []any{2}, ctx), "out of bounds")
			if column.Dictionary != "" {
				require.ErrorContains(t, validateColumnValues("table.sample", wireColumn, typ, []any{0}, validationContext{}), "missing dictionary")
				delete(wireColumn, "dictionary")
				require.ErrorContains(t, validateColumnValues("table.sample", wireColumn, typ, []any{0}, ctx), "without dictionary")
			}
		})
	}
}

func TestTypedSetArrayAndJSONColumnsKeepTheirShape(t *testing.T) {
	for _, typ := range []string{"array", "json"} {
		t.Run(typ, func(t *testing.T) {
			column := Column{ID: "sample", Type: typ, Role: "identity", Aggregation: "set"}
			value := []any{[]any{"nested"}, map[string]any{"attribute": "value"}}
			assertScalarSetCodecs(t, column, value, value, "")
		})
	}
}

func TestTypedSetBuilderAndResponsePreserveMixedRows(t *testing.T) {
	columns := []Column{
		{ID: "id", Type: "string", Role: "identity", Aggregation: "set"},
		{ID: "type", Type: "string"},
		{ID: "sample", Type: "string", Role: "merge_identity", Aggregation: "set"},
	}
	builder := NewTableBuilder(columns...)
	builder.Add("node-a", "node", "key-a")
	builder.Add("process-a", "process", []any{"alpha", "beta"})
	actors, err := builder.Table()
	require.NoError(t, err)
	assert.Equal(t, columns, actors.Columns)
	data := minimalValidationData(&ActorPresentation{LabelPolicy: &LabelPolicy{Columns: []string{"sample"}, Array: "first"}})
	nodeType := data.Types.ActorTypes["node"]
	nodeType.MergeIdentity = []string{"sample"}
	data.Types.ActorTypes["node"] = nodeType
	data.Types.ActorTypes["process"] = ActorType{Layer: "process", Identity: []string{"id"}}
	data.Actors = actors
	payload, err := json.Marshal(NewResponse(data))
	require.NoError(t, err)
	validateAgainstTopologySchema(t, payload)
	var decoded any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.NoError(t, ValidateDecodedResponse(decoded))
	decodedActors := decoded.(map[string]any)["data"].(map[string]any)["actors"].(map[string]any)
	values := decodedActors["values"].([]any)
	assert.Equal(t, []any{"node-a", "process-a"}, values[0].(map[string]any)["values"])
	assert.Equal(t, []any{"key-a", []any{"alpha", "beta"}}, values[2].(map[string]any)["values"])
}

func TestTypedSetErrorIdentifiesRowAndMember(t *testing.T) {
	column := Column{ID: "sample", Type: "uint", Aggregation: "set"}
	_, err := NewTable(2, []Column{column}, []ColumnEncoding{Values(1, []any{2, -1})})
	require.EqualError(t, err, "values[0][1][1] is not a non-negative integer")
	err = validateColumnValues("table.sample", map[string]any{"aggregation": "set"}, "uint", []any{1, []any{2, -1}}, validationContext{})
	require.EqualError(t, err, "table.sample[1][1] is not a non-negative integer")
}

func TestTypedSetCellValidationDoesNotAllocate(t *testing.T) {
	column := Column{ID: "sample", Type: "string", Aggregation: "set"}
	wireColumn := map[string]any{"aggregation": "set"}
	for name, value := range map[string]any{
		"scalar": "alpha",
		"set":    []any{"alpha", "beta"},
	} {
		t.Run(name, func(t *testing.T) {
			values := []any{value}
			var validationErr error
			builderAllocs := testing.AllocsPerRun(100, func() {
				validationErr = validateEncodedColumnValue(0, 0, column, value)
			})
			require.NoError(t, validationErr)
			assert.Zero(t, builderAllocs, "successful cell validation needs no intermediate representation")
			decodedAllocs := testing.AllocsPerRun(100, func() {
				validationErr = validateColumnValues("table.sample", wireColumn, "string", values, validationContext{})
			})
			require.NoError(t, validationErr)
			assert.Zero(t, decodedAllocs, "successful cell validation needs no intermediate representation")
		})
	}
}
