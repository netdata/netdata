// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

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
				Device:  0,
				Timeout: 5 * time.Second,
			},
		},
		"device and timeout": {
			config: func(c *Config) {
				c.Device = 2
				c.Timeout = confopt.Duration(3 * time.Second)
			},
			wantReader: bmc.Config{
				Device:  2,
				Timeout: 3 * time.Second,
			},
		},
		"invalid config": {
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
	reader.snapshot.Sensors[0].Value = nil
	reader.snapshot.Sensors[0].State = bmc.StateUnknown

	points, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.False(t, reader.collectSEL)
	assert.Equal(t, wantSeries(t, reader.snapshot), points, "unavailable readings and SEL leave gaps")
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

func TestCollector_ConcurrentFunctionReads(t *testing.T) {
	c, _ := newTestCollector(t)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				c.funcRouter.Handle(context.Background(), "sensors", funcapi.ResolvedParams{})
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
	})
}

func TestArtifacts_ExperimentalPublicNames(t *testing.T) {
	charts, err := collecttest.ChartTemplateCharts(chartTemplateYAML)
	require.NoError(t, err)
	require.NotEmpty(t, charts)
	for contextName, chart := range charts {
		assert.True(t, strings.HasPrefix(contextName, "ipmi_go."), contextName)
		// Only contexts move: the C plugin's chart IDs stay, including SEL events.
		wantID := strings.TrimPrefix(contextName, "ipmi_go.")
		if wantID == "sel" {
			wantID = "events"
		}
		assert.Equal(t, wantID, chart.ID)
	}

	methods := ipmiMethods()
	require.Len(t, methods, 1)
	assert.Equal(t, "ipmi-go-sensors", methods[0].FunctionName)
}

func TestArtifacts_SensorAlertBinding(t *testing.T) {
	health, err := os.ReadFile("../../../../../health/health.d/ipmi.conf")
	require.NoError(t, err)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)

	var bindings int
	for _, alert := range collecttest.ParseHealthAlerts(health) {
		if alert.On == "ipmi_go.sensor_state" {
			bindings++
		}
	}
	require.Equal(t, 1, bindings, "the renamed sensor context keeps one alert owner")

	opts := collecttest.HealthAlertsCheck{
		ContextPrefix: "ipmi_go.",
	}
	collecttest.AssertHealthAlertsTargetChartTemplateWith(t, health, chartTemplateYAML, opts)
	collecttest.AssertHealthAlertsMatchMetadataWith(t, health, metadata, opts)
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

// healthySnapshot has one nominal sensor per numeric unit and one discrete sensor.
func healthySnapshot() *bmc.Snapshot {
	s := &bmc.Snapshot{
		SELEntries:  new(12),
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
// set per sensor, a reading per available value and the SEL count. A reading
// belongs to the chart template's metric whose chart has the sensor's unit.
func wantSeries(t *testing.T, s *bmc.Snapshot) map[string]metrix.SampleValue {
	t.Helper()
	charts, err := collecttest.ChartTemplateCharts(chartTemplateYAML)
	require.NoError(t, err)
	metricOfUnit := make(map[string]string)
	for _, chart := range charts {
		require.Len(t, chart.Dimensions, 1, chart.ID)
		metricOfUnit[chart.Units] = chart.Dimensions[0].Selector
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
