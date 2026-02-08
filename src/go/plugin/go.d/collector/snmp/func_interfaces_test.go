// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFuncInterfaces creates a funcInterfaces for testing with the given cache.
func newTestFuncInterfaces(cache *ifaceCache) *funcInterfaces {
	r := &funcRouter{ifaceCache: cache, topologyCache: nil}
	return newFuncInterfaces(r)
}

func TestSnmpMethods(t *testing.T) {
	methods := snmpMethods()

	require.Len(t, methods, 2)
	assert.Equal(t, "interfaces", methods[0].ID)
	assert.Equal(t, "Network Interfaces", methods[0].Name)
	require.NotEmpty(t, methods[0].RequiredParams)

	assert.Equal(t, "topology:snmp", methods[1].ID)
	assert.Equal(t, "Topology (SNMP)", methods[1].Name)

	// Verify type group param exists
	var typeGroupParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "if_type_group" {
			typeGroupParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(t, typeGroupParam, "expected if_type_group required param")
	require.NotEmpty(t, typeGroupParam.Options)

	// Verify default type group option exists
	hasDefault := false
	for _, opt := range typeGroupParam.Options {
		if opt.Default {
			hasDefault = true
			assert.Equal(t, "ethernet", opt.ID)
			break
		}
	}
	assert.True(t, hasDefault, "should have a default type group option")
}

func TestFuncIfacesColumns(t *testing.T) {
	tests := map[string]struct {
		validate func(t *testing.T)
	}{
		"has required columns": {
			validate: func(t *testing.T) {
				requiredKeys := []string{
					"Interface", "Type", "Type Group",
					"Admin Status", "Oper Status",
					"Traffic In", "Traffic Out",
					"Unicast In", "Unicast Out",
					"Broadcast In", "Broadcast Out",
					"Packets In", "Packets Out",
					"Errors In", "Errors Out",
					"Discards In", "Discards Out",
					"Multicast In", "Multicast Out",
				}

				cs := snmpColumnSet(snmpAllColumns)
				for _, key := range requiredKeys {
					assert.True(t, cs.ContainsColumn(key), "column %s should be defined", key)
				}
			},
		},
		"has valid metadata": {
			validate: func(t *testing.T) {
				for _, col := range snmpAllColumns {
					assert.NotEmpty(t, col.Name, "column must have ID")
					assert.NotEqual(t, funcapi.FieldTypeNone, col.Type, "column %s must have Type", col.Name)
					assert.NotNil(t, col.Value, "column %s must have Value extractor", col.Name)
				}
			},
		},
		"value extractors work correctly": {
			validate: func(t *testing.T) {
				rate := 100.0
				entry := &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					ifTypeGroup: "ethernet",
					adminStatus: "up",
					operStatus:  "up",
					rates: ifaceRates{
						trafficIn:   &rate,
						ucastPktsIn: &rate,
						errorsIn:    &rate,
						discardsIn:  &rate,
					},
				}

				// Test each column's value extractor
				for _, col := range snmpAllColumns {
					// Should not panic
					_ = col.Value(entry)
				}

				// Verify specific values
				for _, col := range snmpAllColumns {
					switch col.Name {
					case "Interface":
						assert.Equal(t, "eth0", col.Value(entry))
					case "Type":
						assert.Equal(t, "ethernetCsmacd", col.Value(entry))
					case "Type Group":
						assert.Equal(t, "ethernet", col.Value(entry))
					case "Traffic In":
						assert.Equal(t, rate, col.Value(entry))
					case "Packets In":
						assert.Equal(t, rate, col.Value(entry))
					case "Errors In":
						assert.Equal(t, rate, col.Value(entry))
					case "Discards In":
						assert.Equal(t, rate, col.Value(entry))
					case "Admin Status":
						assert.Equal(t, "up", col.Value(entry))
					}
				}
			},
		},
		"column count matches row length": {
			validate: func(t *testing.T) {
				entry := &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					ifTypeGroup: "ethernet",
					adminStatus: "up",
					operStatus:  "up",
				}
				f := &funcInterfaces{}
				row := f.buildRow(entry)
				assert.Len(t, row, len(snmpAllColumns)+1, "row length must match column count")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.validate(t)
		})
	}
}

func TestFuncInterfaces_buildColumns(t *testing.T) {
	f := &funcInterfaces{}
	cs := snmpColumnSet(snmpAllColumns)
	columns := f.buildColumns(cs)

	require.NotEmpty(t, columns)
	assert.Len(t, columns, len(snmpAllColumns)+1)

	// Verify all columns are present
	for _, col := range snmpAllColumns {
		colDef, ok := columns[col.Name]
		assert.True(t, ok, "column %s should be in result", col.Name)
		assert.NotNil(t, colDef)

		// Verify column is a map with expected fields
		colMap, ok := colDef.(map[string]any)
		require.True(t, ok, "column %s should be a map", col.Name)
		assert.Equal(t, col.Tooltip, colMap["name"])
	}

	rowOptions, ok := columns["rowOptions"]
	require.True(t, ok, "rowOptions column should be in result")
	rowOptionsMap, ok := rowOptions.(map[string]any)
	require.True(t, ok, "rowOptions column should be a map")
	assert.Equal(t, "rowOptions", rowOptionsMap["name"])
	assert.Equal(t, "none", rowOptionsMap["type"])
	assert.Equal(t, "rowOptions", rowOptionsMap["visualization"])

}

func TestFuncInterfaces_buildRow(t *testing.T) {
	rate1 := 1000.5
	rate2 := 2000.5

	tests := map[string]struct {
		entry    *ifaceEntry
		validate func(t *testing.T, row []any)
	}{
		"all fields populated": {
			entry: &ifaceEntry{
				name:        "eth0",
				ifType:      "ethernetCsmacd",
				ifTypeGroup: "ethernet",
				adminStatus: "up",
				operStatus:  "up",
				rates: ifaceRates{
					trafficIn:    &rate1,
					trafficOut:   &rate2,
					ucastPktsIn:  &rate1,
					ucastPktsOut: &rate2,
					bcastPktsIn:  &rate1,
					bcastPktsOut: &rate2,
					errorsIn:     &rate1,
					errorsOut:    &rate2,
					discardsIn:   &rate1,
					discardsOut:  &rate2,
					mcastPktsIn:  &rate1,
					mcastPktsOut: &rate2,
				},
			},
			validate: func(t *testing.T, row []any) {
				// Find column indices by key
				nameIdx := findColIdx("Interface")
				typeIdx := findColIdx("Type")
				typeGroupIdx := findColIdx("Type Group")
				trafficInIdx := findColIdx("Traffic In")
				trafficOutIdx := findColIdx("Traffic Out")
				packetsInIdx := findColIdx("Packets In")
				packetsOutIdx := findColIdx("Packets Out")
				adminIdx := findColIdx("Admin Status")
				operIdx := findColIdx("Oper Status")
				rowOptionsIdx := len(snmpAllColumns)

				assert.Equal(t, "eth0", row[nameIdx])
				assert.Equal(t, "ethernetCsmacd", row[typeIdx])
				assert.Equal(t, "ethernet", row[typeGroupIdx])
				assert.Equal(t, rate1, row[trafficInIdx])
				assert.Equal(t, rate2, row[trafficOutIdx])
				assert.Equal(t, rate1*3, row[packetsInIdx])
				assert.Equal(t, rate2*3, row[packetsOutIdx])
				assert.Equal(t, "up", row[adminIdx])
				assert.Equal(t, "up", row[operIdx])
				assert.Nil(t, row[rowOptionsIdx])
			},
		},
		"nil rates produce nil values": {
			entry: &ifaceEntry{
				name:        "eth1",
				ifType:      "other",
				ifTypeGroup: "other",
				adminStatus: "down",
				operStatus:  "down",
				rates:       ifaceRates{}, // all nil
			},
			validate: func(t *testing.T, row []any) {
				nameIdx := findColIdx("Interface")
				typeIdx := findColIdx("Type")
				trafficInIdx := findColIdx("Traffic In")
				trafficOutIdx := findColIdx("Traffic Out")
				adminIdx := findColIdx("Admin Status")
				operIdx := findColIdx("Oper Status")
				rowOptionsIdx := len(snmpAllColumns)

				assert.Equal(t, "eth1", row[nameIdx])
				assert.Equal(t, "other", row[typeIdx])
				assert.Nil(t, row[trafficInIdx])
				assert.Nil(t, row[trafficOutIdx])
				assert.Equal(t, "down", row[adminIdx])
				assert.Equal(t, "down", row[operIdx])
				assert.Nil(t, row[rowOptionsIdx])
			},
		},
		"down interface hides metrics": {
			entry: &ifaceEntry{
				name:        "eth3",
				ifType:      "ethernetCsmacd",
				ifTypeGroup: "ethernet",
				adminStatus: "up",
				operStatus:  "down",
				rates: ifaceRates{
					trafficIn:  &rate1,
					trafficOut: &rate2,
					errorsIn:   &rate1,
				},
			},
			validate: func(t *testing.T, row []any) {
				trafficInIdx := findColIdx("Traffic In")
				trafficOutIdx := findColIdx("Traffic Out")
				errorsInIdx := findColIdx("Errors In")
				rowOptionsIdx := len(snmpAllColumns)

				assert.Nil(t, row[trafficInIdx])
				assert.Nil(t, row[trafficOutIdx])
				assert.Nil(t, row[errorsInIdx])
				assert.Nil(t, row[rowOptionsIdx])
			},
		},
		"partial rates": {
			entry: &ifaceEntry{
				name:        "eth2",
				ifType:      "",
				ifTypeGroup: "",
				adminStatus: "up",
				operStatus:  "unknown",
				rates: ifaceRates{
					trafficIn:  &rate1,
					trafficOut: &rate2,
					// rest nil
				},
			},
			validate: func(t *testing.T, row []any) {
				nameIdx := findColIdx("Interface")
				trafficInIdx := findColIdx("Traffic In")
				trafficOutIdx := findColIdx("Traffic Out")
				ucastInIdx := findColIdx("Unicast In")
				rowOptionsIdx := len(snmpAllColumns)

				assert.Equal(t, "eth2", row[nameIdx])
				assert.Nil(t, row[trafficInIdx])
				assert.Nil(t, row[trafficOutIdx])
				assert.Nil(t, row[ucastInIdx])
				assert.Nil(t, row[rowOptionsIdx])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &funcInterfaces{}
			row := f.buildRow(tc.entry)
			require.Len(t, row, len(snmpAllColumns)+1)
			tc.validate(t, row)
		})
	}
}

func TestFuncInterfaces_sortData(t *testing.T) {
	rate100 := 100.0
	rate200 := 200.0
	rate300 := 300.0

	// Helper to build a test row with name and trafficIn
	buildTestRow := func(name string, trafficIn *float64) []any {
		row := make([]any, len(snmpAllColumns)+1)
		for i, col := range snmpAllColumns {
			switch col.Name {
			case "Interface":
				row[i] = name
			case "Traffic In":
				row[i] = ptrToAny(trafficIn)
			case "Traffic Out":
				row[i] = ptrToAny(trafficIn) // reuse for simplicity
			default:
				if col.Type == funcapi.FieldTypeString {
					row[i] = "test"
				} else {
					row[i] = nil
				}
			}
		}
		row[len(snmpAllColumns)] = nil
		return row
	}

	tests := map[string]struct {
		data       [][]any
		sortColumn string
		expected   []string // expected order of names
	}{
		"sort by name ascending": {
			data: [][]any{
				buildTestRow("eth2", nil),
				buildTestRow("eth0", nil),
				buildTestRow("eth1", nil),
			},
			sortColumn: "Interface",
			expected:   []string{"eth0", "eth1", "eth2"},
		},
		"sort by trafficIn descending": {
			data: [][]any{
				buildTestRow("eth0", &rate100),
				buildTestRow("eth1", &rate300),
				buildTestRow("eth2", &rate200),
			},
			sortColumn: "Traffic In",
			expected:   []string{"eth1", "eth2", "eth0"},
		},
		"nil values go to end": {
			data: [][]any{
				buildTestRow("eth0", nil),
				buildTestRow("eth1", &rate300),
				buildTestRow("eth2", &rate100),
			},
			sortColumn: "Traffic In",
			expected:   []string{"eth1", "eth2", "eth0"},
		},
		"unknown column defaults to name": {
			data: [][]any{
				buildTestRow("eth2", nil),
				buildTestRow("eth0", nil),
				buildTestRow("eth1", nil),
			},
			sortColumn: "unknown_column",
			expected:   []string{"eth0", "eth1", "eth2"},
		},
		"empty data": {
			data:       [][]any{},
			sortColumn: "Interface",
			expected:   nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &funcInterfaces{}
			f.sortData(tc.data, tc.sortColumn)

			nameIdx := findColIdx("Interface")
			var names []string
			for _, row := range tc.data {
				names = append(names, row[nameIdx].(string))
			}
			assert.Equal(t, tc.expected, names)
		})
	}
}

func TestFuncInterfaces_handle(t *testing.T) {
	rate100 := 100.0
	rate200 := 200.0

	tests := map[string]struct {
		setup    func() *funcInterfaces
		method   string
		params   funcapi.ResolvedParams
		validate func(t *testing.T, resp *funcapi.FunctionResponse)
	}{
		"unknown method returns 404": {
			setup: func() *funcInterfaces {
				return newTestFuncInterfaces(newIfaceCache())
			},
			method: "unknown",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 404, resp.Status)
				assert.Contains(t, resp.Message, "unknown method")
			},
		},
		"nil cache returns 503": {
			setup: func() *funcInterfaces {
				return newTestFuncInterfaces(nil)
			},
			method: "interfaces",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 503, resp.Status)
				assert.Contains(t, resp.Message, "not available")
			},
		},
		"empty cache returns 200 with empty data": {
			setup: func() *funcInterfaces {
				return newTestFuncInterfaces(newIfaceCache())
			},
			method: "interfaces",
			params: resolveIfaceParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.NotNil(t, resp.Columns)
				assert.Len(t, resp.Columns, len(snmpAllColumns)+1)

				data, ok := resp.Data.([][]any)
				require.True(t, ok)
				assert.Len(t, data, 0)
			},
		},
		"cache with data returns correct rows": {
			setup: func() *funcInterfaces {
				cache := newIfaceCache()
				cache.interfaces["eth0"] = &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					ifTypeGroup: "ethernet",
					adminStatus: "up",
					operStatus:  "up",
					rates: ifaceRates{
						trafficIn:  &rate100,
						trafficOut: &rate200,
					},
				}
				cache.interfaces["eth1"] = &ifaceEntry{
					name:        "eth1",
					ifType:      "other",
					ifTypeGroup: "virtual",
					adminStatus: "down",
					operStatus:  "down",
				}
				return newTestFuncInterfaces(cache)
			},
			method: "interfaces",
			params: resolveIfaceParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "Interface", resp.DefaultSortColumn)

				data, ok := resp.Data.([][]any)
				require.True(t, ok)
				assert.Len(t, data, 1)

				// Verify Charts
				require.NotNil(t, resp.Charts)
				assert.Contains(t, resp.Charts, "Traffic")
				assert.Contains(t, resp.Charts, "UnicastPackets")
				assert.Equal(t, []string{"Traffic In", "Traffic Out"}, resp.Charts["Traffic"].Columns)

				// Verify DefaultCharts
				require.NotEmpty(t, resp.DefaultCharts)
				assert.ElementsMatch(t, funcapi.DefaultCharts{
					{Chart: "Traffic", GroupBy: "Type"},
					{Chart: "OperationalStatus", GroupBy: "Oper Status"},
				}, resp.DefaultCharts)

				// Verify GroupBy
				require.NotNil(t, resp.GroupBy)
				assert.Contains(t, resp.GroupBy, "Type")
			},
		},
		"filter other includes non-target groups": {
			setup: func() *funcInterfaces {
				cache := newIfaceCache()
				cache.interfaces["eth0"] = &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					ifTypeGroup: "ethernet",
					adminStatus: "up",
					operStatus:  "up",
				}
				cache.interfaces["lo0"] = &ifaceEntry{
					name:        "lo0",
					ifType:      "loopback",
					ifTypeGroup: "virtual",
					adminStatus: "up",
					operStatus:  "up",
				}
				cache.interfaces["vlan0"] = &ifaceEntry{
					name:        "vlan0",
					ifType:      "l2vlan",
					ifTypeGroup: "virtual",
					adminStatus: "up",
					operStatus:  "up",
				}
				cache.interfaces["tun0"] = &ifaceEntry{
					name:        "tun0",
					ifType:      "tunnel",
					ifTypeGroup: "",
					adminStatus: "up",
					operStatus:  "up",
				}
				cache.interfaces["wlan0"] = &ifaceEntry{
					name:        "wlan0",
					ifType:      "ieee80211",
					ifTypeGroup: "wireless",
					adminStatus: "up",
					operStatus:  "up",
				}
				return newTestFuncInterfaces(cache)
			},
			method: "interfaces",
			params: resolveIfaceParams(map[string][]string{"if_type_group": {"other"}}),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)

				data, ok := resp.Data.([][]any)
				require.True(t, ok)
				assert.Len(t, data, 2)

				nameIdx := findColIdx("Interface")
				names := []string{data[0][nameIdx].(string), data[1][nameIdx].(string)}
				assert.ElementsMatch(t, []string{"tun0", "wlan0"}, names)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := tc.setup()
			resp := f.Handle(context.Background(), tc.method, tc.params)
			tc.validate(t, resp)
		})
	}
}

func TestFuncInterfaces_defaultSortColumn(t *testing.T) {
	f := &funcInterfaces{}
	assert.Equal(t, "Interface", f.defaultSortColumn())
}

func TestPtrToAny(t *testing.T) {
	tests := map[string]struct {
		input    *float64
		expected any
	}{
		"nil pointer": {
			input:    nil,
			expected: nil,
		},
		"non-nil pointer": {
			input:    func() *float64 { v := 123.45; return &v }(),
			expected: 123.45,
		},
		"zero value pointer": {
			input:    func() *float64 { v := 0.0; return &v }(),
			expected: 0.0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := ptrToAny(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// findColIdx finds the index of a column by key.
func findColIdx(key string) int {
	for i, col := range snmpAllColumns {
		if col.Name == key {
			return i
		}
	}
	return -1
}

func resolveIfaceParams(values map[string][]string) funcapi.ResolvedParams {
	f := &funcInterfaces{}
	params, err := f.MethodParams(context.Background(), "interfaces")
	if err != nil {
		return nil
	}
	return funcapi.ResolveParams(params, values)
}
