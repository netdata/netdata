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
		return nil, errors.New(
			"jobmgr composition: invalid run-owned Store scope",
		)
	}
	scope, err := stores.AcquireScope(keys)
	if err != nil {
		return nil, err
	}
	return &runOwnedAtomicScope{
		run: run, scope: scope,
	}, nil
}

func (scope *runOwnedAtomicScope) Resolve(
	ctx context.Context,
	reference,
	original string,
) ([]byte, error) {
	if scope == nil || scope.scope == nil {
		return nil, errors.New(
			"jobmgr composition: invalid run-owned Store scope",
		)
	}
	return scope.scope.Resolve(ctx, reference, original)
}

func (scope *runOwnedAtomicScope) Release(
	ctx context.Context,
) error {
	if scope == nil || scope.run == nil || scope.scope == nil {
		return errors.New(
			"jobmgr composition: invalid run-owned Store scope",
		)
	}
	err := scope.scope.Release(ctx)
	if err != nil {
		_ = scope.run.Dirty(err)
	}
	return err
}
