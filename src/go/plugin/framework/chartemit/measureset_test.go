// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasureSetAvailabilityWireLifecycle(t *testing.T) {
	for name, tc := range map[string]struct {
		template string
		chartID  string
	}{
		"autogen": {chartID: "svc.latency-service=api"},
		"authored": {
			template: measureSetAvailabilityTemplate("sum"),
			chartID:  "latency_api",
		},
	} {
		t.Run(name, func(t *testing.T) {
			engine := newMeasureSetAvailabilityEngine(t, tc.template)
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			require.True(t, ok)
			cycle := managed.CycleController()
			meter := store.Write().SnapshotMeter("svc")
			metric := meter.MeasureSetGauge("latency", metrix.WithMeasureSetFields(
				metrix.MeasureFieldSpec{
					Name: "min",
				},
				metrix.MeasureFieldSpec{
					Name:  "max",
					Float: true,
				},
				metrix.MeasureFieldSpec{
					Name:  "mean",
					Float: true,
				},
				metrix.MeasureFieldSpec{
					Name:  "p50",
					Float: true,
				},
				metrix.MeasureFieldSpec{
					Name:  "p95",
					Float: true,
				},
			))
			labels := meter.LabelSet(metrix.Label{
				Key:   "service",
				Value: "api",
			})
			nan := math.NaN()
			empty := map[string]string{"min": "", "max": "", "mean": "", "p50": "", "p95": ""}
			// Current unavailability is a present field, including on cold start.
			steps := []struct {
				name   string
				values []metrix.SampleValue
				want   map[string]string
			}{
				{"cold unavailable", []metrix.SampleValue{nan, nan, nan, nan, nan}, empty},
				{
					"finite",
					[]metrix.SampleValue{-2, 4.5, 0, 1.25, 3.75},
					map[string]string{"min": "-2", "max": "4.5", "mean": "0", "p50": "1.25", "p95": "3.75"},
				},
				{
					"partial",
					[]metrix.SampleValue{nan, 7.5, nan, 0, nan},
					map[string]string{"min": "", "max": "7.5", "mean": "", "p50": "0", "p95": ""},
				},
				{"unavailable 1", []metrix.SampleValue{nan, nan, nan, nan, nan}, empty},
				{"unavailable 2", []metrix.SampleValue{nan, nan, nan, nan, nan}, empty},
				{"unavailable 3", []metrix.SampleValue{nan, nan, nan, nan, nan}, empty},
				{
					"recovery",
					[]metrix.SampleValue{0, 9.5, -1.5, 2.25, 8.75},
					map[string]string{"min": "0", "max": "9.5", "mean": "-1.5", "p50": "2.25", "p95": "8.75"},
				},
			}
			for i, step := range steps {
				t.Run(step.name, func(t *testing.T) {
					cycle.BeginCycle()
					metric.ObservePoint(metrix.MeasureSetPoint{
						Values: step.values,
					}, labels)
					require.NoError(t, cycle.CommitCycleSuccess())
					wire := emitMeasureSetAvailability(t, engine, store.Read(metrix.ReadFlatten()), nil)
					assert.Equal(
						t,
						map[string]map[string]string{"collector.job." + tc.chartID: step.want},
						measureSetWireSamples(t, wire),
					)
					assert.NotContains(t, wire, "obsolete")
					if i == 0 {
						assert.Equal(t, 1, strings.Count(wire, "CHART "))
						assert.Equal(t, 5, strings.Count(wire, "DIMENSION "))
						for _, field := range []string{"min", "max", "mean", "p50", "p95"} {
							assert.Contains(t, wire, "DIMENSION '"+field+"' '"+field+"' 'absolute'")
						}
					} else {
						assert.NotContains(t, wire, "CHART ")
						assert.NotContains(t, wire, "DIMENSION ")
					}
				})
			}
			// Whole-family absence still expires normally after two successful cycles.
			for absent := 1; absent <= 2; absent++ {
				cycle.BeginCycle()
				require.NoError(t, cycle.CommitCycleSuccess())
				wire := emitMeasureSetAvailability(t, engine, store.Read(metrix.ReadFlatten()), nil)
				assert.Empty(t, measureSetWireSamples(t, wire))
				if absent == 1 {
					assert.Empty(t, wire)
				} else {
					assert.Equal(t, 1, strings.Count(wire, "CHART 'collector.job."+tc.chartID+"'"))
					assert.Contains(t, wire, "'obsolete'")
				}
			}
		})
	}
}

func TestMeasureSetAvailabilityWireReducersAndIsolation(t *testing.T) {
	for reducer, want := range map[string]string{"sum": "", "avg": "", "min": "2.5", "max": "8.5"} {
		t.Run(reducer, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			require.True(t, ok)
			cycle := managed.CycleController()
			meter := store.Write().SnapshotMeter("svc")
			scope := metrix.HostScope{
				ScopeKey: "remote",
				GUID:     "remote-guid",
				Hostname: "remote",
			}
			fields := metrix.WithMeasureSetFields(metrix.MeasureFieldSpec{
				Name:  "mean",
				Float: true,
			})
			local := meter.MeasureSetGauge("latency", fields)
			remote := meter.WithHostScope(scope).MeasureSetGauge("latency", fields)
			cycle.BeginCycle()
			for source, value := range map[string]metrix.SampleValue{"a": math.NaN(), "b": 2.5, "c": 8.5} {
				local.ObserveFields(map[string]metrix.SampleValue{"mean": value}, meter.LabelSet(
					metrix.Label{
						Key:   "service",
						Value: "api",
					}, metrix.Label{
						Key:   "source",
						Value: source,
					},
				))
			}
			local.ObserveFields(map[string]metrix.SampleValue{"mean": 19.25}, meter.LabelSet(metrix.Label{
				Key:   "service",
				Value: "worker",
			}))
			remote.ObserveFields(map[string]metrix.SampleValue{"mean": 42.5}, meter.LabelSet(metrix.Label{
				Key:   "service",
				Value: "api",
			}))
			require.NoError(t, cycle.CommitCycleSuccess())

			localEngine := newMeasureSetAvailabilityEngine(t, measureSetAvailabilityTemplate(reducer))
			wire := emitMeasureSetAvailability(t, localEngine, store.Read(metrix.ReadFlatten()), nil)
			assert.Equal(t, map[string]map[string]string{
				"collector.job.latency_api":    {"mean": want},
				"collector.job.latency_worker": {"mean": "19.25"},
			}, measureSetWireSamples(t, wire))
			assert.Contains(t, wire, "HOST ''")

			remoteEngine := newMeasureSetAvailabilityEngine(t, measureSetAvailabilityTemplate(reducer))
			wire = emitMeasureSetAvailability(
				t,
				remoteEngine,
				store.Read(metrix.ReadFlatten(), metrix.ReadHostScope(scope.ScopeKey)),
				&HostScope{
					GUID: scope.GUID,
				},
			)
			assert.Equal(
				t,
				map[string]map[string]string{"collector.job.latency_api": {"mean": "42.5"}},
				measureSetWireSamples(t, wire),
			)
			assert.Contains(t, wire, "HOST 'remote-guid'")
		})
	}
}

func newMeasureSetAvailabilityEngine(t *testing.T, template string) *chartengine.Engine {
	t.Helper()
	engine, err := chartengine.New(chartengine.WithEnginePolicy(chartengine.EnginePolicy{
		Autogen: &chartengine.AutogenPolicy{
			Enabled:                  template == "",
			ExpireAfterSuccessCycles: 2,
		},
	}))
	require.NoError(t, err)
	if template == "" {
		template = strings.ReplaceAll(measureSetAvailabilityTemplate("sum"), "svc.latency_*", "svc.unmatched_*")
	}
	require.NoError(t, engine.LoadYAML([]byte(template), 1))
	return engine
}

func measureSetAvailabilityTemplate(reducer string) string {
	return fmt.Sprintf(`
version: v1
groups:
  - family: Latency
    metrics: [svc.latency_*]
    charts:
      - id: latency
        title: Latency
        context: latency
        units: seconds
        aggregation: %s
        instances:
          by_labels: [service]
        lifecycle:
          expire_after_cycles: 2
          dimensions:
            expire_after_cycles: 2
        dimensions:
          - selector: svc.latency_*
            name_from_label: measure_field
`, reducer)
}

func emitMeasureSetAvailability(
	t *testing.T,
	engine *chartengine.Engine,
	reader metrix.Reader,
	host *HostScope,
) string {
	t.Helper()
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()
	var wire bytes.Buffer
	require.NoError(t, ApplyPlan(netdataapi.New(&wire), attempt.Plan(), EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		HostScope:   host,
	}))
	require.NoError(t, attempt.Commit())
	return wire.String()
}

// Read actual SET commands by chart, retaining empty values and rejecting duplicates.
func measureSetWireSamples(t *testing.T, wire string) map[string]map[string]string {
	t.Helper()
	samples := make(map[string]map[string]string)
	var chart string
	for _, line := range strings.Split(wire, "\n") {
		switch {
		case strings.HasPrefix(line, "BEGIN '"):
			chart = strings.SplitN(line, "'", 3)[1]
			_, exists := samples[chart]
			require.False(t, exists, "duplicate BEGIN: %s", wire)
			samples[chart] = make(map[string]string)
		case strings.HasPrefix(line, "SET '"):
			require.NotEmpty(t, chart, "SET outside BEGIN: %s", wire)
			parts := strings.SplitN(line, "'", 3)
			_, exists := samples[chart][parts[1]]
			require.False(t, exists, "duplicate SET: %s", wire)
			samples[chart][parts[1]] = strings.TrimSpace(strings.TrimPrefix(parts[2], " ="))
		case line == "END":
			chart = ""
		}
	}
	return samples
}
