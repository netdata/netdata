// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/ipmifunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReader struct {
	snapshot              *ipmiapi.Snapshot
	err                   error
	checks, scans, closes int
	sel                   bool
	closeContext          context.Context
}

func (r *fakeReader) Check(ctx context.Context) error { r.checks++; return ctx.Err() }
func (r *fakeReader) Collect(ctx context.Context, sel bool) (*ipmiapi.Snapshot, error) {
	r.scans++
	r.sel = sel
	return r.snapshot, r.err
}
func (r *fakeReader) Close(ctx context.Context) error {
	r.closes++
	r.closeContext = ctx
	return nil
}
func testCollector(t *testing.T) (*Collector, *fakeReader) {
	t.Helper()
	c := New()
	r := &fakeReader{snapshot: healthySnapshot()}
	c.newReader = func(ipmiapi.Config) (reader, error) { return r, nil }
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c, r
}
func healthySnapshot() *ipmiapi.Snapshot {
	count := float64(12)
	s := &ipmiapi.Snapshot{SEL: &count, CollectedAt: time.Now()}
	for _, metric := range []string{"temperature_c", "temperature_f", "voltage", "ampere", "fan_speed", "power", "reading_percent"} {
		value := float64(12.5)
		s.Sensors = append(s.Sensors, ipmiapi.Sensor{Key: "test_" + metric, Name: metric, Type: "test", Component: "System", Metric: metric, Value: &value, State: "nominal"})
	}
	s.Sensors = append(s.Sensors, ipmiapi.Sensor{Key: "unavailable", Name: "Unavailable", State: "unknown"})
	return s
}
func TestLifecycle(t *testing.T) {
	c, r := testCollector(t)
	require.NoError(t, c.Check(t.Context()))
	assert.Equal(t, 1, r.checks)
	assert.Zero(t, r.scans, "Check must not perform full collection")
	assert.Nil(t, c.snapshot.Load())
	points, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.NotEmpty(t, points)
	assert.True(t, r.sel)
	assert.Equal(t, float64(12), points["sel_events"])
	for key := range points {
		assert.False(t, strings.HasPrefix(key, "temperature_c{") && strings.Contains(key, `sensor_id="unavailable"`))
	}
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	assert.Equal(t, 200, c.funcRouter.Handle(t.Context(), "sensors", funcapi.ResolvedParams{}).Status)
	c.Cleanup(t.Context())
	assert.Equal(t, 1, r.closes)
	assert.Nil(t, c.snapshot.Load())
}
func TestFailureAndRecovery(t *testing.T) {
	c, r := testCollector(t)
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	r.err = errors.New("transport failed")
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "transport failed")
	assert.Nil(t, c.snapshot.Load())
	assert.Equal(t, 1, r.closes)
	r.err = nil
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.NotNil(t, c.snapshot.Load())
}
func TestCanceledCycle(t *testing.T) {
	c, r := testCollector(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	assert.Zero(t, r.scans)
	assert.Nil(t, c.snapshot.Load())
}
func TestMissingValuesAndDisabledSEL(t *testing.T) {
	c, r := testCollector(t)
	c.CollectSEL = false
	r.snapshot.SEL = nil
	r.snapshot.Sensors[0].Value = nil
	r.snapshot.Sensors[0].State = "unknown"
	points, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.False(t, r.sel)
	assert.NotContains(t, points, "sel_events")
	for key := range points {
		assert.False(t, strings.HasPrefix(key, "temperature_c{"), key)
	}
}
func TestConcurrentFunctionReads(t *testing.T) {
	c, _ := testCollector(t)
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
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, map[string][]string{"sensor_state": sensorStates})
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml", collecttest.ConfigSchemaCheck{Defaults: true})
	c, _ := testCollector(t)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsFunctions(t, metadata, collecttest.MetadataFunctionsCheck{
		Context: t.Context(), Module: "ipmi", Methods: ipmifunc.Methods(defaultUpdateEvery), Handler: c.funcRouter, JobSelectable: true})
}

func TestCleanupHasFixedDeadline(t *testing.T) {
	c, r := testCollector(t)
	c.Timeout = confopt.Duration(24 * time.Hour)
	require.NoError(t, c.Check(t.Context()))
	before := time.Now()
	c.Cleanup(t.Context())
	require.NotNil(t, r.closeContext)
	deadline, ok := r.closeContext.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, before.Add(2*time.Second), deadline, 100*time.Millisecond)
	c.Cleanup(t.Context())
	assert.Equal(t, 1, r.closes)
}

func TestPartialWarningsAreRateLimited(t *testing.T) {
	c, r := testCollector(t)
	var logs bytes.Buffer
	c.Logger = logger.NewWithWriter(&logs)
	for i := range 10 {
		r.snapshot.Warnings = []string{fmt.Sprintf("%d unsupported sensors", i+1)}
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, strings.Count(logs.String(), "unsupported sensors"))
}
