// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

type runOwnedAtomicScope struct {
	run   *lifecycle.RunSupervisor
	scope secretresolver.AtomicScope
}

func acquireRunOwnedStoreScope(
	run *lifecycle.RunSupervisor,
	stores *secretstore.SecretStore,
	keys []string,
) (secretresolver.AtomicScope, error) {
	if run == nil || stores == nil {
		return nil, errors.New("jobmgr composition: invalid run-owned Store scope")
	}
	scope, err := stores.AcquireScope(keys)
	if err != nil {
		return nil, err
	}
	return &runOwnedAtomicScope{run: run, scope: scope}, nil
}

func (roas *runOwnedAtomicScope) Resolve(ctx context.Context, reference, original string) ([]byte, error) {
	if roas == nil || roas.scope == nil {
		return nil, errors.New("jobmgr composition: invalid run-owned Store scope")
	}
	return roas.scope.Resolve(ctx, reference, original)
}

func (roas *runOwnedAtomicScope) Release(ctx context.Context) error {
	if roas == nil || roas.run == nil || roas.scope == nil {
		return errors.New("jobmgr composition: invalid run-owned Store scope")
	}
	err := roas.scope.Release(ctx)
	if err != nil {
		roas.run.Dirty(err)
	}
	return err
}
