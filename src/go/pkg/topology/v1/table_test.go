// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTableValidatesEncodings(t *testing.T) {
	table, err := NewTable(3,
		[]Column{
			NewColumn("kind", "string"),
			NewColumn("name", "string"),
			NewColumn("state", "string"),
		},
		[]ColumnEncoding{
			Const("process"),
			Values("web", "db", "cache"),
			Dict(StringValues("running", "stopped"), 0, 0, 1),
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 3, table.Rows)
	assert.Len(t, table.Columns, 3)
	assert.Len(t, table.Values, 3)
}

func TestNewTableRejectsColumnValueMismatch(t *testing.T) {
	_, err := NewTable(1,
		[]Column{
			NewColumn("id", "string"),
			NewColumn("name", "string"),
		},
		[]ColumnEncoding{
			Values("actor-1"),
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "columns/values length mismatch")
}

func TestNewTableRejectsDecodedLengthMismatch(t *testing.T) {
	_, err := NewTable(2,
		[]Column{NewColumn("name", "string")},
		[]ColumnEncoding{Values("only-one")},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoded length mismatch")
}

func TestNewTableRejectsDictIndexOutOfBounds(t *testing.T) {
	_, err := NewTable(2,
		[]Column{NewColumn("state", "string")},
		[]ColumnEncoding{Dict(StringValues("up"), 0, 1)},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestNewTableRejectsNilEncoding(t *testing.T) {
	_, err := NewTable(1,
		[]Column{NewColumn("id", "string")},
		[]ColumnEncoding{nil},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is nil")
}

func TestNewTableRejectsDuplicateColumnIDs(t *testing.T) {
	_, err := NewTable(1,
		[]Column{
			NewColumn("id", "string"),
			NewColumn("id", "string"),
		},
		[]ColumnEncoding{
			Const("actor-1"),
			Const("actor-2"),
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicates column")
}

func TestNewTableRejectsColumnValueContractMismatch(t *testing.T) {
	cases := map[string]struct {
		column Column
		value  ColumnEncoding
		want   string
	}{
		"non nullable null": {
			column: NewColumn("name", "string"),
			value:  Const(nil),
			want:   "not nullable",
		},
		"string ref without dictionary": {
			column: NewColumn("name", "string_ref"),
			value:  Const(0),
			want:   "requires dictionary",
		},
		"string ref value must be integer": {
			column: NewColumn("name", "string_ref", WithDictionary("strings")),
			value:  Const("node-a"),
			want:   "dictionary reference",
		},
		"actor ref must be non negative integer": {
			column: NewColumn("actor", "actor_ref"),
			value:  Const(-1),
			want:   "actor_ref reference",
		},
		"dictionary only belongs on reference columns": {
			column: Column{ID: "name", Type: "string", Dictionary: "strings"},
			value:  Const("node-a"),
			want:   "dictionary with non-reference type",
		},
		"unsupported type rejected even when nullable": {
			column: Column{ID: "name", Type: "unknown", Nullable: true},
			value:  Const(nil),
			want:   "unsupported type",
		},
		"uint must be non negative": {
			column: NewColumn("socket_count", "uint"),
			value:  Const(-1),
			want:   "non-negative integer",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewTable(1, []Column{tc.column}, []ColumnEncoding{tc.value})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNewTableAllowsNullableNull(t *testing.T) {
	table, err := NewTable(1,
		[]Column{NewColumn("name", "string", WithNullable())},
		[]ColumnEncoding{Const(nil)},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, table.Rows)
}

func TestStringDictionaryDeduplicatesValues(t *testing.T) {
	dict := NewStringDictionary()

	first := dict.Ref("alpha")
	second := dict.Ref("beta")
	again := dict.Ref("alpha")

	assert.Equal(t, 0, first)
	assert.Equal(t, 1, second)
	assert.Equal(t, first, again)
	assert.Equal(t, []any{"alpha", "beta"}, dict.Values())
}

func TestNilStringDictionaryReturnsEmptyValues(t *testing.T) {
	var dict *StringDictionary

	assert.Equal(t, []any{}, dict.Values())
}

func TestEmptyStringDictionaryReturnsEmptyValues(t *testing.T) {
	dict := NewStringDictionary()

	assert.Equal(t, []any{}, dict.Values())
}
