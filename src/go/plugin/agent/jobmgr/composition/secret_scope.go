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

func (scope *processOwnedAtomicScope) Resolve(
	ctx context.Context,
	reference string,
	original string,
) ([]byte, error) {
	if scope == nil {
		return nil, errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.scope == nil {
		return nil, errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	return scope.scope.Resolve(ctx, reference, original)
}

func (scope *processOwnedAtomicScope) Release(ctx context.Context) error {
	if scope == nil {
		return errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	scope.mu.Lock()
	if scope.generation == 0 || scope.scope == nil {
		scope.mu.Unlock()
		return errors.New("jobmgr composition: invalid process-owned Store scope")
	}
	owned := scope.scope
	scope.scope = nil
	scope.mu.Unlock()
	err := owned.Release(ctx)
	if err != nil {
		jobmgr.ObserveDiagnostic(scope.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "secret Store scope release failed",
			Generation: scope.generation,
			Err:        err,
		})
	}
	return err
}
