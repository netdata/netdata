// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/ipmifunc"
)

type functionDeps struct {
	snapshot *atomic.Pointer[bmc.Snapshot]
}

func (d functionDeps) CurrentSnapshot() *bmc.Snapshot { return d.snapshot.Load() }

func ipmiMethods() []funcapi.FunctionConfig { return ipmifunc.Methods(defaultUpdateEvery) }

func ipmiFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
