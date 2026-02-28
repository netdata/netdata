// SPDX-License-Identifier: GPL-3.0-or-later

package functions

func (m *Manager) runWorker() {
	for {
		if m.scheduler == nil {
			return
		}
		req, ok := m.scheduler.next()
		if !ok {
			return
		}
		if req == nil || req.fn == nil || req.handler == nil {
			continue
		}
		if req.ctx != nil && req.ctx.Err() != nil {
			continue
		}
		if !m.startInvocation(req.fn.UID) {
			continue
		}

		panicked := false
		func() {
			defer func() {
				if recover() != nil {
					panicked = true
				}
			}()
			req.handler(*req.fn)
		}()

		if panicked {
			m.respUID(req.fn.UID, 500, "function handler panic")
			continue
		}

		m.setInvocationState(req.fn.UID, stateAwaitingResult)
	}
}
