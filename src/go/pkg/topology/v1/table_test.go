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
