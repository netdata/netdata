// SPDX-License-Identifier: GPL-3.0-or-later

// Package ipmifunc serves the IPMI Sensors Function from the collector's latest
// published snapshot. It never sends IPMI commands.
package ipmifunc

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
)

// FunctionName is the public Function name, distinct from the C plugin's ipmi-sensors.
const FunctionName = "ipmi-go-sensors"

const methodSensors = "sensors"

// sensorsChart is the summary chart of sensor counts.
const sensorsChart = "Sensors"

type Deps interface {
	CurrentSnapshot() *bmc.Snapshot
}

type router struct {
	deps Deps
}

func NewRouter(deps Deps) funcapi.MethodHandler {
	return &router{
		deps: deps,
	}
}

func Methods(updateEvery int) []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{
		ID:           methodSensors,
		FunctionName: FunctionName,
		Name:         "IPMI Sensors",
		Help:         "IPMI sensor readings and health states",
		UpdateEvery:  updateEvery,
		ResponseType: "table",
	}}
}

func (*router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != methodSensors {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}

func (*router) Cleanup(context.Context) {}

func (r *router) Handle(ctx context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != methodSensors {
		return funcapi.NotFoundResponse(method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "IPMI sensor request canceled")
	}
	s := r.deps.CurrentSnapshot()
	if s == nil {
		return funcapi.UnavailableResponse("Waiting for a successful IPMI collection")
	}

	columns := sensorColumns()
	tableColumns := funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns()
	tableColumns["rowOptions"] = rowOptionsColumn(len(columns))

	rows := make([][]any, 0, len(s.Sensors))
	for _, sensor := range s.Sensors {
		rows = append(rows, sensorRow(sensor))
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Columns:           tableColumns,
		Data:              rows,
		DefaultSortColumn: "Type",
		Help:              "Last successful collection: " + s.CollectedAt.UTC().Format(time.RFC3339),
		ChartingConfig: funcapi.ChartingConfig{
			Charts: map[string]funcapi.ChartConfig{
				sensorsChart: {Name: sensorsChart, Type: "stacked-bar", Columns: []string{"Sensor"}},
			},
			DefaultCharts: funcapi.DefaultCharts{
				{Chart: sensorsChart, GroupBy: "Component"},
				{Chart: sensorsChart, GroupBy: "State"},
			},
		},
	}
}

// sensorColumns lists the data columns in sensorRow order.
func sensorColumns() []funcapi.ColumnMeta {
	sensor := textColumn("Sensor", "Sensor name")
	sensor.Sticky = true
	sensor.FullWidth = true

	return []funcapi.ColumnMeta{
		sensor,
		textColumn("Type", "IPMI sensor type"),
		textColumn("Component", "Component inferred from the sensor name and type"),
		{
			Name:          "Reading",
			Tooltip:       "Current sensor reading; blank when unavailable",
			Type:          funcapi.FieldTypeFloat,
			Visible:       true,
			Sortable:      true,
			Sort:          funcapi.FieldSortDescending,
			Summary:       funcapi.FieldSummarySum,
			Transform:     funcapi.FieldTransformNumber,
			DecimalPoints: 2,
		},
		textColumn("Units", "Reading units"),
		textColumn("State", "Default FreeIPMI health interpretation"),
		{
			Name:      "Key",
			Tooltip:   "Stable sensor identity within this collection job",
			Type:      funcapi.FieldTypeString,
			UniqueKey: true,
		},
	}
}

func sensorRow(s bmc.Sensor) []any {
	var value, units any
	if s.Value != nil {
		value = *s.Value
	}
	if s.Unit != "" {
		units = s.Unit
	}
	return []any{
		s.Name,
		s.Type,
		s.Component,
		value,
		units,
		s.State,
		s.Key,
		map[string]string{"severity": rowSeverity(s.State)},
	}
}

// rowSeverity styles warning and critical rows; unknown keeps normal styling, as in the C plugin.
func rowSeverity(state string) string {
	switch state {
	case bmc.StateWarning, bmc.StateCritical:
		return state
	default:
		return "normal"
	}
}

// rowOptionsColumn declares the trailing per-row styling value. ColumnMeta
// cannot express dummy columns, so it is built directly.
func rowOptionsColumn(index int) map[string]any {
	return funcapi.Column{
		Index:         index,
		Name:          "rowOptions",
		Type:          funcapi.FieldTypeNone,
		Visualization: funcapi.FieldVisualRowOptions,
		Dummy:         true,
	}.BuildColumn()
}

func textColumn(name, tooltip string) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:     name,
		Tooltip:  tooltip,
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Filter:   funcapi.FieldFilterMultiselect,
	}
}
