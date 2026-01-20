// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import "encoding/json"

// FieldType defines the column data type.
type FieldType uint8

const (
	FieldTypeNone FieldType = iota
	FieldTypeInteger
	FieldTypeFloat
	FieldTypeBoolean
	FieldTypeString
	FieldTypeDetailString
	FieldTypeBarWithInteger
	FieldTypeDuration
	FieldTypeTimestamp
	FieldTypeArray
)

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
	default:
		return "none"
	}
}

func (t FieldType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// FieldVisual defines how values are rendered.
type FieldVisual uint8

const (
	FieldVisualValue FieldVisual = iota
	FieldVisualBar
	FieldVisualPill
	FieldVisualRichValue
	FieldVisualRowOptions
)

func (v FieldVisual) String() string {
	switch v {
	case FieldVisualBar:
		return "bar"
	case FieldVisualPill:
		return "pill"
	case FieldVisualRichValue:
		return "richValue"
	case FieldVisualRowOptions:
		return "rowOptions"
	default:
		return "value"
	}
}

func (v FieldVisual) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// FieldTransform defines value formatting.
type FieldTransform uint8

const (
	FieldTransformNone FieldTransform = iota
	FieldTransformNumber
	FieldTransformDuration
	FieldTransformDatetime
	FieldTransformDatetimeUsec
	FieldTransformText
	FieldTransformXML
)

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

func (t FieldTransform) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// FieldSort defines the sort direction for a column.
type FieldSort uint8

const (
	FieldSortAscending FieldSort = iota
	FieldSortDescending
)

func (s FieldSort) String() string {
	switch s {
	case FieldSortDescending:
		return "descending"
	default:
		return "ascending"
	}
}

func (s FieldSort) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// FieldSummary defines aggregation behavior.
type FieldSummary uint8

const (
	FieldSummaryCount FieldSummary = iota
	FieldSummaryUniqueCount
	FieldSummarySum
	FieldSummaryMin
	FieldSummaryMax
	FieldSummaryMean
	FieldSummaryMedian
)

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

func (s FieldSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// FieldFilter defines filter UI type.
type FieldFilter uint8

const (
	FieldFilterNone FieldFilter = iota
	FieldFilterRange
	FieldFilterMultiselect
	FieldFilterText
	FieldFilterFacet
)

func (f FieldFilter) String() string {
	switch f {
	case FieldFilterRange:
		return "range"
	case FieldFilterMultiselect:
		return "multiselect"
	case FieldFilterText:
		return "text"
	case FieldFilterFacet:
		return "facet"
	default:
		return "none"
	}
}

func (f FieldFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}
