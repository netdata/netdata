// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamConfigRequiredParam(t *testing.T) {
	sortDir := FieldSortDescending
	cfg := ParamConfig{
		ID:         "__sort",
		Name:       "Sort",
		Help:       "Select a sort",
		Selection:  ParamSelect,
		UniqueView: true,
		Options: []ParamOption{
			{ID: "calls", Name: "Calls"},
			{ID: "time", Name: "Time", Default: true, Sort: &sortDir},
			{ID: "disabled", Name: "Disabled", Disabled: true},
		},
	}

	param := cfg.RequiredParam()
	assert.Equal(t, "__sort", param["id"])
	assert.Equal(t, "Sort", param["name"])
	assert.Equal(t, "select", param["type"])
	assert.Equal(t, "Select a sort", param["help"])
	assert.Equal(t, true, param["unique_view"])

	options, ok := param["options"].([]map[string]any)
	require.True(t, ok, "options should be []map[string]any")
	require.Len(t, options, 3)

	assert.Equal(t, "calls", options[0]["id"])
	assert.Equal(t, "Calls", options[0]["name"])
	_, hasDefault0 := options[0]["defaultSelected"]
	assert.False(t, hasDefault0)

	assert.Equal(t, "time", options[1]["id"])
	assert.Equal(t, "Time", options[1]["name"])
	assert.Equal(t, "descending", options[1]["sort"])
	assert.Equal(t, true, options[1]["defaultSelected"])

	assert.Equal(t, "disabled", options[2]["id"])
	assert.Equal(t, "Disabled", options[2]["name"])
	assert.Equal(t, true, options[2]["disabled"])

	data, err := json.Marshal(param)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\"defaultSelected\"")
}

func TestBuildParamOptions_DefaultFallback(t *testing.T) {
	cfg := ParamConfig{
		ID:        "db",
		Name:      "Database",
		Selection: ParamSelect,
		Options: []ParamOption{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
		},
	}

	param := cfg.RequiredParam()
	options := param["options"].([]map[string]any)
	require.Len(t, options, 2)
	assert.Equal(t, true, options[0]["defaultSelected"], "first option should be default when none explicitly set")
	_, hasDefault := options[1]["defaultSelected"]
	assert.False(t, hasDefault)
}

func TestResolveParam_Select(t *testing.T) {
	cfg := ParamConfig{
		ID:        "db",
		Name:      "Database",
		Selection: ParamSelect,
		Options: []ParamOption{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B", Default: true},
		},
	}

	resolved := ResolveParam(cfg, []string{"a"})
	assert.Equal(t, []string{"a"}, resolved.IDs)

	resolved = ResolveParam(cfg, []string{"missing"})
	assert.Equal(t, []string{"b"}, resolved.IDs)

	cfg.Options[1].Default = false
	resolved = ResolveParam(cfg, []string{"missing"})
	assert.Equal(t, []string{"a"}, resolved.IDs)
}

func TestResolveParam_MultiSelect(t *testing.T) {
	cfg := ParamConfig{
		ID:        "cols",
		Name:      "Columns",
		Selection: ParamMultiSelect,
		Options: []ParamOption{
			{ID: "a", Name: "A", Default: true},
			{ID: "b", Name: "B"},
			{ID: "c", Name: "C", Default: true},
		},
	}

	resolved := ResolveParam(cfg, []string{"b", "c", "missing"})
	assert.Equal(t, []string{"b", "c"}, resolved.IDs)

	resolved = ResolveParam(cfg, []string{"missing"})
	assert.Equal(t, []string{"a", "c"}, resolved.IDs)

	cfg.Options[0].Default = false
	cfg.Options[2].Default = false
	resolved = ResolveParam(cfg, nil)
	assert.Equal(t, []string{"a"}, resolved.IDs)
}

func TestResolvedParamsHelpers(t *testing.T) {
	params := ResolvedParams{
		"__sort": {
			IDs: []string{"calls"},
			Options: []ParamOption{
				{ID: "calls", Column: "count_calls"},
			},
		},
		"db": {
			IDs: []string{"main"},
			Options: []ParamOption{
				{ID: "main"},
			},
		},
	}

	assert.Equal(t, []string{"calls"}, params.Get("__sort"))
	assert.Equal(t, "calls", params.GetOne("__sort"))
	opt, ok := params.Option("__sort")
	require.True(t, ok)
	assert.Equal(t, "calls", opt.ID)
	assert.Equal(t, "count_calls", params.Column("__sort"))
	assert.Equal(t, "main", params.Column("db"))
	assert.Equal(t, "", params.Column("missing"))
}

func TestMergeParamConfigs(t *testing.T) {
	base := []ParamConfig{
		{ID: "a"},
		{ID: "b"},
	}
	overrides := []ParamConfig{
		{ID: "b", Name: "override"},
		{ID: "c"},
	}

	merged := MergeParamConfigs(base, overrides)
	require.Len(t, merged, 3)
	assert.Equal(t, "a", merged[0].ID)
	assert.Equal(t, "b", merged[1].ID)
	assert.Equal(t, "override", merged[1].Name)
	assert.Equal(t, "c", merged[2].ID)
}
