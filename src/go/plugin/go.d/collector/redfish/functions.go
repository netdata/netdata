// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/redfishfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

type functionDeps struct {
	runtime *redfishruntime.Runtime
}

func redfishFunctions(runtime *redfishruntime.Runtime) func() []funcapi.FunctionConfig {
	return redfishfunc.Configs(functionDeps{runtime: runtime})
}

func redfishFunctionHandler(runtime *redfishruntime.Runtime) func(collectorapi.RuntimeJob) funcapi.MethodHandler {
	return redfishfunc.Handler(functionDeps{runtime: runtime})
}

func (d functionDeps) VisitInventoryCatalog(
	ctx context.Context,
	maxJobs int,
	visitJob func(string) bool,
	visitHost func(uri, name string) bool,
	visitKind func(string) bool,
) bool {
	return d.runtime.VisitInventoryCatalog(ctx, maxJobs, visitJob, visitHost, visitKind)
}

func (d functionDeps) VisitInventorySlice(
	ctx context.Context,
	job, host, resourceKind string,
	visit func(map[string]any) bool,
) (int, bool) {
	return d.runtime.VisitInventorySlice(ctx, job, host, resourceKind, visit)
}
