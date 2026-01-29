// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestRedisMethods(t *testing.T) {
	methods := redisMethods()

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

	hasDefault := false
	for _, opt := range sortParam.Options {
		if opt.Default {
			hasDefault = true
			require.Equal("duration", opt.ID)
			break
		}
	}
	require.True(hasDefault, "should have a default sort option")
}

func TestRedisAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"timestamp", "command", "duration"}

	cs := redisColumnSet(redisAllColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func TestMapRedisSortColumn(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"duration":  {input: "duration", expected: "duration"},
		"timestamp": {input: "timestamp", expected: "timestamp"},
		"id":        {input: "id", expected: "id"},
		"unknown":   {input: "unknown", expected: "duration"},
		"empty":     {input: "", expected: "duration"},
		"injection": {input: "duration;drop table", expected: "duration"},
		"cmd_name":  {input: "command_name", expected: "command_name"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, mapRedisSortColumn(tc.input))
		})
	}
}
