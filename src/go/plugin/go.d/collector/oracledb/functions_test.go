// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestOracleMethods(t *testing.T) {
	methods := oracleMethods()

	require := assert.New(t)
	require.Len(methods, 2)

	ids := map[string]bool{}
	for _, m := range methods {
		ids[m.ID] = true
		var sortParam *funcapi.ParamConfig
		for i := range m.RequiredParams {
			if m.RequiredParams[i].ID == "__sort" {
				sortParam = &m.RequiredParams[i]
				break
			}
		}
		require.NotNil(sortParam, "expected __sort required param for %s", m.ID)
		require.NotEmpty(sortParam.Options)
	}

	require.True(ids["top-queries"], "top-queries method missing")
	require.True(ids["running-queries"], "running-queries method missing")
}

func TestOracleTopColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"query", "executions", "totalTime"}

	uiKeys := make(map[string]bool)
	for _, col := range oracleTopColumns {
		uiKeys[col.id] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}

func TestOracleRunningColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"sessionId", "query", "lastCallMs"}

	uiKeys := make(map[string]bool)
	for _, col := range oracleRunningColumns {
		uiKeys[col.id] = true
	}

	for _, key := range required {
		assert.True(t, uiKeys[key], "column %s should be defined", key)
	}
}
