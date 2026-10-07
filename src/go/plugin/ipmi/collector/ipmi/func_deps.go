// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
	"sync/atomic"
)

type functionDeps struct {
	snapshot *atomic.Pointer[ipmiapi.Snapshot]
}

func (d functionDeps) CurrentSnapshot() *ipmiapi.Snapshot { return d.snapshot.Load() }
func functionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
