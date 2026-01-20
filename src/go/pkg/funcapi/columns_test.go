// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColumnBuildColumn_AllFields(t *testing.T) {
	max := 99.5
	col := Column{
		Index:                 3,
		Name:                  "CPU",
		Type:                  FieldTypeInteger,
		Units:                 "ms",
		Visualization:         FieldVisualBar,
		Sort:                  FieldSortDescending,
		Sortable:              true,
		Sticky:                true,
		Summary:               FieldSummarySum,
		Filter:                FieldFilterRange,
		FullWidth:             true,
		Wrap:                  true,
		DefaultExpandedFilter: true,
		UniqueKey:             true,
		Visible:               true,
		Max:                   &max,
		PointerTo:             "details",
		Dummy:                 true,
		ValueOptions: ValueOptions{
			Transform:     FieldTransformDuration,
			DecimalPoints: 2,
			DefaultValue:  0,
		},
	}

	result := col.BuildColumn()

	assert.Equal(t, 3, result["index"])
	assert.Equal(t, "CPU", result["name"])
	assert.Equal(t, "integer", result["type"])
	assert.Equal(t, "bar", result["visualization"])
	assert.Equal(t, "descending", result["sort"])
	assert.Equal(t, true, result["sortable"])
	assert.Equal(t, true, result["sticky"])
	assert.Equal(t, "sum", result["summary"])
	assert.Equal(t, "range", result["filter"])
	assert.Equal(t, true, result["full_width"])
	assert.Equal(t, true, result["wrap"])
	assert.Equal(t, true, result["default_expanded_filter"])
	assert.Equal(t, true, result["unique_key"])
	assert.Equal(t, true, result["visible"])
	assert.Equal(t, "ms", result["units"])
	assert.Equal(t, max, result["max"])
	assert.Equal(t, "details", result["pointer_to"])
	assert.Equal(t, true, result["dummy"])

	valueOpts, ok := result["value_options"].(map[string]any)
	require.True(t, ok, "value_options should be a map")
	assert.Equal(t, "duration", valueOpts["transform"])
	assert.Equal(t, 2, valueOpts["decimal_points"])
	assert.Equal(t, 0, valueOpts["default_value"])
	assert.Equal(t, "ms", valueOpts["units"])
}

func TestColumnBuildColumn_NoOptionalFields(t *testing.T) {
	col := Column{
		Index: 0,
		Name:  "Query",
		Type:  FieldTypeString,
		ValueOptions: ValueOptions{
			Transform: FieldTransformNone,
		},
	}

	result := col.BuildColumn()

	_, hasUnits := result["units"]
	_, hasMax := result["max"]
	_, hasPointer := result["pointer_to"]
	_, hasDummy := result["dummy"]
	assert.False(t, hasUnits, "units should be omitted when empty")
	assert.False(t, hasMax, "max should be omitted when nil")
	assert.False(t, hasPointer, "pointer_to should be omitted when empty")
	assert.False(t, hasDummy, "dummy should be omitted when false")

	valueOpts, ok := result["value_options"].(map[string]any)
	require.True(t, ok, "value_options should be a map")
	_, hasValueUnits := valueOpts["units"]
	assert.False(t, hasValueUnits, "value_options.units should be omitted when Units empty")
}
