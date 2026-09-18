// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardwareInventoryThroughCollection(t *testing.T) {
	const root = "/redfish/v1/"
	const system = root + "Systems/1"
	const storage = system + "/Storage/1"
	const cpu = system + "/Processors/1"
	const memory = system + "/Memory/1"
	const drive = storage + "/Drives/1"
	const volume = storage + "/Volumes/1"
	const firmware = root + "UpdateService/FirmwareInventory/1"
	const software = root + "UpdateService/SoftwareInventory/1"
	const chassis = root + "Chassis/1"
	const assembly = chassis + "/Assembly"
	const board = assembly + "#/Assemblies/0"
	const date = "2025-03-04T12:30:00+02:00"
	resource := testutil.Resource
	link := testutil.Link
	collection := testutil.Collection
	documents := map[string]map[string]any{
		root: resource(root, "ServiceRoot", "Root", map[string]any{
			"RedfishVersion": "1.20.0", "Systems": link(root + "Systems"),
			"Chassis": link(root + "Chassis"), "UpdateService": link(root + "UpdateService"),
		}),
		root + "Systems": collection(root+"Systems", "ComputerSystem", system),
		system: resource(system, "ComputerSystem", "Server", map[string]any{
			"Processors": link(system + "/Processors"), "Memory": link(system + "/Memory"),
			"Storage": link(system + "/Storage"), "BiosVersion": "BIOS-2",
		}),
		system + "/Processors": collection(system+"/Processors", "Processor", cpu),
		cpu: resource(cpu, "Processor", "CPU", map[string]any{
			"TotalCores": 32, "TotalEnabledCores": 0, "TotalThreads": 64,
		}),
		system + "/Memory": collection(system+"/Memory", "Memory", memory),
		memory: resource(memory, "Memory", "DIMM", map[string]any{
			"CapacityMiB": 32768, "MemoryDeviceType": "DDR5",
		}),
		system + "/Storage": collection(system+"/Storage", "Storage", storage),
		storage: resource(storage, "Storage", "Storage", map[string]any{
			"Drives": []any{link(drive)}, "Volumes": link(storage + "/Volumes"),
		}),
		drive: resource(drive, "Drive", "Drive", map[string]any{
			"CapacityBytes": int64(1920383410176), "MediaType": "SSD", "Protocol": "NVMe", "FirmwareVersion": "D1",
		}),
		storage + "/Volumes": collection(storage+"/Volumes", "Volume", volume),
		volume: resource(volume, "Volume", "Volume", map[string]any{
			"CapacityBytes": int64(3840766820352), "RAIDType": "RAID1",
		}),
		root + "UpdateService": resource(root+"UpdateService", "UpdateService", "Updates", map[string]any{
			"FirmwareInventory": link(root + "UpdateService/FirmwareInventory"),
			"SoftwareInventory": link(root + "UpdateService/SoftwareInventory"),
		}),
		root + "UpdateService/FirmwareInventory": collection(
			root+"UpdateService/FirmwareInventory",
			"SoftwareInventory",
			firmware,
		),
		firmware: resource(firmware, "SoftwareInventory", "Firmware", map[string]any{
			"Version": "F1", "ReleaseDate": date,
		}),
		root + "UpdateService/SoftwareInventory": collection(
			root+"UpdateService/SoftwareInventory",
			"SoftwareInventory",
			software,
		),
		software: resource(software, "SoftwareInventory", "Software", map[string]any{
			"Version": "S1", "ReleaseDate": date,
		}),
		root + "Chassis": collection(root+"Chassis", "Chassis", chassis),
		chassis: resource(chassis, "Chassis", "Chassis", map[string]any{
			"Assembly": link(assembly), "CapacityBytes": 123, "TotalCores": 99,
		}),
		assembly: resource(assembly, "Assembly", "Assembly", map[string]any{
			"Assemblies": []any{map[string]any{
				"@odata.id": board, "MemberId": "0", "Name": "Board", "Version": "HW-3", "EngineeringChangeLevel": "EC-4",
			}},
		}),
	}
	var phase atomic.Int64
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		if phase.Load() == 1 && r.URL.Path == drive {
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
			return
		}
		doc, ok := documents[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		// Only mutate a request-local copy; the fixture remains immutable under concurrent GETs.
		if phase.Load() == 1 {
			doc = maps.Clone(doc)
			switch r.URL.Path {
			case cpu:
				doc["TotalCores"], doc["TotalEnabledCores"], doc["TotalThreads"] = 48, 40, 96
			case memory:
				doc["CapacityMiB"], doc["MemoryDeviceType"] = nil, nil
			case firmware:
				doc["ReleaseDate"] = "not a date"
			}
		}
		testutil.WriteJSON(w, doc)
	}))
	t.Cleanup(server.Close)
	collector := newFunctionCollector(t, server.URL, "inventory")
	handler := collectorapi.DefaultRegistry["redfish"].MethodHandler(functionTestJob{collector})
	collectFunctionCycle(t, collector)
	response := handler.Handle(t.Context(), "hardware", nil)
	rows := functionRows(t, response)
	columns := []string{
		"Processor cores", "Enabled cores", "Supported threads", "Capacity", "Memory type", "Media type",
		"Drive protocol", "RAID type", "Release date", "Hardware version", "Engineering revision", "Firmware",
	}
	assertDetails := func(rows []map[string]any, uri string, reported map[string]any) {
		t.Helper()
		row := functionRowByURI(t, rows, uri)
		want, got := make(map[string]any), make(map[string]any)
		for _, column := range columns {
			want[column], got[column] = reported[column], row[column]
		}
		assert.Equal(t, want, got, uri)
	}
	released, err := time.Parse(time.RFC3339, date)
	require.NoError(t, err)
	assertDetails(
		rows,
		cpu,
		map[string]any{"Processor cores": float64(32), "Enabled cores": float64(0), "Supported threads": float64(64)},
	)
	assertDetails(rows, memory, map[string]any{"Capacity": float64(34359738368), "Memory type": "DDR5"})
	assertDetails(
		rows,
		drive,
		map[string]any{
			"Capacity":       float64(1920383410176),
			"Media type":     "SSD",
			"Drive protocol": "NVMe",
			"Firmware":       "D1",
		},
	)
	assertDetails(rows, volume, map[string]any{"Capacity": float64(3840766820352), "RAID type": "RAID1"})
	assertDetails(rows, firmware, map[string]any{"Firmware": "F1", "Release date": released.UnixMilli()})
	assertDetails(rows, software, map[string]any{"Firmware": "S1", "Release date": released.UnixMilli()})
	assertDetails(rows, "", map[string]any{"Hardware version": "HW-3", "Engineering revision": "EC-4"})
	assert.Equal(t, "Board", functionRowByURI(t, rows, "")["Component"])
	assertDetails(rows, chassis, nil)
	assertDetails(rows, system, map[string]any{"Firmware": "BIOS-2"})
	for _, column := range columns {
		assert.Equal(t, false, response.Columns[column].(map[string]any)["visible"], column)
	}
	assert.Equal(t, "bytes", response.Columns["Capacity"].(map[string]any)["units"])
	assert.Equal(
		t,
		"count",
		response.Columns["Capacity"].(map[string]any)["summary"],
		"do not sum physical and logical capacity",
	)

	wantRequests := make(map[string]int)
	for path := range documents {
		wantRequests["GET "+path] = 1
	}
	// Initial connection reads the service root before graph acquisition.
	wantRequests["GET "+root] = 2
	mu.Lock()
	assert.Equal(t, wantRequests, requests, "inventory uses only existing resource requests")
	mu.Unlock()
	before, err := json.Marshal(response)
	require.NoError(t, err)
	for range 3 {
		handler.Handle(t.Context(), "hardware", nil)
	}
	mu.Lock()
	assert.Equal(t, wantRequests, requests, "Function calls do not query the BMC")
	mu.Unlock()

	phase.Store(1)
	collectFunctionCycle(t, collector)
	updated := handler.Handle(t.Context(), "hardware", nil)
	assert.Contains(t, updated.Help, "Partial")
	newRows := functionRows(t, updated)
	assertDetails(
		newRows,
		cpu,
		map[string]any{"Processor cores": float64(48), "Enabled cores": float64(40), "Supported threads": float64(96)},
	)
	assertDetails(newRows, memory, nil)
	assertDetails(newRows, drive, nil)
	assert.Equal(t, "Unknown", functionRowByURI(t, newRows, drive)["Data availability"])
	assertDetails(newRows, firmware, map[string]any{"Firmware": "F1"})
	after, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Equal(t, before, after, "completed responses retain their own copied inventory")
}
