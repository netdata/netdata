// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNumericScalarCells(t *testing.T) {
	cases := map[string]struct {
		value                     any
		number, integer, unsigned bool
	}{
		"int zero":                   {0, true, true, true},
		"int negative":               {-2, true, true, false},
		"int64 minimum":              {int64(math.MinInt64), true, true, false},
		"int64 maximum":              {int64(math.MaxInt64), true, true, true},
		"int64 above uint32":         {int64(1 << 32), true, true, true},
		"negative int64 low zero":    {int64(-1 << 32), true, true, false},
		"uint64 above machine int":   {uint64(maxInt()) + 1, true, true, true},
		"uint64 maximum":             {uint64(math.MaxUint64), true, true, true},
		"float zero":                 {float64(0), true, true, true},
		"float negative zero":        {math.Copysign(0, -1), true, true, true},
		"float negative integer":     {float64(-2), true, true, false},
		"float fraction":             {1.5, true, false, false},
		"float negative fraction":    {-1.5, true, false, false},
		"float subnormal":            {math.SmallestNonzeroFloat64, true, false, false},
		"float above signed range":   {float64(1 << 63), true, true, true},
		"float uint64 roundtrip":     {float64(uint64(math.MaxUint64)), true, true, true},
		"float maximum":              {math.MaxFloat64, true, true, true},
		"NaN":                        {math.NaN(), false, false, false},
		"positive infinity":          {math.Inf(1), false, false, false},
		"negative infinity":          {math.Inf(-1), false, false, false},
		"JSON zero":                  {json.Number("0"), true, true, true},
		"JSON negative zero":         {json.Number("-0"), true, true, true},
		"JSON negative integer":      {json.Number("-2"), true, true, false},
		"JSON int64 minimum":         {json.Number("-9223372036854775808"), true, true, false},
		"JSON int64 maximum":         {json.Number("9223372036854775807"), true, true, true},
		"JSON uint64 maximum":        {json.Number("18446744073709551615"), true, true, true},
		"JSON below int64":           {json.Number("-9223372036854775809"), true, false, false},
		"JSON above uint64":          {json.Number("18446744073709551616"), true, false, false},
		"JSON fraction":              {json.Number("1.5"), true, false, false},
		"JSON rounded fraction":      {json.Number("9007199254740993.1"), true, false, false},
		"JSON decimal integer":       {json.Number("1.0"), true, false, false},
		"JSON exponent integer":      {json.Number("1e3"), true, false, false},
		"JSON exponent sign":         {json.Number("1e+3"), true, false, false},
		"JSON overflow":              {json.Number("1e400"), false, false, false},
		"JSON NaN":                   {json.Number("NaN"), false, false, false},
		"JSON infinity":              {json.Number("Inf"), false, false, false},
		"JSON negative infinity":     {json.Number("-Infinity"), false, false, false},
		"JSON leading plus":          {json.Number("+1"), false, false, false},
		"JSON leading zero":          {json.Number("01"), false, false, false},
		"JSON negative leading zero": {json.Number("-01"), false, false, false},
		"JSON hexadecimal":           {json.Number("0x1p4"), false, false, false},
		"JSON underscore":            {json.Number("1_0"), false, false, false},
		"JSON incomplete fraction":   {json.Number("1."), false, false, false},
		"JSON incomplete exponent":   {json.Number("1e"), false, false, false},
		"JSON empty":                 {json.Number(""), false, false, false},
		"JSON whitespace":            {json.Number(" 1 "), false, false, false},
		"JSON string":                {json.Number(`"1"`), false, false, false},
		"JSON boolean":               {json.Number("true"), false, false, false},
		"JSON array":                 {json.Number("[1]"), false, false, false},
		"numeric string":             {"1", false, false, false},
		"boolean":                    {true, false, false, false},
		"unsupported int32":          {int32(1), false, false, false},
		"unsupported uint":           {uint(1), false, false, false},
		"unsupported float32":        {float32(1), false, false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for typ, valid := range map[string]bool{"int": tc.integer, "uint": tc.unsigned, "float": tc.number, "duration": tc.number} {
				t.Run(typ, func(t *testing.T) {
					for _, aggregation := range []string{"none", "set"} {
						t.Run(aggregation, func(t *testing.T) {
							value := tc.value
							if aggregation == "set" {
								value = []any{1, value}
							}
							assertNumericCellCodecs(t, Column{ID: "sample", Type: typ, Aggregation: aggregation}, value, valid)
						})
					}
				})
			}
		})
	}
}

func assertNumericCellCodecs(t *testing.T, column Column, value any, valid bool) {
	t.Helper()
	for codec, tc := range map[string]struct {
		encoding ColumnEncoding
		wire     map[string]any
	}{
		"const":  {Const(value), map[string]any{"codec": "const", "value": value}},
		"values": {Values(1, value), map[string]any{"codec": "values", "values": []any{1, value}}},
		"dict": {Dict([]any{1, value}, 1, 0), map[string]any{
			"codec": "dict", "values": []any{1, value}, "indexes": []any{1, 0},
		}},
	} {
		t.Run(codec, func(t *testing.T) {
			columns := []Column{column}
			encodings := []ColumnEncoding{tc.encoding}
			table, builderErr := NewTable(2, columns, encodings)
			wire := map[string]any{
				"rows":    2,
				"columns": []any{map[string]any{"id": column.ID, "type": column.Type, "aggregation": column.Aggregation}},
				"values":  []any{tc.wire},
			}
			_, decodedErr := validateCompactTable("table", wire, validationContext{})
			if !valid {
				assert.Error(t, builderErr, "builder must reject the numeric cell")
				assert.Error(t, decodedErr, "decoded validation must reject the numeric cell")
				return
			}
			require.NoError(t, builderErr)
			require.NoError(t, decodedErr)
			before, err := json.Marshal(Table{Rows: 2, Columns: columns, Values: encodings})
			require.NoError(t, err)
			after, err := json.Marshal(table)
			require.NoError(t, err)
			assert.Equal(t, before, after, "numeric validation must preserve cell representation")
			for _, useNumber := range []bool{false, true} {
				decoder := json.NewDecoder(strings.NewReader(string(after)))
				if useNumber {
					decoder.UseNumber()
				}
				var decoded any
				require.NoError(t, decoder.Decode(&decoded))
				// UseNumber retains decimal/exponent syntax, which integer cells do not accept.
				if useNumber && (column.Type == "int" || column.Type == "uint") {
					continue
				}
				_, err = validateCompactTable("table", decoded, validationContext{})
				require.NoError(t, err)
			}
		})
	}
}

func TestNumericScalarErrorIdentifiesRowAndMember(t *testing.T) {
	column := Column{ID: "sample", Type: "float", Aggregation: "set"}
	_, err := NewTable(2, []Column{column}, []ColumnEncoding{Values(1, []any{2, math.Inf(1)})})
	require.EqualError(t, err, "values[0][1][1] is not a number")
	err = validateColumnValues("table.sample", map[string]any{"aggregation": "set"}, "float",
		[]any{1, []any{2, math.Inf(1)}}, validationContext{})
	require.EqualError(t, err, "table.sample[1][1] is not a number")
}

func TestNumericScalarRangeDoesNotBroadenReferences(t *testing.T) {
	for _, typ := range []string{"string_ref", "ip_ref", "mac_ref", "actor_ref", "link_ref", "evidence_ref"} {
		t.Run(typ, func(t *testing.T) {
			column := Column{ID: "sample", Type: typ, Aggregation: "set"}
			if typ == "string_ref" || typ == "ip_ref" || typ == "mac_ref" {
				column.Dictionary = "items"
			}
			ctx := validationContext{dictionaries: map[string]any{"items": []any{"item"}}, actorRows: 1, linkRows: 1, evidenceRows: 1}
			for _, value := range []any{uint64(math.MaxUint64), json.Number("18446744073709551615")} {
				_, err := NewTable(1, []Column{column}, []ColumnEncoding{Const(value)})
				require.Error(t, err)
				err = validateColumnValues("table.sample", map[string]any{"aggregation": "set", "dictionary": column.Dictionary},
					typ, []any{value}, ctx)
				require.Error(t, err)
			}
		})
	}
	_, err := decodeColumn("table", 0, 1, map[string]any{
		"codec": "dict", "values": []any{1}, "indexes": []any{uint64(math.MaxUint64)},
	})
	require.Error(t, err)
	_, err = decodedTableRows(map[string]any{"rows": uint64(math.MaxUint64)})
	require.Error(t, err)
}

func TestNumericScalarValidationAllocations(t *testing.T) {
	for typ, value := range map[string]any{"int": int64(-2), "uint": uint64(math.MaxUint64), "float": 1.25, "duration": 1.25} {
		for _, aggregation := range []string{"none", "set"} {
			t.Run(typ+"/"+aggregation, func(t *testing.T) {
				cell := value
				if aggregation == "set" {
					cell = []any{value, value}
				}
				column := Column{ID: "sample", Type: typ, Aggregation: aggregation}
				metadata := map[string]any{"aggregation": aggregation}
				values := []any{cell}
				builderAllocs := testing.AllocsPerRun(100, func() {
					require.NoError(t, validateEncodedColumnValue(0, 0, column, cell))
				})
				decodedAllocs := testing.AllocsPerRun(100, func() {
					require.NoError(t, validateColumnValues("table.sample", metadata, typ, values, validationContext{}))
				})
				assert.Zero(t, builderAllocs, "valid native numeric cells require no builder allocations")
				assert.Zero(t, decodedAllocs, "valid native numeric cells require no decoded-validation allocations")
			})
		}
	}
}

// Timings are workstation trends, not CI gates; successful native cells allocate nothing.
func BenchmarkNumericScalarValidation(b *testing.B) {
	for typ, value := range map[string]any{"int": int64(-2), "uint": uint64(42), "float": 1.25, "duration": 1.25} {
		for _, aggregation := range []string{"none", "set"} {
			b.Run(typ+"/"+aggregation, func(b *testing.B) {
				cell := value
				if aggregation == "set" {
					cell = []any{value, value}
				}
				column := Column{ID: "sample", Type: typ, Aggregation: aggregation}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if err := validateEncodedColumnValue(0, 0, column, cell); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
