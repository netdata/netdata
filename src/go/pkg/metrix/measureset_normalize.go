// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

// Validation borrows the input; each backend copies or consumes it before returning.
// Only snapshot gauges may represent unavailable fields with NaN.
func validateMeasureSetPoint(point MeasureSetPoint, schema *measureSetSchema, allowUnavailable bool) {
	if schema == nil {
		panic(errMeasureSetSchema)
	}
	if len(point.Values) != len(schema.fields) {
		panic(errMeasureSetPoint)
	}

	for _, v := range point.Values {
		if math.IsInf(v, 0) || (!allowUnavailable && math.IsNaN(v)) {
			panic(errInvalidSampleValue)
		}
	}
}

// Map the complete named shape; the backend validates values for its write mode.
func measureSetPointFromFields(fields map[string]SampleValue, schema *measureSetSchema) MeasureSetPoint {
	if schema == nil {
		panic(errMeasureSetSchema)
	}
	if len(fields) != len(schema.fields) {
		panic(errMeasureSetFields)
	}

	values := make([]SampleValue, len(schema.fields))
	for field, value := range fields {
		idx, ok := schema.index[field]
		if !ok {
			panic(errMeasureSetField)
		}
		values[idx] = value
	}
	return MeasureSetPoint{Values: values}
}

func singleMeasureSetPoint(field string, value SampleValue, schema *measureSetSchema) MeasureSetPoint {
	values := make([]SampleValue, len(schema.fields))
	idx := mustMeasureSetFieldIndex(field, schema)
	mustFiniteSample(value)
	values[idx] = value
	return MeasureSetPoint{Values: values}
}

func mustMeasureSetFieldIndex(field string, schema *measureSetSchema) int {
	if schema == nil {
		panic(errMeasureSetSchema)
	}
	idx, ok := schema.index[field]
	if !ok {
		panic(errMeasureSetField)
	}
	return idx
}

func validateMeasureSetCounterDelta(delta MeasureSetPoint, schema *measureSetSchema) {
	validateMeasureSetPoint(delta, schema, false)
	for _, v := range delta.Values {
		if v < 0 {
			panic(errCounterNegativeDelta)
		}
	}
}
