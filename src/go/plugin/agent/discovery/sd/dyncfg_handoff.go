// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const sdShuttingDownMsg = "Service discovery is shutting down."

// enqueueDyncfgFunction blocks until the function is accepted by the run loop
// or service discovery shuts down. We deliberately do NOT honor a per-function
// timeout here: dropping an awaited enable/disable would wedge the wait gate
// (since waitDecisionTimeout was removed) because the caller would keep
// waiting for a decision that can never be produced. Back-pressure flows
// upstream to netdata via the OS pipe.
func (d *ServiceDiscovery) enqueueDyncfgFunction(fn dyncfg.Function) {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case d.dyncfgCh <- fn:
	case <-ctx.Done():
		d.dyncfgApi.SendCodef(fn, 503, sdShuttingDownMsg)
	}
}
