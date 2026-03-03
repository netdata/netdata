// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	dyncfgBusyMsg         = "Job manager is busy, try again later."
	dyncfgShuttingDownMsg = "Job manager is shutting down."
)

func (m *Manager) enqueueDyncfgFunction(fn dyncfg.Function) {
	handoffCtx, cancel := m.dyncfgHandoffContext(fn)
	defer cancel()

	switch dyncfg.BoundedSend(handoffCtx, m.dyncfgCh, fn, dyncfg.DefaultDownstreamHandoffCap) {
	case dyncfg.BoundedSendOK:
		return
	case dyncfg.BoundedSendContextDone:
		if m.baseContext().Err() != nil {
			m.dyncfgApi.SendCodef(fn, 503, dyncfgShuttingDownMsg)
			return
		}
		m.dyncfgApi.SendCodef(fn, 503, dyncfgBusyMsg)
	case dyncfg.BoundedSendTimeout:
		m.dyncfgApi.SendCodef(fn, 503, dyncfgBusyMsg)
	}
}

func (m *Manager) dyncfgHandoffContext(fn dyncfg.Function) (context.Context, context.CancelFunc) {
	ctx := m.baseContext()
	if timeout := fn.Fn().Timeout; timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}
