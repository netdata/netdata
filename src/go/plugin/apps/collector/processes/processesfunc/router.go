// SPDX-License-Identifier: GPL-3.0-or-later

// Package processesfunc serves only completed, immutable Go-owned snapshots.
package processesfunc

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

const FunctionName = "appsgo:processes"
const MethodID = "processes"

type Deps interface{ CurrentSnapshot() *model.Snapshot }
type router struct{ deps Deps }

func NewRouter(deps Deps) funcapi.MethodHandler { return &router{deps: deps} }
func Methods(updateEvery int) []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{ID: MethodID, FunctionName: FunctionName, Name: "Processes", UpdateEvery: updateEvery, Help: "Processes from the latest completed apps POC collection; raw command lines are omitted", ResponseType: "table"}}
}
func (r *router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != MethodID {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	names := make(map[string]bool)
	if s := r.deps.CurrentSnapshot(); s != nil {
		for _, p := range s.Processes {
			names[p.Application] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	options := []funcapi.ParamOption{{ID: "all", Name: "All applications", Default: true}}
	for _, name := range sorted {
		options = append(options, funcapi.ParamOption{ID: "app:" + name, Name: name})
	}
	return []funcapi.ParamConfig{{ID: "application", Name: "Application", Help: "Limit rows to one application group", Options: options}}, nil
}
func (*router) Cleanup(context.Context) {}
func (r *router) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != MethodID {
		return funcapi.NotFoundResponse(method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "Process request canceled")
	}
	s := r.deps.CurrentSnapshot()
	if s == nil {
		return funcapi.UnavailableResponse("Waiting for the first completed process collection")
	}
	rows := make([][]any, 0, len(s.Processes))
	filter := params.GetOne("application")
	for _, p := range s.Processes {
		if ctx.Err() != nil {
			return funcapi.ErrorResponse(499, "Process request canceled")
		}
		if filter != "" && filter != "all" && filter != "app:"+p.Application {
			continue
		}
		rows = append(rows, []any{fmt.Sprintf("%d:%d", p.Key.PID, p.Key.StartTime), p.Key.PID, p.PPID, p.Comm, p.Application, p.UID, p.GID, p.State,
			sum(p, model.CPUUser, model.CPUSystem, model.CPUGuest, model.CPUChildrenUser, model.CPUChildrenSystem, model.CPUChildrenGuest),
			value(p, model.ResidentMemory), value(p, model.ProportionalMemory), value(p, model.PSSAge), value(p, model.ReadBytes), value(p, model.WriteBytes), value(p, model.Threads), value(p, model.Uptime)})
	}
	age := time.Since(s.CollectedAt).Seconds()
	if age < 0 {
		age = 0
	}
	return &funcapi.FunctionResponse{Status: 200, Columns: funcapi.Columns(columns(), func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns(), Data: rows, DefaultSortColumn: "CPU", Help: fmt.Sprintf("Completed snapshot: %s (age %.1f seconds). CPU includes reconciled exited-child usage; 100%% is one core. PSS is sampled; its age is shown. Missing measurements are null. Raw command lines are omitted.", s.CollectedAt.UTC().Format(time.RFC3339Nano), age)}
}
func value(p model.Process, m int) any {
	if !p.Has(m) {
		return nil
	}
	return p.Values[m]
}
func sum(p model.Process, metrics ...int) any {
	var v float64
	for _, m := range metrics {
		if !p.Has(m) {
			return nil
		}
		v += p.Values[m]
	}
	return v
}
func columns() []funcapi.ColumnMeta {
	key := textColumn("Key")
	key.UniqueKey = true
	key.Visible = false
	return []funcapi.ColumnMeta{key, numberColumn("PID", "", true), numberColumn("Parent PID", "", true), textColumn("Command"), textColumn("Application"), numberColumn("UID", "", true), numberColumn("GID", "", true), textColumn("State"), numberColumn("CPU", "%", false), numberColumn("RSS", "bytes", false), numberColumn("PSS", "bytes", false), numberColumn("PSS age", "seconds", false), numberColumn("Read", "bytes/s", false), numberColumn("Write", "bytes/s", false), numberColumn("Threads", "threads", true), numberColumn("Uptime", "seconds", false)}
}
func textColumn(name string) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{Name: name, Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect}
}
func numberColumn(name, units string, integer bool) funcapi.ColumnMeta {
	typ := funcapi.FieldTypeFloat
	if integer {
		typ = funcapi.FieldTypeInteger
	}
	return funcapi.ColumnMeta{Name: name, Type: typ, Units: units, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2}
}
