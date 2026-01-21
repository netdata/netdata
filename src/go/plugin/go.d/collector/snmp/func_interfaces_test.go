// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnmpMethods(t *testing.T) {
	methods := snmpMethods()

	require.Len(t, methods, 1)
	assert.Equal(t, "interfaces", methods[0].ID)
	assert.Equal(t, "Network Interfaces", methods[0].Name)
	require.NotEmpty(t, methods[0].RequiredParams)

	// Verify sort param exists
	var sortParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "__sort" {
			sortParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(t, sortParam, "expected __sort required param")
	require.NotEmpty(t, sortParam.Options)

	// Verify default sort option exists
	hasDefault := false
	for _, opt := range sortParam.Options {
		if opt.Default {
			hasDefault = true
			assert.Equal(t, "name", opt.ID)
			break
		}
	}
	assert.True(t, hasDefault, "should have a default sort option")
}

func TestFuncIfacesColumns(t *testing.T) {
	tests := map[string]struct {
		validate func(t *testing.T)
	}{
		"has required columns": {
			validate: func(t *testing.T) {
				requiredKeys := []string{
					"name", "type",
					"trafficIn", "trafficOut",
					"ucastPktsIn", "ucastPktsOut",
					"bcastPktsIn", "bcastPktsOut",
					"mcastPktsIn", "mcastPktsOut",
					"adminStatus", "operStatus",
				}

				keys := make(map[string]bool)
				for _, col := range funcIfacesColumns {
					keys[col.key] = true
				}

				for _, key := range requiredKeys {
					assert.True(t, keys[key], "column %s should be defined", key)
				}
			},
		},
		"has valid metadata": {
			validate: func(t *testing.T) {
				for _, col := range funcIfacesColumns {
					assert.NotEmpty(t, col.key, "column must have key")
					assert.NotEmpty(t, col.name, "column %s must have name", col.key)
					assert.NotEqual(t, funcapi.FieldTypeNone, col.dataType, "column %s must have dataType", col.key)
					assert.NotNil(t, col.value, "column %s must have value extractor", col.key)

					if col.sortOption != "" {
						assert.NotEmpty(t, col.sortOption, "sort option column %s must have sortOption label", col.key)
					}
				}
			},
		},
		"value extractors work correctly": {
			validate: func(t *testing.T) {
				rate := 100.0
				entry := &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					adminStatus: "up",
					operStatus:  "up",
					rates: ifaceRates{
						trafficIn: &rate,
					},
				}

				// Test each column's value extractor
				for _, col := range funcIfacesColumns {
					// Should not panic
					_ = col.value(entry)
				}

				// Verify specific values
				for _, col := range funcIfacesColumns {
					switch col.key {
					case "name":
						assert.Equal(t, "eth0", col.value(entry))
					case "type":
						assert.Equal(t, "ethernetCsmacd", col.value(entry))
					case "trafficIn":
						assert.Equal(t, rate, col.value(entry))
					case "adminStatus":
						assert.Equal(t, "up", col.value(entry))
					}
				}
			},
		},
		"column count matches row length": {
			validate: func(t *testing.T) {
				entry := &ifaceEntry{
					name:        "eth0",
					ifType:      "ethernetCsmacd",
					adminStatus: "up",
					operStatus:  "up",
				}
				f := &funcInterfaces{}
				row := f.buildRow(entry)
				assert.Len(t, row, len(funcIfacesColumns), "row length must match column count")
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
	columns := f.buildColumns()

	require.NotEmpty(t, columns)
	assert.Len(t, columns, len(funcIfacesColumns))

	// Verify all columns are present
	for _, col := range funcIfacesColumns {
		colDef, ok := columns[col.key]
		assert.True(t, ok, "column %s should be in result", col.key)
		assert.NotNil(t, colDef)

		// Verify column is a map with expected fields
		colMap, ok := colDef.(map[string]any)
		require.True(t, ok, "column %s should be a map", col.key)
		assert.Equal(t, col.name, colMap["name"])
	}
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
				adminStatus: "up",
				operStatus:  "up",
				rates: ifaceRates{
					trafficIn:    &rate1,
					trafficOut:   &rate2,
					ucastPktsIn:  &rate1,
					ucastPktsOut: &rate2,
					bcastPktsIn:  &rate1,
					bcastPktsOut: &rate2,
					mcastPktsIn:  &rate1,
					mcastPktsOut: &rate2,
				},
			},
			validate: func(t *testing.T, row []any) {
				// Find column indices by key
				nameIdx := findColIdx("name")
				typeIdx := findColIdx("type")
				trafficInIdx := findColIdx("trafficIn")
				trafficOutIdx := findColIdx("trafficOut")
				adminIdx := findColIdx("adminStatus")
				operIdx := findColIdx("operStatus")

				assert.Equal(t, "eth0", row[nameIdx])
				assert.Equal(t, "ethernetCsmacd", row[typeIdx])
				assert.Equal(t, rate1, row[trafficInIdx])
				assert.Equal(t, rate2, row[trafficOutIdx])
				assert.Equal(t, "up", row[adminIdx])
				assert.Equal(t, "up", row[operIdx])
			},
		},
		"nil rates produce nil values": {
			entry: &ifaceEntry{
				name:        "eth1",
				ifType:      "other",
				adminStatus: "down",
				operStatus:  "down",
				rates:       ifaceRates{}, // all nil
			},
			validate: func(t *testing.T, row []any) {
				nameIdx := findColIdx("name")
				typeIdx := findColIdx("type")
				trafficInIdx := findColIdx("trafficIn")
				trafficOutIdx := findColIdx("trafficOut")
				adminIdx := findColIdx("adminStatus")
				operIdx := findColIdx("operStatus")

				assert.Equal(t, "eth1", row[nameIdx])
				assert.Equal(t, "other", row[typeIdx])
				assert.Nil(t, row[trafficInIdx])
				assert.Nil(t, row[trafficOutIdx])
				assert.Equal(t, "down", row[adminIdx])
				assert.Equal(t, "down", row[operIdx])
			},
		},
		"partial rates": {
			entry: &ifaceEntry{
				name:        "eth2",
				ifType:      "",
				adminStatus: "up",
				operStatus:  "unknown",
				rates: ifaceRates{
					trafficIn:  &rate1,
					trafficOut: &rate2,
					// rest nil
				},
			},
			validate: func(t *testing.T, row []any) {
				nameIdx := findColIdx("name")
				trafficInIdx := findColIdx("trafficIn")
				trafficOutIdx := findColIdx("trafficOut")
				ucastInIdx := findColIdx("ucastPktsIn")

				assert.Equal(t, "eth2", row[nameIdx])
				assert.Equal(t, rate1, row[trafficInIdx])
				assert.Equal(t, rate2, row[trafficOutIdx])
				assert.Nil(t, row[ucastInIdx])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &funcInterfaces{}
			row := f.buildRow(tc.entry)
			require.Len(t, row, len(funcIfacesColumns))
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
		row := make([]any, len(funcIfacesColumns))
		for i, col := range funcIfacesColumns {
			switch col.key {
			case "name":
				row[i] = name
			case "trafficIn":
				row[i] = ptrToAny(trafficIn)
			case "trafficOut":
				row[i] = ptrToAny(trafficIn) // reuse for simplicity
			default:
				if col.dataType == funcapi.FieldTypeString {
					row[i] = "test"
				} else {
					row[i] = nil
				}
			}
		}
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
			sortColumn: "name",
			expected:   []string{"eth0", "eth1", "eth2"},
		},
		"sort by trafficIn descending": {
			data: [][]any{
				buildTestRow("eth0", &rate100),
				buildTestRow("eth1", &rate300),
				buildTestRow("eth2", &rate200),
			},
			sortColumn: "trafficIn",
			expected:   []string{"eth1", "eth2", "eth0"},
		},
		"nil values go to end": {
			data: [][]any{
				buildTestRow("eth0", nil),
				buildTestRow("eth1", &rate300),
				buildTestRow("eth2", &rate100),
			},
			sortColumn: "trafficIn",
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
			sortColumn: "name",
			expected:   nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &funcInterfaces{}
			f.sortData(tc.data, tc.sortColumn)

			nameIdx := findColIdx("name")
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
		validate func(t *testing.T, resp *module.FunctionResponse)
	}{
		"unknown method returns 404": {
			setup: func() *funcInterfaces {
				return newFuncInterfaces(newIfaceCache())
			},
			method: "unknown",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *module.FunctionResponse) {
				assert.Equal(t, 404, resp.Status)
				assert.Contains(t, resp.Message, "unknown method")
			},
		},
		"nil cache returns 503": {
			setup: func() *funcInterfaces {
				return &funcInterfaces{cache: nil}
			},
			method: "interfaces",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *module.FunctionResponse) {
				assert.Equal(t, 503, resp.Status)
				assert.Contains(t, resp.Message, "not available")
			},
		},
		"empty cache returns 200 with empty data": {
			setup: func() *funcInterfaces {
				return newFuncInterfaces(newIfaceCache())
			},
			method: "interfaces",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *module.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.NotNil(t, resp.Columns)
				assert.Len(t, resp.Columns, len(funcIfacesColumns))

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
					adminStatus: "down",
					operStatus:  "down",
				}
				return newFuncInterfaces(cache)
			},
			method: "interfaces",
			params: funcapi.ResolvedParams{},
			validate: func(t *testing.T, resp *module.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "name", resp.DefaultSortColumn)

				data, ok := resp.Data.([][]any)
				require.True(t, ok)
				assert.Len(t, data, 2)

				// Verify Charts
				require.NotNil(t, resp.Charts)
				assert.Contains(t, resp.Charts, "Traffic")
				assert.Contains(t, resp.Charts, "UnicastPackets")
				assert.Equal(t, []string{"trafficIn", "trafficOut"}, resp.Charts["Traffic"].Columns)

				// Verify DefaultCharts
				require.NotEmpty(t, resp.DefaultCharts)
				assert.Equal(t, [][]string{{"Traffic", "type"}}, resp.DefaultCharts)

				// Verify GroupBy
				require.NotNil(t, resp.GroupBy)
				assert.Contains(t, resp.GroupBy, "type")
				assert.Contains(t, resp.GroupBy, "operStatus")
				assert.Contains(t, resp.GroupBy, "adminStatus")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := tc.setup()
			resp := f.handle(tc.method, tc.params)
			tc.validate(t, resp)
		})
	}
}

func TestFuncInterfaces_defaultSortColumn(t *testing.T) {
	f := &funcInterfaces{}
	assert.Equal(t, "name", f.defaultSortColumn())
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
	for i, col := range funcIfacesColumns {
		if col.key == key {
			return i
		}
	}
	return -1
}
