// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func (m *Manager) dyncfgSecretStorePrefixValue() string {
	return m.secretsCtl.Prefix()
}

func (m *Manager) dyncfgSecretStoreExec(fn dyncfg.Function) {
	if fn.Command() == dyncfg.CommandSchema {
		m.dyncfgSecretStoreSeqExec(fn)
		return
	}
	m.enqueueDyncfgFunction(fn)
}

func (m *Manager) dyncfgSecretStoreSeqExec(fn dyncfg.Function) {
	m.secretsCtl.SeqExec(fn)
}

func (m *Manager) restartDependentCollectorJob(fullName string) error {
	entry, ok := m.lookupExposedByFullName(fullName)
	if !ok {
		return fmt.Errorf("job '%s' is not exposed", fullName)
	}

	oldStatus := entry.Status
	switch oldStatus {
	case dyncfg.StatusRunning, dyncfg.StatusFailed:
	default:
		return fmt.Errorf("job '%s' restart is not allowed in '%s' state", fullName, oldStatus)
	}

	m.collectorCallbacks.Stop(entry.Cfg)

	if err := m.collectorCallbacks.Start(entry.Cfg); err != nil {
		entry.Status = dyncfg.StatusFailed
		m.collectorHandler.NotifyJobStatus(entry.Cfg, dyncfg.StatusFailed)
		m.collectorCallbacks.OnStatusChange(entry, oldStatus, dyncfg.NewFunction(functions.Function{}))
		return fmt.Errorf("job '%s' restart failed: %w", fullName, err)
	}

	entry.Status = dyncfg.StatusRunning
	m.collectorHandler.NotifyJobStatus(entry.Cfg, dyncfg.StatusRunning)
	m.collectorCallbacks.OnStatusChange(entry, oldStatus, dyncfg.NewFunction(functions.Function{}))
	return nil
}

func (m *Manager) lookupExposedByFullName(fullName string) (*dyncfg.Entry[confgroup.Config], bool) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return nil, false
	}
	return m.collectorExposed.LookupByKey(fullName)
}
