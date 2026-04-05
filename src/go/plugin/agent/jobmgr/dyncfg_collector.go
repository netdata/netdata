// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	dyncfgCollectorPrefixf = "%s:collector:"
	dyncfgCollectorPath    = "/collectors/%s/Jobs"
)

func (m *Manager) dyncfgCollectorPrefixValue() string {
	return fmt.Sprintf(dyncfgCollectorPrefixf, m.pluginName)
}

func (m *Manager) dyncfgModID(name string) string {
	return fmt.Sprintf("%s%s", m.dyncfgCollectorPrefixValue(), name)
}

func (m *Manager) dyncfgJobID(cfg confgroup.Config) string {
	return fmt.Sprintf("%s%s:%s", m.dyncfgCollectorPrefixValue(), cfg.Module(), cfg.Name())
}

func dyncfgCollectorModCmds() string {
	return dyncfg.JoinCommands(
		dyncfg.CommandAdd,
		dyncfg.CommandSchema,
		dyncfg.CommandEnable,
		dyncfg.CommandDisable,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig)
}

func (m *Manager) dyncfgCollectorModuleCreate(name string) {
	m.dyncfgResponder.ConfigCreate(netdataapi.ConfigOpts{
		ID:                m.dyncfgModID(name),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgCollectorPath, m.pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgCollectorModCmds(),
	})
}

// exposedLookupByName looks up an exposed config by module + job name.
func (m *Manager) exposedLookupByName(module, job string) (*dyncfg.Entry[confgroup.Config], bool) {
	key := module + "_" + job
	if module == job {
		key = job
	}
	return m.collectorExposed.LookupByKey(key)
}

func (m *Manager) dyncfgCollectorExec(fn dyncfg.Function) {
	switch fn.Command() {
	case dyncfg.CommandUserconfig:
		m.dyncfgCmdUserconfig(fn)
		return
	case dyncfg.CommandSchema:
		m.dyncfgCmdSchema(fn)
		return
	}
	m.enqueueDyncfgFunction(fn)
}

func (m *Manager) dyncfgCollectorSeqExec(fn dyncfg.Function) {
	cmd := fn.Command()
	m.collectorHandler.SyncDecision(fn)

	switch cmd {
	case dyncfg.CommandAdd:
		m.collectorHandler.CmdAdd(fn)
		m.syncSecretStoreDepsByFunction(fn)
	case dyncfg.CommandUpdate:
		m.collectorHandler.CmdUpdate(fn)
		m.syncSecretStoreDepsByFunction(fn)
	case dyncfg.CommandEnable:
		m.collectorHandler.CmdEnable(fn)
	case dyncfg.CommandDisable:
		m.collectorHandler.CmdDisable(fn)
	case dyncfg.CommandRemove:
		m.collectorHandler.CmdRemove(fn)
		m.syncSecretStoreDepsByFunction(fn)
	case dyncfg.CommandRestart:
		m.collectorHandler.CmdRestart(fn)
	case dyncfg.CommandTest:
		m.dyncfgCmdTest(fn)
	case dyncfg.CommandSchema:
		m.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		m.dyncfgCmdGet(fn)
	default:
		m.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, cmd)
		m.dyncfgResponder.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, cmd)
	}
}
