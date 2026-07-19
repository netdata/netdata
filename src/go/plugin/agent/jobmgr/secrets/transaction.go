// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

type preparedSecretSpec struct {
	scope   lifecycle.ResourceTransactionScope
	current lifecycle.ReadyResource
	permit  lifecycle.LongLivedPermit

	store    *secretstore.SecretStore
	storeKey string
	mutation *secretstore.PreparedSecretMutation
	abort    bool
	remove   bool
	restarts *SecretRestartCommand

	result  lifecycle.SealedResult
	cleanup lifecycle.TaskCleanup

	controller  *Controller
	entry       *secretEntry
	removeEntry bool
}

type preparedSecretTransaction struct {
	mu sync.Mutex

	consumed bool
	spec     preparedSecretSpec
}

func newPreparedSecretTransaction(
	spec preparedSecretSpec,
) (*preparedSecretTransaction, error) {
	if !spec.scope.Valid() ||
		(spec.current == nil) != !spec.scope.Current.Valid() ||
		spec.current != nil &&
			spec.current.Identity() != spec.scope.Current ||
		spec.cleanup == nil ||
		spec.controller == nil {
		return nil, errors.New(
			"jobmgr secrets: invalid prepared transaction",
		)
	}
	if spec.mutation == nil && spec.permit.Valid() &&
		(!spec.scope.Successor.Valid() ||
			spec.permit.Owner() != spec.scope.Successor) {
		return nil, errors.New(
			"jobmgr secrets: no-op permit differs from scope",
		)
	}
	return &preparedSecretTransaction{spec: spec}, nil
}

func (transaction *preparedSecretTransaction) Scope() lifecycle.ResourceTransactionScope {
	if transaction == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return transaction.spec.scope
}

func (transaction *preparedSecretTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	return transaction.apply(ctx, nil)
}

func (transaction *preparedSecretTransaction) ApplyComposite(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	if commands == nil {
		return lifecycle.AppliedResourceTransaction{},
			errors.New("jobmgr secrets: nil composite command scope")
	}
	return transaction.apply(ctx, commands)
}

func (transaction *preparedSecretTransaction) apply(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	spec, err := transaction.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if ctx == nil {
		return lifecycle.AppliedResourceTransaction{},
			errors.New("jobmgr secrets: nil transaction apply context")
	}
	if spec.abort {
		if spec.mutation == nil {
			return lifecycle.AppliedResourceTransaction{},
				errors.New(
					"jobmgr secrets: validation transaction lost its mutation",
				)
		}
		if err := spec.mutation.Abort(ctx); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		spec.mutation = nil
	}
	if spec.mutation == nil {
		if spec.permit.Valid() && !spec.abort {
			if err := spec.permit.AbortUnused(); err != nil {
				return lifecycle.AppliedResourceTransaction{}, err
			}
		}
		if spec.removeEntry {
			spec.controller.commitEntry(spec.storeKey, nil)
		} else if spec.entry != nil {
			spec.controller.commitEntry(
				spec.entry.config.ExposedKey(),
				spec.entry,
			)
		}
		return lifecycle.NewAppliedResourceTransaction(
			spec.scope,
			lifecycle.ResourceTransactionUnchanged,
			spec.current,
			spec.result,
			spec.cleanup,
		)
	}

	commitCalled := false
	commit := func(
		commitCtx context.Context,
	) (secretstore.SecretMutationResult, error) {
		commitCalled = true
		return spec.mutation.Commit(commitCtx)
	}
	var result secretstore.SecretMutationResult
	var message string
	var postCommitErr error
	var predecessorRestored bool
	if spec.restarts != nil && !spec.remove {
		result, message, predecessorRestored, postCommitErr =
			spec.restarts.Apply(
				ctx,
				commands,
				spec.storeKey,
				commit,
			)
	} else {
		result, postCommitErr = commit(ctx)
		predecessorRestored = !result.Retained
	}
	if !result.Applied {
		var abortErr error
		if !commitCalled {
			rollbackCtx := ctx
			if commands != nil {
				var rollbackErr error
				rollbackCtx, rollbackErr =
					commands.RollbackContext()
				postCommitErr = errors.Join(
					postCommitErr,
					rollbackErr,
				)
			}
			if rollbackCtx != nil {
				abortErr = spec.mutation.Abort(rollbackCtx)
			}
		}
		if predecessorRestored && abortErr == nil {
			return lifecycle.NewAppliedResourceTransaction(
				spec.scope,
				lifecycle.ResourceTransactionUnchanged,
				spec.current,
				mustSecretMessage(
					500,
					"Secretstore change was not applied; dependent collectors were restored.",
				),
				func() error { return nil },
			)
		}
		return lifecycle.AppliedResourceTransaction{},
			errors.Join(
				postCommitErr,
				abortErr,
				errors.New(
					"jobmgr secrets: Store mutation was not applied",
				),
			)
	}

	if current, ok := spec.current.(*storeGenerationResource); ok {
		postCommitErr = errors.Join(
			postCommitErr,
			current.supersede(),
		)
	} else if spec.current != nil {
		postCommitErr = errors.Join(
			postCommitErr,
			errors.New(
				"jobmgr secrets: current Store resource type differs",
			),
		)
	}

	var next lifecycle.ReadyResource
	disposition := lifecycle.ResourceTransactionRemoved
	if !spec.remove {
		resource, resourceErr := newStoreGenerationResource(
			spec.scope.Successor,
			spec.store,
			spec.storeKey,
			result.Generation,
		)
		if resourceErr != nil {
			return lifecycle.AppliedResourceTransaction{},
				errors.Join(postCommitErr, resourceErr)
		}
		next = resource
		disposition = lifecycle.ResourceTransactionInstalled
		if spec.scope.Current.Valid() {
			disposition = lifecycle.ResourceTransactionReplaced
		}
	}
	if spec.removeEntry {
		spec.controller.commitEntry(spec.storeKey, nil)
	} else if spec.entry != nil {
		spec.controller.commitEntry(
			spec.entry.config.ExposedKey(),
			spec.entry,
		)
	}
	resultFrame := spec.result
	if message != "" {
		resultFrame = mustSecretMessage(200, message)
	}
	cleanup := spec.cleanup
	if postCommitErr != nil {
		nextCleanup := cleanup
		cleanup = func() error {
			return errors.Join(nextCleanup(), postCommitErr)
		}
	}
	return lifecycle.NewAppliedResourceTransaction(
		spec.scope,
		disposition,
		next,
		resultFrame,
		cleanup,
	)
}

func (transaction *preparedSecretTransaction) Dispose(
	ctx context.Context,
) (lifecycle.ReadyResource, error) {
	spec, err := transaction.take()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New(
			"jobmgr secrets: nil transaction dispose context",
		)
	}
	if spec.mutation != nil {
		err = spec.mutation.Abort(ctx)
	} else if spec.permit.Valid() {
		err = spec.permit.AbortUnused()
	}
	return spec.current, err
}

func (transaction *preparedSecretTransaction) take() (
	preparedSecretSpec,
	error,
) {
	if transaction == nil {
		return preparedSecretSpec{},
			errors.New("jobmgr secrets: nil prepared transaction")
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return preparedSecretSpec{},
			errors.New(
				"jobmgr secrets: prepared transaction consumed",
			)
	}
	transaction.consumed = true
	spec := transaction.spec
	transaction.spec = preparedSecretSpec{}
	return spec, nil
}
