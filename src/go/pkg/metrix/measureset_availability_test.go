// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasureSetSnapshotAvailability(t *testing.T) {
	for name, tc := range map[string]struct {
		named  bool
		vector bool
		scoped bool
	}{
		"positional":          {},
		"named scoped":        {named: true, scoped: true},
		"vector positional":   {vector: true},
		"vector named scoped": {named: true, vector: true, scoped: true},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewCollectorStore(WithExpireAfterSuccessCycles(2))
			cc := cycleController(t, s)
			scope := HostScope{}
			if tc.scoped {
				scope = HostScope{
					ScopeKey: "remote",
					GUID:     "remote-guid",
					Hostname: "remote-host",
				}
			}
			meter := s.Write().SnapshotMeter("svc").WithLabels(Label{
				Key:   "app",
				Value: "demo",
			})
			fields := []MeasureFieldSpec{
				{Name: "min"}, {Name: "max", Float: true}, {Name: "mean", Float: true},
				{Name: "p50", Float: true}, {Name: "p95", Float: true},
			}
			opts := []InstrumentOption{
				WithMeasureSetFields(fields...),
				WithDescription("Values"),
				WithUnit("seconds"),
				WithChartFamily("Service"),
			}
			labels := Labels{
				"app":      "demo",
				"instance": "a",
			}
			var write func([]SampleValue)
			var gauge SnapshotMeasureSetGauge
			var sets []LabelSet
			if tc.vector {
				gauge = meter.Vec("instance").
					MeasureSetGauge("values", opts...).
					WithHostScope(scope).
					WithLabelValues("a")
			} else {
				gauge = meter.WithHostScope(scope).MeasureSetGauge("values", opts...)
				sets = []LabelSet{meter.LabelSet(Label{
					Key:   "instance",
					Value: "a",
				})}
			}
			write = func(values []SampleValue) {
				if tc.named {
					point := make(map[string]SampleValue, len(fields))
					for i, field := range fields {
						point[field.Name] = values[i]
					}
					gauge.ObserveFields(point, sets...)
					point["min"] = 999 // Writes must not retain caller-owned input.
				} else {
					point := append([]SampleValue(nil), values...)
					gauge.ObservePoint(MeasureSetPoint{
						Values: point,
					}, sets...)
					point[0] = 999
				}
			}
			// A different label identity in the same scope must stay independent.
			sibling := meter.WithHostScope(scope).
				WithLabels(Label{
					Key:   "instance",
					Value: "b",
				}).
				MeasureSetGauge("values", opts...)
			nan := math.NaN()
			steps := [][]SampleValue{
				{-2, 9.5, 0, 1.5, 8},
				{0, 4.25, -0.5, nan, 3},
				{nan, nan, nan, nan, nan},
				{nan, nan, nan, nan, nan},
				{nan, nan, nan, nan, nan},
				{1, 7.5, 2.25, 2, 7},
			}
			var firstReader, firstFlat Reader
			var firstIDs map[string]SeriesIdentity
			for i, want := range steps {
				cc.BeginCycle()
				write(want)
				sibling.ObservePoint(MeasureSetPoint{
					Values: []SampleValue{10, 20, 15, 15, 19},
				})
				require.NoError(t, cc.CommitCycleSuccess())
				reader := s.Read(ReadHostScope(scope.ScopeKey))
				flat := s.Read(ReadHostScope(scope.ScopeKey), ReadFlatten())
				mustMeasureSet(t, reader, "svc.values", labels, want)
				mustMeasureSet(
					t,
					reader,
					"svc.values",
					Labels{
						"app":      "demo",
						"instance": "b",
					},
					[]SampleValue{10, 20, 15, 15, 19},
				)
				point, ok := reader.MeasureSet("svc.values", labels)
				require.True(t, ok)
				point.Values[0] = 999
				mustMeasureSet(t, reader, "svc.values", labels, want)
				ids := make(map[string]SeriesIdentity)
				flat.ForEachSeriesIdentity(
					func(id SeriesIdentity, meta SeriesMeta, metric string, lv LabelView, value SampleValue) {
						instance, _ := lv.Get("instance")
						if instance == "a" {
							ids[metric] = id
						}
					},
				)
				require.Len(t, ids, 5)
				for j, field := range fields {
					fl := Labels{
						"app":                "demo",
						"instance":           "a",
						MeasureSetFieldLabel: field.Name,
					}
					value, ok := flat.Value("svc.values_"+field.Name, fl)
					require.True(t, ok, "unavailable still means present")
					assertMeasureSetSample(t, want[j], value)
					_, hasDelta := flat.Delta("svc.values_"+field.Name, fl)
					assert.False(t, hasDelta)
					meta, ok := flat.MetricMeta("svc.values_" + field.Name)
					require.True(t, ok)
					assert.Equal(t, "seconds", meta.Unit)
					assert.Equal(t, "Values", meta.Description)
					assert.Equal(t, "Service", meta.ChartFamily)
					assert.Equal(t, field.Float, meta.Float)
					series, ok := flat.SeriesMeta("svc.values_"+field.Name, fl)
					require.True(t, ok)
					assert.Equal(
						t,
						SeriesMeta{
							LastSeenSuccessSeq: uint64(i + 1),
							Kind:               MetricKindGauge,
							SourceKind:         MetricKindMeasureSet,
							FlattenRole:        FlattenRoleMeasureSetField,
						},
						series,
					)
				}
				if i == 0 {
					firstReader, firstFlat, firstIDs = reader, flat, ids
				} else {
					assert.Equal(t, firstIDs, ids)
					mustMeasureSet(t, firstReader, "svc.values", labels, steps[0])
					mustValue(t, firstFlat, "svc.values_min", Labels{
						"app":                "demo",
						"instance":           "a",
						MeasureSetFieldLabel: "min",
					}, -2)
				}
				if tc.scoped {
					_, ok := s.Read().MeasureSet("svc.values", labels)
					assert.False(t, ok, "scoped family must not leak into the default scope")
				}
			}
			cc.BeginCycle()
			require.NoError(t, cc.CommitCycleSuccess())
			_, visible := s.Read(ReadHostScope(scope.ScopeKey)).MeasureSet("svc.values", labels)
			assert.False(t, visible, "not writing is different from writing unavailable")
			mustMeasureSet(
				t,
				s.Read(ReadRaw(), ReadHostScope(scope.ScopeKey)),
				"svc.values",
				labels,
				steps[len(steps)-1],
			)
			for range 2 {
				cc.BeginCycle()
				require.NoError(t, cc.CommitCycleSuccess())
			}
			_, retained := s.Read(ReadRaw(), ReadHostScope(scope.ScopeKey)).MeasureSet("svc.values", labels)
			assert.False(t, retained, "absent family follows normal expiry")
		})
	}
}

func TestMeasureSetAvailabilityWriteReplacementAndFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		first, last SampleValue
	}{
		"finite to unavailable": {first: 4, last: math.NaN()},
		"unavailable to finite": {first: math.NaN(), last: 0},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewCollectorStore()
			cc := cycleController(t, s)
			m := s.Write().SnapshotMeter("svc")
			g := m.MeasureSetGauge("value", WithMeasureSetFields(MeasureFieldSpec{
				Name: "field",
			}))
			cc.BeginCycle()
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{tc.first},
			})
			g.ObserveFields(map[string]SampleValue{"field": tc.last})
			require.NoError(t, cc.CommitCycleSuccess())
			mustMeasureSet(t, s.Read(), "svc.value", nil, []SampleValue{tc.last})
			for _, failCommit := range []bool{false, true} {
				cc.BeginCycle()
				g.ObservePoint(MeasureSetPoint{
					Values: []SampleValue{tc.first},
				})
				if failCommit {
					m.Counter("value").ObserveTotal(7)
					require.Error(t, cc.CommitCycleSuccess())
				} else {
					cc.AbortCycle()
				}
				mustMeasureSet(t, s.Read(ReadRaw()), "svc.value", nil, []SampleValue{tc.last})
				_, fresh := s.Read().MeasureSet("svc.value", nil)
				assert.False(t, fresh, "failed attempts must not republish old snapshot values")
				assert.Equal(t, CollectStatusFailed, s.Read().CollectMeta().LastAttemptStatus)
			}
			cc.BeginCycle()
			g.ObserveFields(map[string]SampleValue{"field": tc.first})
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{tc.last},
			})
			require.NoError(t, cc.CommitCycleSuccess())
			mustMeasureSet(t, s.Read(), "svc.value", nil, []SampleValue{tc.last})
		})
	}
}

func assertMeasureSetSample(t *testing.T, want, got SampleValue) {
	t.Helper()
	if math.IsNaN(want) {
		assert.True(t, math.IsNaN(got), "expected unavailable, got %v", got)
	} else {
		assert.Equal(t, want, got)
	}
}

func TestMeasureSetSnapshotInvalidWritesPreserveStagedPoint(t *testing.T) {
	for name, write := range map[string]func(SnapshotMeasureSetGauge, SnapshotMeasureSetCounter){
		"gauge positive infinity point": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{1, math.Inf(1)},
			})
		},
		"gauge negative infinity point": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{1, math.Inf(-1)},
			})
		},
		"gauge positive infinity fields": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObserveFields(map[string]SampleValue{"a": math.NaN(), "b": math.Inf(1)})
		},
		"gauge negative infinity fields": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObserveFields(map[string]SampleValue{"a": 1, "b": math.Inf(-1)})
		},
		"gauge short point": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{math.NaN()},
			})
		},
		"gauge long point": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObservePoint(MeasureSetPoint{
				Values: []SampleValue{1, 2, math.NaN()},
			})
		},
		"gauge missing field": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObserveFields(map[string]SampleValue{"a": math.NaN()})
		},
		"gauge unknown field with exact count": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObserveFields(map[string]SampleValue{"a": math.NaN(), "typo": 2})
		},
		"gauge empty map": func(g SnapshotMeasureSetGauge, _ SnapshotMeasureSetCounter) {
			g.ObserveFields(nil)
		},
		"counter NaN point": func(_ SnapshotMeasureSetGauge, c SnapshotMeasureSetCounter) {
			c.ObserveTotalPoint(MeasureSetPoint{
				Values: []SampleValue{1, math.NaN()},
			})
		},
		"counter NaN fields": func(_ SnapshotMeasureSetGauge, c SnapshotMeasureSetCounter) {
			c.ObserveTotalFields(map[string]SampleValue{"a": 1, "b": math.NaN()})
		},
		"counter infinity point": func(_ SnapshotMeasureSetGauge, c SnapshotMeasureSetCounter) {
			c.ObserveTotalPoint(MeasureSetPoint{
				Values: []SampleValue{1, math.Inf(1)},
			})
		},
		"counter infinity fields": func(_ SnapshotMeasureSetGauge, c SnapshotMeasureSetCounter) {
			c.ObserveTotalFields(map[string]SampleValue{"a": 1, "b": math.Inf(-1)})
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewCollectorStore()
			cc := cycleController(t, s)
			m := s.Write().SnapshotMeter("")
			fields := WithMeasureSetFields(MeasureFieldSpec{
				Name: "a",
			}, MeasureFieldSpec{
				Name: "b",
			})
			g := m.MeasureSetGauge("g", fields)
			c := m.MeasureSetCounter("c", fields)
			cc.BeginCycle()
			g.ObserveFields(map[string]SampleValue{"a": math.NaN(), "b": 4})
			point := MeasureSetPoint{
				Values: []SampleValue{5, 6},
			}
			c.ObserveTotalPoint(point)
			point.Values[0] = 999
			require.Panics(t, func() { write(g, c) })
			require.NoError(t, cc.CommitCycleSuccess())
			mustMeasureSet(t, s.Read(), "g", nil, []SampleValue{math.NaN(), 4})
			mustMeasureSet(t, s.Read(), "c", nil, []SampleValue{5, 6})
		})
	}
}

func TestMeasureSetStatefulWritesRemainFinite(t *testing.T) {
	for name, runtime := range map[string]bool{"collector": false, "runtime": true} {
		t.Run(name, func(t *testing.T) {
			var m StatefulMeter
			var read func() Reader
			var cc CycleController
			if runtime {
				s := NewRuntimeStore()
				m = s.Write().StatefulMeter("")
				read = func() Reader { return s.Read() }
			} else {
				s := NewCollectorStore(WithExpireAfterSuccessCycles(0))
				m = s.Write().StatefulMeter("")
				read = func() Reader { return s.Read() }
				cc = cycleController(t, s)
				cc.BeginCycle()
			}
			fields := WithMeasureSetFields(MeasureFieldSpec{
				Name: "a",
			}, MeasureFieldSpec{
				Name: "b",
			})
			g := m.MeasureSetGauge("g", fields)
			c := m.MeasureSetCounter("c", fields)
			point := MeasureSetPoint{
				Values: []SampleValue{3, 4},
			}
			g.SetPoint(point)
			c.AddPoint(point)
			point.Values[0] = 999
			if cc != nil {
				require.NoError(t, cc.CommitCycleSuccess())
			}
			for method, write := range map[string]func(SampleValue){
				"gauge SetPoint": func(v SampleValue) {
					g.SetPoint(MeasureSetPoint{
						Values: []SampleValue{1, v},
					})
				},
				"gauge SetFields": func(v SampleValue) { g.SetFields(map[string]SampleValue{"a": 1, "b": v}) },
				"gauge SetField":  func(v SampleValue) { g.SetField("b", v) },
				"gauge AddPoint": func(v SampleValue) {
					g.AddPoint(MeasureSetPoint{
						Values: []SampleValue{1, v},
					})
				},
				"gauge AddFields": func(v SampleValue) { g.AddFields(map[string]SampleValue{"a": 1, "b": v}) },
				"gauge AddField":  func(v SampleValue) { g.AddField("b", v) },
				"counter AddPoint": func(v SampleValue) {
					c.AddPoint(MeasureSetPoint{
						Values: []SampleValue{1, v},
					})
				},
				"counter AddFields": func(v SampleValue) { c.AddFields(map[string]SampleValue{"a": 1, "b": v}) },
				"counter AddField":  func(v SampleValue) { c.AddField("b", v) },
			} {
				t.Run(method, func(t *testing.T) {
					for _, value := range []SampleValue{math.NaN(), math.Inf(1), math.Inf(-1)} {
						if cc != nil {
							cc.BeginCycle()
						}
						require.Panics(t, func() { write(value) })
						if cc != nil {
							require.NoError(t, cc.CommitCycleSuccess())
						}
						mustMeasureSet(t, read(), "g", nil, []SampleValue{3, 4})
						mustMeasureSet(t, read(), "c", nil, []SampleValue{3, 4})
					}
				})
			}
		})
	}
}
