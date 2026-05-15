// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (m *Manager) dyncfgConfig(fn dyncfg.Function) {
	if err := fn.ValidateArgs(2); err != nil {
		m.Warningf("dyncfg: %v", err)
		m.dyncfgResponder.SendCodef(fn, 400, "%v", err)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgResponder.SendCodef(fn, 503, "Job manager is shutting down.")
		return
	default:
	}

	m.dyncfgQueuedExec(fn)
}

func (m *Manager) dyncfgQueuedExec(fn dyncfg.Function) {
	switch {
	case strings.HasPrefix(fn.ID(), m.dyncfgSecretStorePrefixValue()):
		m.dyncfgSecretStoreExec(fn)
	case strings.HasPrefix(fn.ID(), m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorExec(fn)
	case strings.HasPrefix(fn.ID(), m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeExec(fn)
	default:
		m.dyncfgResponder.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Fn().Name, fn.ID())
	}
}

func (m *Manager) dyncfgSeqExec(fn dyncfg.Function) {
	switch {
	case strings.HasPrefix(fn.ID(), m.dyncfgSecretStorePrefixValue()):
		m.dyncfgSecretStoreSeqExec(fn)
	case strings.HasPrefix(fn.ID(), m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorSeqExec(fn)
	case strings.HasPrefix(fn.ID(), m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeSeqExec(fn)
	default:
		m.dyncfgResponder.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Fn().Name, fn.ID())
	}
}
