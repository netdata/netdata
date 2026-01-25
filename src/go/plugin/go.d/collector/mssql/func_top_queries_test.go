// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestMSSQLMethods(t *testing.T) {
	methods := mssqlMethods()

	require := assert.New(t)
	require.Len(methods, 2)

	topIdx := -1
	deadlockIdx := -1
	for i := range methods {
		switch methods[i].ID {
		case "top-queries":
			topIdx = i
		case "deadlock-info":
			deadlockIdx = i
		}
	}

	require.NotEqual(-1, topIdx, "expected top-queries method")
	require.NotEqual(-1, deadlockIdx, "expected deadlock-info method")

	topMethod := methods[topIdx]
	require.Equal("Top Queries", topMethod.Name)
	require.NotEmpty(topMethod.RequiredParams)

	deadlockMethod := methods[deadlockIdx]
	require.Equal("Deadlock Info", deadlockMethod.Name)
	require.Empty(deadlockMethod.RequiredParams)

	var sortParam *funcapi.ParamConfig
	for i := range topMethod.RequiredParams {
		if topMethod.RequiredParams[i].ID == "__sort" {
			sortParam = &topMethod.RequiredParams[i]
			break
		}
	}
	require.NotNil(sortParam, "expected __sort required param")
	require.NotEmpty(sortParam.Options)
}

func TestTopQueriesColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"query", "totalTime", "calls"}

	f := &funcTopQueries{}
	cs := f.columnSet(topQueriesColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}
