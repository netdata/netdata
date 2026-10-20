// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
)

// Measure a complete HTTP walk and metric projection. Work should scale with
// current resources and selected observations, without replaying retained data.
// Local ns/op includes loopback HTTP scheduling; allocations are the stable comparison.
func BenchmarkCollectionCycle(b *testing.B) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/1"
	const sensors = chassis + "/Sensors"
	docs := map[string]map[string]any{
		root: testutil.Resource(
			root,
			"ServiceRoot",
			"Root",
			map[string]any{"RedfishVersion": "1.20.0", "Chassis": testutil.Link(root + "Chassis")},
		),
		root + "Chassis": testutil.Collection(root+"Chassis", "Chassis", chassis),
		chassis: testutil.Resource(
			chassis,
			"Chassis",
			"Chassis",
			map[string]any{"Sensors": testutil.Link(sensors)},
		),
	}
	var members []string
	for index := range 32 {
		uri := fmt.Sprintf("%s/%d", sensors, index)
		members = append(members, uri)
		docs[uri] = testutil.Resource(uri, "Sensor", fmt.Sprintf("Sensor %d", index), map[string]any{
			"Reading": index, "ReadingType": "Temperature", "ReadingUnits": "Cel", "Status": map[string]any{"Health": "OK", "State": "Enabled"},
		})
	}
	docs[sensors] = testutil.Collection(sensors, "Sensor", members...)
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { testutil.WriteJSON(w, docs[r.URL.Path]) }),
	)
	b.Cleanup(server.Close)
	cfg := testConfig(server.URL, "none")
	client := New()
	client.Config = cfg
	if err := client.Init(b.Context()); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { client.Cleanup(b.Context()) })
	if _, err := client.collect(b.Context()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		result, err := client.collect(b.Context())
		if err != nil || !result.Complete {
			b.Fatalf("complete=%v err=%v", result.Complete, err)
		}
	}
}
