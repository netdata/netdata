// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
)

func TestExtractParamValues(t *testing.T) {
	tests := map[string]struct {
		payload  map[string]any
		key      string
		expected []string
	}{
		"string value": {
			payload:  map[string]any{"__method": "top-queries"},
			key:      "__method",
			expected: []string{"top-queries"},
		},
		"array value (single element)": {
			payload:  map[string]any{"__method": []any{"top-queries"}},
			key:      "__method",
			expected: []string{"top-queries"},
		},
		"array value (multiple elements)": {
			payload:  map[string]any{"__sort": []any{"calls", "total_time"}},
			key:      "__sort",
			expected: []string{"calls", "total_time"},
		},
		"string array value": {
			payload:  map[string]any{"__job": []string{"master-db"}},
			key:      "__job",
			expected: []string{"master-db"},
		},
		"missing key": {
			payload:  map[string]any{"__method": "top-queries"},
			key:      "__job",
			expected: nil,
		},
		"empty payload": {
			payload:  map[string]any{},
			key:      "__method",
			expected: nil,
		},
		"nil in array": {
			payload:  map[string]any{"__method": []any{nil, "test"}},
			key:      "__method",
			expected: []string{"test"},
		},
		"empty array": {
			payload:  map[string]any{"__method": []any{}},
			key:      "__method",
			expected: nil,
		},
		"non-string value": {
			payload:  map[string]any{"__method": 123},
			key:      "__method",
			expected: nil,
		},
		"prefers selections": {
			payload: map[string]any{
				"__method": "root",
				"selections": map[string]any{
					"__method": []any{"selected"},
				},
			},
			key:      "__method",
			expected: []string{"selected"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := extractParamValues(tc.payload, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildMethodParamConfig(t *testing.T) {
	tests := map[string]struct {
		methods  []module.MethodConfig
		expected []map[string]any
	}{
		"single method": {
			methods: []module.MethodConfig{
				{ID: "top-queries", Name: "Top Queries"},
			},
			expected: []map[string]any{
				{"id": "top-queries", "name": "Top Queries", "defaultSelected": true},
			},
		},
		"multiple methods": {
			methods: []module.MethodConfig{
				{ID: "top-queries", Name: "Top Queries"},
				{ID: "slow-queries", Name: "Slow Queries"},
			},
			expected: []map[string]any{
				{"id": "top-queries", "name": "Top Queries", "defaultSelected": true},
				{"id": "slow-queries", "name": "Slow Queries"},
			},
		},
		"empty methods": {
			methods:  []module.MethodConfig{},
			expected: []map[string]any{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := buildMethodParamConfig(tc.methods)
			param := cfg.RequiredParam()
			assert.Equal(t, "select", param["type"])
			assert.Equal(t, "__method", param["id"])
			assert.Equal(t, tc.expected, param["options"])
		})
	}
}

func TestBuildAcceptedParams(t *testing.T) {
	sortDir := funcapi.FieldSortDescending
	methodCfg := &module.MethodConfig{
		RequiredParams: []funcapi.ParamConfig{
			{ID: "__sort", Selection: funcapi.ParamSelect, Options: []funcapi.ParamOption{{ID: "calls", Name: "Calls", Sort: &sortDir}}},
			{ID: "db"},
		},
	}

	overrides := []funcapi.ParamConfig{
		{ID: "__sort", Selection: funcapi.ParamSelect, Options: []funcapi.ParamOption{{ID: "time", Name: "Time", Sort: &sortDir}}},
		{ID: "extra"},
	}

	result := buildAcceptedParams(methodCfg, overrides)
	assert.Equal(t, []string{"__method", "__job", "__sort", "db", "extra"}, result)
}

func TestFindMethod(t *testing.T) {
	methods := []module.MethodConfig{
		{ID: "top-queries", Name: "Top Queries"},
		{ID: "slow-queries", Name: "Slow Queries"},
	}

	tests := map[string]struct {
		id       string
		expected *module.MethodConfig
	}{
		"found": {
			id:       "top-queries",
			expected: &methods[0],
		},
		"found second": {
			id:       "slow-queries",
			expected: &methods[1],
		},
		"not found": {
			id:       "nonexistent",
			expected: nil,
		},
		"empty id": {
			id:       "",
			expected: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := findMethod(methods, tc.id)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMethodIDs(t *testing.T) {
	tests := map[string]struct {
		methods  []module.MethodConfig
		expected []string
	}{
		"multiple methods": {
			methods: []module.MethodConfig{
				{ID: "top-queries"},
				{ID: "slow-queries"},
			},
			expected: []string{"top-queries", "slow-queries"},
		},
		"single method": {
			methods: []module.MethodConfig{
				{ID: "top-queries"},
			},
			expected: []string{"top-queries"},
		},
		"empty methods": {
			methods:  []module.MethodConfig{},
			expected: []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := methodIDs(tc.methods)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestBuildRequiredParams_TypeSelect verifies that all selectors use type "select" (single-select)
// This is critical because type "multiselect" would show checkboxes instead of dropdowns
func TestBuildRequiredParams_TypeSelect(t *testing.T) {
	// Setup a minimal manager with test data
	r := newModuleFuncRegistry()
	r.registerModule("postgres", module.Creator{
		Methods: func() []module.MethodConfig {
			return []module.MethodConfig{{
				ID:   "top-queries",
				Name: "Top Queries",
				RequiredParams: []funcapi.ParamConfig{
					{
						ID:         "__sort",
						Name:       "Filter By",
						Selection:  funcapi.ParamSelect,
						UniqueView: true,
						Options: []funcapi.ParamOption{
							{ID: "total_time", Name: "By Total Time", Default: true},
						},
					},
				},
			}}
		},
	})
	r.addJob("postgres", "master-db", newTestModuleFuncsJob("master-db"))

	// Create a manager with the registry
	mgr := &Manager{moduleFuncs: r}

	// Get required_params through the public method
	methods := r.getMethods("postgres")
	methodCfg := &methods[0]
	params := mgr.buildRequiredParams("postgres", methodCfg, nil)

	// Verify structure
	assert.Len(t, params, 3, "should have 3 required params: __method, __job, __sort")

	// All params should have type: "select" (NOT "multiselect")
	for _, param := range params {
		paramType, ok := param["type"]
		assert.True(t, ok, "param should have type field")
		assert.Equal(t, "select", paramType, "param type must be 'select' for single-select, not 'multiselect'")

		// Verify required fields exist
		assert.Contains(t, param, "id", "param should have id")
		assert.Contains(t, param, "name", "param should have name")
		assert.Contains(t, param, "options", "param should have options")
		assert.Contains(t, param, "unique_view", "param should have unique_view")

		// Verify unique_view is true
		uniqueView, _ := param["unique_view"].(bool)
		assert.True(t, uniqueView, "unique_view should be true")
	}

	// Verify specific param IDs
	assert.Equal(t, "__method", params[0]["id"])
	assert.Equal(t, "__job", params[1]["id"])
	assert.Equal(t, "__sort", params[2]["id"])
}
