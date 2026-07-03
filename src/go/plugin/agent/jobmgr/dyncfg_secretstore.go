// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
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

	// Secretstore commands run loop-synchronously; a dependent whose key has
	// work in flight is SKIPPED and reported (waiting would self-deadlock:
	// effect completions arrive via this loop) - the report format is the
	// terminal message's existing per-job failure entry. The busy check
	// precedes the status check: a dependent whose enable is still in
	// flight reads accepted but must report as in-progress, not as a state
	// rejection.
	sk := collectorStateKey(entry.Cfg.ExposedKey())
	if !m.executor.tryLockIdleKey(sk) {
		return fmt.Errorf("skipped: operation in progress")
	}
	defer m.executor.unlockIdleKey(sk)

	oldStatus := entry.Status
	switch oldStatus {
	case dyncfg.StatusRunning, dyncfg.StatusFailed:
	default:
		return fmt.Errorf("job '%s' restart is not allowed in '%s' state", fullName, oldStatus)
	}

	m.collectorCallbacks.Stop(context.Background(), entry.Cfg)

	if err := m.collectorCallbacks.Start(context.Background(), entry.Cfg); err != nil {
		entry.Status = dyncfg.StatusFailed
		m.collectorHandler.NotifyConfigStatus(entry.Cfg, dyncfg.StatusFailed)
		m.collectorCallbacks.OnStatusChange(entry, oldStatus, dyncfg.NewFunction(functions.Function{}))
		return fmt.Errorf("job '%s' restart failed: %w", fullName, err)
	}

	entry.Status = dyncfg.StatusRunning
	m.collectorHandler.NotifyConfigStatus(entry.Cfg, dyncfg.StatusRunning)
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
