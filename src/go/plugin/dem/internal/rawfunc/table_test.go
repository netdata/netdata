// SPDX-License-Identifier: GPL-3.0-or-later

package rawfunc

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

type point struct {
	name string
	x    int
}

func TestColumnsIndexAndRowFollowDefinitionOrder(t *testing.T) {
	columns := Columns[point]{
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "name",
				Tooltip:   "Name",
				Type:      funcapi.FieldTypeString,
				Visible:   true,
				UniqueKey: true,
			},
			Value: func(p point) any { return p.name },
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "x",
				Tooltip:   "X",
				Type:      funcapi.FieldTypeInteger,
				Units:     "px",
				Sortable:  true,
				Transform: funcapi.FieldTransformNumber,
			},
			Value: func(p point) any { return p.x },
		},
	}
	built := columns.Build()
	assert.Equal(t, map[string]any{
		"name": funcapi.Column{
			Index:         0,
			Name:          "Name",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Visible:       true,
			UniqueKey:     true,
		}.BuildColumn(),
		"x": funcapi.Column{
			Index:         1,
			Name:          "X",
			Type:          funcapi.FieldTypeInteger,
			Units:         "px",
			Visualization: funcapi.FieldVisualValue,
			Sortable:      true,
			ValueOptions: funcapi.ValueOptions{
				Transform: funcapi.FieldTransformNumber,
			},
		}.BuildColumn(),
	}, built)
	assert.Equal(t, []any{"a", 7}, columns.Row(point{name: "a", x: 7}))
}

func TestMethodDeclarationAndTable(t *testing.T) {
	m := Method{
		ID:          "dem-example",
		Title:       "Example",
		Help:        "Example help",
		Sort:        "name",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"site"},
		Columns:     map[string]any{"name": map[string]any{}},
	}
	assert.Equal(t, funcapi.FunctionConfig{
		ID:                "dem-example",
		FunctionName:      "dem-example",
		Name:              "Example",
		UpdateEvery:       10,
		Help:              "Example help",
		Tags:              "dem",
		RequireCloud:      true,
		RequireCommercial: true,
		RawRequest:        true,
		ManagedInfo:       true,
		HasHistory:        true,
		AcceptedParams:    []string{"site"},
	}, m.Declaration("dem"))
	assert.Equal(t, map[string]any{
		"status":              200,
		"type":                "table",
		"has_history":         true,
		"update_every":        10,
		"help":                "Example help",
		"columns":             m.Columns,
		"data":                [][]any{},
		"default_sort_column": "name",
		"accepted_params":     []string{"site"},
	}, m.Table())

	m.Params = nil
	assert.NotContains(t, m.Table(), "accepted_params")
	assert.Nil(t, m.Declaration("dem").AcceptedParams)
}
