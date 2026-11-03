// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ipmi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config     func(*Config)
		wantErr    bool
		wantReader bmc.Config
	}{
		"defaults": {
			wantReader: bmc.Config{
				Driver:      driverOpen,
				Device:      0,
				Timeout:     5 * time.Second,
				UpdateEvery: 10 * time.Second,
			},
		},
		"device and timeout": {
			config: func(c *Config) {
				c.Device = 2
				c.Timeout = confopt.Duration(3 * time.Second)
				c.UpdateEvery = 30
			},
			wantReader: bmc.Config{
				Driver:      driverOpen,
				Device:      2,
				Timeout:     3 * time.Second,
				UpdateEvery: 30 * time.Second,
			},
		},
		"remote defaults": {
			config:     func(c *Config) { c.Driver, c.Hostname, c.Device = driverLANPlus, " bmc.example ", -1 },
			wantReader: bmc.Config{Driver: driverLANPlus, Hostname: "bmc.example", Port: 623, PrivilegeLevel: "user", Timeout: 5 * time.Second, UpdateEvery: 10 * time.Second},
		},
		"remote overrides": {
			config: func(c *Config) {
				c.Driver, c.Hostname, c.Port = driverLAN, "192.0.2.1", 9623
				c.Username, c.Password, c.PrivilegeLevel = "monitor", "test-secret", "operator"
				c.Timeout = confopt.Duration(3 * time.Second)
			},
			wantReader: bmc.Config{Driver: driverLAN, Hostname: "192.0.2.1", Port: 9623, Username: "monitor", Password: "test-secret", PrivilegeLevel: "operator", Timeout: 3 * time.Second, UpdateEvery: 10 * time.Second},
		},
		"remote hostname missing": {
			config:  func(c *Config) { c.Driver = "lanplus" },
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			if tc.config != nil {
				tc.config(&c.Config)
			}
			var got *bmc.Config
			c.newReader = func(cfg bmc.Config) sensorReader {
				got = &cfg
				return &fakeReader{}
			}

			err := c.Init(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, got, "no reader for an invalid config")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantReader, *got)
		})
	}
}

func TestCollector_Lifecycle(t *testing.T) {
	c, reader := newTestCollector(t)

	require.NoError(t, c.Check(t.Context()))
	assert.Equal(t, 1, reader.checks)
	assert.Zero(t, reader.collects, "Check must not perform a full collection")
	assert.Nil(t, c.snapshot.Load())

	points, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.True(t, reader.collectSEL)
	assert.Equal(t, wantSeries(t, reader.snapshot), points)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	assert.Equal(t, 200, c.funcRouter.Handle(t.Context(), "sensors", funcapi.ResolvedParams{}).Status)

	c.Cleanup(t.Context())
	assert.Equal(t, 1, reader.closes)
	assert.Nil(t, c.snapshot.Load(), "Cleanup unpublishes the Function snapshot")
	assert.NotPanics(t, func() { c.Cleanup(t.Context()) }, "Cleanup is idempotent")
}

func TestCollector_Cleanup_BeforeInit(t *testing.T) {
	c := New()
	c.Cleanup(t.Context())
	assert.Nil(t, c.snapshot.Load())
}

func TestCollector_Collect_MissingData(t *testing.T) {
	c, reader := newTestCollector(t)
	c.CollectSEL = false
	reader.snapshot.SELEntries = nil
	reader.snapshot.SEL = nil
	reader.snapshot.Sensors[0].Value = nil
	reader.snapshot.Sensors[0].State = bmc.StateUnknown

	points, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.False(t, reader.collectSEL)
	assert.Equal(t, wantSeries(t, reader.snapshot), points, "unavailable readings and SEL leave gaps")
}

func TestCollector_Collect_NewSELRecordCounts(t *testing.T) {
	c, reader := newTestCollector(t)
	reader.snapshot.SEL.New = bmc.SELCounts{
		Threshold: bmc.SELStateCounts{Nominal: 1, Critical: 2},
		Discrete:  bmc.SELStateCounts{Critical: 1, Unknown: 1},
	}

	points, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	got := make(map[string]metrix.SampleValue)
	for key, value := range points {
		if strings.HasPrefix(key, "sel_new_records{") {
			got[key] = value
		}
	}
	assert.Equal(t, map[string]metrix.SampleValue{
		`sel_new_records{event_type="threshold",state="nominal"}`:  1,
		`sel_new_records{event_type="threshold",state="warning"}`:  0,
		`sel_new_records{event_type="threshold",state="critical"}`: 2,
		`sel_new_records{event_type="threshold",state="unknown"}`:  0,
		`sel_new_records{event_type="discrete",state="nominal"}`:   0,
		`sel_new_records{event_type="discrete",state="warning"}`:   0,
		`sel_new_records{event_type="discrete",state="critical"}`:  1,
		`sel_new_records{event_type="discrete",state="unknown"}`:   1,
	}, got)

	// The Reader reports no new records when it cannot tell.
	reader.snapshot.SEL.New, reader.snapshot.SEL.NewKnown = bmc.SELCounts{}, false
	points, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, wantSeries(t, reader.snapshot), points, "unknown new records leave a gap, not zeros")
}

func TestCollector_SELChartsSurviveFailedReads(t *testing.T) {
	c, reader := newTestCollector(t)
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), defaultUpdateEvery))

	healthy := healthySnapshot()
	failed := healthySnapshot()
	failed.SELEntries = nil
	failed.SEL.ReadFailed, failed.SEL.New, failed.SEL.NewKnown = true, bmc.SELCounts{}, false
	created := make(map[string]bool)
	// eventTypes are the event_type labels of the created charts, with their dimensions.
	eventTypes := make(map[string][]string)
	chartEventType := make(map[string]string)
	for phase, tc := range []struct {
		snapshot *bmc.Snapshot
		cycles   int
	}{
		{snapshot: healthy, cycles: 1},
		// Sensors keep collecting beyond chart expiry and series retention.
		{snapshot: failed, cycles: 25},
		{snapshot: healthy, cycles: 1},
	} {
		reader.snapshot = tc.snapshot
		for cycle := range tc.cycles {
			points, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, wantSeries(t, tc.snapshot), points)

			attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
			require.NoError(t, err)
			for _, action := range attempt.Plan().Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					assert.Zero(t, phase, "chart %s created again after a failed SEL read", action.ChartID)
					created[action.ChartID] = true
					if eventType, ok := action.Labels["event_type"]; ok {
						chartEventType[action.ChartID] = eventType
						eventTypes[eventType] = nil
					}
				case chartengine.CreateDimensionAction:
					if eventType, ok := chartEventType[action.ChartID]; ok {
						eventTypes[eventType] = append(eventTypes[eventType], action.Name)
					}
				case chartengine.RemoveChartAction:
					assert.Fail(t, "chart removed", "%s at phase %d cycle %d", action.ChartID, phase, cycle+1)
				}
			}
			require.NoError(t, attempt.Commit())
		}
	}
	assert.NotEmpty(t, created)
	// The alert's chart label filter and dimension reach the emitted charts.
	for _, eventType := range selEventTypes {
		assert.Contains(t, eventTypes[eventType], bmc.StateCritical, "event_type %q", eventType)
	}
}

func TestCollector_Collect_FailureAndRecovery(t *testing.T) {
	c, reader := newTestCollector(t)
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	reader.err = errors.New("transport failed")
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "transport failed")
	assert.Nil(t, c.snapshot.Load(), "the Function must not serve data older than a failure")

	reader.err = nil
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.NotNil(t, c.snapshot.Load())
}

func TestCollector_Collect_Canceled(t *testing.T) {
	c, _ := newTestCollector(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	assert.Nil(t, c.snapshot.Load(), "a canceled cycle publishes nothing")
}

func TestCollector_Collect_PartialWarningsAreRateLimited(t *testing.T) {
	c, reader := newTestCollector(t)
	var logs bytes.Buffer
	c.Logger = logger.NewWithWriter(&logs)

	for i := range 10 {
		reader.snapshot.Warnings = []string{fmt.Sprintf("%d unsupported sensors", i+1)}
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, strings.Count(logs.String(), "unsupported sensors"))
}

func TestCollector_FunctionAvailable(t *testing.T) {
	for _, collectSEL := range []bool{true, false} {
		c := New()
		c.CollectSEL = collectSEL
		assert.True(t, c.FunctionAvailable("sensors"))
		assert.Equal(t, collectSEL, c.FunctionAvailable("sel"), "collect_sel %v", collectSEL)
	}
}

func TestCollector_ConcurrentFunctionReads(t *testing.T) {
	c, _ := newTestCollector(t)
	raw := c.funcRouter.(funcapi.RawMethodHandler)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				c.funcRouter.Handle(context.Background(), "sensors", funcapi.ResolvedParams{})
				raw.HandleRaw(context.Background(), funcapi.RawMethodRequest{Method: "sel"})
			}
		})
	}
	for range 30 {
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}
	wg.Wait()
}

func TestArtifacts(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, defaultUpdateEvery)
	require.NoError(t, err)

	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(
		t,
		metadata,
		chartTemplateYAML,
		map[string][]string{"sensor_state": sensorStates},
	)
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)

	c, _ := newTestCollector(t)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsFunctions(t, metadata, collecttest.MetadataFunctionsCheck{
		Context:       t.Context(),
		Module:        "ipmi",
		Methods:       ipmiMethods(),
		Handler:       c.funcRouter,
		JobSelectable: true,
		RawTables:     true,
	})
}

func TestArtifacts_PublicNames(t *testing.T) {
	charts, err := collecttest.ChartTemplateCharts(chartTemplateYAML)
	require.NoError(t, err)
	actual := make(map[string]string, len(charts))
	for contextName, chart := range charts {
		actual[contextName] = chart.ID
	}
	assert.Equal(t, map[string]string{
		"ipmi.sensor_temperature_c":   "sensor_temperature_c",
		"ipmi.sensor_temperature_f":   "sensor_temperature_f",
		"ipmi.sensor_voltage":         "sensor_voltage",
		"ipmi.sensor_ampere":          "sensor_ampere",
		"ipmi.sensor_fan_speed":       "sensor_fan_speed",
		"ipmi.sensor_power":           "sensor_power",
		"ipmi.sensor_reading_percent": "sensor_reading_percent",
		"ipmi.sensor_state":           "sensor_state",
		"ipmi.sel":                    "events",
		"ipmi.sel_new_records":        "sel_new_records",
	}, actual)

	var names []string
	for _, method := range ipmiMethods() {
		names = append(names, method.FunctionName)
	}
	assert.Equal(t, []string{"ipmi-sensors", "ipmi-sel"}, names)
}

func TestArtifacts_AlertBindings(t *testing.T) {
	health, err := os.ReadFile("../../../../../health/health.d/ipmi.conf")
	require.NoError(t, err)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)

	bindings := make(map[string]int)
	for _, alert := range collecttest.ParseHealthAlerts(health) {
		bindings[alert.On]++
	}
	require.Equal(t, map[string]int{"ipmi.sensor_state": 1, "ipmi.sel": 1, "ipmi.sel_new_records": 1}, bindings,
		"both implementations and any job name share one alert owner per context")

	// Chart label filters must select labels the collector emits.
	emitted := map[string][]string{"event_type": selEventTypes}
	for _, filter := range healthChartLabelFilters(health) {
		key, value, ok := strings.Cut(filter, "=")
		require.True(t, ok, filter)
		assert.Contains(t, emitted[key], value, "chart label filter %q", filter)
	}

	opts := collecttest.HealthAlertsCheck{
		ContextPrefix: "ipmi.",
	}
	collecttest.AssertHealthAlertsTargetChartTemplateWith(t, health, chartTemplateYAML, opts)
	collecttest.AssertHealthAlertsMatchMetadataWith(t, health, metadata, opts)
}

// healthChartLabelFilters returns the key=value terms of every chart labels line.
func healthChartLabelFilters(health []byte) []string {
	var out []string
	for line := range strings.SplitSeq(string(health), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "chart labels" {
			out = append(out, strings.Fields(value)...)
		}
	}
	return out
}

type fakeReader struct {
	snapshot *bmc.Snapshot
	err      error

	checks     int
	collects   int
	closes     int
	collectSEL bool
}

func (r *fakeReader) Check(ctx context.Context) error {
	r.checks++
	return ctx.Err()
}

func (r *fakeReader) Collect(_ context.Context, collectSEL bool) (*bmc.Snapshot, error) {
	r.collects++
	r.collectSEL = collectSEL
	return r.snapshot, r.err
}

func (r *fakeReader) Close(context.Context) error {
	r.closes++
	return nil
}

func newTestCollector(t *testing.T) (*Collector, *fakeReader) {
	t.Helper()
	c := New()
	reader := &fakeReader{
		snapshot: healthySnapshot(),
	}
	c.newReader = func(bmc.Config) sensorReader { return reader }
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c, reader
}

// healthySnapshot has one nominal sensor per numeric unit, one discrete sensor
// and a three-record SEL whose two newest records are new.
func healthySnapshot() *bmc.Snapshot {
	records := []bmc.SELRecord{
		{
			ID:    1,
			Event: "OEM record type 0xe0",
			State: bmc.StateUnknown,
		},
		{
			ID:        2,
			Event:     "Upper Critical - going high",
			State:     bmc.StateCritical,
			Threshold: true,
		},
		{
			ID:    3,
			Event: "Timestamp Clock Synch",
			State: bmc.StateWarning,
		},
	}
	s := &bmc.Snapshot{
		SELEntries: new(3),
		SEL: &bmc.SEL{
			Records:  records,
			Loaded:   true,
			Entries:  3,
			New:      bmc.SELCounts{Threshold: bmc.SELStateCounts{Critical: 1}, Discrete: bmc.SELStateCounts{Warning: 1}},
			NewKnown: true,
		},
		CollectedAt: time.Now(),
	}
	for _, unit := range []string{
		bmc.UnitCelsius,
		bmc.UnitFahrenheit,
		bmc.UnitVolts,
		bmc.UnitAmps,
		bmc.UnitRPM,
		bmc.UnitWatts,
		bmc.UnitPercent,
	} {
		s.Sensors = append(s.Sensors, bmc.Sensor{
			Key:       "test_" + unit,
			Name:      unit + " sensor",
			Type:      "test",
			Component: "System",
			Unit:      unit,
			State:     bmc.StateNominal,
			Value:     new(12.5),
		})
	}
	s.Sensors = append(s.Sensors, bmc.Sensor{
		Key:       "presence",
		Name:      "Presence",
		Type:      "Entity Presence",
		Component: "Other",
		State:     bmc.StateUnknown,
	})
	return s
}

// wantSeries derives the expected flattened series of a snapshot: a state
// set per sensor, a reading per available value, the SEL count and, when
// known, the new SEL records per event type and state. A reading belongs to
// the chart template's metric whose chart has the sensor's unit.
func wantSeries(t *testing.T, s *bmc.Snapshot) map[string]metrix.SampleValue {
	t.Helper()
	charts, err := collecttest.ChartTemplateCharts(chartTemplateYAML)
	require.NoError(t, err)
	metricOfUnit := make(map[string]string)
	for _, chart := range charts {
		if len(chart.Dimensions) == 1 {
			metricOfUnit[chart.Units] = chart.Dimensions[0].Selector
		}
	}

	want := make(map[string]metrix.SampleValue)
	for _, sensor := range s.Sensors {
		labels := map[string]string{
			"sensor_id": sensor.Key,
			"sensor":    sensor.Name,
			"type":      sensor.Type,
			"component": sensor.Component,
		}
		for _, state := range []string{bmc.StateNominal, bmc.StateWarning, bmc.StateCritical, bmc.StateUnknown} {
			var active metrix.SampleValue
			if state == sensor.State {
				active = 1
			}
			stateLabels := maps.Clone(labels)
			stateLabels["sensor_state"] = state
			want[seriesKey("sensor_state", stateLabels)] = active
		}
		if sensor.Value != nil {
			metric, ok := metricOfUnit[sensor.Unit]
			require.True(t, ok, "no chart for unit %q", sensor.Unit)
			want[seriesKey(metric, labels)] = *sensor.Value
		}
	}
	if s.SELEntries != nil {
		want["sel_events"] = metrix.SampleValue(*s.SELEntries)
	}
	if s.SEL != nil && s.SEL.NewKnown {
		for _, eventType := range []string{"threshold", "discrete"} {
			for _, state := range []string{bmc.StateNominal, bmc.StateWarning, bmc.StateCritical, bmc.StateUnknown} {
				states := s.SEL.New.Discrete
				if eventType == "threshold" {
					states = s.SEL.New.Threshold
				}
				var n metrix.SampleValue
				switch state {
				case bmc.StateNominal:
					n = metrix.SampleValue(states.Nominal)
				case bmc.StateWarning:
					n = metrix.SampleValue(states.Warning)
				case bmc.StateCritical:
					n = metrix.SampleValue(states.Critical)
				case bmc.StateUnknown:
					n = metrix.SampleValue(states.Unknown)
				}
				want[seriesKey("sel_new_records", map[string]string{"event_type": eventType, "state": state})] = n
			}
		}
	}
	return want
}

// seriesKey formats a series the way collecttest reports it: labels sorted by key.
func seriesKey(name string, labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		parts = append(parts, fmt.Sprintf("%s=%q", key, labels[key]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}
