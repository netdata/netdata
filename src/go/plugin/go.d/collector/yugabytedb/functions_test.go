// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestYugabyteDBMethods(t *testing.T) {
	methods := yugabyteMethods()

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

func TestYugabyteDBTopColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"queryId", "query", "calls", "totalTime"}

	uiKeys := make(map[string]bool)
	for _, col := range ybTopColumns {
		uiKeys[col.id] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}

func TestYugabyteDBRunningColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"pid", "query", "elapsedMs"}

	uiKeys := make(map[string]bool)
	for _, col := range ybRunningColumns {
		uiKeys[col.id] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}
