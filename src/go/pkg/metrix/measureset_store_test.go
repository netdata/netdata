// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func TestMeasureSetDeclarationValidation(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot MeasureSetGauge declaration requires WithMeasureSetFields": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency")
				})
			},
		},
		"stateful MeasureSetCounter declaration requires WithMeasureSetFields": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().StatefulMeter("svc").MeasureSetCounter("requests")
				})
			},
		},
		"MeasureSet declaration rejects duplicate field names": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(
							FieldSpec{Name: "value"},
							FieldSpec{Name: "value"},
						),
					)
				})
			},
		},
		"MeasureSet declaration rejects empty field names": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(FieldSpec{Name: "   "}),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on field drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(
						FieldSpec{Name: "value"},
						FieldSpec{Name: "max"},
					),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(
							FieldSpec{Name: "value"},
							FieldSpec{Name: "min"},
						),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on field float drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(FieldSpec{Name: "value", Float: false}),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(FieldSpec{Name: "value", Float: true}),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on semantics drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(FieldSpec{Name: "value"}),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetCounter("latency",
						WithMeasureSetFields(FieldSpec{Name: "value"}),
					)
				})
			},
		},
		"MeasureSet options are invalid for other instrument kinds": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("latency",
						WithMeasureSetFields(FieldSpec{Name: "value"}),
					)
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
