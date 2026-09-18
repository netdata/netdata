// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDerivedHealthCollectionAndFunctionLifecycle(t *testing.T) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/1"
	const sensors = chassis + "/Sensors"
	const sensor = sensors + "/temperature"
	var failure, reading atomic.Int64
	reading.Store(110)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failure.Load() == 1 || failure.Load() == 2 && r.URL.Path == sensor {
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case root:
			testutil.WriteJSON(w, testutil.Resource(root, "ServiceRoot", "Root", map[string]any{
				"RedfishVersion": "1.20.0", "Chassis": testutil.Link(root + "Chassis"),
			}))
		case root + "Chassis":
			testutil.WriteJSON(w, testutil.Collection(r.URL.Path, "Chassis", chassis))
		case chassis:
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Chassis", "Enclosure", map[string]any{
				"Sensors": testutil.Link(sensors),
			}))
		case sensors:
			testutil.WriteJSON(w, testutil.Collection(r.URL.Path, "Sensor", sensor))
		case sensor:
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Inlet", map[string]any{
				"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": reading.Load(),
				"Status": map[string]any{"Health": "OK", "State": "Enabled"},
				"Thresholds": map[string]any{"UpperCritical": map[string]any{
					"Reading": 100, "DwellTime": "PT10S",
				}},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c := newFunctionCollector(t, server.URL, "thresholds")
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	check := func(seconds int, want string) {
		t.Helper()
		now = time.Unix(1000+int64(seconds), 0)
		collectFunctionCycle(t, c)
		rows := functionRows(t, c.funcRouter.Handle(t.Context(), "sensors", nil))
		require.Len(t, rows, 1)
		assert.Equal(t, "OK", rows[0]["Health"], "source health remains authoritative for its own signal")
		assert.Equal(t, want, rows[0]["Derived health (thresholds)"])
		var active []string
		c.store.Read(metrix.ReadFlatten()).
			ForEachByName("derived_health", func(labels metrix.LabelView, value metrix.SampleValue) {
				if value > 0 {
					state, _ := labels.Get("derived_health")
					active = append(active, state)
				}
			})
		switch want {
		case "Unavailable":
			assert.Empty(t, active)
		case "OK":
			assert.Equal(t, []string{"ok"}, active)
		case "Critical":
			assert.Equal(t, []string{"critical"}, active)
		}
		if want == "Critical" {
			assert.Equal(t, "critical", rows[0]["rowOptions"].(map[string]any)["severity"])
		}
		collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	}
	check(0, "Unavailable")
	check(10, "Critical")
	failure.Store(1)
	now = now.Add(10 * time.Second)
	collectFunctionCycle(t, c)
	assert.Equal(t, 503, c.funcRouter.Handle(t.Context(), "sensors", nil).Status)
	failure.Store(0)
	check(30, "Unavailable")
	check(40, "Critical")
	failure.Store(2)
	now = now.Add(10 * time.Second)
	collectFunctionCycle(t, c)
	assert.Empty(t, functionRows(t, c.funcRouter.Handle(t.Context(), "sensors", nil)))
	failure.Store(0)
	check(60, "Unavailable")
	check(70, "Critical")
	reading.Store(95)
	check(80, "OK")

	// Cancel after projection, when collect records its duration, before publication.
	reading.Store(110)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clockCalls := 0
	c.now = func() time.Time {
		clockCalls++
		if clockCalls == 2 {
			cancel()
		}
		return time.Unix(1090, 0)
	}
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	cycle.AbortCycle()
	assert.Equal(t, 503, c.funcRouter.Handle(t.Context(), "sensors", nil).Status)
	c.now = func() time.Time { return now }
	check(100, "Unavailable")
	check(110, "Critical")
}
