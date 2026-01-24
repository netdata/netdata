// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestElasticsearchMethods(t *testing.T) {
	methods := elasticsearchMethods()

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

func TestElasticsearchAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"taskId", "description", "runningTime"}

	cs := esColumnSet(esAllColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func TestMapElasticsearchSortColumn(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"runningTime": {input: "runningTime", expected: "runningTime"},
		"startTime":   {input: "startTime", expected: "startTime"},
		"taskId":      {input: "taskId", expected: "taskId"},
		"invalid":     {input: "bad", expected: "runningTime"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, mapElasticsearchSortColumn(tc.input))
		})
	}
}
