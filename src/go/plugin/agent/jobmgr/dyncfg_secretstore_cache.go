// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"strings"

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

type secretStoreRestartFailure struct {
	ref secretstore.JobRef
	err error
}

func (m *Manager) restartDependentJobs(key string) string {
	failures := m.restartDependentJobsBestEffort(key)
	if len(failures) == 0 {
		return ""
	}

	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		name := failure.ref.Display
		if name == "" {
			name = failure.ref.ID
		}
		parts = append(parts, fmt.Sprintf("%s (%v)", name, failure.err))
	}

	return fmt.Sprintf("Secretstore change applied, but dependent collector restarts failed: %s.", strings.Join(parts, "; "))
}

func (m *Manager) restartDependentJobsBestEffort(key string) []secretStoreRestartFailure {
	if m == nil {
		return nil
	}

	var failures []secretStoreRestartFailure
	for _, job := range m.restartableAffectedJobs(key) {
		if err := m.restartDependentCollectorJob(job.ID); err != nil {
			m.Warningf("dyncfg: secretstore: failed to restart dependent job '%s' after store '%s' change: %v", job.ID, key, err)
			failures = append(failures, secretStoreRestartFailure{ref: job, err: err})
		}
	}
	return failures
}
