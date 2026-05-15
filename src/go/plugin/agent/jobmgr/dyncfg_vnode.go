// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func (m *Manager) dyncfgVnodePrefixValue() string {
	return m.vnodesCtl.Prefix()
}

func (m *Manager) dyncfgVnodeExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandSchema, dyncfg.CommandUserconfig:
		m.dyncfgVnodeSeqExec(fn)
		return
	}
	m.enqueueDyncfgFunction(fn)
}

func (m *Manager) dyncfgVnodeSeqExec(fn dyncfg.Function) {
	m.vnodesCtl.SeqExec(fn)
}

func (m *Manager) affectedVnodeJobs(vnode string) []string {
	var jobs []string
	m.collectorExposed.ForEach(func(_ string, entry *dyncfg.Entry[confgroup.Config]) bool {
		if entry.Cfg.Vnode() == vnode {
			jobs = append(jobs, fmt.Sprintf("%s:%s", entry.Cfg.Module(), entry.Cfg.Name()))
		}
		return true
	})
	return jobs
}

func (m *Manager) applyVnodeUpdate(name string, cfg *vnodes.VirtualNode) {
	for _, job := range m.runningJobs.snapshot() {
		if job.Vnode().Name == name {
			job.UpdateVnode(cfg)
		}
	}
}
