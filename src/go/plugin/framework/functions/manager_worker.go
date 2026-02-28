// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "runtime/debug"

func (m *Manager) runWorker() {
	for {
		if m.scheduler == nil {
			return
		}
		req, ok := m.scheduler.next()
		m.observeSchedulerPending()
		if !ok {
			return
		}
		if req == nil || req.fn == nil || req.handler == nil {
			continue
		}
		// Safe to skip: cancel/finalization path calls tryFinalize(), which in turn
		// advances per-key lanes via scheduler.complete().
		if req.ctx != nil && req.ctx.Err() != nil {
			continue
		}
		if !m.startInvocation(req.fn.UID) {
			continue
		}

		panicked := false
		func() {
			defer func() {
				if v := recover(); v != nil {
					m.Errorf("function handler panic (uid=%s): %v\n%s", req.fn.UID, v, string(debug.Stack()))
					panicked = true
				}
			}()
			req.handler(*req.fn)
		}()

		if panicked {
			m.respUID(req.fn.UID, 500, "function handler panic")
			continue
		}

		m.setAwaitingResultState(req.fn.UID, req.fn.Timeout)
	}
}
