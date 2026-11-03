// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Manager port collections contain Port resources, and shared members also belong to an adapter's Ports collection.
// The independent DMTF/HPE source evidence for these synthetic responses is recorded in testdata/ATTRIBUTION.md.
func TestDecodedCollectorCollectsManagerPorts(t *testing.T) {
	const root = "/redfish/v1/"
	const manager = root + "Managers/BMC"
	const chassis = root + "Chassis/1"
	const adapter = chassis + "/NetworkAdapters/1"
	const legacyPort = adapter + "/NetworkPorts/1"
	for name, test := range map[string]struct {
		relationship string
		memberURI    string
		shared       bool
	}{
		"dedicated": {
			relationship: "DedicatedNetworkPorts",
			memberURI:    manager + "/DedicatedNetworkPorts/1/",
		},
		"shared with adapter": {
			relationship: "SharedNetworkPorts",
			memberURI:    adapter + "/Ports/1",
			shared:       true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			collection := manager + "/" + test.relationship
			metricsURI := strings.TrimSuffix(test.memberURI, "/") + "/Metrics"
			environmentURI := strings.TrimSuffix(test.memberURI, "/") + "/EnvironmentMetrics"
			adapterProperties := map[string]any{"NetworkPorts": testutil.Link(adapter + "/NetworkPorts")}
			if test.shared {
				adapterProperties["Ports"] = testutil.Link(adapter + "/Ports")
			}
			documents := map[string]map[string]any{
				root: testutil.Resource(root, "ServiceRoot", "Service", map[string]any{
					"RedfishVersion": "1.20.0",
					"Managers":       testutil.Link(root + "Managers"),
					"Chassis":        testutil.Link(root + "Chassis"),
				}),
				root + "Managers": testutil.Collection(root+"Managers", "Manager", manager),
				manager: testutil.Resource(
					manager,
					"Manager",
					"BMC",
					map[string]any{test.relationship: testutil.Link(collection)},
				),
				collection:       testutil.Collection(collection, "Port", test.memberURI),
				root + "Chassis": testutil.Collection(root+"Chassis", "Chassis", chassis),
				chassis: testutil.Resource(chassis, "Chassis", "Chassis", map[string]any{
					"NetworkAdapters": testutil.Link(chassis + "/NetworkAdapters"),
				}),
				chassis + "/NetworkAdapters": testutil.Collection(
					chassis+"/NetworkAdapters",
					"NetworkAdapter",
					adapter,
				),
				adapter: testutil.Resource(
					adapter,
					"NetworkAdapter",
					"Adapter",
					adapterProperties,
				),
				adapter + "/Ports":        testutil.Collection(adapter+"/Ports", "Port", test.memberURI),
				adapter + "/NetworkPorts": testutil.Collection(adapter+"/NetworkPorts", "NetworkPort", legacyPort),
				legacyPort: testutil.Resource(legacyPort, "NetworkPort", "Legacy port", map[string]any{
					"LinkStatus":           "Up",
					"CurrentLinkSpeedMbps": 1000,
				}),
				test.memberURI: testutil.Resource(
					strings.TrimSuffix(test.memberURI, "/"),
					"Port",
					"Manager port",
					map[string]any{
						"@odata.type":        "#Port.v1_12_0.Port",
						"LinkState":          "Enabled",
						"Metrics":            testutil.Link(metricsURI),
						"EnvironmentMetrics": testutil.Link(environmentURI),
					},
				),
			}
			cycles := []struct {
				linkStatus string
				state      string
				speed      any
				speeds     map[string]float64
			}{
				{
					linkStatus: "LinkUp",
					state:      "link_up",
					speed:      0.1,
					speeds:     map[string]float64{"Manager port": 100_000_000},
				},
				{linkStatus: "LinkDown", state: "link_down", speed: 0, speeds: map[string]float64{"Manager port": 0}},
				{linkStatus: "NoLink", state: "no_link", speeds: map[string]float64{}},
			}
			var phase, memberRequests, metricsRequests, environmentRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := phase.Load()
				document := documents[r.URL.Path]
				switch r.URL.Path {
				case test.memberURI:
					memberRequests.Add(1)
					document = maps.Clone(document)
					document["LinkStatus"] = cycles[current].linkStatus
					document["CurrentSpeedGbps"] = cycles[current].speed
				case metricsURI:
					metricsRequests.Add(1)
					document = testutil.Resource(
						metricsURI,
						"PortMetrics",
						"Port metrics",
						map[string]any{"RXBytes": 100 + current*200},
					)
				case environmentURI:
					environmentRequests.Add(1)
					document = testutil.Resource(environmentURI, "EnvironmentMetrics", "Environment", map[string]any{
						"TemperatureCelsius": map[string]any{"Reading": 30 + current},
					})
				}
				if document == nil {
					http.NotFound(w, r)
					return
				}
				testutil.WriteJSON(w, document)
			}))
			t.Cleanup(server.Close)
			collector := sourceTestDecodedCollector(t, server.URL)
			collector.(*Collector).now = func() time.Time { return time.Unix(100+int64(phase.Load())*10, 0) }
			_, origin, err := acquisition.NormalizeServiceRoot(server.URL)
			require.NoError(t, err)
			handler := collectorapi.DefaultRegistry["redfish"].MethodHandler(functionTestJob{collector.(*Collector)})
			for cycle, observation := range cycles {
				phase.Store(int32(cycle))
				memberRequests.Store(0)
				metricsRequests.Store(0)
				environmentRequests.Store(0)
				sourceTestCollectCycle(t, collector)
				reader := collector.MetricStore().Read(metrix.ReadFlatten())
				statuses := make(map[string]float64)
				reader.ForEachByName("collection_status", func(labels metrix.LabelView, value metrix.SampleValue) {
					state, _ := labels.Get("collection_status")
					statuses[state] = float64(value)
				})
				assert.Equal(t, map[string]float64{"success": 1, "partial": 0, "unavailable": 0}, statuses)
				assert.Equal(t, observation.speeds, sourceTestMetricByResource(t, reader, "port_link_speed_speed", ""))
				assert.Equal(t,
					sourceTestStateValues(map[string]string{"Manager port": observation.state},
						[]string{"link_up", "starting", "training", "link_down", "no_link", "unknown"}),
					sourceTestMetricByResource(t, reader, "port_link_status", "port_link_status"))
				assert.Equal(t, map[string]float64{"Manager port": float64(30 + cycle)},
					sourceTestMetricByResource(t, reader, "reading_temperature_value", ""))
				traffic := map[string]float64{}
				if cycle > 0 {
					traffic["Manager port"] = 20
				}
				assert.Equal(t, traffic, sourceTestMetricByResource(t, reader, "port_traffic_received", ""))
				portKeys := make(map[string]bool)
				reader.ForEachByName("port_acquisition_state", func(labels metrix.LabelView, _ metrix.SampleValue) {
					key, _ := labels.Get("resource_key")
					portKeys[key] = true
				})
				assert.Equal(t, map[string]bool{identity.ResourceKey(origin, "port", test.memberURI): true}, portKeys)
				assert.Equal(t, map[string]float64{"Legacy port": 1_000_000_000},
					sourceTestMetricByResource(t, reader, "network_port_link_speed", ""))
				assert.Equal(t, sourceTestStateValues(map[string]string{"Legacy port": "up"},
					[]string{"down", "up", "starting", "training", "unknown"}),
					sourceTestMetricByResource(t, reader, "network_port_link_status", "network_port_link_status"))
				rows := functionRows(t, handler.Handle(t.Context(), "hardware", nil))
				memberRows := make(map[string]int)
				for _, row := range rows {
					if row["Resource URI"] == test.memberURI {
						memberRows[test.memberURI]++
						assert.Equal(t, "Available", row["Data availability"])
					}
				}
				assert.Equal(t, map[string]int{test.memberURI: 1}, memberRows)
				assert.Equal(t, int32(1), memberRequests.Load(), "shared member must be fetched once per cycle")
				assert.Equal(t, int32(1), metricsRequests.Load(), "PortMetrics must be fetched once per cycle")
				assert.Equal(
					t,
					int32(1),
					environmentRequests.Load(),
					"EnvironmentMetrics must be fetched once per cycle",
				)
			}
		})
	}
}
