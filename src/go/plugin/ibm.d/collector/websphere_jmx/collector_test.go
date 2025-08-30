// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	assert.NotNil(t, New())
}

func TestWebSphereJMX_Configuration(t *testing.T) {
	module := New()
	assert.NotNil(t, module.Configuration())
}

func TestWebSphereJMX_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"empty config": {
			config:   Config{},
			wantFail: true,
		},
		"missing jmx_url": {
			config: Config{
				UpdateEvery: 5,
			},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ws := New()
			ws.Config = test.config

			if test.wantFail {
				assert.Error(t, ws.Init(context.Background()))
			} else {
				assert.NoError(t, ws.Init(context.Background()))
			}
		})
	}
}

func TestWebSphereJMX_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestWebSphereJMX_Cleanup(t *testing.T) {
	// Cleanup should not panic
	New().Cleanup(context.Background())
}

func TestCleanName(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"simple":       {input: "test", expected: "test"},
		"with spaces":  {input: "test name", expected: "test_name"},
		"with dots":    {input: "test.name", expected: "test_name"},
		"with slashes": {input: "test/name", expected: "test_name"},
		"mixed":        {input: "Test-Name:123", expected: "test_name_123"},
		"complex":      {input: "jdbc/DB2(user)", expected: "jdbc_db2_user_"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, cleanName(test.input))
		})
	}
}

func TestGetFloat(t *testing.T) {
	tests := map[string]struct {
		data     map[string]interface{}
		key      string
		expected float64
	}{
		"float64": {
			data:     map[string]interface{}{"value": float64(123.45)},
			key:      "value",
			expected: 123.45,
		},
		"int": {
			data:     map[string]interface{}{"value": 123},
			key:      "value",
			expected: 123.0,
		},
		"string": {
			data:     map[string]interface{}{"value": "123.45"},
			key:      "value",
			expected: 123.45,
		},
		"missing": {
			data:     map[string]interface{}{},
			key:      "value",
			expected: 0.0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, getFloat(test.data, test.key))
		})
	}
}

func TestWebSphereJMX_ValidateCardinality(t *testing.T) {
	ws := New()

	// Test negative values get reset to 0
	ws.Config.MaxThreadPools = -1
	ws.Config.MaxJDBCPools = -5
	ws.Config.MaxJMSDestinations = -10
	ws.Config.MaxApplications = -20

	require.NoError(t, ws.Init(context.Background()))

	assert.Equal(t, 0, ws.Config.MaxThreadPools)
	assert.Equal(t, 0, ws.Config.MaxJDBCPools)
	assert.Equal(t, 0, ws.Config.MaxJMSDestinations)
	assert.Equal(t, 0, ws.Config.MaxApplications)
}
