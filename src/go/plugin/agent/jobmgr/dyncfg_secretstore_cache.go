// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

func (m *Manager) affectedJobs(key string) []secretstore.JobRef {
	if m == nil || m.secretStoreDeps == nil {
		return nil
	}

	exposed, _ := m.secretStoreDeps.Impacted(key)
	return exposed
}

// restartableAffectedJobs selects the store's dependents that a store change
// restarts: running or failed jobs referencing it. A dependent whose command
// is in flight is not a special case here - the store command's write claim
// on the dependent's key parks the store command until that work commits,
// and the claim-set recomputation re-selects dependents at that point.
// Loop-owned state (entry status): call only on the run-loop goroutine.
func (m *Manager) restartableAffectedJobs(key string) []secretstore.JobRef {
	if m == nil || m.secretStoreDeps == nil {
		return nil
	}

	exposed, _ := m.secretStoreDeps.Impacted(key)
	refs := make([]secretstore.JobRef, 0, len(exposed))
	for _, job := range exposed {
		entry, ok := m.lookupExposedByFullName(job.ID)
		if !ok {
			continue
		}
		switch entry.Status {
		case dyncfg.StatusRunning, dyncfg.StatusFailed:
			refs = append(refs, job)
		}
	}
	return refs
}
