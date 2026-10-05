// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/smbiosfunc"
)

type functionDeps struct {
	snapshot *atomic.Pointer[inventory.Snapshot]
}

func (d functionDeps) CurrentSnapshot() *inventory.Snapshot { return d.snapshot.Load() }

func smbiosMethods() []funcapi.FunctionConfig { return smbiosfunc.Methods(defaultUpdateEvery) }

func smbiosFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
