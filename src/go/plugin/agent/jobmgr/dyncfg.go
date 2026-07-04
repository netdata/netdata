// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (m *Manager) dyncfgConfig(fn dyncfg.Function) {
	// The shutdown check precedes even argument validation: once shutdown
	// begins EVERY non-terminal command answers 503-publish-nothing (one
	// rule), malformed ones included.
	select {
	case <-m.ctx.Done():
		m.dyncfgResponder.SendCodef(fn, 503, "Job manager is shutting down.")
		return
	default:
	}

	if err := fn.ValidateArgs(2); err != nil {
		m.Warningf("dyncfg: %v", err)
		m.dyncfgResponder.SendCodef(fn, 400, "%v", err)
		return
	}

	m.dyncfgQueuedExec(fn)
}

func (m *Manager) dyncfgQueuedExec(fn dyncfg.Function) {
	switch m.dyncfgDomain(fn) {
	case domainSecretStore:
		m.dyncfgSecretStoreExec(fn)
	case domainCollector:
		m.dyncfgCollectorExec(fn)
	case domainVnode:
		m.dyncfgVnodeExec(fn)
	default:
		m.dyncfgRespondUnknown(fn)
	}
}

func (m *Manager) dyncfgRespondUnknown(fn dyncfg.Function) {
	m.dyncfgResponder.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Fn().Name, fn.ID())
}
