// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestCockroachDBMethods(t *testing.T) {
	methods := cockroachMethods()

	require := assert.New(t)
	require.Len(methods, 2)

	for _, method := range methods {
		require.NotEmpty(method.ID)
		require.NotEmpty(method.Name)
		require.NotEmpty(method.RequiredParams)

		var sortParam *funcapi.ParamConfig
		for i := range method.RequiredParams {
			if method.RequiredParams[i].ID == "__sort" {
				sortParam = &method.RequiredParams[i]
				break
			}
		}
		require.NotNil(sortParam, "expected __sort required param")
		require.NotEmpty(sortParam.Options)
	}
}

func TestCockroachDBTopColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"fingerprintId", "query", "executions", "totalTime"}

	cs := crdbColumnSet(crdbTopColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func TestCockroachDBRunningColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"queryId", "query", "elapsedMs"}

	cs := crdbColumnSet(crdbRunningColumns)
	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}
