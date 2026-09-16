// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func graphExcerptDocuments() map[string]map[string]any {
	const b = "/redfish/v1/"
	return map[string]map[string]any{
		b: sourceTestResource(
			b,
			"ServiceRoot",
			"Service",
			map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(b + "Chassis")},
		),
		b + "Chassis": sourceTestCollection(b+"Chassis", "Chassis", b+"Chassis/C"),
		b + "Chassis/C": sourceTestResource(
			b+"Chassis/C",
			"Chassis",
			"C",
			map[string]any{"EnvironmentMetrics": sourceTestLink(b + "Chassis/C/EnvironmentMetrics")},
		),
	}
}

func TestGraphEmbeddedIdentityModesDoNotCollide(t *testing.T) {
	for _, test := range []struct {
		name          string
		first, second map[string]any
	}{
		{"position and opaque member ID", map[string]any{"Name": "First", "Reading": 10}, map[string]any{"MemberId": "position:0", "Name": "Second", "Reading": 20}},
		{"distinct opaque member IDs", map[string]any{"MemberId": "a:b", "Name": "First", "Reading": 10}, map[string]any{"MemberId": "a", "Name": "Second", "Reading": 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			docs := graphExcerptDocuments()
			const uri = "/redfish/v1/Chassis/C/EnvironmentMetrics"
			docs[uri] = sourceTestResource(
				uri,
				"EnvironmentMetrics",
				"Metrics",
				map[string]any{"FanSpeedsPercent": []any{test.first, test.second}},
			)
			c := sourceTestDecodedCollector(t, sourceTestServeDocuments(t, docs))
			sourceTestCollectCycle(t, c)
			assert.Equal(
				t,
				map[string]float64{"First": 10, "Second": 20},
				sourceTestMetricByResource(
					t,
					c.MetricStore().Read(metrix.ReadFlatten()),
					"reading_percentage_value",
					"",
				),
			)
		})
	}
}

func TestGraphLinkedEnrichmentUsesItsOwnFragmentBase(t *testing.T) {
	const uri = "/redfish/v1/Chassis/C/EnvironmentMetrics"
	for _, test := range []struct {
		name, property, pointer, kind, locator, metric, role string
		array                                                bool
	}{
		{"array", "FanSpeedsPercent", "#/FanSpeedsPercent/0/Reading", "sensor", uri + "#/FanSpeedsPercent/0/Reading", "reading_percentage_value", "input", true},
		{"scalar", "PowerWatts", "#/PowerWatts/Reading", "chassis", "/redfish/v1/Chassis/C", "reading_power_value", "power", false},
	} {
		for _, relative := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/relative_%t", test.name, relative), func(t *testing.T) {
				source := uri + test.pointer
				if relative {
					source = test.pointer
				}
				docs := graphExcerptDocuments()
				var property any = map[string]any{"Name": "Fragment", "Reading": 10, "DataSourceUri": source, "Status": map[string]any{"Health": "OK"}}
				if test.array {
					property = []any{property}
				}
				docs[uri] = sourceTestResource(
					uri,
					"EnvironmentMetrics",
					"Metrics",
					map[string]any{test.property: property},
				)
				c := sourceTestDecodedCollector(t, sourceTestServeDocuments(t, docs))
				sourceTestCollectCycle(t, c)
				client := c.(*Collector).client.(*protocolClient)
				expected := resourceKey(client.origin, test.kind, test.locator)
				samples := 0
				c.MetricStore().
					Read(metrix.ReadFlatten()).
					ForEachByName(test.metric, func(labels metrix.LabelView, value metrix.SampleValue) {
						samples++
						assert.Equal(t, float64(10), value)
						key, _ := labels.Get("resource_key")
						assert.Equal(t, expected, key)
						key, _ = labels.Get("reading_key")
						assert.Equal(
							t,
							stableKey(
								"netdata:redfish:reading:v1",
								expected+"\x00"+uri+test.pointer+"\x00"+test.role,
								32,
							),
							key,
						)
					})
				require.Equal(t, 1, samples)
			})
		}
	}
}

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
					doc = sourceTestResource(
						b,
						"ServiceRoot",
						"Service",
						map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(b + "Chassis")},
					)
				case b + "Chassis":
					if unknown && phase.Load() > 0 {
						http.Error(w, "unavailable", 503)
						return
					}
					doc = sourceTestCollection(b+"Chassis", "Chassis", b+"Chassis/B", current)
				case b + "Chassis/B":
					doc = sourceTestResource(
						b+"Chassis/B",
						"Chassis",
						"Broken",
						map[string]any{"Sensors": sourceTestLink(b + "Chassis/B/Sensors")},
					)
				case b + "Chassis/B/Sensors":
					http.Error(w, "unavailable", 503)
					return
				case current:
					doc = sourceTestResource(
						current,
						"Chassis",
						"Current",
						map[string]any{"Sensors": sourceTestLink(current + "/Sensors")},
					)
				case current + "/Sensors":
					doc = sourceTestCollection(current+"/Sensors", "Sensor", current+"/Sensors/1")
				case current + "/Sensors/1":
					doc = sourceTestResource(current+"/Sensors/1", "Sensor", "Sensor", map[string]any{
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
			c := sourceTestDecodedCollector(t, server.URL)
			for p := int32(0); p < 4; p++ {
				phase.Store(p)
				sourceTestCollectCycle(t, c)
			}
			client := c.(*Collector).client.(*protocolClient)
			require.Len(t, client.baseMembership["chassis"], 2)
			assert.Len(t, client.graphMembership, 2, "only the current or unknown chassis subtree survives")
			current := b + "Chassis/C3"
			if unknown {
				current = b + "Chassis/C0"
			}
			parents := map[string]bool{
				resourceKey(client.origin, "chassis", current):             true,
				resourceKey(client.origin, "sensor", current+"/Sensors/1"): true,
			}
			for _, snapshot := range client.graphMembership {
				assert.True(t, parents[snapshot.ParentKey], "removed-parent snapshot survived")
			}
			readings := sourceTestMetricByResource(
				t,
				c.MetricStore().Read(metrix.ReadFlatten()),
				"reading_percentage_value",
				"",
			)
			if unknown {
				assert.Empty(t, readings, "unknown membership must not replay measurements")
			} else {
				assert.Equal(t, map[string]float64{"Sensor": 10}, readings)
			}
		})
	}
}

func TestGraphSensorAddressabilitySurvivesExcerptFallback(t *testing.T) {
	const b = "/redfish/v1/"
	for _, first := range []string{"A", "B"} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_first/fallback_%t", first, fallback), func(t *testing.T) {
				order := []string{b + "Chassis/A", b + "Chassis/B"}
				if first == "B" {
					order[0], order[1] = order[1], order[0]
				}
				docs := map[string]map[string]any{
					b: sourceTestResource(
						b,
						"ServiceRoot",
						"Service",
						map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(b + "Chassis")},
					),
					b + "Chassis": sourceTestCollection(b+"Chassis", "Chassis", order...),
					b + "Chassis/A": sourceTestResource(
						b+"Chassis/A",
						"Chassis",
						"A",
						map[string]any{"EnvironmentMetrics": sourceTestLink(b + "Chassis/A/EnvironmentMetrics")},
					),
					b + "Chassis/B": sourceTestResource(
						b+"Chassis/B",
						"Chassis",
						"B",
						map[string]any{"Sensors": sourceTestLink(b + "Chassis/B/Sensors")},
					),
					b + "Chassis/B/Sensors": sourceTestCollection(
						b+"Chassis/B/Sensors",
						"Sensor",
						b+"Chassis/B/Sensors/1",
					),
					b + "Chassis/B/Sensors/1": sourceTestResource(
						b+"Chassis/B/Sensors/1",
						"Sensor",
						"Sensor",
						map[string]any{
							"ReadingType":  "Percent",
							"ReadingUnits": "%",
							"Reading":      42,
							"Status":       map[string]any{"Health": "Warning"},
						},
					),
					b + "Chassis/A/EnvironmentMetrics": sourceTestResource(
						b+"Chassis/A/EnvironmentMetrics",
						"EnvironmentMetrics",
						"Metrics",
						map[string]any{"FanSpeedsPercent": []any{
							map[string]any{
								"Name":          "Excerpt",
								"Reading":       43,
								"DataSourceUri": b + "Chassis/B/Sensors/1#/Reading",
								"Status":        map[string]any{"Health": "OK"},
							},
						}},
					),
				}
				var phase atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					p := phase.Load()
					if (p == 1 && r.URL.Path == b+"Chassis/B/Sensors/1") ||
						(p == 3 && (r.URL.Path == b+"Chassis/B/Sensors" || r.URL.Path == b+"Chassis/A/EnvironmentMetrics")) {
						http.Error(w, "unavailable", 503)
						return
					}
					doc := docs[r.URL.Path]
					if p == 2 && r.URL.Path == b+"Chassis/B/Sensors" {
						doc = sourceTestCollection(b+"Chassis/B/Sensors", "Sensor")
					}
					if p == 2 && r.URL.Path == b+"Chassis/A/EnvironmentMetrics" {
						copy := make(map[string]any)
						for k, v := range doc {
							copy[k] = v
						}
						copy["FanSpeedsPercent"] = append([]any{nil}, doc["FanSpeedsPercent"].([]any)...)
						doc = copy
					}
					if doc == nil {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("OData-Version", "4.0")
					_ = json.NewEncoder(w).Encode(doc)
				}))
				t.Cleanup(server.Close)
				c := sourceTestDecodedCollector(t, server.URL)
				client := c.(*Collector).client.(*protocolClient)
				expected := resourceKey(client.origin, "sensor", b+"Chassis/B/Sensors/1")
				for p := int32(0); p <= 4; p++ {
					if p == 1 && !fallback {
						continue
					}
					phase.Store(p)
					sourceTestCollectCycle(t, c)
					reader := c.MetricStore().Read(metrix.ReadFlatten())
					keys := map[string]bool{}
					reader.ForEachByName(
						"sensor_acquisition_state",
						func(labels metrix.LabelView, _ metrix.SampleValue) {
							key, _ := labels.Get("resource_key")
							keys[key] = true
						},
					)
					assert.Equal(t, map[string]bool{expected: true}, keys, "phase %d", p)
					values := sourceTestMetricByResource(t, reader, "reading_percentage_value", "")
					switch p {
					case 0, 4:
						assert.Equal(t, map[string]float64{"Sensor": 42}, values)
					case 1, 2:
						assert.Equal(t, map[string]float64{"Excerpt": 43}, values)
					case 3:
						assert.Empty(t, values, "failed reads cannot replay retained samples")
					}
					if p != 3 {
						name, state := "Excerpt", "clear"
						if p == 0 || p == 4 {
							name, state = "Sensor", "warning"
						}
						assert.Equal(
							t,
							sourceTestStateValues(
								map[string]string{name: state},
								[]string{"clear", "warning", "critical"},
							),
							sourceTestMetricByResource(t, reader, "reading_alarm", "reading_alarm"),
							"phase %d source health",
							p,
						)
					}
				}
			})
		}
	}
}
