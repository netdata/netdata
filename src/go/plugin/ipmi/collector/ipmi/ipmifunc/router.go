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

const FunctionName = "ipmi-sensors"
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
	columns := []funcapi.ColumnMeta{
		textColumn("Sensor", "Sensor name"), textColumn("Type", "IPMI sensor type"),
		textColumn("Component", "Component inferred from the sensor name and type"),
		{Name: "Reading", Tooltip: "Current sensor reading; blank when unavailable", Type: funcapi.FieldTypeFloat,
			Visible: true, Sortable: true, Transform: funcapi.FieldTransformNumber, DecimalPoints: 3},
		textColumn("Units", "Reading units"), textColumn("State", "Default FreeIPMI health interpretation"),
		{Name: "rowOptions", Type: funcapi.FieldTypeNone, Visualization: funcapi.FieldVisualRowOptions},
	}
	rows := make([][]any, 0, len(s.Sensors))
	for _, sensor := range s.Sensors {
		var value any
		if sensor.Value != nil {
			value = *sensor.Value
		}
		severity := sensor.State
		if severity == "nominal" {
			severity = "normal"
		}
		rows = append(rows, []any{sensor.Name, sensor.Type, sensor.Component, value, sensor.Unit, sensor.State,
			map[string]string{"severity": severity}})
	}
	return &funcapi.FunctionResponse{Status: 200, Columns: funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns(),
		Data: rows, DefaultSortColumn: "Sensor", Help: "Last successful collection: " + s.CollectedAt.UTC().Format(time.RFC3339)}
}
func textColumn(name, help string) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{Name: name, Tooltip: help, Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect}
}
