// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcInterfaces)(nil)

// funcInterfaces handles the "interfaces" function for SNMP devices.
// It provides network interface traffic and status metrics from cached SNMP data.
type funcInterfaces struct {
	router *funcRouter
}

func newFuncInterfaces(r *funcRouter) *funcInterfaces {
	return &funcInterfaces{router: r}
}

const (
	ifacesMethodID         = "interfaces"
	ifacesParamTypeGroup   = "if_type_group"
	ifacesDefaultTypeGroup = "ethernet"
)

func ifacesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:          ifacesMethodID,
		Name:        "Network Interfaces",
		UpdateEvery: 10,
		Help:        "Network interface traffic and status metrics",
		RequiredParams: []funcapi.ParamConfig{{
			ID:        ifacesParamTypeGroup,
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
	}
}

// MethodParams implements funcapi.MethodHandler.
func (f *funcInterfaces) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != ifacesMethodID {
		return nil, nil
	}
	return []funcapi.ParamConfig{{
		ID:        ifacesParamTypeGroup,
		Name:      "Type Group",
		Help:      "Filter by interface type group",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: "ethernet", Name: "Ethernet", Default: true},
			{ID: "aggregation", Name: "Aggregation"},
			{ID: "virtual", Name: "Virtual"},
			{ID: "other", Name: "Other"},
		},
	}}, nil
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcInterfaces) Cleanup(_ context.Context) {}

// Handle implements funcapi.MethodHandler.
func (f *funcInterfaces) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != ifacesMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if f.router.ifaceCache == nil {
		return funcapi.UnavailableResponse("interface data not available yet, please retry after data collection")
	}

	f.router.ifaceCache.mu.RLock()
	defer f.router.ifaceCache.mu.RUnlock()

	typeGroupFilter := params.GetOne(ifacesParamTypeGroup)
	if typeGroupFilter == "" {
		typeGroupFilter = ifacesDefaultTypeGroup
	}

	// Build data rows from cache
	data := make([][]any, 0, len(f.router.ifaceCache.interfaces))
	for _, entry := range f.router.ifaceCache.interfaces {
		if !matchesTypeGroup(entry.ifTypeGroup, typeGroupFilter) {
			continue
		}
		data = append(data, f.buildRow(entry))
	}

	f.sortData(data, f.defaultSortColumn())

	cs := snmpColumnSet(snmpAllColumns)

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Network interface traffic and status metrics",
		Columns:           f.buildColumns(cs),
		Data:              data,
		DefaultSortColumn: f.defaultSortColumn(),
		ChartingConfig:    cs.BuildCharting(),
	}
}

// buildColumns builds column definitions for the response.
func (f *funcInterfaces) buildColumns(cs funcapi.ColumnSet[snmpColumn]) map[string]any {
	columns := cs.BuildColumns()

	// Add rowOptions column (not part of the regular column set)
	rowOptions := funcapi.Column{
		Index:         cs.Len(),
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
func (f *funcInterfaces) buildRow(entry *ifaceEntry) []any {
	row := make([]any, len(snmpAllColumns)+1)
	for i, col := range snmpAllColumns {
		row[i] = col.Value(entry)
	}
	if isIfaceDown(entry) {
		for i, col := range snmpAllColumns {
			if col.Type == funcapi.FieldTypeFloat {
				row[i] = nil
			}
		}
	}
	row[len(snmpAllColumns)] = rowOptionsForIface(entry)
	return row
}

// sortData sorts the data rows by the specified column.
func (f *funcInterfaces) sortData(data [][]any, sortColumn string) {
	if len(data) == 0 {
		return
	}

	colIdx := 0
	sortDir := funcapi.FieldSortAscending

	for i, col := range snmpAllColumns {
		if col.Name == sortColumn {
			colIdx = i
			sortDir = col.Sort
			break
		}
	}

	sort.Slice(data, func(i, j int) bool {
		vi := data[i][colIdx]
		vj := data[j][colIdx]

		if vi == nil && vj == nil {
			return false
		}
		if vi == nil {
			return false
		}
		if vj == nil {
			return true
		}

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
	for _, col := range snmpAllColumns {
		if col.DefaultSort {
			return col.Name
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

func ptrToAny(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
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

// snmpColumn defines a column for SNMP interfaces function.
type snmpColumn struct {
	funcapi.ColumnMeta
	Value       func(*ifaceEntry) any // Extracts value from entry
	DefaultSort bool                  // Is default sort column
}

func snmpColumnSet(cols []snmpColumn) funcapi.ColumnSet[snmpColumn] {
	return funcapi.Columns(cols, func(c snmpColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var snmpAllColumns = []snmpColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "Interface", Tooltip: "Interface", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, Sortable: true}, Value: func(e *ifaceEntry) any { return e.name }, DefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Type", Tooltip: "IANA ifType (IF-MIB)", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, Value: func(e *ifaceEntry) any { return e.ifType }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Type Group", Tooltip: "Custom mapping of IANA ifType into groups", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(e *ifaceEntry) any { return e.ifTypeGroup }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Admin Status", Tooltip: "Administrative status: up, down, testing", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(e *ifaceEntry) any { return e.adminStatus }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Oper Status", Tooltip: "Operational status: up, down, testing, unknown, dormant, notPresent, lowerLayerDown", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, Chart: &funcapi.ChartOptions{Group: "OperationalStatus", Title: "Operational Status", IsDefault: true, DefaultGroupBy: "Oper Status"}, GroupBy: &funcapi.GroupByOptions{}}, Value: func(e *ifaceEntry) any { return e.operStatus }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Traffic In", Tooltip: "Traffic In", Type: funcapi.FieldTypeFloat, Units: "bit/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "Traffic", IsDefault: true, DefaultGroupBy: "Type"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.trafficIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Traffic Out", Tooltip: "Traffic Out", Type: funcapi.FieldTypeFloat, Units: "bit/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "Traffic"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.trafficOut) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Unicast In", Tooltip: "Unicast In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "UnicastPackets", Title: "Unicast Packets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.ucastPktsIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Unicast Out", Tooltip: "Unicast Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "UnicastPackets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.ucastPktsOut) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Broadcast In", Tooltip: "Broadcast In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "BroadcastPackets", Title: "Broadcast Packets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.bcastPktsIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Broadcast Out", Tooltip: "Broadcast Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "BroadcastPackets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.bcastPktsOut) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Packets In", Tooltip: "Packets In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any {
		return ptrToAny(sumRates(e.rates.ucastPktsIn, e.rates.bcastPktsIn, e.rates.mcastPktsIn))
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Packets Out", Tooltip: "Packets Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any {
		return ptrToAny(sumRates(e.rates.ucastPktsOut, e.rates.bcastPktsOut, e.rates.mcastPktsOut))
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Errors In", Tooltip: "Errors In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.errorsIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Errors Out", Tooltip: "Errors Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.errorsOut) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Discards In", Tooltip: "Discards In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.discardsIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Discards Out", Tooltip: "Discards Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.discardsOut) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Multicast In", Tooltip: "Multicast In", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "MulticastPackets", Title: "Multicast Packets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.mcastPktsIn) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Multicast Out", Tooltip: "Multicast Out", Type: funcapi.FieldTypeFloat, Units: "packets/s", Visualization: funcapi.FieldVisualBar, Visible: false, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true, Chart: &funcapi.ChartOptions{Group: "MulticastPackets"}}, Value: func(e *ifaceEntry) any { return ptrToAny(e.rates.mcastPktsOut) }},
}
