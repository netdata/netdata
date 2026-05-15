// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gateCheckName = "check_gate"

func TestV2Gate_G1_TemplateCompileProof(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	specYAML, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, specYAML.Validate())
	_, err = chartengine.Compile(specYAML, 1)
	require.NoError(t, err)
}

func TestV2Gate_G2_PerfdataRouting(t *testing.T) {
	router := newPerfdataRouter(64)
	warnLow := 100.0
	warnHigh := 500.0
	critLow := 200.0
	critHigh := 900.0
	samples := router.route(gateCheckName, []output.PerfDatum{
		{
			Label: "latency", Unit: "ms", Value: 120,
			Warn: &output.ThresholdRange{Inclusive: true, Low: &warnLow, High: &warnHigh},
			Crit: &output.ThresholdRange{Inclusive: true, Low: &critLow, High: &critHigh},
		},
		{Label: "throughput", Unit: "KB", Value: 30},
		{Label: "wire_rate", Unit: "kb", Value: 80},
		{Label: "free_pct", Unit: "%", Value: 40},
		{Label: "requests", Unit: "c", Value: 42},
		{Label: "custom", Unit: "widgets", Value: 3.14},
		{Label: "dup-one", Unit: "widgets", Value: 11}, // collides with dup_one
		{Label: "dup_one", Unit: "widgets", Value: 22},
	})

	byName := valueSampleMap(samples.values)
	byUnit := valueSampleUnits(samples.values)
	byThreshold := thresholdStateMap(samples.thresholdStates)
	assertNear(t, byName["perfdata.check_gate.time_latency_value"], 0.12)
	assertNear(t, byName["perfdata.check_gate.bytes_throughput_value"], 30000)
	assertNear(t, byName["perfdata.check_gate.bits_wire_rate_value"], 80000)
	assertNear(t, byName["perfdata.check_gate.percent_free_pct_value"], 40)
	assertNear(t, byName["perfdata.check_gate.counter_requests_value"], 42)
	assertNear(t, byName["perfdata.check_gate.generic_custom_value"], 3.14)
	assertNear(t, byName["perfdata.check_gate.generic_dup_one_value"], 11)
	assertString(t, byThreshold["perfdata.check_gate.time_latency_threshold_state"], perfThresholdStateWarning)
	assertString(t, byThreshold["perfdata.check_gate.bytes_throughput_threshold_state"], perfThresholdStateNone)
	assertString(t, byThreshold["perfdata.check_gate.bits_wire_rate_threshold_state"], perfThresholdStateNone)
	assertString(t, byThreshold["perfdata.check_gate.percent_free_pct_threshold_state"], perfThresholdStateNone)
	assertString(t, byThreshold["perfdata.check_gate.generic_custom_threshold_state"], perfThresholdStateNone)
	_, hasCounterThreshold := byThreshold["perfdata.check_gate.counter_requests_threshold_state"]
	assert.False(t, hasCounterThreshold)

	assertString(t, byUnit["perfdata.check_gate.time_latency_value"], "seconds")
	assertString(t, byUnit["perfdata.check_gate.bytes_throughput_value"], "bytes")
	assertString(t, byUnit["perfdata.check_gate.bits_wire_rate_value"], "bits")
	assertString(t, byUnit["perfdata.check_gate.percent_free_pct_value"], "%")
	assertString(t, byUnit["perfdata.check_gate.counter_requests_value"], "c")
	assertString(t, byUnit["perfdata.check_gate.generic_custom_value"], "generic")

	store := metrix.NewCollectorStore()
	cc := gateCycleController(t, store)
	cc.BeginCycle()
	sm := store.Write().SnapshotMeter("nagios")
	labels := sm.LabelSet(
		metrix.Label{Key: "nagios_job", Value: "gate_job"},
	)
	for _, measureSet := range samples.values {
		fields := perfMeasureSetValues(measureSet.value)
		if measureSet.counter {
			sm.MeasureSetCounter(
				measureSet.name,
				metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
				metrix.WithChartFamily(perfdataFamily(measureSet.checkName)),
				metrix.WithUnit(measureSet.unit),
			).ObserveTotalFields(fields, labels)
			continue
		}
		sm.MeasureSetGauge(
			measureSet.name,
			metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
			metrix.WithChartFamily(perfdataFamily(measureSet.checkName)),
			metrix.WithUnit(measureSet.unit),
		).ObserveFields(fields, labels)
	}
	for _, thresholdState := range samples.thresholdStates {
		sm.WithLabelSet(labels).StateSet(
			thresholdState.name,
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates(perfThresholdStateNames...),
			metrix.WithChartFamily(perfdataFamily(thresholdState.checkName)),
			metrix.WithUnit("state"),
		).Enable(thresholdState.state)
		sm.WithLabelSet(labels).WithLabels(
			metrix.Label{Key: perfdataValueLabelKey, Value: thresholdState.perfdataValue},
		).StateSet(
			jobPerfdataThresholdMetricName,
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates(perfThresholdAlertStateNames...),
			metrix.WithUnit("state"),
		).Enable(thresholdState.state)
	}
	cc.CommitCycleSuccess()

	reader := store.Read(metrix.ReadFlatten())
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.time_latency_value", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.bytes_throughput_value", "bytes", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.bits_wire_rate_value", "bits", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.percent_free_pct_value", "%", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.counter_requests_value", "c", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.generic_custom_value", "generic", true)
	assertMetricMeta(t, reader, "nagios.perfdata.check_gate.time_latency_threshold_state", "state", false)
	assertMetricMeta(t, reader, "nagios.job.perfdata.threshold_state", "state", false)
	assertMetricChartFamily(t, reader, "nagios.perfdata.check_gate.time_latency_value", "Perfdata/check_gate")
	assertMetricChartFamily(t, reader, "nagios.perfdata.check_gate.time_latency_threshold_state", "Perfdata/check_gate")
	assertMetricValue(t, reader, "nagios.perfdata.check_gate.time_latency_threshold_state", metrix.Labels{
		"nagios_job": "gate_job",
		"nagios.perfdata.check_gate.time_latency_threshold_state": perfThresholdStateWarning,
	}, 1)
	assertMetricValue(t, reader, "nagios.job.perfdata.threshold_state", metrix.Labels{
		"nagios_job":                          "gate_job",
		perfdataValueLabelKey:                 "time_latency",
		"nagios.job.perfdata.threshold_state": perfThresholdStateWarning,
	}, 1)
	assertMetricValue(t, reader, "nagios.job.perfdata.threshold_state", metrix.Labels{
		"nagios_job":                          "gate_job",
		perfdataValueLabelKey:                 "time_latency",
		"nagios.job.perfdata.threshold_state": perfThresholdStateRetry,
	}, 0)
	assertSeriesKind(t, reader, "nagios.perfdata.check_gate.time_latency_value", metrix.Labels{
		"nagios_job":                "gate_job",
		metrix.MeasureSetFieldLabel: perfFieldValue,
	}, metrix.MetricKindGauge)
	assertSeriesKind(t, reader, "nagios.perfdata.check_gate.counter_requests_value", metrix.Labels{
		"nagios_job":                "gate_job",
		metrix.MeasureSetFieldLabel: perfFieldValue,
	}, metrix.MetricKindCounter)

	changedClass := router.route(gateCheckName, []output.PerfDatum{
		{Label: "latency", Unit: "%", Value: 1}, // same label, different class => new identity
	})
	changedSamples := valueSampleMap(changedClass.values)
	assertNear(t, changedSamples["perfdata.check_gate.percent_latency_value"], 1)
}

func TestV2Gate_G3_ChartLifecycleChurn(t *testing.T) {
	newHarness := func(t *testing.T) (*chartengine.Engine, metrix.CollectorStore, func(includeB bool) chartengine.Plan) {
		t.Helper()
		engine, err := chartengine.New()
		require.NoError(t, err)
		require.NoError(t, engine.LoadYAML([]byte(New().ChartTemplateYAML()), 1))

		store := metrix.NewCollectorStore()
		emit := func(includeB bool) chartengine.Plan {
			cc := gateCycleController(t, store)
			cc.BeginCycle()
			sm := store.Write().SnapshotMeter("nagios")
			ls := sm.LabelSet(
				metrix.Label{Key: "nagios_job", Value: "gate_job"},
			)
			aFields := defaultPerfMeasureSetValues()
			aFields[perfFieldValue] = 1
			sm.MeasureSetGauge(
				"perfdata.check_gate.bytes_a",
				metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
				metrix.WithChartFamily(perfdataFamily("check_gate")),
				metrix.WithUnit("bytes"),
			).ObserveFields(aFields, ls)
			if includeB {
				bFields := defaultPerfMeasureSetValues()
				bFields[perfFieldValue] = 2
				sm.MeasureSetGauge(
					"perfdata.check_gate.bytes_b",
					metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
					metrix.WithChartFamily(perfdataFamily("check_gate")),
					metrix.WithUnit("bytes"),
				).ObserveFields(bFields, ls)
			}
			cc.CommitCycleSuccess()

			plan, err := prepareCommittedPlan(engine, store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)
			return plan
		}
		return engine, store, emit
	}

	t.Run("abort-cycle does not remove", func(t *testing.T) {
		engine, store, emit := newHarness(t)

		plan1 := emit(true)
		assert.NotZero(t, countActions[chartengine.CreateChartAction](plan1.Actions))

		cc := gateCycleController(t, store)
		cc.BeginCycle()
		sm := store.Write().SnapshotMeter("nagios")
		ls := sm.LabelSet(
			metrix.Label{Key: "nagios_job", Value: "gate_job"},
		)
		aFields := defaultPerfMeasureSetValues()
		aFields[perfFieldValue] = 1
		sm.MeasureSetGauge(
			"perfdata.check_gate.bytes_a",
			metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
			metrix.WithChartFamily(perfdataFamily("check_gate")),
			metrix.WithUnit("bytes"),
		).ObserveFields(aFields, ls)
		cc.AbortCycle()
		assert.Equal(t, metrix.CollectStatusFailed, store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus)

		planAbort, err := prepareCommittedPlan(engine, store.Read(metrix.ReadFlatten()))
		require.NoError(t, err)
		assert.Zero(t, removeActionsCount(planAbort.Actions))
	})

	t.Run("failed-attempt gap does not count toward expiry", func(t *testing.T) {
		_, store, emit := newHarness(t)

		plan1 := emit(true)
		assert.NotZero(t, countActions[chartengine.CreateChartAction](plan1.Actions))
		plan2 := emit(false)
		assert.Zero(t, removeActionsCount(plan2.Actions))
		assertPlanHasUpdateForTarget(t, plan2, "nagios.perfdata.check_gate.bytes_a")
		assertPlanHasNoRemoveForTarget(t, plan2, "nagios.perfdata.check_gate.bytes_b")

		cc := gateCycleController(t, store)
		cc.BeginCycle()
		sm := store.Write().SnapshotMeter("nagios")
		ls := sm.LabelSet(
			metrix.Label{Key: "nagios_job", Value: "gate_job"},
		)
		aFields := defaultPerfMeasureSetValues()
		aFields[perfFieldValue] = 1
		sm.MeasureSetGauge(
			"perfdata.check_gate.bytes_a",
			metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
			metrix.WithChartFamily(perfdataFamily("check_gate")),
			metrix.WithUnit("bytes"),
		).ObserveFields(aFields, ls)
		cc.AbortCycle()
		assert.Equal(t, metrix.CollectStatusFailed, store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus)

		plan3 := emit(false)
		assert.Zero(t, removeActionsCount(plan3.Actions))
		assertPlanHasUpdateForTarget(t, plan3, "nagios.perfdata.check_gate.bytes_a")
		assertPlanHasNoRemoveForTarget(t, plan3, "nagios.perfdata.check_gate.bytes_b")
		plan4 := emit(false)
		assert.Zero(t, removeActionsCount(plan4.Actions))
	})
}

func TestV2Gate_G5_ScalingPrecisionEquivalence(t *testing.T) {
	tests := map[string]struct {
		unit         string
		raw          float64
		expectedUnit string
	}{
		"time":    {unit: "ms", raw: 5.2, expectedUnit: "seconds"},
		"bytes":   {unit: "KB", raw: 1024, expectedUnit: "bytes"},
		"bits":    {unit: "kb", raw: 8, expectedUnit: "bits"},
		"percent": {unit: "%", raw: 99.5, expectedUnit: "%"},
		"counter": {unit: "c", raw: 42, expectedUnit: "c"},
		"generic": {unit: "widgets", raw: 3.14, expectedUnit: "generic"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			router := newPerfdataRouter(64)
			displayV1 := legacyDisplayValue(tc.unit, tc.raw)

			samples := router.route(gateCheckName, []output.PerfDatum{
				{Label: "sample", Unit: tc.unit, Value: tc.raw},
			})
			var (
				candidate     float64
				candidateKey  string
				candidateUnit string
			)
			found := false
			for key, value := range valueSampleMap(samples.values) {
				if len(key) >= 6 && key[len(key)-6:] == "_value" {
					candidate = value
					candidateKey = key
					candidateUnit = valueSampleUnits(samples.values)[key]
					found = true
					break
				}
			}
			require.True(t, found, "missing routed value sample")

			if displayV1 == 0 {
				assert.InDelta(t, displayV1, candidate, 1e-9)
				return
			}
			rel := math.Abs(candidate-displayV1) / math.Abs(displayV1)
			assert.LessOrEqual(t, rel, 0.001)
			assert.Equal(t, tc.expectedUnit, candidateUnit)
			assert.True(t, perfMeasureFieldFloat(perfFieldValue))

			store := metrix.NewCollectorStore()
			cc := gateCycleController(t, store)
			cc.BeginCycle()
			sm := store.Write().SnapshotMeter("nagios")
			for _, measureSet := range samples.values {
				fields := perfMeasureSetValues(measureSet.value)
				if measureSet.counter {
					sm.MeasureSetCounter(
						measureSet.name,
						metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
						metrix.WithChartFamily(perfdataFamily(measureSet.checkName)),
						metrix.WithUnit(measureSet.unit),
					).ObserveTotalFields(fields, sm.LabelSet())
					continue
				}
				sm.MeasureSetGauge(
					measureSet.name,
					metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
					metrix.WithChartFamily(perfdataFamily(measureSet.checkName)),
					metrix.WithUnit(measureSet.unit),
				).ObserveFields(fields, sm.LabelSet())
			}
			cc.CommitCycleSuccess()
			flat := store.Read(metrix.ReadFlatten())
			assertMetricMeta(t, flat, "nagios."+candidateKey, tc.expectedUnit, true)
			assertMetricChartFamily(t, flat, "nagios."+candidateKey, "Perfdata/check_gate")
		})
	}
}

func countActions[T any](actions []chartengine.EngineAction) int {
	n := 0
	for _, action := range actions {
		if _, ok := action.(T); ok {
			n++
		}
	}
	return n
}

func removeActionsCount(actions []chartengine.EngineAction) int {
	return countActions[chartengine.RemoveChartAction](actions) + countActions[chartengine.RemoveDimensionAction](actions)
}

func gateCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func prepareCommittedPlan(engine *chartengine.Engine, reader metrix.Reader) (chartengine.Plan, error) {
	attempt, err := engine.PreparePlan(reader)
	if err != nil {
		return chartengine.Plan{}, err
	}
	defer attempt.Abort()

	plan := attempt.Plan()
	if err := attempt.Commit(); err != nil {
		return chartengine.Plan{}, err
	}
	return plan, nil
}

func assertMetricMeta(t *testing.T, reader metrix.Reader, metricName, unit string, isFloat bool) {
	t.Helper()
	meta, ok := reader.MetricMeta(metricName)
	require.True(t, ok, "missing metric metadata for %q", metricName)
	assert.Equal(t, unit, meta.Unit)
	assert.Equal(t, isFloat, meta.Float)
}

func assertMetricChartFamily(t *testing.T, reader metrix.Reader, metricName, chartFamily string) {
	t.Helper()
	meta, ok := reader.MetricMeta(metricName)
	require.True(t, ok, "missing metric metadata for %q", metricName)
	assert.Equal(t, chartFamily, meta.ChartFamily)
}

func assertSeriesKind(t *testing.T, reader metrix.Reader, metricName string, labels metrix.Labels, want metrix.MetricKind) {
	t.Helper()
	meta, ok := reader.SeriesMeta(metricName, labels)
	require.True(t, ok, "missing series metadata for %q with labels %v", metricName, labels)
	assert.Equal(t, want, meta.Kind)
}

func assertPlanHasUpdateForTarget(t *testing.T, plan chartengine.Plan, updateMetricPrefix string) {
	t.Helper()
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok {
			continue
		}
		if strings.HasPrefix(update.ChartID, updateMetricPrefix) {
			return
		}
	}
	assert.FailNow(t, "expected update action", "%q", updateMetricPrefix)
}

func assertPlanHasNoRemoveForTarget(t *testing.T, plan chartengine.Plan, removeMetricPrefix string) {
	t.Helper()
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case chartengine.RemoveDimensionAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				assert.FailNow(t, "unexpected remove dimension action", "%q", removeMetricPrefix)
			}
		case chartengine.RemoveChartAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				assert.FailNow(t, "unexpected remove chart action", "%q", removeMetricPrefix)
			}
		}
	}
}

func TestV2Gate_SmokeCollect(t *testing.T) {
	coll := newTestCollector()
	coll.runner = &fakeRunner{}
	coll.Config.JobConfig.Plugin = writeTestPluginFile(t, "true")
	coll.Config.JobConfig.Name = "smoke"
	require.NoError(t, coll.Check(context.Background()))
}
