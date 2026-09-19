// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type functionTestJob struct{ collector *Collector }

func (j functionTestJob) FullName() string   { return "redfish_" + j.Name() }
func (j functionTestJob) ModuleName() string { return "redfish" }
func (j functionTestJob) Name() string       { return "test-job" }
func (j functionTestJob) IsRunning() bool    { return true }
func (j functionTestJob) Collector() any     { return j.collector }

func TestFunctionsFollowRealCollection(t *testing.T) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/1"
	const sensors = chassis + "/Sensors"
	var phase, requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		state := phase.Load()
		if state == 2 || (state == 1 && r.URL.Path == sensors+"/temperature") {
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case root:
			testutil.WriteJSON(w, testutil.Resource(root, "ServiceRoot", "Root", map[string]any{
				"RedfishVersion": "1.20.0", "Chassis": testutil.Link(root + "Chassis"),
			}))
		case root + "Chassis":
			testutil.WriteJSON(w, testutil.Collection(root+"Chassis", "Chassis", chassis))
		case chassis:
			testutil.WriteJSON(w, testutil.Resource(chassis, "Chassis", "Enclosure", map[string]any{
				"Sensors": testutil.Link(sensors), "Model": "Rack unit", "SerialNumber": "fixture-serial",
				"Status": map[string]any{
					"Health":       "OK",
					"HealthRollup": "Warning",
					"State":        "Enabled",
					"Conditions": []any{
						map[string]any{"Message": "Fan redundancy lost", "Severity": "Warning"},
						map[string]any{"Severity": "Critical"},
					},
				},
			}))
		case sensors:
			testutil.WriteJSON(
				w,
				testutil.Collection(sensors, "Sensor", sensors+"/temperature", sensors+"/energy", sensors+"/missing"),
			)
		case sensors + "/temperature":
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Inlet", map[string]any{
				"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": 0,
				"Status": map[string]any{"Health": "OK"}, "AverageReading": 5, "PeakReading": 8,
			}))
		case sensors + "/energy":
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Energy", map[string]any{
				"ReadingType": "EnergyJoules", "ReadingUnits": "J", "Reading": 100 + 100*state,
				"Status": map[string]any{"Health": "Critical"},
			}))
		case sensors + "/missing":
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Missing", map[string]any{
				"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": nil,
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	collector := newFunctionCollector(t, server.URL)
	now := time.Unix(1000, 0)
	collector.now = func() time.Time { return now }
	handler := collectorapi.DefaultRegistry["redfish"].MethodHandler(functionTestJob{collector})
	require.NotNil(t, handler)
	call := func(method string) *funcapi.FunctionResponse { return handler.Handle(t.Context(), method, nil) }
	for _, method := range []string{"sensors", "hardware"} {
		require.Equal(t, 503, call(method).Status)
	}
	require.NoError(t, collector.Check(t.Context()))
	require.Equal(t, 503, call("sensors").Status, "Check does not publish inventory")
	collectFunctionCycle(t, collector)
	sensorsResponse := call("sensors")
	require.Equal(t, 200, sensorsResponse.Status)
	rows := functionRows(t, sensorsResponse)
	require.Len(t, rows, 3, "only primary readings, excluding average and peak")
	assert.Equal(t, "Critical", rows[0]["Health"], "source faults sort first")
	inlet := functionRowByURI(t, rows, sensors+"/temperature")
	assert.Equal(t, float64(0), inlet["Reading"])
	assert.Equal(t, "OK", inlet["Health"])
	assert.Equal(t, "Available", inlet["Data availability"])
	missing := functionRowByURI(t, rows, sensors+"/missing")
	assert.Nil(t, missing["Reading"])
	assert.Equal(t, "Not reported", missing["Health"])
	hardwareResponse := call("hardware")
	enclosure := functionRowByURI(t, functionRows(t, hardwareResponse), chassis)
	assert.Equal(t, "OK", enclosure["Health"])
	assert.Equal(t, "Warning", enclosure["Health rollup"])
	assert.Contains(t, enclosure["Reported issue"], "Fan redundancy lost")
	assert.Contains(t, enclosure["Reported issue"], "Condition: Critical")
	assert.Equal(t, "fixture-serial", enclosure["Serial number"])
	oldResponse, err := json.Marshal(sensorsResponse)
	require.NoError(t, err)
	oldSensors := sensorsResponse
	before := requests.Load()
	for range 5 {
		call("sensors")
		call("hardware")
	}
	assert.Equal(t, before, requests.Load(), "Function requests do not acquire BMC data")

	phase.Store(1)
	now = now.Add(10 * time.Second)
	collectFunctionCycle(t, collector)
	sensorsResponse = call("sensors")
	assert.Contains(t, sensorsResponse.Help, "Partial")
	for _, row := range functionRows(t, sensorsResponse) {
		assert.NotEqual(t, sensors+"/temperature", row["Resource URI"], "failed reads must not keep old readings")
		if row["Source"] == "Calculated from energy" {
			assert.Equal(t, float64(10), row["Reading"])
			assert.Equal(t, "Not applicable", row["Health"])
		}
	}
	readings := functionRows(t, sensorsResponse)
	var calculated int
	for _, row := range readings {
		if row["Source"] == "Calculated from energy" {
			calculated++
		}
	}
	assert.Equal(t, 1, calculated, "Function requests did not advance energy baselines")
	inlet = functionRowByURI(t, functionRows(t, call("hardware")), sensors+"/temperature")
	assert.Equal(t, "Unreadable", inlet["Data availability"])
	assert.Equal(t, "Not reported", inlet["Health"])
	assert.Nil(t, inlet["Observed"])
	oldAgain, err := json.Marshal(oldSensors)
	require.NoError(t, err)
	assert.Equal(t, oldResponse, oldAgain, "old responses are immutable after the next collection")

	phase.Store(2)
	collectFunctionCycle(t, collector)
	for _, method := range []string{"sensors", "hardware"} {
		assert.Equal(t, 503, call(method).Status)
	}
	phase.Store(3)
	now = now.Add(10 * time.Second)
	collectFunctionCycle(t, collector)
	assert.Equal(t, 200, call("sensors").Status)
	assert.Contains(t, call("hardware").Help, "Complete")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	require.ErrorIs(t, collector.Collect(ctx), context.Canceled)
	managed.CycleController().AbortCycle()
	assert.Equal(t, 503, call("sensors").Status)
	collectFunctionCycle(t, collector)
	assert.Equal(t, 200, call("sensors").Status)
	collector.Cleanup(t.Context())
	for _, method := range []string{"sensors", "hardware"} {
		assert.Equal(t, 503, call(method).Status)
	}
}

func TestFunctionSnapshotsAreJobOwnedAndConcurrent(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{})
	t.Cleanup(server.Close)
	first := newFunctionCollector(t, server.URL)
	second := newFunctionCollector(t, server.URL)
	collectFunctionCycle(t, first)
	assert.Equal(t, 503, second.funcRouter.Handle(t.Context(), "hardware", nil).Status)
	collectFunctionCycle(t, second)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 30 {
				response := first.funcRouter.Handle(t.Context(), "hardware", nil)
				assert.Equal(t, 200, response.Status)
				_, err := json.Marshal(response)
				assert.NoError(t, err)
			}
		})
	}
	for range 5 {
		collectFunctionCycle(t, first)
	}
	wg.Wait()
	first.Cleanup(t.Context())
	assert.Equal(t, 200, second.funcRouter.Handle(t.Context(), "hardware", nil).Status)
}

func newFunctionCollector(t *testing.T, url string) *Collector {
	t.Helper()
	c := New()
	c.Config = testConfig(url, "none")
	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c
}

func collectFunctionCycle(t *testing.T, c *Collector) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	if err := c.Collect(t.Context()); err != nil {
		cycle.AbortCycle()
		require.NoError(t, err)
	}
	require.NoError(t, cycle.CommitCycleSuccess())
}

func functionRows(t *testing.T, response *funcapi.FunctionResponse) []map[string]any {
	t.Helper()
	require.Equal(t, 200, response.Status)
	var rows []map[string]any
	for _, values := range response.Data.([][]any) {
		require.Len(t, values, len(response.Columns))
		row := make(map[string]any)
		for name, column := range response.Columns {
			row[name] = values[column.(map[string]any)["index"].(int)]
		}
		rows = append(rows, row)
	}
	return rows
}

func functionRowByURI(t *testing.T, rows []map[string]any, uri string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row["Resource URI"] == uri {
			return row
		}
	}
	t.Fatalf("resource %s missing from Function", uri)
	return nil
}
