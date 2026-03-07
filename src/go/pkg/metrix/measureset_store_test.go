// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
							MeasureFieldSpec{Name: "value"},
							MeasureFieldSpec{Name: "value"},
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
						WithMeasureSetFields(MeasureFieldSpec{Name: "   "}),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on field drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "value"},
						MeasureFieldSpec{Name: "max"},
					),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(
							MeasureFieldSpec{Name: "value"},
							MeasureFieldSpec{Name: "min"},
						),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on field float drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(MeasureFieldSpec{Name: "value", Float: false}),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
						WithMeasureSetFields(MeasureFieldSpec{Name: "value", Float: true}),
					)
				})
			},
		},
		"MeasureSet schema mismatch panics on semantics drift": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				_ = s.Write().SnapshotMeter("svc").MeasureSetGauge("latency",
					WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
				)
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").MeasureSetCounter("latency",
						WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
					)
				})
			},
		},
		"MeasureSet options are invalid for other instrument kinds": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				expectPanic(t, func() {
					_ = s.Write().SnapshotMeter("svc").Gauge("latency",
						WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
					)
				})
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestMeasureSetStoreScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"snapshot MeasureSet gauge read and flatten metadata": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().SnapshotMeter("svc").MeasureSetGauge(
					"latency",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "value"},
						MeasureFieldSpec{Name: "ratio", Float: true},
					),
					WithDescription("Latency"),
					WithChartFamily("Service"),
					WithUnit("seconds"),
				)

				cc.BeginCycle()
				ms.ObservePoint(MeasureSetPoint{Values: []SampleValue{1.5, 0.5}})
				cc.CommitCycleSuccess()

				mustMeasureSet(t, s.Read(), "svc.latency", nil, []SampleValue{1.5, 0.5})

				rawMeta, ok := s.Read().SeriesMeta("svc.latency", nil)
				require.True(t, ok)
				require.Equal(t, MetricKindMeasureSet, rawMeta.Kind)
				require.Equal(t, MetricKindMeasureSet, rawMeta.SourceKind)
				require.Equal(t, FlattenRoleNone, rawMeta.FlattenRole)

				flat := s.Read(ReadFlatten())
				mustValue(t, flat, "svc.latency_value", measureSetFieldLabels("svc.latency", "value"), 1.5)
				mustValue(t, flat, "svc.latency_ratio", measureSetFieldLabels("svc.latency", "ratio"), 0.5)
				_, ok = flat.Value("svc.latency_value", nil)
				require.False(t, ok, "expected flattened MeasureSet scalar lookup without synthetic field label to miss")
				_, ok = flat.MeasureSet("svc.latency", nil)
				require.False(t, ok, "expected flattened view to hide typed MeasureSet getter")

				flatMeta, ok := flat.SeriesMeta("svc.latency_ratio", measureSetFieldLabels("svc.latency", "ratio"))
				require.True(t, ok)
				require.Equal(t, MetricKindGauge, flatMeta.Kind)
				require.Equal(t, MetricKindMeasureSet, flatMeta.SourceKind)
				require.Equal(t, FlattenRoleMeasureSetField, flatMeta.FlattenRole)

				meta, ok := flat.MetricMeta("svc.latency_ratio")
				require.True(t, ok)
				require.Equal(t, "Latency", meta.Description)
				require.Equal(t, "Service", meta.ChartFamily)
				require.Equal(t, "seconds", meta.Unit)
				require.True(t, meta.Float)
			},
		},
		"stateful MeasureSet gauge add baselines from committed and remains visible": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().StatefulMeter("svc").MeasureSetGauge(
					"usage",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "value"},
						MeasureFieldSpec{Name: "limit"},
					),
				)

				cc.BeginCycle()
				ms.SetPoint(MeasureSetPoint{Values: []SampleValue{10, 20}})
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				ms.AddPoint(MeasureSetPoint{Values: []SampleValue{2, 3}})
				ms.AddPoint(MeasureSetPoint{Values: []SampleValue{1, 0}})
				cc.CommitCycleSuccess()
				mustMeasureSet(t, s.Read(), "svc.usage", nil, []SampleValue{13, 23})

				cc.BeginCycle()
				cc.CommitCycleSuccess()
				mustMeasureSet(t, s.Read(), "svc.usage", nil, []SampleValue{13, 23})
				mustValue(t, s.Read(ReadFlatten()), "svc.usage_value", measureSetFieldLabels("svc.usage", "value"), 13)
				mustValue(t, s.Read(ReadFlatten()), "svc.usage_limit", measureSetFieldLabels("svc.usage", "limit"), 23)
			},
		},
		"snapshot MeasureSet counter flatten delta and reset-aware semantics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().SnapshotMeter("svc").MeasureSetCounter(
					"requests",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "ok"},
						MeasureFieldSpec{Name: "failed"},
					),
				)

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{100, 40}})
				cc.CommitCycleSuccess()
				mustMeasureSet(t, s.Read(), "svc.requests", nil, []SampleValue{100, 40})
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.requests_ok", measureSetFieldLabels("svc.requests", "ok"))

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{150, 50}})
				cc.CommitCycleSuccess()
				mustDelta(t, s.Read(ReadFlatten()), "svc.requests_ok", measureSetFieldLabels("svc.requests", "ok"), 50)
				mustDelta(t, s.Read(ReadFlatten()), "svc.requests_failed", measureSetFieldLabels("svc.requests", "failed"), 10)

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{20, 5}})
				cc.CommitCycleSuccess()
				mustDelta(t, s.Read(ReadFlatten()), "svc.requests_ok", measureSetFieldLabels("svc.requests", "ok"), 20)
				mustDelta(t, s.Read(ReadFlatten()), "svc.requests_failed", measureSetFieldLabels("svc.requests", "failed"), 5)
			},
		},
		"snapshot MeasureSet counter delta unavailable on attempt gap": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().SnapshotMeter("svc").MeasureSetCounter(
					"jobs",
					WithMeasureSetFields(MeasureFieldSpec{Name: "done"}),
				)

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{10}})
				cc.CommitCycleSuccess()

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{20}})
				cc.CommitCycleSuccess()
				mustDelta(t, s.Read(ReadFlatten()), "svc.jobs_done", measureSetFieldLabels("svc.jobs", "done"), 10)

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{30}})
				cc.AbortCycle()

				cc.BeginCycle()
				ms.ObserveTotalPoint(MeasureSetPoint{Values: []SampleValue{40}})
				cc.CommitCycleSuccess()
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.jobs_done", measureSetFieldLabels("svc.jobs", "done"))
			},
		},
		"stateful MeasureSet counter add accumulates and flattened delta works": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().StatefulMeter("svc").MeasureSetCounter(
					"events",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "ok"},
						MeasureFieldSpec{Name: "failed"},
					),
				)

				cc.BeginCycle()
				ms.AddPoint(MeasureSetPoint{Values: []SampleValue{5, 1}})
				cc.CommitCycleSuccess()
				mustNoDelta(t, s.Read(ReadFlatten()), "svc.events_ok", measureSetFieldLabels("svc.events", "ok"))

				cc.BeginCycle()
				ms.AddPoint(MeasureSetPoint{Values: []SampleValue{2, 3}})
				ms.AddPoint(MeasureSetPoint{Values: []SampleValue{1, 0}})
				cc.CommitCycleSuccess()
				mustMeasureSet(t, s.Read(), "svc.events", nil, []SampleValue{8, 4})
				mustDelta(t, s.Read(ReadFlatten()), "svc.events_ok", measureSetFieldLabels("svc.events", "ok"), 3)
				mustDelta(t, s.Read(ReadFlatten()), "svc.events_failed", measureSetFieldLabels("svc.events", "failed"), 3)
			},
		},
		"stateful MeasureSet counter negative add panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().StatefulMeter("svc").MeasureSetCounter(
					"events",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "ok"},
						MeasureFieldSpec{Name: "failed"},
					),
				)

				cc.BeginCycle()
				expectPanic(t, func() {
					ms.AddPoint(MeasureSetPoint{Values: []SampleValue{1, -1}})
				})
				cc.AbortCycle()
			},
		},
		"MeasureSet direct read returns a copy": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().SnapshotMeter("svc").MeasureSetGauge(
					"latency",
					WithMeasureSetFields(MeasureFieldSpec{Name: "value"}),
				)

				cc.BeginCycle()
				ms.ObservePoint(MeasureSetPoint{Values: []SampleValue{7}})
				cc.CommitCycleSuccess()

				p, ok := s.Read().MeasureSet("svc.latency", nil)
				require.True(t, ok)
				p.Values[0] = 99

				mustMeasureSet(t, s.Read(), "svc.latency", nil, []SampleValue{7})
			},
		},
		"MeasureSet point length mismatch panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				ms := s.Write().SnapshotMeter("svc").MeasureSetGauge(
					"latency",
					WithMeasureSetFields(
						MeasureFieldSpec{Name: "value"},
						MeasureFieldSpec{Name: "max"},
					),
				)

				cc.BeginCycle()
				expectPanic(t, func() {
					ms.ObservePoint(MeasureSetPoint{Values: []SampleValue{1}})
				})
				cc.AbortCycle()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func mustMeasureSet(t *testing.T, r Reader, name string, labels Labels, want []SampleValue) {
	t.Helper()
	got, ok := r.MeasureSet(name, labels)
	require.True(t, ok, "expected measureset for %s", name)
	require.Len(t, got.Values, len(want), "unexpected measureset size for %s", name)
	for i, w := range want {
		require.Equal(t, w, got.Values[i], "unexpected measureset value %d for %s", i, name)
	}
}
