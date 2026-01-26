// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestProxySQLMethods(t *testing.T) {
	methods := proxysqlMethods()

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

func TestProxySQLAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"digest", "query", "calls", "totalTime"}

	uiKeys := make(map[string]bool)
	for _, col := range proxysqlAllColumns {
		uiKeys[col.uiKey] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}

func TestCollector_mapAndValidateProxySQLSortColumn(t *testing.T) {
	tests := map[string]struct {
		available []proxysqlColumnMeta
		input     string
		expected  string
	}{
		"valid totalTime": {
			available: []proxysqlColumnMeta{{uiKey: "totalTime"}, {uiKey: "calls"}},
			input:     "totalTime",
			expected:  "totalTime",
		},
		"invalid falls back to totalTime": {
			available: []proxysqlColumnMeta{{uiKey: "totalTime"}, {uiKey: "calls"}},
			input:     "bad",
			expected:  "totalTime",
		},
		"fallback to calls": {
			available: []proxysqlColumnMeta{{uiKey: "calls"}},
			input:     "bad",
			expected:  "calls",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			assert.Equal(t, tc.expected, c.mapAndValidateProxySQLSortColumn(tc.input, tc.available))
		})
	}
}

func TestCollector_buildProxySQLDynamicSQL(t *testing.T) {
	c := &Collector{}

	cols := []proxysqlColumnMeta{
		{dbColumn: "digest", uiKey: "digest", dataType: ftString},
		{dbColumn: "digest_text", uiKey: "query", dataType: ftString},
		{dbColumn: "count_star", uiKey: "calls", dataType: ftInteger},
		{dbColumn: "sum_time", uiKey: "totalTime", dataType: ftDuration, isMicroseconds: true},
	}

	sql := c.buildProxySQLDynamicSQL(cols, "totalTime", 500)

	assert.Contains(t, sql, "stats_mysql_query_digest")
	assert.Contains(t, sql, "ORDER BY `totalTime` DESC")
	assert.Contains(t, sql, "LIMIT 500")
	assert.Contains(t, sql, "sum_time/1000 AS `totalTime`")
}
