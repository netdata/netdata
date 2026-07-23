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
	scope   lifecycle.ResourceTransactionScope // transaction identity triple (current/successor) being applied
	current lifecycle.ReadyResource            // existing ready store resource; nil on install
	permit  lifecycle.LongLivedPermit          // long-lived resource/external ownership for the successor generation

	store    *secretstore.SecretStore            // target secret store
	storeKey string                              // the store's kind_name key
	mutation *secretstore.PreparedSecretMutation // prepared store mutation; nil means a no-op transaction
	abort    bool                                // commit-less path: abort the mutation (validation-only failure)
	remove   bool                                // removal transaction (no successor resource installed)
	restarts *SecretRestartCommand               // dependent stop->commit->start orchestrator; nil skips restarts

	result  lifecycle.SealedResult // sealed dyncfg response
	cleanup lifecycle.TaskCleanup  // post-commit protocol emit

	controller *Controller  // controller to publish the entry into
	entry      *secretEntry // the entry to commit
}

// commitEntryDisposition applies the entry side of a committed transaction:
// delete the entry on removal, publish the committed entry, or do nothing.
func (spec preparedSecretSpec) commitEntryDisposition() {
	if spec.remove {
		spec.controller.commitEntry(spec.storeKey, nil)
	} else if spec.entry != nil {
		spec.controller.commitEntry(spec.entry.config.ExposedKey(), spec.entry)
	}
}

type preparedSecretTransaction struct {
	mu sync.Mutex

	consumed bool
	spec     preparedSecretSpec
}

func newPreparedSecretTransaction(spec preparedSecretSpec) (*preparedSecretTransaction, error) {
	if !spec.scope.Valid() ||
		(spec.current == nil) == spec.scope.Current.Valid() ||
		spec.current != nil &&
			spec.current.Identity() != spec.scope.Current ||
		spec.cleanup == nil ||
		spec.controller == nil {
		return nil, errors.New("jobmgr secrets: invalid prepared transaction")
	}
	if spec.mutation == nil && spec.permit.Valid() &&
		(!spec.scope.Successor.Valid() ||
			spec.permit.Owner() != spec.scope.Successor) {
		return nil, errors.New("jobmgr secrets: no-op permit differs from scope")
	}
	return &preparedSecretTransaction{
		spec: spec,
	}, nil
}

func (pst *preparedSecretTransaction) Scope() lifecycle.ResourceTransactionScope {
	if pst == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	pst.mu.Lock()
	defer pst.mu.Unlock()
	if pst.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return pst.spec.scope
}

func (pst *preparedSecretTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	return pst.apply(ctx, nil)
}

func (pst *preparedSecretTransaction) ApplyComposite(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	if commands == nil {
		return lifecycle.AppliedResourceTransaction{}, errors.New("jobmgr secrets: nil composite command scope")
	}
	return pst.apply(ctx, commands)
}

func (pst *preparedSecretTransaction) apply(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	spec, err := pst.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if ctx == nil {
		return lifecycle.AppliedResourceTransaction{}, errors.New("jobmgr secrets: nil transaction apply context")
	}
	if spec.abort {
		if spec.mutation == nil {
			return lifecycle.AppliedResourceTransaction{},
				errors.New("jobmgr secrets: validation transaction lost its mutation")
		}
		if err := spec.mutation.Abort(); err != nil {
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
		spec.commitEntryDisposition()
		return lifecycle.NewAppliedResourceTransaction(
			spec.scope,
			lifecycle.ResourceTransactionUnchanged,
			spec.current,
			spec.result,
			spec.cleanup,
		)
	}

	commitCalled := false
	commit := func(commitCtx context.Context) (secretstore.SecretMutationResult, error) {
		commitCalled = true
		return spec.mutation.Commit(commitCtx)
	}
	var result secretstore.SecretMutationResult
	var message string
	var postCommitErr error
	var predecessorRestored bool
	if spec.restarts != nil && !spec.remove {
		result, message, predecessorRestored, postCommitErr = spec.restarts.Apply(ctx, commands, spec.storeKey, commit)
	} else {
		result, postCommitErr = commit(ctx)
		predecessorRestored = !result.Retained
	}
	if !result.Applied {
		var abortErr error
		if !commitCalled {
			if commands != nil {
				_, rollbackErr := commands.RollbackContext()
				postCommitErr = errors.Join(postCommitErr, rollbackErr)
			}
			abortErr = spec.mutation.Abort()
		}
		if predecessorRestored && abortErr == nil {
			return lifecycle.NewAppliedResourceTransaction(
				spec.scope,
				lifecycle.ResourceTransactionUnchanged,
				spec.current,
				mustSecretMessage(500, "Secretstore change was not applied; dependent collectors were restored."),
				func() error { return nil },
			)
		}
		return lifecycle.AppliedResourceTransaction{},
			errors.Join(postCommitErr, abortErr, errors.New("jobmgr secrets: Store mutation was not applied"))
	}

	if current, ok := spec.current.(*storeGenerationResource); ok {
		postCommitErr = errors.Join(postCommitErr, current.supersede())
	} else if spec.current != nil {
		postCommitErr = errors.Join(postCommitErr, errors.New("jobmgr secrets: current Store resource type differs"))
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
			return lifecycle.AppliedResourceTransaction{}, errors.Join(postCommitErr, resourceErr)
		}
		next = resource
		disposition = lifecycle.ResourceTransactionInstalled
		if spec.scope.Current.Valid() {
			disposition = lifecycle.ResourceTransactionReplaced
		}
	}
	spec.commitEntryDisposition()
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
	return lifecycle.NewAppliedResourceTransaction(spec.scope, disposition, next, resultFrame, cleanup)
}

func (pst *preparedSecretTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	spec, err := pst.take()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New("jobmgr secrets: nil transaction dispose context")
	}
	if spec.mutation != nil {
		err = spec.mutation.Abort()
	} else if spec.permit.Valid() {
		err = spec.permit.AbortUnused()
	}
	return spec.current, err
}

func (pst *preparedSecretTransaction) take() (preparedSecretSpec, error) {
	if pst == nil {
		return preparedSecretSpec{}, errors.New("jobmgr secrets: nil prepared transaction")
	}
	pst.mu.Lock()
	defer pst.mu.Unlock()
	if pst.consumed {
		return preparedSecretSpec{}, errors.New("jobmgr secrets: prepared transaction consumed")
	}
	pst.consumed = true
	spec := pst.spec
	pst.spec = preparedSecretSpec{}
	return spec, nil
}
