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
				{"id": "calls", "name": "By Calls"},
				{"id": "total_time", "name": "By Total Time", "defaultSelected": true},
			},
		},
		"no explicit default (first becomes default)": {
			sortOpts: []module.SortOption{
				{ID: "calls", Column: "COUNT_STAR", Label: "By Calls"},
				{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "By Total Time"},
			},
			expected: []map[string]any{
				{"id": "calls", "name": "By Calls", "defaultSelected": true},
				{"id": "total_time", "name": "By Total Time"},
			},
		},
		"single option": {
			sortOpts: []module.SortOption{
				{ID: "total_time", Column: "SUM_TIMER_WAIT", Label: "By Total Time"},
			},
			expected: []map[string]any{
				{"id": "total_time", "name": "By Total Time", "defaultSelected": true},
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
