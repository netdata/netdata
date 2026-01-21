// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcInterfaces handles the "interfaces" function for SNMP devices.
// It provides network interface traffic and status metrics from cached SNMP data.
type funcInterfaces struct {
	cache *ifaceCache
}

func newFuncInterfaces(cache *ifaceCache) *funcInterfaces {
	return &funcInterfaces{cache: cache}
}

// methods returns the method configurations for this function.
func (f *funcInterfaces) methods() []module.MethodConfig {
	var sortOptions []funcapi.ParamOption

	for _, col := range funcIfacesColumns {
		if col.sortOption != "" {
			sd := col.sortDir
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.key,
				Column:  col.key,
				Name:    col.sortOption,
				Default: col.defaultSort,
				Sort:    &sd,
			})
		}
	}

	return []module.MethodConfig{{
		ID:   "interfaces",
		Name: "Network Interfaces",
		Help: "Network interface traffic and status metrics",
		RequiredParams: []funcapi.ParamConfig{{
			ID:         funcIfacesParamSort,
			Name:       "Sort By",
			Help:       "Select sort order",
			Selection:  funcapi.ParamSelect,
			Options:    sortOptions,
			UniqueView: true,
		}},
	}}
}

// methodParams returns params for the given method.
func (f *funcInterfaces) methodParams(method string) ([]funcapi.ParamConfig, error) {
	if method != "interfaces" {
		return nil, fmt.Errorf("unknown method: %s", method)
	}

	methods := f.methods()
	if len(methods) > 0 {
		return methods[0].RequiredParams, nil
	}
	return nil, nil
}

// handle processes a function request and returns the response.
func (f *funcInterfaces) handle(method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	if method != "interfaces" {
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}

	if f.cache == nil {
		return &module.FunctionResponse{
			Status:  503,
			Message: "interface data not available yet, please retry after data collection",
		}
	}

	f.cache.mu.RLock()
	defer f.cache.mu.RUnlock()

	// Build data rows from cache
	data := make([][]any, 0, len(f.cache.interfaces))
	for _, entry := range f.cache.interfaces {
		row := f.buildRow(entry)
		data = append(data, row)
	}

	// Sort data based on params
	sortColumn := params.Column(funcIfacesParamSort)
	f.sortData(data, sortColumn)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Network interface traffic and status metrics",
		Columns:           f.buildColumns(),
		Data:              data,
		DefaultSortColumn: f.defaultSortColumn(),

		// Charts for aggregated visualization
		Charts: map[string]module.ChartConfig{
			"Traffic": {
				Name:    "Traffic",
				Type:    "stacked-bar",
				Columns: []string{"trafficIn", "trafficOut"},
			},
			"UnicastPackets": {
				Name:    "Unicast Packets",
				Type:    "stacked-bar",
				Columns: []string{"ucastPktsIn", "ucastPktsOut"},
			},
			"BroadcastPackets": {
				Name:    "Broadcast Packets",
				Type:    "stacked-bar",
				Columns: []string{"bcastPktsIn", "bcastPktsOut"},
			},
			"MulticastPackets": {
				Name:    "Multicast Packets",
				Type:    "stacked-bar",
				Columns: []string{"mcastPktsIn", "mcastPktsOut"},
			},
			"OperationalStatus": {
				Name:    "Operational Status",
				Type:    "stacked-bar",
				Columns: []string{"operStatus"},
			},
		},
		DefaultCharts: [][]string{
			{"Traffic", "type"},
			{"OperationalStatus", "operStatus"},
		},
		GroupBy: map[string]module.GroupByConfig{
			"type": {
				Name:    "Group by Type",
				Columns: []string{"type"},
			},
		},
	}
}

// buildColumns builds column definitions for the response.
func (f *funcInterfaces) buildColumns() map[string]any {
	columns := make(map[string]any)

	for i, col := range funcIfacesColumns {
		colDef := funcapi.Column{
			Index:         i,
			Name:          col.name,
			Type:          col.dataType,
			Units:         col.units,
			Visualization: funcapi.FieldVisualValue,
			Sort:          col.sortDir,
			Sortable:      true,
			Sticky:        col.sticky,
			Summary:       col.summary,
			Filter:        col.filter,
			Visible:       col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimals,
				DefaultValue:  nil,
			},
		}
		columns[col.key] = colDef.BuildColumn()
	}

	return columns
}

// buildRow builds a data row from an interface entry.
// Column order is determined by funcIfacesColumns - each column's value() extracts the data.
func (f *funcInterfaces) buildRow(entry *ifaceEntry) []any {
	row := make([]any, len(funcIfacesColumns))
	for i, col := range funcIfacesColumns {
		row[i] = col.value(entry)
	}
	return row
}

// sortData sorts the data rows by the specified column.
func (f *funcInterfaces) sortData(data [][]any, sortColumn string) {
	if len(data) == 0 {
		return
	}

	// Find column index and sort direction
	colIdx := 0
	sortDir := funcapi.FieldSortAscending

	for i, col := range funcIfacesColumns {
		if col.key == sortColumn {
			colIdx = i
			sortDir = col.sortDir
			break
		}
	}

	sort.Slice(data, func(i, j int) bool {
		vi := data[i][colIdx]
		vj := data[j][colIdx]

		// Handle nil values - put them at the end
		if vi == nil && vj == nil {
			return false
		}
		if vi == nil {
			return false
		}
		if vj == nil {
			return true
		}

		// Compare based on type
		switch a := vi.(type) {
		case string:
			b := vj.(string)
			if sortDir == funcapi.FieldSortAscending {
				return a < b
			}
			return a > b
		case float64:
			b := vj.(float64)
			if sortDir == funcapi.FieldSortAscending {
				return a < b
			}
			return a > b
		default:
			return false
		}
	})
}

// defaultSortColumn returns the default sort column key.
func (f *funcInterfaces) defaultSortColumn() string {
	for _, col := range funcIfacesColumns {
		if col.defaultSort {
			return col.key
		}
	}
	return "name"
}

// ptrToAny converts a *float64 to any, returning nil if the pointer is nil.
func ptrToAny(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Package-level registration functions that delegate to funcInterfaces.

func snmpMethods() []module.MethodConfig {
	return (&funcInterfaces{}).methods()
}

func snmpMethodParams(_ context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	return c.funcIfaces.methodParams(method)
}

func snmpHandleMethod(_ context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	c, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}
	return c.funcIfaces.handle(method, params)
}

// funcIfacesParamSort is the parameter ID for sort selection.
const funcIfacesParamSort = "__sort"

// funcIfacesColumn defines a column with its metadata and value extractor.
// The value function extracts the column's data from an ifaceEntry.
type funcIfacesColumn struct {
	key         string                 // unique column identifier
	name        string                 // display name
	value       func(*ifaceEntry) any  // extracts value from entry
	dataType    funcapi.FieldType      // string, float, etc.
	units       string                 // display units (bytes/s, packets/s)
	visible     bool                   // shown by default
	transform   funcapi.FieldTransform // number formatting
	decimals    int                    // decimal points
	sortDir     funcapi.FieldSort      // asc or desc
	summary     funcapi.FieldSummary   // count, sum, etc.
	filter      funcapi.FieldFilter    // multiselect, range, etc.
	sortOption  string                 // if non-empty, appears in sort dropdown
	defaultSort bool                   // is default sort column
	sticky      bool                   // sticky column
}

// funcIfacesColumns defines all columns for the interfaces function.
// Each column includes its value extractor - single source of truth.
var funcIfacesColumns = []funcIfacesColumn{
	{
		key:         "name",
		name:        "Interface",
		value:       func(e *ifaceEntry) any { return e.name },
		dataType:    funcapi.FieldTypeString,
		visible:     true,
		sortDir:     funcapi.FieldSortAscending,
		summary:     funcapi.FieldSummaryCount,
		filter:      funcapi.FieldFilterMultiselect,
		sortOption:  "Sort by Interface Name",
		defaultSort: true,
		sticky:      true,
	},
	{
		key:      "type",
		name:     "Type",
		value:    func(e *ifaceEntry) any { return e.ifType },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:        "trafficIn",
		name:       "Traffic In",
		value:      func(e *ifaceEntry) any { return ptrToAny(e.rates.trafficIn) },
		dataType:   funcapi.FieldTypeFloat,
		units:      "bit/s",
		visible:    true,
		transform:  funcapi.FieldTransformNumber,
		sortDir:    funcapi.FieldSortDescending,
		summary:    funcapi.FieldSummarySum,
		filter:     funcapi.FieldFilterRange,
		sortOption: "Top by Traffic In",
	},
	{
		key:        "trafficOut",
		name:       "Traffic Out",
		value:      func(e *ifaceEntry) any { return ptrToAny(e.rates.trafficOut) },
		dataType:   funcapi.FieldTypeFloat,
		units:      "bit/s",
		visible:    true,
		transform:  funcapi.FieldTransformNumber,
		sortDir:    funcapi.FieldSortDescending,
		summary:    funcapi.FieldSummarySum,
		filter:     funcapi.FieldFilterRange,
		sortOption: "Top by Traffic Out",
	},
	{
		key:       "ucastPktsIn",
		name:      "Unicast In",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.ucastPktsIn) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "ucastPktsOut",
		name:      "Unicast Out",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.ucastPktsOut) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "bcastPktsIn",
		name:      "Broadcast In",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.bcastPktsIn) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "bcastPktsOut",
		name:      "Broadcast Out",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.bcastPktsOut) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "mcastPktsIn",
		name:      "Multicast In",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.mcastPktsIn) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "mcastPktsOut",
		name:      "Multicast Out",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.mcastPktsOut) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:      "adminStatus",
		name:     "Admin Status",
		value:    func(e *ifaceEntry) any { return e.adminStatus },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:      "operStatus",
		name:     "Oper Status",
		value:    func(e *ifaceEntry) any { return e.operStatus },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
}
