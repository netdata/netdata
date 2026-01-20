// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldType_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldType
		expected string
	}{
		{FieldTypeNone, "none"},
		{FieldTypeInteger, "integer"},
		{FieldTypeFloat, "float"},
		{FieldTypeBoolean, "boolean"},
		{FieldTypeString, "string"},
		{FieldTypeDetailString, "detail-string"},
		{FieldTypeBarWithInteger, "bar-with-integer"},
		{FieldTypeDuration, "duration"},
		{FieldTypeTimestamp, "timestamp"},
		{FieldTypeArray, "array"},
		{FieldType(250), "none"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := tc.value.MarshalJSON()
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
			data, err = json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestFieldVisual_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldVisual
		expected string
	}{
		{FieldVisualValue, "value"},
		{FieldVisualBar, "bar"},
		{FieldVisualPill, "pill"},
		{FieldVisualRichValue, "richValue"},
		{FieldVisualRowOptions, "rowOptions"},
		{FieldVisual(250), "value"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestFieldTransform_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldTransform
		expected string
	}{
		{FieldTransformNone, "none"},
		{FieldTransformNumber, "number"},
		{FieldTransformDuration, "duration"},
		{FieldTransformDatetime, "datetime"},
		{FieldTransformDatetimeUsec, "datetime_usec"},
		{FieldTransformText, "text"},
		{FieldTransformXML, "xml"},
		{FieldTransform(250), "none"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestFieldSort_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldSort
		expected string
	}{
		{FieldSortAscending, "ascending"},
		{FieldSortDescending, "descending"},
		{FieldSort(250), "ascending"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestFieldSummary_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldSummary
		expected string
	}{
		{FieldSummaryCount, "count"},
		{FieldSummaryUniqueCount, "uniqueCount"},
		{FieldSummarySum, "sum"},
		{FieldSummaryMin, "min"},
		{FieldSummaryMax, "max"},
		{FieldSummaryMean, "mean"},
		{FieldSummaryMedian, "median"},
		{FieldSummary(250), "count"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestFieldFilter_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    FieldFilter
		expected string
	}{
		{FieldFilterNone, "none"},
		{FieldFilterRange, "range"},
		{FieldFilterMultiselect, "multiselect"},
		{FieldFilterText, "text"},
		{FieldFilterFacet, "facet"},
		{FieldFilter(250), "none"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}

func TestParamSelection_StringAndJSON(t *testing.T) {
	cases := []struct {
		value    ParamSelection
		expected string
	}{
		{ParamSelect, "select"},
		{ParamMultiSelect, "multiselect"},
		{ParamSelection(250), "select"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, `"`+tc.expected+`"`, string(data))
		})
	}
}
