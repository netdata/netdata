// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic HTTP responses exercise the slash aliases and URI remapping documented at
// https://servermanagementportal.ext.hpe.com/docs/redfishservices/ilos/ilo5/ilo5_changelog
// (iLO 5 1.40). They model issue #24183, whose attachment does not include response bodies.
func TestDecodedCollectorAcceptsResourceIdentityAliases(t *testing.T) {
	const root = "/redfish/v1/"
	const system = root + "Systems/1"
	const storage = system + "/Storage/1/"
	const volume = storage + "Volumes/1"
	const port = system + "/NetworkInterfaces/1/NetworkPorts/1/"
	for name, test := range map[string]struct {
		storageID string
		redirect  string
		absolute  bool
	}{
		"trailing slash removed":            {storageID: strings.TrimSuffix(storage, "/")},
		"trailing slash added":              {storageID: storage, redirect: strings.TrimSuffix(storage, "/")},
		"remapped storage":                  {storageID: system + "/Storage/SATA/1"},
		"absolute remapped storage":         {storageID: system + "/Storage/SATA/1", absolute: true},
		"redirect retains advertised alias": {storageID: storage, redirect: system + "/Storage/Redirected"},
	} {
		t.Run(name, func(t *testing.T) {
			finalStorage := storage
			if test.redirect != "" {
				finalStorage = test.redirect
			}
			docs := map[string]map[string]any{
				root: testutil.Resource(root, "ServiceRoot", "Service", map[string]any{
					"RedfishVersion": "1.20.0", "Systems": testutil.Link(root + "Systems"),
				}),
				root + "Systems": testutil.Collection(root+"Systems", "ComputerSystem", system),
				system: testutil.Resource(system+"/", "ComputerSystem", "System", map[string]any{
					"Storage":           testutil.Link(system + "/Storage"),
					"NetworkInterfaces": testutil.Link(system + "/NetworkInterfaces"),
				}),
				system + "/Storage": testutil.Collection(system+"/Storage", "Storage", storage),
				finalStorage: testutil.Resource(test.storageID, "Storage", "Storage", map[string]any{
					"Volumes": testutil.Link(storage + "Volumes"), "Status": map[string]any{"Health": "OK"},
				}),
				storage + "Volumes": testutil.Collection(storage+"Volumes", "Volume", volume),
				volume: testutil.Resource(
					volume,
					"Volume",
					"Volume",
					map[string]any{"RemainingCapacityPercent": 37},
				),
				system + "/NetworkInterfaces": testutil.Collection(
					system+"/NetworkInterfaces",
					"NetworkInterface",
					system+"/NetworkInterfaces/1",
				),
				system + "/NetworkInterfaces/1": testutil.Resource(
					system+"/NetworkInterfaces/1/",
					"NetworkInterface",
					"Interface",
					map[string]any{
						"NetworkPorts": testutil.Link(system + "/NetworkInterfaces/1/NetworkPorts"),
					},
				),
				system + "/NetworkInterfaces/1/NetworkPorts": testutil.Collection(
					system+"/NetworkInterfaces/1/NetworkPorts",
					"NetworkPort",
					port,
				),
				port: testutil.Resource(
					root+"Chassis/1/NetworkAdapters/1/NetworkPorts/1",
					"NetworkPort",
					"Port",
					map[string]any{
						"Status": map[string]any{"Health": "OK"},
					},
				),
			}
			var mu sync.Mutex
			requests := make(map[string]int)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests[r.URL.Path]++
				mu.Unlock()
				if r.URL.Path == storage && test.redirect != "" {
					http.Redirect(w, r, test.redirect, http.StatusPermanentRedirect)
					return
				}
				doc := docs[r.URL.Path]
				if doc == nil {
					http.NotFound(w, r)
					return
				}
				if r.URL.Path == finalStorage && test.absolute {
					doc = maps.Clone(doc)
					doc["@odata.id"] = "http://" + r.Host + test.storageID
				}
				testutil.WriteJSON(w, doc)
			}))
			t.Cleanup(server.Close)
			collector := sourceTestDecodedCollector(t, server.URL)
			_, origin, err := acquisition.NormalizeServiceRoot(server.URL)
			require.NoError(t, err)
			handler := collectorapi.DefaultRegistry["redfish"].MethodHandler(functionTestJob{collector.(*Collector)})
			for range 2 {
				sourceTestCollectCycle(t, collector)
				reader := collector.MetricStore().Read(metrix.ReadFlatten())
				statuses := make(map[string]float64)
				reader.ForEachByName("collection_status", func(labels metrix.LabelView, value metrix.SampleValue) {
					state, _ := labels.Get("collection_status")
					statuses[state] = float64(value)
				})
				assert.Equal(t, map[string]float64{"success": 1, "partial": 0, "unavailable": 0}, statuses)
				assert.Equal(
					t,
					map[string]float64{"Volume": 37},
					sourceTestMetricByResource(t, reader, "volume_remaining_capacity", ""),
				)
				storageKeys := make(map[string]bool)
				reader.ForEachByName("storage_health_status", func(labels metrix.LabelView, _ metrix.SampleValue) {
					key, _ := labels.Get("resource_key")
					storageKeys[key] = true
				})
				assert.Equal(
					t,
					map[string]bool{identity.ResourceKey(origin, "storage", finalStorage): true},
					storageKeys,
				)
				rows := functionRows(t, handler.Handle(t.Context(), "hardware", nil))
				for _, uri := range []string{system, finalStorage, volume, port} {
					assert.Equal(t, "Available", functionRowByURI(t, rows, uri)["Data availability"], uri)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for uri := range requests {
				_, served := docs[uri]
				assert.True(t, served || uri == storage, "unexpected request to advertised alias: %s", uri)
			}
		})
	}
}
