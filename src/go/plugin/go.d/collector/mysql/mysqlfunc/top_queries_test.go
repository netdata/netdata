// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMySQLMethods(t *testing.T) {
	methods := Methods()

	req := require.New(t)
	req.Len(methods, 3)

	topIdx := -1
	deadlockIdx := -1
	errorIdx := -1
	for i := range methods {
		switch methods[i].ID {
		case "top-queries":
			topIdx = i
		case "deadlock-info":
			deadlockIdx = i
		case "error-info":
			errorIdx = i
		}
	}

	req.NotEqual(-1, topIdx, "expected top-queries method")
	req.NotEqual(-1, deadlockIdx, "expected deadlock-info method")
	req.NotEqual(-1, errorIdx, "expected error-info method")

	topMethod := methods[topIdx]
	req.Equal("Top Queries", topMethod.Name)
	req.NotEmpty(topMethod.RequiredParams)

	deadlockMethod := methods[deadlockIdx]
	req.Equal("Deadlock Info", deadlockMethod.Name)
	req.Empty(deadlockMethod.RequiredParams)

	errorMethod := methods[errorIdx]
	req.Equal("Error Info", errorMethod.Name)
	req.Empty(errorMethod.RequiredParams)

	var sortParam *funcapi.ParamConfig
	for i := range topMethod.RequiredParams {
		if topMethod.RequiredParams[i].ID == "__sort" {
			sortParam = &topMethod.RequiredParams[i]
			break
		}
	}
	req.NotNil(sortParam, "expected __sort required param")
	req.NotEmpty(sortParam.Options)
}

func TestTopQueriesColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"digest", "query", "totalTime", "calls"}

	f := &funcTopQueries{}
	cs := f.columnSet(topQueriesColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}
