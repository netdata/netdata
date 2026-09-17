// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodedCollectorPreservesSourceMetricsAcrossPartialAndRemovedResources(t *testing.T) {
	var phase atomic.Int32
	const base = "/redfish/v1/"
	documents := map[string]map[string]any{
		base: sourceTestResource(base, "ServiceRoot", "Service", map[string]any{
			"RedfishVersion": "1.20.0", "Systems": sourceTestLink(base + "Systems"), "Chassis": sourceTestLink(base + "Chassis"), "Managers": sourceTestLink(base + "Managers"),
		}),
		base + "Systems":  sourceTestCollection(base+"Systems", "ComputerSystem", base+"Systems/A", base+"Systems/B"),
		base + "Managers": sourceTestCollection(base+"Managers", "Manager"),
		base + "Chassis":  sourceTestCollection(base+"Chassis", "Chassis", base+"Chassis/1"),
		base + "Systems/A": sourceTestResource(base+"Systems/A", "ComputerSystem", "System A", map[string]any{
			"Status": map[string]any{
				"Health": "OK",
			}, "Storage": sourceTestLink(base + "Storage"), "Memory": sourceTestLink(base + "Memory"),
		}),
		base + "Systems/B": sourceTestResource(base+"Systems/B", "ComputerSystem", "System B", map[string]any{
			"Status": map[string]any{
				"Health": "Warning",
			}, "EthernetInterfaces": sourceTestLink(base + "EthernetInterfaces"),
		}),
		base + "Memory":  sourceTestCollection(base+"Memory", "Memory"),
		base + "Storage": sourceTestCollection(base+"Storage", "Storage", base+"Storage/1"),
		base + "Storage/1": sourceTestResource(base+"Storage/1", "Storage", "Storage", map[string]any{
			"Controllers": sourceTestLink(base + "Controllers"), "Volumes": sourceTestLink(base + "Volumes"),
		}),
		base + "Controllers": sourceTestCollection(base+"Controllers", "StorageController", base+"Controllers/1"),
		base + "Controllers/1": sourceTestResource(
			base+"Controllers/1",
			"StorageController",
			"Controller",
			map[string]any{
				"PCIeInterface": map[string]any{"LanesInUse": 8}, "Status": map[string]any{"Health": "Warning"},
			},
		),
		base + "Volumes": sourceTestCollection(base+"Volumes", "Volume", base+"Volumes/1"),
		base + "Volumes/1": sourceTestResource(
			base+"Volumes/1",
			"Volume",
			"Volume",
			map[string]any{"RemainingCapacityPercent": 37},
		),
		base + "EthernetInterfaces": sourceTestCollection(
			base+"EthernetInterfaces",
			"EthernetInterface",
			base+"EthernetInterfaces/1",
		),
		base + "EthernetInterfaces/1": sourceTestResource(
			base+"EthernetInterfaces/1",
			"EthernetInterface",
			"NIC",
			map[string]any{"SpeedMbps": 2500},
		),
		base + "Chassis/1": sourceTestResource(base+"Chassis/1", "Chassis", "Chassis", map[string]any{
			"PowerSubsystem": sourceTestLink(
				base + "PowerSubsystem",
			), "ThermalSubsystem": sourceTestLink(base + "ThermalSubsystem"), "Sensors": sourceTestLink(base + "Sensors"),
		}),
		base + "PowerSubsystem": sourceTestResource(
			base+"PowerSubsystem",
			"PowerSubsystem",
			"Power",
			map[string]any{"Batteries": sourceTestLink(base + "Batteries")},
		),
		base + "Batteries": sourceTestCollection(base+"Batteries", "Battery", base+"Batteries/1"),
		base + "Batteries/1": sourceTestResource(base+"Batteries/1", "Battery", "Battery", map[string]any{
			"CapacityActualWattHours": 480, "Metrics": sourceTestLink(base + "Batteries/1/Metrics"),
		}),
		base + "Batteries/1/Metrics": sourceTestResource(
			base+"Batteries/1/Metrics",
			"BatteryMetrics",
			"Battery metrics",
			map[string]any{
				"ChargePercent":         map[string]any{"Reading": 0, "Status": map[string]any{"Health": "Warning"}},
				"StoredEnergyWattHours": map[string]any{"Reading": 0},
			},
		),
		base + "ThermalSubsystem": sourceTestResource(
			base+"ThermalSubsystem",
			"ThermalSubsystem",
			"Thermal",
			map[string]any{"ThermalMetrics": sourceTestLink(base + "ThermalMetrics")},
		),
		base + "ThermalMetrics": sourceTestResource(
			base+"ThermalMetrics",
			"ThermalMetrics",
			"Thermal metrics",
			map[string]any{
				"TemperatureReadingsCelsius": []any{
					map[string]any{
						"MemberId": "intake",
						"Name":     "Intake",
						"Reading":  21.5,
						"Status":   map[string]any{"Health": "OK"},
					},
				},
			},
		),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := phase.Load()
		if current == 1 &&
			(r.URL.Path == base+"Memory" || r.URL.Path == base+"Sensors/Old" || r.URL.Path == base+"ThermalMetrics") {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		document := documents[r.URL.Path]
		switch r.URL.Path {
		case base + "ThermalMetrics":
			if current >= 2 {
				document = sourceTestResource(
					r.URL.Path,
					"ThermalMetrics",
					"Thermal metrics",
					map[string]any{"TemperatureReadingsCelsius": []any{}},
				)
			}
		case base + "Sensors":
			members := []string{}
			if current < 2 {
				members = append(members, base+"Sensors/Old")
			}
			if current == 1 || current == 2 {
				members = append(members, base+"Sensors/New")
			}
			document = sourceTestCollection(r.URL.Path, "Sensor", members...)
		case base + "Sensors/Old":
			document = sourceTestResource(
				r.URL.Path,
				"Sensor",
				"Old sensor",
				map[string]any{
					"ReadingType":  "Temperature",
					"ReadingUnits": "Cel",
					"Reading":      31,
					"Status":       map[string]any{"Health": "Critical"},
				},
			)
		case base + "Sensors/New":
			var value any = 42
			health := "OK"
			if current == 2 {
				value, health = nil, "Warning"
			}
			document = sourceTestResource(
				r.URL.Path,
				"Sensor",
				"New sensor",
				map[string]any{
					"ReadingType":  "Temperature",
					"ReadingUnits": "Cel",
					"Reading":      value,
					"Status":       map[string]any{"Health": health},
				},
			)
		}
		if document == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("OData-Version", "4.0")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(server.Close)
	collector := sourceTestDecodedCollector(t, server.URL)

	for step, expectation := range []struct {
		status string
		values map[string]float64
		alarms map[string]string
	}{
		{status: "success", values: map[string]float64{"Old sensor": 31, "Intake": 21.5}, alarms: map[string]string{"Old sensor": "critical", "Intake": "clear"}},
		{status: "partial", values: map[string]float64{"New sensor": 42}, alarms: map[string]string{"New sensor": "clear"}},
		{status: "success", values: map[string]float64{}, alarms: map[string]string{"New sensor": "warning"}},
		{status: "success", values: map[string]float64{}, alarms: map[string]string{}},
	} {
		t.Run(fmt.Sprintf("cycle_%d", step), func(t *testing.T) {
			phase.Store(int32(step))
			sourceTestCollectCycle(t, collector)
			reader := collector.MetricStore().Read(metrix.ReadFlatten())
			assert.Equal(
				t,
				[]metrix.HostScope{{}},
				reader.HostScopes(),
				"all endpoint resources use the ordinary job scope",
			)
			assert.Equal(
				t,
				expectation.values,
				sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_input", ""),
			)
			assert.Equal(
				t,
				sourceTestStateValues(expectation.alarms, []string{"clear", "warning", "critical"}),
				sourceTestMetricByResource(
					t,
					reader,
					"system_hw_sensor_temperature_alarm",
					"system_hw_sensor_temperature_alarm",
				),
			)
			for metric, expected := range map[string]map[string]float64{
				"storage_controller_pcie_lanes_active": {"Controller": 8},
				"volume_remaining_capacity":            {"Volume": 37},
				"ethernet_interface_link_speed":        {"NIC": 2_500_000_000},
				"battery_energy_capacity_actual":       {"Battery": 480},
				"reading_percentage_value":             {"Battery": 0},
				"reading_stored_energy_value":          {"Battery": 0},
			} {
				assert.Equal(t, expected, sourceTestMetricByResource(t, reader, metric, ""), metric)
			}
			assert.Equal(
				t,
				sourceTestStateValues(
					map[string]string{"System A": "ok", "System B": "warning"},
					[]string{"ok", "warning", "critical", "unknown"},
				),
				sourceTestMetricByResource(t, reader, "system_health", "system_health"),
			)
			assert.Equal(
				t,
				sourceTestStateValues(
					map[string]string{"Controller": "warning"},
					[]string{"ok", "warning", "critical", "unknown"},
				),
				sourceTestMetricByResource(t, reader, "storage_controller_health", "storage_controller_health"),
			)
			assert.Equal(
				t,
				sourceTestStateValues(
					map[string]string{"Battery": "warning"},
					[]string{"clear", "warning", "critical"},
				),
				sourceTestMetricByResource(t, reader, "reading_alarm", "reading_alarm"),
			)
			var statuses []string
			reader.ForEachByName("collection_status", func(labels metrix.LabelView, value metrix.SampleValue) {
				if value == 1 {
					state, _ := labels.Get("collection_status")
					statuses = append(statuses, state)
				}
			})
			assert.Equal(t, []string{expectation.status}, statuses)
			contexts := map[string][]string{
				"redfish.storage_controller.pcie_lanes": {"active"},
				"redfish.volume.remaining_capacity":     {"remaining"},
				"redfish.ethernet_interface.link_speed": {"speed"},
				"redfish.battery.energy_capacity":       {"actual"},
			}
			if len(expectation.values) > 0 {
				contexts["system.hw.sensor.temperature.input"] = []string{"input"}
			}
			collecttest.AssertChartCoverage(
				t,
				collector,
				collecttest.ChartCoverageExpectation{
					RequiredContexts: contexts,
				},
			)
		})
	}
	t.Run("first collection with partial branches", func(t *testing.T) {
		phase.Store(1)
		fresh := sourceTestDecodedCollector(t, server.URL)
		sourceTestCollectCycle(t, fresh)
		reader := fresh.MetricStore().Read(metrix.ReadFlatten())
		assert.Equal(
			t,
			map[string]float64{"New sensor": 42},
			sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_input", ""),
		)
		assert.Equal(
			t,
			sourceTestStateValues(map[string]string{"New sensor": "clear"}, []string{"clear", "warning", "critical"}),
			sourceTestMetricByResource(
				t,
				reader,
				"system_hw_sensor_temperature_alarm",
				"system_hw_sensor_temperature_alarm",
			),
		)
		assert.Equal(
			t,
			map[string]float64{"Controller": 8},
			sourceTestMetricByResource(t, reader, "storage_controller_pcie_lanes_active", ""),
		)
		collecttest.AssertChartCoverage(
			t,
			fresh,
			collecttest.ChartCoverageExpectation{
				RequiredContexts: map[string][]string{"system.hw.sensor.temperature.input": {"input"}},
			},
		)
	})
}

// 101 is the boundary of the removed 100-component suppression policy, not a workload limit.
func TestDecodedCollectorPublishesEverySensorAboveFormerDetailCap(t *testing.T) {
	const count = 101
	const base = "/redfish/v1/"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var document map[string]any
		switch r.URL.Path {
		case base:
			document = sourceTestResource(
				base,
				"ServiceRoot",
				"Service",
				map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(base + "Chassis")},
			)
		case base + "Chassis":
			document = sourceTestCollection(r.URL.Path, "Chassis", base+"Chassis/1")
		case base + "Chassis/1":
			document = sourceTestResource(
				r.URL.Path,
				"Chassis",
				"Chassis",
				map[string]any{"Sensors": sourceTestLink(base + "Sensors")},
			)
		case base + "Sensors":
			members := make([]string, count)
			for i := range members {
				members[i] = fmt.Sprintf("%sSensors/%d", base, i)
			}
			document = sourceTestCollection(r.URL.Path, "Sensor", members...)
		default:
			var i int
			if _, err := fmt.Sscanf(r.URL.Path, base+"Sensors/%d", &i); err != nil {
				http.NotFound(w, r)
				return
			}
			document = sourceTestResource(
				r.URL.Path,
				"Sensor",
				fmt.Sprintf("Sensor %d", i),
				map[string]any{
					"ReadingType":  "Temperature",
					"ReadingUnits": "Cel",
					"Reading":      i,
					"Status":       map[string]any{"Health": "OK"},
				},
			)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("OData-Version", "4.0")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(server.Close)
	collector := sourceTestDecodedCollector(t, server.URL)
	sourceTestCollectCycle(t, collector)
	expected := make(map[string]float64, count)
	for i := range count {
		expected[fmt.Sprintf("Sensor %d", i)] = float64(i)
	}
	assert.Equal(
		t,
		expected,
		sourceTestMetricByResource(t, collector.MetricStore().Read(), "system_hw_sensor_temperature_input", ""),
	)
	collecttest.AssertChartCoverage(
		t,
		collector,
		collecttest.ChartCoverageExpectation{
			RequiredContexts: map[string][]string{"system.hw.sensor.temperature.input": {"input"}},
		},
	)
}

func sourceTestDecodedCollector(t *testing.T, endpoint string) collectorapi.CollectorV2 {
	t.Helper()
	creator, ok := collectorapi.DefaultRegistry.Lookup("redfish")
	require.True(t, ok)
	collector := creator.CreateV2()
	payload, err := json.Marshal(
		map[string]any{"name": "source-contract", "url": endpoint, "auth_method": "none"},
	)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(payload, collector))
	require.NoError(t, collector.Init(t.Context()))
	t.Cleanup(func() { collector.Cleanup(context.Background()) })
	require.NoError(t, collector.Check(t.Context()))
	return collector
}

func sourceTestCollectCycle(t *testing.T, collector collectorapi.CollectorV2) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	require.NoError(t, collector.Collect(t.Context()))
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
}

func sourceTestMetricByResource(t *testing.T, reader metrix.Reader, metric, stateLabel string) map[string]float64 {
	t.Helper()
	result := make(map[string]float64)
	reader.ForEachByName(metric, func(labels metrix.LabelView, value metrix.SampleValue) {
		name, ok := labels.Get("resource_name")
		require.True(t, ok)
		key, ok := labels.Get("resource_key")
		require.True(t, ok)
		require.NotEmpty(t, key)
		if stateLabel != "" {
			state, _ := labels.Get(stateLabel)
			name += "/" + state
		}
		_, duplicate := result[name]
		require.False(t, duplicate, "duplicate source metric %s for %s", metric, name)
		result[name] = value
	})
	return result
}

func sourceTestStateValues(states map[string]string, allowed []string) map[string]float64 {
	result := make(map[string]float64)
	for resource, enabled := range states {
		for _, state := range allowed {
			value := float64(0)
			if state == enabled {
				value = 1
			}
			result[resource+"/"+state] = value
		}
	}
	return result
}

func sourceTestLink(uri string) map[string]any { return map[string]any{"@odata.id": uri} }

func sourceTestResource(uri, schema, name string, properties map[string]any) map[string]any {
	document := map[string]any{
		"@odata.id":   uri,
		"@odata.type": "#" + schema + ".v1_0_0." + schema,
		"Id":          name,
		"Name":        name,
	}
	for key, value := range properties {
		document[key] = value
	}
	return document
}

func sourceTestCollection(uri, schema string, members ...string) map[string]any {
	values := make([]any, 0, len(members))
	for _, member := range members {
		values = append(values, sourceTestLink(member))
	}
	return map[string]any{
		"@odata.id":           uri,
		"@odata.type":         "#" + schema + "Collection." + schema + "Collection",
		"Members@odata.count": len(values),
		"Members":             values,
		"Name":                strings.TrimPrefix(uri, "/redfish/v1/"),
	}
}

func TestDecodedCollectorKeepsSharedResourceAfterEarlierBranchFailure(t *testing.T) {
	var phase atomic.Int32
	const b = "/redfish/v1/"
	docs := map[string]map[string]any{
		b: sourceTestResource(
			b,
			"ServiceRoot",
			"Service",
			map[string]any{
				"RedfishVersion": "1.20.0",
				"Systems":        sourceTestLink(b + "Systems"),
				"Chassis":        sourceTestLink(b + "Chassis"),
			},
		),
		b + "Systems": sourceTestCollection(b+"Systems", "ComputerSystem", b+"Systems/1"),
		b + "Chassis": sourceTestCollection(b+"Chassis", "Chassis", b+"Chassis/1"),
		b + "Systems/1": sourceTestResource(
			b+"Systems/1",
			"ComputerSystem",
			"System",
			map[string]any{"Memory": sourceTestLink(b + "Systems/1/Memory")},
		),
		b + "Chassis/1": sourceTestResource(
			b+"Chassis/1",
			"Chassis",
			"Chassis",
			map[string]any{"Memory": sourceTestLink(b + "Chassis/1/Memory")},
		),
		b + "Systems/1/Memory": sourceTestCollection(b+"Systems/1/Memory", "Memory", b+"Memory/1"),
		b + "Chassis/1/Memory": sourceTestCollection(b+"Chassis/1/Memory", "Memory", b+"Memory/1"),
		b + "Memory/1": sourceTestResource(
			b+"Memory/1",
			"Memory",
			"Shared memory",
			map[string]any{"Status": map[string]any{"Health": "OK"}, "CapacityMiB": 1024},
		),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if phase.Load() == 1 && r.URL.Path == b+"Systems/1/Memory" {
			http.Error(w, "unavailable", 503)
			return
		}
		doc := docs[r.URL.Path]
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
	sourceTestCollectCycle(t, c)
	want := sourceTestMetricByResource(t, c.MetricStore().Read(metrix.ReadFlatten()), "memory_health", "memory_health")
	assert.Equal(
		t,
		sourceTestStateValues(
			map[string]string{"Shared memory": "ok"},
			[]string{"ok", "warning", "critical", "unknown"},
		),
		want,
	)
	phase.Store(1)
	sourceTestCollectCycle(t, c)
	got := sourceTestMetricByResource(t, c.MetricStore().Read(metrix.ReadFlatten()), "memory_health", "memory_health")
	assert.Equal(
		t,
		want,
		got,
		"a failed first membership read must not hide the successful shared memory read through the chassis",
	)
}

func TestDecodedCollectorMergesSensorExcerptWhenReadingIsAbsent(t *testing.T) {
	const b = "/redfish/v1/"
	for _, test := range []struct {
		name    string
		present bool
		value   any
		want    map[string]float64
	}{
		{name: "absent", want: map[string]float64{"Sensor": 43}},
		{name: "null", present: true, want: map[string]float64{}},
		{name: "zero", present: true, value: 0, want: map[string]float64{"Sensor": 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sensor := sourceTestResource(
				b+"Sensors/1",
				"Sensor",
				"Sensor",
				map[string]any{
					"ReadingType":  "Temperature",
					"ReadingUnits": "Cel",
					"Status":       map[string]any{"Health": "Warning"},
				},
			)
			if test.present {
				sensor["Reading"] = test.value
			}
			documents := map[string]map[string]any{
				b: sourceTestResource(
					b,
					"ServiceRoot",
					"Service",
					map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(b + "Chassis")},
				),
				b + "Chassis": sourceTestCollection(b+"Chassis", "Chassis", b+"Chassis/1"),
				b + "Chassis/1": sourceTestResource(
					b+"Chassis/1",
					"Chassis",
					"Chassis",
					map[string]any{
						"Sensors":          sourceTestLink(b + "Sensors"),
						"ThermalSubsystem": sourceTestLink(b + "ThermalSubsystem"),
					},
				),
				b + "Sensors":   sourceTestCollection(b+"Sensors", "Sensor", b+"Sensors/1"),
				b + "Sensors/1": sensor,
				b + "ThermalSubsystem": sourceTestResource(
					b+"ThermalSubsystem",
					"ThermalSubsystem",
					"Thermal",
					map[string]any{"ThermalMetrics": sourceTestLink(b + "ThermalMetrics")},
				),
				b + "ThermalMetrics": sourceTestResource(
					b+"ThermalMetrics",
					"ThermalMetrics",
					"Metrics",
					map[string]any{
						"TemperatureReadingsCelsius": []any{
							map[string]any{
								"Reading":       43,
								"DataSourceUri": b + "Sensors/1",
								"Status":        map[string]any{"Health": "OK"},
							},
						},
					},
				),
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				document := documents[r.URL.Path]
				if document == nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("OData-Version", "4.0")
				_ = json.NewEncoder(w).Encode(document)
			}))
			t.Cleanup(server.Close)
			collector := sourceTestDecodedCollector(t, server.URL)
			sourceTestCollectCycle(t, collector)
			reader := collector.MetricStore().Read(metrix.ReadFlatten())
			assert.Equal(t, test.want, sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_input", ""))
			assert.Equal(
				t,
				sourceTestStateValues(map[string]string{"Sensor": "warning"}, []string{"clear", "warning", "critical"}),
				sourceTestMetricByResource(
					t,
					reader,
					"system_hw_sensor_temperature_alarm",
					"system_hw_sensor_temperature_alarm",
				),
			)
		})
	}
}

func TestDecodedCollectorTraversesSharedResourceAfterEarlierUnknownVisit(t *testing.T) {
	var phase atomic.Int32
	const b = "/redfish/v1/"
	documents := map[string]map[string]any{
		b: sourceTestResource(
			b,
			"ServiceRoot",
			"Service",
			map[string]any{"RedfishVersion": "1.20.0", "Systems": sourceTestLink(b + "Systems")},
		),
		b + "Systems": sourceTestCollection(b+"Systems", "ComputerSystem", b+"Systems/1"),
		b + "Systems/1": sourceTestResource(
			b+"Systems/1",
			"ComputerSystem",
			"System",
			map[string]any{"Processors": sourceTestLink(b + "Processors"), "Storage": sourceTestLink(b + "Storage")},
		),
		b + "Processors": sourceTestCollection(b+"Processors", "Processor", b+"Processors/1"),
		b + "Processors/1": sourceTestResource(
			b+"Processors/1",
			"Processor",
			"Processor",
			map[string]any{"Ports": sourceTestLink(b + "Processors/1/Ports")},
		),
		b + "Processors/1/Ports": sourceTestCollection(b+"Processors/1/Ports", "Port", b+"Ports/1"),
		b + "Storage":            sourceTestCollection(b+"Storage", "Storage", b+"Storage/1"),
		b + "Storage/1": sourceTestResource(
			b+"Storage/1",
			"Storage",
			"Storage",
			map[string]any{"Controllers": sourceTestLink(b + "Controllers")},
		),
		b + "Controllers": sourceTestCollection(b+"Controllers", "StorageController", b+"Controllers/1"),
		b + "Controllers/1": sourceTestResource(
			b+"Controllers/1",
			"StorageController",
			"Controller",
			map[string]any{"Ports": sourceTestLink(b + "Controllers/1/Ports")},
		),
		b + "Controllers/1/Ports": sourceTestCollection(b+"Controllers/1/Ports", "Port", b+"Ports/1"),
		b + "Ports/1": sourceTestResource(
			b+"Ports/1",
			"Port",
			"Shared port",
			map[string]any{
				"CurrentSpeedGbps":   10,
				"EnvironmentMetrics": sourceTestLink(b + "Ports/1/EnvironmentMetrics"),
			},
		),
	}
	var metricsRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if phase.Load() == 1 && r.URL.Path == b+"Processors/1/Ports" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		document := documents[r.URL.Path]
		if r.URL.Path == b+"Ports/1/EnvironmentMetrics" {
			metricsRequests.Add(1)
			document = sourceTestResource(
				r.URL.Path,
				"EnvironmentMetrics",
				"Environment",
				map[string]any{"TemperatureCelsius": map[string]any{"Reading": 30 + phase.Load()}},
			)
		}
		if document == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("OData-Version", "4.0")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(server.Close)
	collector := sourceTestDecodedCollector(t, server.URL)
	for cycle := range 2 {
		phase.Store(int32(cycle))
		metricsRequests.Store(0)
		sourceTestCollectCycle(t, collector)
		reader := collector.MetricStore().Read(metrix.ReadFlatten())
		assert.Equal(
			t,
			map[string]float64{"Shared port": 10_000_000_000},
			sourceTestMetricByResource(t, reader, "port_link_speed_speed", ""),
		)
		assert.Equal(
			t,
			map[string]float64{"Shared port": float64(30 + cycle)},
			sourceTestMetricByResource(t, reader, "reading_temperature_value", ""),
		)
		assert.Equal(
			t,
			int32(1),
			metricsRequests.Load(),
			"enrichment is fetched once per cycle, even when a shared node is promoted",
		)
	}
}

func sourceTestServeDocuments(t *testing.T, docs map[string]map[string]any) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := docs[r.URL.Path]
		if doc == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("OData-Version", "4.0")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestDecodedCollectorRejectsWrongDescendantSchema(t *testing.T) {
	const b = "/redfish/v1/"
	docs := map[string]map[string]any{
		b: sourceTestResource(
			b,
			"ServiceRoot",
			"Service",
			map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(b + "Chassis")},
		),
		b + "Chassis": sourceTestCollection(b+"Chassis", "Chassis", b+"Chassis/1"),
		b + "Chassis/1": sourceTestResource(
			b+"Chassis/1",
			"Chassis",
			"Chassis",
			map[string]any{"Sensors": sourceTestLink(b + "Sensors")},
		),
		b + "Sensors": sourceTestCollection(b+"Sensors", "Sensor", b+"Sensors/1", b+"Sensors/2"),
		b + "Sensors/2": sourceTestResource(
			b+"Sensors/2",
			"Sensor",
			"Valid sensor",
			map[string]any{
				"ReadingType":  "Temperature",
				"ReadingUnits": "Cel",
				"Reading":      17,
				"Status":       map[string]any{"Health": "OK"},
			},
		),
		b + "Sensors/1": sourceTestResource(
			b+"Sensors/1",
			"Memory",
			"Wrong schema",
			map[string]any{
				"ReadingType":  "Temperature",
				"ReadingUnits": "Cel",
				"Reading":      42,
				"Status":       map[string]any{"Health": "OK"},
			},
		),
	}
	c := sourceTestDecodedCollector(t, sourceTestServeDocuments(t, docs))
	sourceTestCollectCycle(t, c)
	got := sourceTestMetricByResource(
		t,
		c.MetricStore().Read(metrix.ReadFlatten()),
		"system_hw_sensor_temperature_input",
		"",
	)
	assert.Equal(t, map[string]float64{"Valid sensor": 17}, got, "reject wrong schema and retain valid siblings")
}

func TestDecodedCollectorSensorExcerptIdentityIsOrderIndependent(t *testing.T) {
	const b = "/redfish/v1/"
	for name, test := range map[string]struct {
		present bool
		value   any
		want    map[string]float64
	}{
		"value":  {present: true, value: 42, want: map[string]float64{"Sensor": 42}},
		"zero":   {present: true, value: 0, want: map[string]float64{"Sensor": 0}},
		"null":   {present: true, want: map[string]float64{}},
		"absent": {want: map[string]float64{"Sensor": 43}},
	} {
		for _, first := range []string{"A", "B"} {
			t.Run(name+"/"+first+"_first", func(t *testing.T) {
				order := []string{b + "Chassis/A", b + "Chassis/B"}
				if first == "B" {
					order[0], order[1] = order[1], order[0]
				}
				sensor := sourceTestResource(
					b+"Chassis/B/Sensors/1",
					"Sensor",
					"Sensor",
					map[string]any{
						"ReadingType":  "Percent",
						"ReadingUnits": "%",
						"Status":       map[string]any{"Health": "Warning"},
					},
				)
				if test.present {
					sensor["Reading"] = test.value
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
						"Chassis A",
						map[string]any{"EnvironmentMetrics": sourceTestLink(b + "Chassis/A/EnvironmentMetrics")},
					),
					b + "Chassis/B": sourceTestResource(
						b+"Chassis/B",
						"Chassis",
						"Chassis B",
						map[string]any{"Sensors": sourceTestLink(b + "Chassis/B/Sensors")},
					),
					b + "Chassis/A/EnvironmentMetrics": sourceTestResource(
						b+"Chassis/A/EnvironmentMetrics",
						"EnvironmentMetrics",
						"Environment",
						map[string]any{
							"FanSpeedsPercent": []any{
								map[string]any{
									"MemberId":      "fan",
									"Name":          "Excerpt",
									"Reading":       43,
									"DataSourceUri": b + "Chassis/B/Sensors/1#/Reading",
									"Status":        map[string]any{"Health": "OK"},
								},
							},
						},
					),
					b + "Chassis/B/Sensors": sourceTestCollection(
						b+"Chassis/B/Sensors",
						"Sensor",
						b+"Chassis/B/Sensors/1",
					),
					b + "Chassis/B/Sensors/1": sensor,
				}
				var phase atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if phase.Load() == 1 &&
						(r.URL.Path == b+"Chassis/A/EnvironmentMetrics" || r.URL.Path == b+"Chassis/B/Sensors/1") {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					doc := docs[r.URL.Path]
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
				sourceTestCollectCycle(t, c)
				reader := c.MetricStore().Read(metrix.ReadFlatten())
				assert.Equal(t, test.want, sourceTestMetricByResource(t, reader, "reading_percentage_value", ""))
				assert.Equal(
					t,
					sourceTestStateValues(
						map[string]string{"Sensor": "warning"},
						[]string{"clear", "warning", "critical"},
					),
					sourceTestMetricByResource(t, reader, "reading_alarm", "reading_alarm"),
				)
				sensorKeys := func(reader metrix.Reader) map[string]bool {
					keys := make(map[string]bool)
					reader.ForEachByName(
						"sensor_acquisition_state",
						func(labels metrix.LabelView, _ metrix.SampleValue) {
							key, present := labels.Get("resource_key")
							require.True(t, present)
							keys[key] = true
						},
					)
					return keys
				}
				keys := sensorKeys(reader)
				require.Len(t, keys, 1)
				phase.Store(1)
				sourceTestCollectCycle(t, c)
				reader = c.MetricStore().Read(metrix.ReadFlatten())
				assert.Empty(
					t,
					sourceTestMetricByResource(t, reader, "reading_percentage_value", ""),
					"failed reads cannot replay retained samples",
				)
				assert.Equal(t, keys, sensorKeys(reader), "failed reads retain one canonical sensor identity")
			})
		}
	}
}

func TestDecodedCollectorReadsLargeDriveFixture(t *testing.T) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/Fixture-1"
	drive := loadFixture(t, "telegraf-hpe-modern-drive.min.json")
	driveURI := drive["@odata.id"].(string)
	endpoint := sourceTestServeDocuments(t, map[string]map[string]any{
		root: sourceTestResource(
			root,
			"ServiceRoot",
			"Root",
			map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(root + "Chassis")},
		),
		root + "Chassis": sourceTestCollection(root+"Chassis", "Chassis", chassis),
		chassis: sourceTestResource(
			chassis,
			"Chassis",
			"Chassis",
			map[string]any{"Drives": []any{sourceTestLink(driveURI)}},
		),
		driveURI: drive,
	})
	collector := sourceTestDecodedCollector(t, endpoint)
	sourceTestCollectCycle(t, collector)
	// CapacityBytes is 1.6 TB in this vendor-derived fixture. A typed SDK int
	// overflows on 32-bit builds before any of these useful readings are decoded.
	assert.Equal(
		t,
		map[string]float64{"Synthetic SSD": 98},
		sourceTestMetricByResource(t, collector.MetricStore().Read(), "drive_media_life", ""),
	)
	assert.Equal(
		t,
		map[string]float64{"Synthetic SSD": 12e9},
		sourceTestMetricByResource(t, collector.MetricStore().Read(), "drive_link_speed_negotiated", ""),
	)
}
