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
			"Status": map[string]any{"Health": "OK"}, "Storage": sourceTestLink(base + "Storage"), "Memory": sourceTestLink(base + "Memory"),
		}),
		base + "Systems/B": sourceTestResource(base+"Systems/B", "ComputerSystem", "System B", map[string]any{
			"Status": map[string]any{"Health": "Warning"}, "EthernetInterfaces": sourceTestLink(base + "EthernetInterfaces"),
		}),
		base + "Memory":  sourceTestCollection(base+"Memory", "Memory"),
		base + "Storage": sourceTestCollection(base+"Storage", "Storage", base+"Storage/1"),
		base + "Storage/1": sourceTestResource(base+"Storage/1", "Storage", "Storage", map[string]any{
			"Controllers": sourceTestLink(base + "Controllers"), "Volumes": sourceTestLink(base + "Volumes"),
		}),
		base + "Controllers": sourceTestCollection(base+"Controllers", "StorageController", base+"Controllers/1"),
		base + "Controllers/1": sourceTestResource(base+"Controllers/1", "StorageController", "Controller", map[string]any{
			"PCIeInterface": map[string]any{"LanesInUse": 8}, "Status": map[string]any{"Health": "Warning"},
		}),
		base + "Volumes":              sourceTestCollection(base+"Volumes", "Volume", base+"Volumes/1"),
		base + "Volumes/1":            sourceTestResource(base+"Volumes/1", "Volume", "Volume", map[string]any{"RemainingCapacityPercent": 37}),
		base + "EthernetInterfaces":   sourceTestCollection(base+"EthernetInterfaces", "EthernetInterface", base+"EthernetInterfaces/1"),
		base + "EthernetInterfaces/1": sourceTestResource(base+"EthernetInterfaces/1", "EthernetInterface", "NIC", map[string]any{"SpeedMbps": 2500}),
		base + "Chassis/1": sourceTestResource(base+"Chassis/1", "Chassis", "Chassis", map[string]any{
			"PowerSubsystem": sourceTestLink(base + "PowerSubsystem"), "ThermalSubsystem": sourceTestLink(base + "ThermalSubsystem"), "Sensors": sourceTestLink(base + "Sensors"),
		}),
		base + "PowerSubsystem": sourceTestResource(base+"PowerSubsystem", "PowerSubsystem", "Power", map[string]any{"Batteries": sourceTestLink(base + "Batteries")}),
		base + "Batteries":      sourceTestCollection(base+"Batteries", "Battery", base+"Batteries/1"),
		base + "Batteries/1": sourceTestResource(base+"Batteries/1", "Battery", "Battery", map[string]any{
			"CapacityActualWattHours": 480, "Metrics": sourceTestLink(base + "Batteries/1/Metrics"),
		}),
		base + "Batteries/1/Metrics": sourceTestResource(base+"Batteries/1/Metrics", "BatteryMetrics", "Battery metrics", map[string]any{
			"ChargePercent":         map[string]any{"Reading": 0, "Status": map[string]any{"Health": "Warning"}},
			"StoredEnergyWattHours": map[string]any{"Reading": 0},
		}),
		base + "ThermalSubsystem": sourceTestResource(base+"ThermalSubsystem", "ThermalSubsystem", "Thermal", map[string]any{"ThermalMetrics": sourceTestLink(base + "ThermalMetrics")}),
		base + "ThermalMetrics": sourceTestResource(base+"ThermalMetrics", "ThermalMetrics", "Thermal metrics", map[string]any{
			"TemperatureReadingsCelsius": []any{map[string]any{"MemberId": "intake", "Name": "Intake", "Reading": 21.5, "Status": map[string]any{"Health": "OK"}}},
		}),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := phase.Load()
		if current == 1 && (r.URL.Path == base+"Memory" || r.URL.Path == base+"Sensors/Old" || r.URL.Path == base+"ThermalMetrics") {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		document := documents[r.URL.Path]
		switch r.URL.Path {
		case base + "ThermalMetrics":
			if current >= 2 {
				document = sourceTestResource(r.URL.Path, "ThermalMetrics", "Thermal metrics", map[string]any{"TemperatureReadingsCelsius": []any{}})
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
			document = sourceTestResource(r.URL.Path, "Sensor", "Old sensor", map[string]any{"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": 31, "Status": map[string]any{"Health": "Critical"}})
		case base + "Sensors/New":
			var value any = 42
			health := "OK"
			if current == 2 {
				value, health = nil, "Warning"
			}
			document = sourceTestResource(r.URL.Path, "Sensor", "New sensor", map[string]any{"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": value, "Status": map[string]any{"Health": health}})
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
			assert.Equal(t, []metrix.HostScope{{}}, reader.HostScopes(), "all endpoint resources use the ordinary job scope")
			assert.Equal(t, expectation.values, sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_input", ""))
			assert.Equal(t, sourceTestStateValues(expectation.alarms, []string{"clear", "warning", "critical"}), sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_alarm", "system_hw_sensor_temperature_alarm"))
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
			assert.Equal(t, sourceTestStateValues(map[string]string{"System A": "ok", "System B": "warning"}, []string{"ok", "warning", "critical", "unknown"}), sourceTestMetricByResource(t, reader, "system_health", "system_health"))
			assert.Equal(t, sourceTestStateValues(map[string]string{"Controller": "warning"}, []string{"ok", "warning", "critical", "unknown"}), sourceTestMetricByResource(t, reader, "storage_controller_health", "storage_controller_health"))
			assert.Equal(t, sourceTestStateValues(map[string]string{"Battery": "warning"}, []string{"clear", "warning", "critical"}), sourceTestMetricByResource(t, reader, "reading_alarm", "reading_alarm"))
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
			collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{RequiredContexts: contexts})
		})
	}
	t.Run("first collection with partial branches", func(t *testing.T) {
		phase.Store(1)
		fresh := sourceTestDecodedCollector(t, server.URL)
		sourceTestCollectCycle(t, fresh)
		reader := fresh.MetricStore().Read(metrix.ReadFlatten())
		assert.Equal(t, map[string]float64{"New sensor": 42}, sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_input", ""))
		assert.Equal(t, sourceTestStateValues(map[string]string{"New sensor": "clear"}, []string{"clear", "warning", "critical"}), sourceTestMetricByResource(t, reader, "system_hw_sensor_temperature_alarm", "system_hw_sensor_temperature_alarm"))
		assert.Equal(t, map[string]float64{"Controller": 8}, sourceTestMetricByResource(t, reader, "storage_controller_pcie_lanes_active", ""))
		collecttest.AssertChartCoverage(t, fresh, collecttest.ChartCoverageExpectation{RequiredContexts: map[string][]string{"system.hw.sensor.temperature.input": {"input"}}})
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
			document = sourceTestResource(base, "ServiceRoot", "Service", map[string]any{"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(base + "Chassis")})
		case base + "Chassis":
			document = sourceTestCollection(r.URL.Path, "Chassis", base+"Chassis/1")
		case base + "Chassis/1":
			document = sourceTestResource(r.URL.Path, "Chassis", "Chassis", map[string]any{"Sensors": sourceTestLink(base + "Sensors")})
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
			document = sourceTestResource(r.URL.Path, "Sensor", fmt.Sprintf("Sensor %d", i), map[string]any{"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": i, "Status": map[string]any{"Health": "OK"}})
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
	assert.Equal(t, expected, sourceTestMetricByResource(t, collector.MetricStore().Read(), "system_hw_sensor_temperature_input", ""))
	collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{RequiredContexts: map[string][]string{"system.hw.sensor.temperature.input": {"input"}}})
}

func sourceTestDecodedCollector(t *testing.T, endpoint string) collectorapi.CollectorV2 {
	t.Helper()
	creator, ok := collectorapi.DefaultRegistry.Lookup("redfish")
	require.True(t, ok)
	collector := creator.CreateV2()
	payload, err := json.Marshal(map[string]any{"name": "source-contract", "url": endpoint, "auth_method": "none", "retries": 0})
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
	document := map[string]any{"@odata.id": uri, "@odata.type": "#" + schema + ".v1_0_0." + schema, "Id": name, "Name": name}
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
	return map[string]any{"@odata.id": uri, "@odata.type": "#" + schema + "Collection." + schema + "Collection", "Members@odata.count": len(values), "Members": values, "Name": strings.TrimPrefix(uri, "/redfish/v1/")}
}
