// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestClickHouseMethods(t *testing.T) {
	methods := clickhouseMethods()

	require := assert.New(t)
	require.Len(methods, 1)
	require.Equal("top-queries", methods[0].ID)
	require.Equal("Top Queries", methods[0].Name)
	require.NotEmpty(methods[0].RequiredParams)

	var sortParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "__sort" {
			sortParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(sortParam, "expected __sort required param")
	require.NotEmpty(sortParam.Options)
}

func TestClickHouseAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"query", "calls", "totalTime"}

	uiKeys := make(map[string]bool)
	for _, col := range clickhouseAllColumns {
		uiKeys[col.uiKey] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}

func TestCollector_mapAndValidateClickHouseSortColumn(t *testing.T) {
	tests := map[string]struct {
		available []clickhouseColumnMeta
		input     string
		expected  string
	}{
		"valid totalTime": {
			available: []clickhouseColumnMeta{{uiKey: "totalTime"}, {uiKey: "calls"}},
			input:     "totalTime",
			expected:  "totalTime",
		},
		"invalid falls back to totalTime": {
			available: []clickhouseColumnMeta{{uiKey: "totalTime"}, {uiKey: "calls"}},
			input:     "bad",
			expected:  "totalTime",
		},
		"fallback to calls": {
			available: []clickhouseColumnMeta{{uiKey: "calls"}},
			input:     "bad",
			expected:  "calls",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			assert.Equal(t, tc.expected, c.mapAndValidateClickHouseSortColumn(tc.input, tc.available))
		})
	}
}
