// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

type processOwnedAtomicScope struct {
	mu sync.Mutex

	generation  uint64
	diagnostics jobmgr.DiagnosticObserver
	scope       secretresolver.AtomicScope
}

func (poas *processOwnedAtomicScope) Resolve(
	ctx context.Context,
	reference string,
	original string,
) ([]byte, error) {
	if poas == nil {
		return nil, errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	poas.mu.Lock()
	defer poas.mu.Unlock()
	if poas.scope == nil {
		return nil, errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	return poas.scope.Resolve(ctx, reference, original)
}

func (poas *processOwnedAtomicScope) Snapshot() secretresolver.AtomicScopeSnapshot {
	if poas == nil {
		return nil
	}
	poas.mu.Lock()
	defer poas.mu.Unlock()
	if poas.scope == nil {
		return nil
	}
	snapshotter, ok := poas.scope.(secretresolver.AtomicScopeSnapshotter)
	if !ok {
		return nil
	}
	return snapshotter.Snapshot()
}

func (poas *processOwnedAtomicScope) Release(ctx context.Context) error {
	if poas == nil {
		return errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	poas.mu.Lock()
	if poas.generation == 0 || poas.scope == nil {
		poas.mu.Unlock()
		return errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	owned := poas.scope
	poas.scope = nil
	poas.mu.Unlock()
	err := owned.Release(ctx)
	if err != nil {
		jobmgr.ObserveDiagnostic(poas.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "secret Store scope release failed",
			Generation: poas.generation,
			Err:        err,
		})
	}
	return err
}
