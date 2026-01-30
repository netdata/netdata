// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestRethinkDBMethods(t *testing.T) {
	methods := rethinkdbMethods()

	require := assert.New(t)
	require.Len(methods, 1)
	require.Equal("running-queries", methods[0].ID)
	require.Equal("Running Queries", methods[0].Name)
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

func TestRethinkDBRunningColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"jobId", "query", "durationMs"}

	cs := rethinkColumnSet(rethinkRunningColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}
