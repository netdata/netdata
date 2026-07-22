// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func normalizeMeasureSetPoint(point MeasureSetPoint, schema *measureSetSchema) []SampleValue {
	if schema == nil {
		panic(errMeasureSetSchema)
	}
	if len(point.Values) != len(schema.fields) {
		panic(errMeasureSetPoint)
	}

	values := make([]SampleValue, len(point.Values))
	for i, v := range point.Values {
		mustFiniteSample(v)
		values[i] = v
	}
	return values
}

func measureSetPointFromFields(fields map[string]SampleValue, schema *measureSetSchema) MeasureSetPoint {
	return MeasureSetPoint{Values: normalizeMeasureSetFields(fields, schema)}
}

func normalizeMeasureSetFields(fields map[string]SampleValue, schema *measureSetSchema) []SampleValue {
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
		mustFiniteSample(value)
		values[idx] = value
	}
	return values
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

func normalizeMeasureSetCounterDelta(delta MeasureSetPoint, schema *measureSetSchema) []SampleValue {
	values := normalizeMeasureSetPoint(delta, schema)
	for _, v := range values {
		if v < 0 {
			panic(errCounterNegativeDelta)
		}
	}
	return values
}
