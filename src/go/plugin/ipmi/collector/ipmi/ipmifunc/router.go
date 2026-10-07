// SPDX-License-Identifier: GPL-3.0-or-later

// Package ipmifunc serves the latest complete IPMI sensor snapshot.
package ipmifunc

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
)

const FunctionName = "ipmi-go-sensors"
const methodSensors = "sensors"

type Deps interface{ CurrentSnapshot() *ipmiapi.Snapshot }
type router struct{ deps Deps }

func NewRouter(deps Deps) funcapi.MethodHandler { return &router{deps: deps} }
func Methods(updateEvery int) []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{ID: methodSensors, FunctionName: FunctionName,
		Name: "IPMI Sensors", Help: "IPMI sensor readings and health states", UpdateEvery: updateEvery, ResponseType: "table"}}
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
	sensorColumn := textColumn("Sensor", "Sensor name")
	sensorColumn.Sticky = true
	sensorColumn.FullWidth = true
	columns := []funcapi.ColumnMeta{
		sensorColumn,
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
	tableColumns := funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns()
	tableColumns["rowOptions"] = funcapi.Column{
		Index:         len(columns),
		Name:          "rowOptions",
		Type:          funcapi.FieldTypeNone,
		Visualization: funcapi.FieldVisualRowOptions,
		Dummy:         true,
	}.BuildColumn()
	rows := make([][]any, 0, len(s.Sensors))
	for _, sensor := range s.Sensors {
		var value, units any
		if sensor.Value != nil {
			value = *sensor.Value
		}
		if sensor.Unit != "" {
			units = sensor.Unit
		}
		severity := "normal"
		if sensor.State == "warning" || sensor.State == "critical" {
			severity = sensor.State
		}
		rows = append(rows, []any{sensor.Name, sensor.Type, sensor.Component, value, units, sensor.State, sensor.Key,
			map[string]string{"severity": severity}})
	}
	return &funcapi.FunctionResponse{
		Status:            200,
		Columns:           tableColumns,
		Data:              rows,
		DefaultSortColumn: "Type",
		Help:              "Last successful collection: " + s.CollectedAt.UTC().Format(time.RFC3339),
		ChartingConfig: funcapi.ChartingConfig{
			Charts: map[string]funcapi.ChartConfig{
				"Sensors": {Name: "Sensors", Type: "stacked-bar", Columns: []string{"Sensor"}},
			},
			DefaultCharts: funcapi.DefaultCharts{
				{Chart: "Sensors", GroupBy: "Component"},
				{Chart: "Sensors", GroupBy: "State"},
			},
		},
	}
}

func textColumn(name, help string) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:     name,
		Tooltip:  help,
		Type:     funcapi.FieldTypeString,
		Visible:  true,
		Sortable: true,
		Filter:   funcapi.FieldFilterMultiselect,
	}
}
