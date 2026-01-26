// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestCouchbaseMethods(t *testing.T) {
	methods := couchbaseMethods()

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

func TestCouchbaseAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"requestId", "statement", "elapsedTime"}

	uiKeys := make(map[string]bool)
	for _, col := range couchbaseAllColumns {
		uiKeys[col.id] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}

func TestMapCouchbaseSortColumn(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"elapsedTime": {input: "elapsedTime", expected: "elapsedTime"},
		"serviceTime": {input: "serviceTime", expected: "serviceTime"},
		"requestTime": {input: "requestTime", expected: "requestTime"},
		"resultCount": {input: "resultCount", expected: "resultCount"},
		"invalid":     {input: "bad", expected: "elapsedTime"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, mapCouchbaseSortColumn(tc.input))
		})
	}
}
