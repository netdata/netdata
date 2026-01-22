// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import "encoding/json"

// FieldType defines the column data type used for rendering and alignment.
type FieldType uint8

const (
	// FieldTypeNone uses no special type handling.
	FieldTypeNone FieldType = iota
	// FieldTypeInteger is for integer counts and metrics.
	FieldTypeInteger
	// FieldTypeFloat is for fractional metrics.
	FieldTypeFloat
	// FieldTypeBoolean is for true/false values.
	FieldTypeBoolean
	// FieldTypeString is for text and categorical values.
	FieldTypeString
	// FieldTypeDetailString is used when the UI expects the detail-string type.
	FieldTypeDetailString
	// FieldTypeBarWithInteger is for progress-style values; set Max for bars.
	FieldTypeBarWithInteger
	// FieldTypeDuration is for duration values paired with a duration transform.
	FieldTypeDuration
	// FieldTypeTimestamp is for epoch timestamps paired with datetime transforms.
	FieldTypeTimestamp
	// FieldTypeArray is for array values.
	FieldTypeArray
	// FieldTypeFeedTemplate is for feed template rows.
	FieldTypeFeedTemplate
)

// String returns the UI keyword used for this field type.
func (t FieldType) String() string {
	switch t {
	case FieldTypeInteger:
		return "integer"
	case FieldTypeFloat:
		return "float"
	case FieldTypeBoolean:
		return "boolean"
	case FieldTypeString:
		return "string"
	case FieldTypeDetailString:
		return "detail-string"
	case FieldTypeBarWithInteger:
		return "bar-with-integer"
	case FieldTypeDuration:
		return "duration"
	case FieldTypeTimestamp:
		return "timestamp"
	case FieldTypeArray:
		return "array"
	case FieldTypeFeedTemplate:
		return "feedTemplate"
	default:
		return "none"
	}
}

// MarshalJSON encodes the field type as a UI keyword.
func (t FieldType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// FieldVisual defines how values are rendered in the UI.
type FieldVisual uint8

const (
	// FieldVisualValue renders plain values.
	FieldVisualValue FieldVisual = iota
	// FieldVisualBar renders values as bars.
	FieldVisualBar
	// FieldVisualPill renders values as pills for categories/status.
	FieldVisualPill
	// FieldVisualRichValue selects the richValue visualization.
	FieldVisualRichValue
	// FieldVisualFeedTemplate selects the feedTemplate visualization.
	FieldVisualFeedTemplate
	// FieldVisualRowOptions selects the rowOptions visualization.
	FieldVisualRowOptions
)

// String returns the UI keyword used for this visualization.
func (v FieldVisual) String() string {
	switch v {
	case FieldVisualBar:
		return "bar"
	case FieldVisualPill:
		return "pill"
	case FieldVisualRichValue:
		return "richValue"
	case FieldVisualFeedTemplate:
		return "feedTemplate"
	case FieldVisualRowOptions:
		return "rowOptions"
	default:
		return "value"
	}
}

// MarshalJSON encodes the visualization as a UI keyword.
func (v FieldVisual) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// FieldTransform defines how raw values are formatted for display.
type FieldTransform uint8

const (
	// FieldTransformNone leaves values unformatted.
	FieldTransformNone FieldTransform = iota
	// FieldTransformNumber formats numbers and respects DecimalPoints.
	FieldTransformNumber
	// FieldTransformDuration formats duration values.
	FieldTransformDuration
	// FieldTransformDatetime formats millisecond timestamps (simple tables).
	FieldTransformDatetime
	// FieldTransformDatetimeUsec formats microsecond timestamps (log explorers).
	FieldTransformDatetimeUsec
	// FieldTransformText selects the text transform.
	FieldTransformText
	// FieldTransformXML selects the xml transform.
	FieldTransformXML
)

// String returns the UI keyword used for this transform.
func (t FieldTransform) String() string {
	switch t {
	case FieldTransformNumber:
		return "number"
	case FieldTransformDuration:
		return "duration"
	case FieldTransformDatetime:
		return "datetime"
	case FieldTransformDatetimeUsec:
		return "datetime_usec"
	case FieldTransformText:
		return "text"
	case FieldTransformXML:
		return "xml"
	default:
		return "none"
	}
}

// MarshalJSON encodes the transform as a UI keyword.
func (t FieldTransform) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// FieldSort defines the default sort direction for a column.
type FieldSort uint8

const (
	// FieldSortAscending orders values from low to high.
	FieldSortAscending FieldSort = iota
	// FieldSortDescending orders values from high to low.
	FieldSortDescending
)

// String returns the UI keyword used for this sort direction.
func (s FieldSort) String() string {
	switch s {
	case FieldSortDescending:
		return "descending"
	default:
		return "ascending"
	}
}

// MarshalJSON encodes the sort direction as a UI keyword.
func (s FieldSort) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// FieldSummary defines aggregation behavior when grouping rows.
type FieldSummary uint8

const (
	// FieldSummaryCount counts rows in each group.
	FieldSummaryCount FieldSummary = iota
	// FieldSummaryUniqueCount counts unique values in each group.
	FieldSummaryUniqueCount
	// FieldSummarySum sums numeric values in each group.
	FieldSummarySum
	// FieldSummaryMin takes the minimum value in each group.
	FieldSummaryMin
	// FieldSummaryMax takes the maximum value in each group.
	FieldSummaryMax
	// FieldSummaryMean averages values in each group.
	FieldSummaryMean
	// FieldSummaryMedian takes the median value in each group.
	FieldSummaryMedian
)

// String returns the UI keyword used for this summary.
func (s FieldSummary) String() string {
	switch s {
	case FieldSummaryUniqueCount:
		return "uniqueCount"
	case FieldSummarySum:
		return "sum"
	case FieldSummaryMin:
		return "min"
	case FieldSummaryMax:
		return "max"
	case FieldSummaryMean:
		return "mean"
	case FieldSummaryMedian:
		return "median"
	default:
		return "count"
	}
}

// MarshalJSON encodes the summary as a UI keyword.
func (s FieldSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// FieldFilter defines the filter UI type for a column.
type FieldFilter uint8

const (
	// FieldFilterNone disables filtering for the column.
	FieldFilterNone FieldFilter = iota
	// FieldFilterRange is for numeric ranges.
	FieldFilterRange
	// FieldFilterMultiselect is for categorical values.
	FieldFilterMultiselect
	// FieldFilterFacet enables faceted filters (log explorers).
	FieldFilterFacet
)

// String returns the UI keyword used for this filter.
func (f FieldFilter) String() string {
	switch f {
	case FieldFilterRange:
		return "range"
	case FieldFilterMultiselect:
		return "multiselect"
	case FieldFilterFacet:
		return "facet"
	default:
		return "none"
	}
}

// MarshalJSON encodes the filter as a UI keyword.
func (f FieldFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}
