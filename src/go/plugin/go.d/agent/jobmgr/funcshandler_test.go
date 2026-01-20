// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
)

func TestExtractParamValue(t *testing.T) {
	tests := map[string]struct {
		payload  map[string]any
		key      string
		expected string
	}{
		"string value": {
			payload:  map[string]any{"__method": "top-queries"},
			key:      "__method",
			expected: "top-queries",
		},
		"array value (single element)": {
			payload:  map[string]any{"__method": []any{"top-queries"}},
			key:      "__method",
			expected: "top-queries",
		},
		"array value (multiple elements - takes first)": {
			payload:  map[string]any{"__sort": []any{"calls", "total_time"}},
			key:      "__sort",
			expected: "calls",
		},
		"string array value": {
			payload:  map[string]any{"__job": []string{"master-db"}},
			key:      "__job",
			expected: "master-db",
		},
		"missing key": {
			payload:  map[string]any{"__method": "top-queries"},
			key:      "__job",
			expected: "",
		},
		"empty payload": {
			payload:  map[string]any{},
			key:      "__method",
			expected: "",
		},
		"nil in array": {
			payload:  map[string]any{"__method": []any{nil, "test"}},
			key:      "__method",
			expected: "",
		},
		"empty array": {
			payload:  map[string]any{"__method": []any{}},
			key:      "__method",
			expected: "",
		},
		"non-string value": {
			payload:  map[string]any{"__method": 123},
			key:      "__method",
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := extractParamValue(tc.payload, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildMethodOptions(t *testing.T) {
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
			result := buildMethodOptions(tc.methods)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildSortOptions(t *testing.T) {
	tests := map[string]struct {
		sortOpts []module.SortOption
		expected []map[string]any
	}{
		"with explicit default": {
			sortOpts: []module.SortOption{
				{ID: "calls", Column: "COUNT_STAR", Label: "By Calls"},
				{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "By Total Time", Default: true},
			},
			expected: []map[string]any{
				{"id": "calls", "name": "By Calls", "sort": "descending"},
				{"id": "total_time", "name": "By Total Time", "defaultSelected": true, "sort": "descending"},
			},
		},
		"no explicit default (first becomes default)": {
			sortOpts: []module.SortOption{
				{ID: "calls", Column: "COUNT_STAR", Label: "By Calls"},
				{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "By Total Time"},
			},
			expected: []map[string]any{
				{"id": "calls", "name": "By Calls", "defaultSelected": true, "sort": "descending"},
				{"id": "total_time", "name": "By Total Time", "sort": "descending"},
			},
		},
		"single option": {
			sortOpts: []module.SortOption{
				{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "By Total Time"},
			},
			expected: []map[string]any{
				{"id": "total_time", "name": "By Total Time", "defaultSelected": true, "sort": "descending"},
			},
		},
		"empty options": {
			sortOpts: []module.SortOption{},
			expected: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := buildSortOptions(tc.sortOpts)
			assert.Equal(t, tc.expected, result)
		})
	}
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
				SortOptions: []module.SortOption{
					{ID: "total_time", Column: "total_exec_time", Label: "By Total Time", Default: true},
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
