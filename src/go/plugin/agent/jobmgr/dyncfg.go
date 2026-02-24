// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// dyncfgConfigHandler wraps dyncfgConfig to convert functions.Function to dyncfg.Function.
// This is needed because functions.Registry expects func(functions.Function).
func (m *Manager) dyncfgConfigHandler(fn functions.Function) {
	m.dyncfgConfig(dyncfg.NewFunction(fn))
}

func (m *Manager) dyncfgConfig(fn dyncfg.Function) {
	if err := fn.ValidateArgs(2); err != nil {
		m.Warningf("dyncfg: %v", err)
		m.dyncfgApi.SendCodef(fn, 400, "%v", err)
		return
	}

	select {
	case <-m.ctx.Done():
		m.dyncfgApi.SendCodef(fn, 503, "Job manager is shutting down.")
		return
	default:
	}

	m.dyncfgQueuedExec(fn)
}

func (m *Manager) dyncfgQueuedExec(fn dyncfg.Function) {
	id := fn.ID()

	switch {
	case strings.HasPrefix(id, m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorExec(fn)
	case strings.HasPrefix(id, m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeExec(fn)
	default:
		m.dyncfgApi.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Fn().Name, id)
	}
}

func (m *Manager) dyncfgSeqExec(fn dyncfg.Function) {
	id := fn.ID()

	switch {
	case strings.HasPrefix(id, m.dyncfgCollectorPrefixValue()):
		m.dyncfgCollectorSeqExec(fn)
	case strings.HasPrefix(id, m.dyncfgVnodePrefixValue()):
		m.dyncfgVnodeSeqExec(fn)
	default:
		m.dyncfgApi.SendCodef(fn, 503, "unknown function '%s' (%s).", fn.Fn().Name, id)
	}
}
