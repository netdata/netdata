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
	return []module.MethodConfig{{
		ID:   "interfaces",
		Name: "Network Interfaces",
		Help: "Network interface traffic and status metrics",
		RequiredParams: []funcapi.ParamConfig{{
			ID:        funcIfacesParamTypeGroup,
			Name:      "Type Group",
			Help:      "Filter by interface type group",
			Selection: funcapi.ParamSelect,
			Options: []funcapi.ParamOption{
				{ID: "ethernet", Name: "Ethernet", Default: true},
				{ID: "aggregation", Name: "Aggregation"},
				{ID: "virtual", Name: "Virtual"},
				{ID: "other", Name: "Other"},
			},
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

	typeGroupFilter := params.GetOne(funcIfacesParamTypeGroup)
	if typeGroupFilter == "" {
		typeGroupFilter = "ethernet"
	}

	// Build data rows from cache
	data := make([][]any, 0, len(f.cache.interfaces))
	for _, entry := range f.cache.interfaces {
		if !matchesTypeGroup(entry.ifTypeGroup, typeGroupFilter) {
			continue
		}
		row := f.buildRow(entry)
		data = append(data, row)
	}

	// Sort data based on params
	f.sortData(data, f.defaultSortColumn())

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
				Columns: []string{"Traffic In", "Traffic Out"},
			},
			"UnicastPackets": {
				Name:    "Unicast Packets",
				Type:    "stacked-bar",
				Columns: []string{"Unicast In", "Unicast Out"},
			},
			"BroadcastPackets": {
				Name:    "Broadcast Packets",
				Type:    "stacked-bar",
				Columns: []string{"Broadcast In", "Broadcast Out"},
			},
			"MulticastPackets": {
				Name:    "Multicast Packets",
				Type:    "stacked-bar",
				Columns: []string{"Multicast In", "Multicast Out"},
			},
			"OperationalStatus": {
				Name:    "Operational Status",
				Type:    "stacked-bar",
				Columns: []string{"Oper Status"},
			},
		},
		DefaultCharts: [][]string{
			{"Traffic", "Type"},
			{"OperationalStatus", "Oper Status"},
		},
		GroupBy: map[string]module.GroupByConfig{
			"Type": {
				Name:    "Group by Type",
				Columns: []string{"Type"},
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
			Visualization: col.visual,
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

	rowOptions := funcapi.Column{
		Index:         len(funcIfacesColumns),
		Name:          "rowOptions",
		Type:          funcapi.FieldTypeNone,
		Visualization: funcapi.FieldVisualRowOptions,
		Sort:          funcapi.FieldSortAscending,
		Sortable:      false,
		Sticky:        false,
		Summary:       funcapi.FieldSummaryCount,
		Filter:        funcapi.FieldFilterNone,
		Visible:       false,
		Dummy:         true,
		ValueOptions: funcapi.ValueOptions{
			Transform:     funcapi.FieldTransformNone,
			DecimalPoints: 0,
			DefaultValue:  nil,
		},
	}
	columns["rowOptions"] = rowOptions.BuildColumn()

	return columns
}

// buildRow builds a data row from an interface entry.
// Column order is determined by funcIfacesColumns - each column's value() extracts the data.
func (f *funcInterfaces) buildRow(entry *ifaceEntry) []any {
	row := make([]any, len(funcIfacesColumns)+1)
	for i, col := range funcIfacesColumns {
		row[i] = col.value(entry)
	}
	if isIfaceDown(entry) {
		for i, col := range funcIfacesColumns {
			if col.dataType == funcapi.FieldTypeFloat {
				row[i] = nil
			}
		}
	}
	row[len(funcIfacesColumns)] = rowOptionsForIface(entry)
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
	return "Interface"
}

func rowOptionsForIface(entry *ifaceEntry) any {
	// TODO: Re-enable row coloring once the UI supports more severity options.
	return nil
}

func isIfaceDown(entry *ifaceEntry) bool {
	if entry == nil {
		return false
	}
	return entry.adminStatus != "up" || entry.operStatus != "up"
}

func matchesTypeGroup(group, filter string) bool {
	knownGroups := map[string]bool{
		"ethernet":    true,
		"aggregation": true,
		"virtual":     true,
	}

	if filter == "other" {
		return !knownGroups[group]
	}

	return group == filter
}

// ptrToAny converts a *float64 to any, returning nil if the pointer is nil.
func ptrToAny(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func ptrToAnyScale(p *float64, scale float64) any {
	if p == nil {
		return nil
	}
	return *p / scale
}

func sumRates(vals ...*float64) *float64 {
	var sum float64
	hasValue := false
	for _, v := range vals {
		if v == nil {
			continue
		}
		sum += *v
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return &sum
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

// funcIfacesParamTypeGroup is the parameter ID for type group filtering.
const funcIfacesParamTypeGroup = "if_type_group"

// funcIfacesColumn defines a column with its metadata and value extractor.
// The value function extracts the column's data from an ifaceEntry.
type funcIfacesColumn struct {
	key         string                 // column header value (must be unique)
	name        string                 // tooltip value
	value       func(*ifaceEntry) any  // extracts value from entry
	dataType    funcapi.FieldType      // string, float, etc.
	units       string                 // display units (bytes/s, packets/s)
	visual      funcapi.FieldVisual    // visualization type
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
		key:         "Interface",
		name:        "",
		value:       func(e *ifaceEntry) any { return e.name },
		dataType:    funcapi.FieldTypeString,
		visible:     true,
		sortDir:     funcapi.FieldSortAscending,
		summary:     funcapi.FieldSummaryCount,
		filter:      funcapi.FieldFilterMultiselect,
		defaultSort: true,
		sticky:      true,
	},
	{
		key:      "Type",
		name:     "IANA ifType (IF-MIB)",
		value:    func(e *ifaceEntry) any { return e.ifType },
		dataType: funcapi.FieldTypeString,
		visible:  false,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:      "Type Group",
		name:     "Custom mapping of IANA ifType into groups",
		value:    func(e *ifaceEntry) any { return e.ifTypeGroup },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:      "Admin Status",
		name:     "Administrative status: up, down, testing",
		value:    func(e *ifaceEntry) any { return e.adminStatus },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:      "Oper Status",
		name:     "Operational status: up, down, testing, unknown, dormant, notPresent, lowerLayerDown",
		value:    func(e *ifaceEntry) any { return e.operStatus },
		dataType: funcapi.FieldTypeString,
		visible:  true,
		sortDir:  funcapi.FieldSortAscending,
		summary:  funcapi.FieldSummaryCount,
		filter:   funcapi.FieldFilterMultiselect,
	},
	{
		key:       "Traffic In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.trafficIn, 1_000_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Mbits",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Traffic Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.trafficOut, 1_000_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Mbits",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Unicast In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.ucastPktsIn, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Unicast Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.ucastPktsOut, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Broadcast In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.bcastPktsIn, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Broadcast Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.bcastPktsOut, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:  "Packets In",
		name: "",
		value: func(e *ifaceEntry) any {
			return ptrToAnyScale(sumRates(e.rates.ucastPktsIn, e.rates.bcastPktsIn, e.rates.mcastPktsIn), 1_000)
		},
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:  "Packets Out",
		name: "",
		value: func(e *ifaceEntry) any {
			return ptrToAnyScale(sumRates(e.rates.ucastPktsOut, e.rates.bcastPktsOut, e.rates.mcastPktsOut), 1_000)
		},
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Errors In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.errorsIn) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Errors Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.errorsOut) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Discards In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.discardsIn) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Discards Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAny(e.rates.discardsOut) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "packets/s",
		visual:    funcapi.FieldVisualBar,
		visible:   true,
		transform: funcapi.FieldTransformNumber,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Multicast In",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.mcastPktsIn, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
	{
		key:       "Multicast Out",
		name:      "",
		value:     func(e *ifaceEntry) any { return ptrToAnyScale(e.rates.mcastPktsOut, 1_000) },
		dataType:  funcapi.FieldTypeFloat,
		units:     "Kpps",
		visual:    funcapi.FieldVisualBar,
		visible:   false,
		transform: funcapi.FieldTransformNumber,
		decimals:  2,
		sortDir:   funcapi.FieldSortDescending,
		summary:   funcapi.FieldSummarySum,
		filter:    funcapi.FieldFilterRange,
	},
}
