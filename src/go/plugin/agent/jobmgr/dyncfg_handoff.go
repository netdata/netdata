// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const dyncfgShuttingDownMsg = "Job manager is shutting down."

// enqueueDyncfgFunction blocks until the function is accepted by the run loop
// or the manager shuts down. We deliberately do NOT honor a per-function
// timeout here: dropping an awaited enable/disable would wedge jobmgr's wait
// gate (since waitDecisionTimeout was removed). Back-pressure flows upstream:
// dyncfgCh full -> framework worker blocks here -> scheduler fills ->
// dispatchInvocation blocks -> stdin reader pauses -> netdata's pipe write
// blocks. This is intentional so awaited state transitions preserve ordering
// and eventually slow the producer instead of being dropped.
func (m *Manager) enqueueDyncfgFunction(fn dyncfg.Function) {
	select {
	case m.dyncfgCh <- fn:
	case <-m.baseContext().Done():
		m.dyncfgResponder.SendCodef(fn, 503, dyncfgShuttingDownMsg)
	}
}
