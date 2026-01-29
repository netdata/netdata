// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestOracleDBMethods(t *testing.T) {
	methods := oracledbMethods()

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

func TestQueriesTopColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"sqlId", "query", "totalTime", "executions"}

	for _, id := range required {
		found := false
		for _, col := range topQueriesColumns {
			if col.Name == id {
				found = true
				break
			}
		}
		assert.True(t, found, "column %s should be defined", id)
	}
}

func TestQueriesRunningColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"sessionId", "query", "lastCallMs"}

	for _, id := range required {
		found := false
		for _, col := range runningQueriesColumns {
			if col.Name == id {
				found = true
				break
			}
		}
		assert.True(t, found, "column %s should be defined", id)
	}
}
