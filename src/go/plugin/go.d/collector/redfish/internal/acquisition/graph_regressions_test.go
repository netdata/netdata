// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphMembershipPrunesRemovedSubtreesAndRetainsUnknown(t *testing.T) {
	const b = "/redfish/v1/"
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown_%t", unknown), func(t *testing.T) {
			var phase atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				current := fmt.Sprintf("%sChassis/C%d", b, phase.Load())
				var doc map[string]any
				switch r.URL.Path {
				case b:
					doc = testutil.Resource(
						b,
						"ServiceRoot",
						"Service",
						map[string]any{"RedfishVersion": "1.20.0", "Chassis": testutil.Link(b + "Chassis")},
					)
				case b + "Chassis":
					if unknown && phase.Load() > 0 {
						http.Error(w, "unavailable", 503)
						return
					}
					doc = testutil.Collection(b+"Chassis", "Chassis", b+"Chassis/B", current)
				case b + "Chassis/B":
					doc = testutil.Resource(
						b+"Chassis/B",
						"Chassis",
						"Broken",
						map[string]any{"Sensors": testutil.Link(b + "Chassis/B/Sensors")},
					)
				case b + "Chassis/B/Sensors":
					http.Error(w, "unavailable", 503)
					return
				case current:
					doc = testutil.Resource(
						current,
						"Chassis",
						"Current",
						map[string]any{"Sensors": testutil.Link(current + "/Sensors")},
					)
				case current + "/Sensors":
					doc = testutil.Collection(current+"/Sensors", "Sensor", current+"/Sensors/1")
				case current + "/Sensors/1":
					doc = testutil.Resource(current+"/Sensors/1", "Sensor", "Sensor", map[string]any{
						"ReadingType": "Percent", "ReadingUnits": "%", "Reading": 10, "Status": map[string]any{"Health": "OK"},
						"SensorGroup": map[string]any{"Name": "Group", "RedundancyType": "NPlusM"},
					})
				default:
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("OData-Version", "4.0")
				_ = json.NewEncoder(w).Encode(doc)
			}))
			t.Cleanup(server.Close)
			client := newTestProtocolClient(t, testConfig(server.URL, "none"))
			var acquired Result
			for p := int32(0); p < 4; p++ {
				phase.Store(p)
				acquired, _ = client.Acquire(t.Context())
			}
			origin := client.origin
			require.Len(t, client.baseMembership["chassis"], 2)
			assert.Len(t, client.graphMembership, 2, "only the current or unknown chassis subtree survives")
			current := b + "Chassis/C3"
			if unknown {
				current = b + "Chassis/C0"
			}
			parents := map[string]bool{
				identity.ResourceKey(origin, "chassis", current):             true,
				identity.ResourceKey(origin, "sensor", current+"/Sensors/1"): true,
			}
			for _, snapshot := range client.graphMembership {
				assert.True(t, parents[snapshot.ParentKey], "removed-parent snapshot survived")
			}
			readings := make(map[string]float64)
			projector := measurement.New(origin, "", ReadingProvenanceResolver(client.root, origin))
			projected, err := projector.Project(acquired.Resources, acquired.GraphComplete, time.Unix(10, 0))
			require.NoError(t, err)
			for _, observation := range projected.Observations {
				if observation.Metric == "reading_percentage_value" {
					readings[measurementTestLabel(observation, "resource_name")] = observation.Value
				}
			}

			if unknown {
				assert.Empty(t, readings, "unknown membership must not replay measurements")
			} else {
				assert.Equal(t, map[string]float64{"Sensor": 10}, readings)
			}
		})
	}
}
