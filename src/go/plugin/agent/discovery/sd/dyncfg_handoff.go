// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const sdBusyMsg = "Service discovery is busy, try again later."

func (d *ServiceDiscovery) enqueueDyncfgFunction(fn dyncfg.Function) {
	handoffCtx, cancel := d.dyncfgHandoffContext(fn)
	defer cancel()

	switch dyncfg.BoundedSend(handoffCtx, d.dyncfgCh, fn, dyncfg.DefaultDownstreamHandoffCap) {
	case dyncfg.BoundedSendOK:
		return
	case dyncfg.BoundedSendContextDone:
		if d.ctx != nil && d.ctx.Err() != nil {
			d.dyncfgApi.SendCodef(fn, 503, "Service discovery is shutting down.")
			return
		}
		d.dyncfgApi.SendCodef(fn, 503, sdBusyMsg)
	case dyncfg.BoundedSendTimeout:
		d.dyncfgApi.SendCodef(fn, 503, sdBusyMsg)
	}
}

func (d *ServiceDiscovery) dyncfgHandoffContext(fn dyncfg.Function) (context.Context, context.CancelFunc) {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout := fn.Fn().Timeout; timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}
