// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/redfishfunc"
)

type functionDeps struct {
	snapshot *atomic.Pointer[redfishfunc.Snapshot]
}

func (d functionDeps) CurrentSnapshot() *redfishfunc.Snapshot { return d.snapshot.Load() }

func redfishMethods() []funcapi.FunctionConfig { return redfishfunc.Methods(defaultUpdateEvery) }

func redfishFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
