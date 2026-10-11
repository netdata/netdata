// SPDX-License-Identifier: GPL-3.0-or-later

package javafunc

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

type Application struct {
	Name, Instance, Runtime, Location, Status, JVM, HTTP, Pools, LastSample, Detail string
	PID                                                                             int
}

type Deps interface {
	Applications() []Application
}

type router struct{ deps Deps }

func New(deps Deps) funcapi.MethodHandler { return &router{deps: deps} }

func Methods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{
		ID:           "applications",
		Name:         "Java applications",
		Help:         "Discovered Java applications and observed JVM, HTTP and connection-pool coverage",
		UpdateEvery:  1,
		ResponseType: "table",
	}}
}

func (*router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != "applications" {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}

func (*router) Cleanup(context.Context) {}

func (r *router) Handle(ctx context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != "applications" {
		return funcapi.NotFoundResponse(method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "Java application request canceled")
	}
	columns := []funcapi.ColumnMeta{}
	for _, name := range []string{"Application", "Instance", "PID", "Runtime", "Location", "Status", "JVM", "HTTP", "Pools", "Last sample", "Details"} {
		columns = append(columns, funcapi.ColumnMeta{
			Name:      name,
			Tooltip:   name,
			Type:      funcapi.FieldTypeString,
			Visible:   name != "Instance" && name != "PID" && name != "Location" && name != "Details",
			Sortable:  true,
			UniqueKey: name == "Instance",
			Filter:    funcapi.FieldFilterMultiselect,
		})
	}
	rows := make([][]any, 0)
	for _, app := range r.deps.Applications() {
		rows = append(rows, []any{app.Name, app.Instance, fmt.Sprint(app.PID), app.Runtime, app.Location,
			app.Status, app.JVM, app.HTTP, app.Pools, app.LastSample, app.Detail})
	}
	return &funcapi.FunctionResponse{
		Status:            200,
		Columns:           funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns(),
		Data:              rows,
		DefaultSortColumn: "Application",
		Help:              "Coverage reports fresh measurements received from each process. Not observed does not mean zero activity or an unsupported library.",
	}
}
