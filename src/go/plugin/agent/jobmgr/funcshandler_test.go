// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
)

// mockMethodHandler implements funcapi.MethodHandler for testing.
type mockMethodHandler struct {
	job        *jobruntime.Job
	paramsFunc func(ctx context.Context, method string) ([]funcapi.ParamConfig, error)
	handleFunc func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse
}

func (m *mockMethodHandler) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if m.paramsFunc != nil {
		return m.paramsFunc(ctx, method)
	}
	return nil, nil
}

func (m *mockMethodHandler) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, method, params)
	}
	return nil
}

func (m *mockMethodHandler) Cleanup(ctx context.Context) {}

func TestExtractParamValues(t *testing.T) {
	tests := map[string]struct {
		payload  map[string]any
		key      string
		expected []string
	}{
		"string value": {
			payload:  map[string]any{"__job": "local"},
			key:      "__job",
			expected: []string{"local"},
		},
		"array value (single element)": {
			payload:  map[string]any{"__job": []any{"local"}},
			key:      "__job",
			expected: []string{"local"},
		},
		"array value (multiple elements)": {
			payload:  map[string]any{"__sort": []any{"calls", "total_time"}},
			key:      "__sort",
			expected: []string{"calls", "total_time"},
		},
		"string array value": {
			payload:  map[string]any{"__job": []string{"local"}},
			key:      "__job",
			expected: []string{"local"},
		},
		"missing key": {
			payload:  map[string]any{"__job": "local"},
			key:      "__sort",
			expected: nil,
		},
		"empty payload": {
			payload:  map[string]any{},
			key:      "__job",
			expected: nil,
		},
		"nil in array": {
			payload:  map[string]any{"__job": []any{nil, "test"}},
			key:      "__job",
			expected: []string{"test"},
		},
		"empty array": {
			payload:  map[string]any{"__job": []any{}},
			key:      "__job",
			expected: nil,
		},
		"non-string value": {
			payload:  map[string]any{"__job": 123},
			key:      "__job",
			expected: nil,
		},
		"prefers selections": {
			payload: map[string]any{
				"__job": "root",
				"selections": map[string]any{
					"__job": []any{"selected"},
				},
			},
			key:      "__job",
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

func TestBuildParams(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"build accepted params": {
			run: func(t *testing.T) {
				sortDir := funcapi.FieldSortDescending
				methodParams := []funcapi.ParamConfig{
					{ID: "__sort", Selection: funcapi.ParamSelect, Options: []funcapi.ParamOption{{ID: "calls", Name: "Calls", Sort: &sortDir}}},
					{ID: "db"},
					{ID: "extra"},
				}

				result := buildAcceptedParams(methodParams)
				assert.Equal(t, []string{"__job", "__sort", "db", "extra"}, result)
			},
		},
		"build required params uses select type": {
			run: func(t *testing.T) {
				// Setup a minimal manager with test data.
				r := newModuleFuncRegistry()
				r.registerModule("postgres", collectorapi.Creator{
					Methods: func() []funcapi.MethodConfig {
						return []funcapi.MethodConfig{{
							ID:   "top-queries",
							Name: "Top Queries",
						}}
					},
				})
				r.addJob("postgres", "master-db", newTestModuleFuncsJob("master-db"))

				mgr := &Manager{moduleFuncs: r}
				methodParams := []funcapi.ParamConfig{
					{
						ID:         "__sort",
						Name:       "Filter By",
						Selection:  funcapi.ParamSelect,
						UniqueView: true,
						Options: []funcapi.ParamOption{
							{ID: "total_time", Name: "By Total Time", Default: true},
						},
					},
				}
				params := mgr.buildRequiredParams("postgres", methodParams)

				assert.Len(t, params, 2, "should have 2 required params: __job, __sort")
				for _, param := range params {
					paramType, ok := param["type"]
					assert.True(t, ok, "param should have type field")
					assert.Equal(t, "select", paramType, "param type must be 'select' for single-select, not 'multiselect'")

					assert.Contains(t, param, "id", "param should have id")
					assert.Contains(t, param, "name", "param should have name")
					assert.Contains(t, param, "options", "param should have options")
					assert.Contains(t, param, "unique_view", "param should have unique_view")

					uniqueView, _ := param["unique_view"].(bool)
					assert.True(t, uniqueView, "unique_view should be true")
				}

				assert.Equal(t, "__job", params[0]["id"])
				assert.Equal(t, "__sort", params[1]["id"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
