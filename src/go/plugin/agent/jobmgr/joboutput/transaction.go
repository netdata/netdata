// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type ResourceTransactionSpec struct {
	Scope        lifecycle.ResourceTransactionScope
	Disposition  lifecycle.ResourceTransactionDisposition
	Current      lifecycle.ReadyResource
	Successor    lifecycle.PreparedResource
	UnusedPermit lifecycle.LongLivedPermit
	Graph        *dyncfg.Graph
	Mutation     dyncfg.GraphMutation
	Result       lifecycle.SealedResult
	Cleanup      lifecycle.TaskCleanup
}

// PreparedResourceTransaction owns one unpublished graph postimage and the
// optional prepared successor job until Apply or Dispose consumes both.
type PreparedResourceTransaction struct {
	mu sync.Mutex

	consumed bool
	spec     ResourceTransactionSpec
}

type PreparedNoopResourceTransaction struct {
	mu sync.Mutex

	consumed bool
	scope    lifecycle.ResourceTransactionScope
	current  lifecycle.ReadyResource
	permit   lifecycle.LongLivedPermit
	result   lifecycle.SealedResult
	cleanup  lifecycle.TaskCleanup
}

func PrepareNoopResourceTransaction(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
) (*PreparedNoopResourceTransaction, error) {
	if !scope.Valid() ||
		(current == nil) != !scope.Current.Valid() ||
		current != nil && current.Identity() != scope.Current ||
		cleanup == nil {
		return nil, errors.New("job output: invalid no-op transaction")
	}
	if scope.Successor.Valid() {
		if !permit.Valid() ||
			permit.Owner() != scope.Successor ||
			permit.Class() != lifecycle.LongLivedJob {
			return nil, errors.New(
				"job output: no-op transaction has invalid successor permit",
			)
		}
	} else if permit.Valid() {
		return nil, errors.New(
			"job output: no-op transaction has an unexpected permit",
		)
	}
	return &PreparedNoopResourceTransaction{
		scope: scope, current: current, permit: permit,
		result: result, cleanup: cleanup,
	}, nil
}

func (transaction *PreparedNoopResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	if transaction == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return transaction.scope
}

func (transaction *PreparedNoopResourceTransaction) Apply(
	context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	scope, current, permit, result, cleanup, err := transaction.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if permit.Valid() {
		if err := permit.AbortUnused(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	return lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		current,
		result,
		cleanup,
	)
}

func (transaction *PreparedNoopResourceTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	_, current, permit, _, _, err := transaction.take()
	if err != nil {
		return nil, err
	}
	if permit.Valid() {
		err = permit.AbortUnused()
	}
	return current, err
}

func (transaction *PreparedNoopResourceTransaction) take() (
	lifecycle.ResourceTransactionScope,
	lifecycle.ReadyResource,
	lifecycle.LongLivedPermit,
	lifecycle.SealedResult,
	lifecycle.TaskCleanup,
	error,
) {
	if transaction == nil {
		return lifecycle.ResourceTransactionScope{},
			nil,
			lifecycle.LongLivedPermit{},
			lifecycle.SealedResult{},
			nil,
			errors.New("job output: nil no-op transaction")
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return lifecycle.ResourceTransactionScope{},
			nil,
			lifecycle.LongLivedPermit{},
			lifecycle.SealedResult{},
			nil,
			errors.New("job output: no-op transaction consumed")
	}
	transaction.consumed = true
	return transaction.scope,
		transaction.current,
		transaction.permit,
		transaction.result,
		transaction.cleanup,
		nil
}

func PrepareResourceTransaction(
	spec ResourceTransactionSpec,
) (*PreparedResourceTransaction, error) {
	if err := validateResourceTransactionSpec(spec); err != nil {
		return nil, err
	}
	return &PreparedResourceTransaction{spec: spec}, nil
}

func (transaction *PreparedResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	if transaction == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return transaction.spec.Scope
}

func (transaction *PreparedResourceTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	spec, err := transaction.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if ctx == nil {
		return lifecycle.AppliedResourceTransaction{},
			errors.New("job output: nil transaction apply context")
	}

	switch spec.Disposition {
	case lifecycle.ResourceTransactionRemoved,
		lifecycle.ResourceTransactionReplaced:
		if err := spec.Current.Stop(ctx); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if err := spec.Current.Finalize(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}

	var current lifecycle.ReadyResource
	switch spec.Disposition {
	case lifecycle.ResourceTransactionUnchanged:
		if spec.UnusedPermit.Valid() {
			if err := spec.UnusedPermit.AbortUnused(); err != nil {
				return lifecycle.AppliedResourceTransaction{}, err
			}
		}
		current = spec.Current
	case lifecycle.ResourceTransactionInstalled,
		lifecycle.ResourceTransactionReplaced:
		current, err = spec.Successor.AcceptStart(
			ctx,
			spec.Scope.Successor.Generation,
		)
		if err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if err := current.Publish(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	if spec.Graph != nil {
		if err := spec.Graph.Commit(spec.Mutation); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	return lifecycle.NewAppliedResourceTransaction(
		spec.Scope,
		spec.Disposition,
		current,
		spec.Result,
		spec.Cleanup,
	)
}

func (transaction *PreparedResourceTransaction) Dispose(
	ctx context.Context,
) (lifecycle.ReadyResource, error) {
	spec, err := transaction.take()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New("job output: nil transaction dispose context")
	}
	var abortErr error
	if spec.Graph != nil {
		abortErr = spec.Graph.Abort(spec.Mutation)
	}
	var successorErr error
	if spec.Successor != nil {
		successorErr = spec.Successor.Dispose(ctx)
	} else if spec.UnusedPermit.Valid() {
		successorErr = spec.UnusedPermit.AbortUnused()
	}
	return spec.Current, errors.Join(abortErr, successorErr)
}

func (transaction *PreparedResourceTransaction) take() (
	ResourceTransactionSpec,
	error,
) {
	if transaction == nil {
		return ResourceTransactionSpec{},
			errors.New("job output: nil prepared resource transaction")
	}
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.consumed {
		return ResourceTransactionSpec{},
			errors.New("job output: prepared resource transaction consumed")
	}
	transaction.consumed = true
	spec := transaction.spec
	transaction.spec = ResourceTransactionSpec{}
	return spec, nil
}

func validateResourceTransactionSpec(spec ResourceTransactionSpec) error {
	if !spec.Scope.Valid() || spec.Cleanup == nil {
		return errors.New("job output: invalid resource transaction")
	}
	if spec.Current != nil &&
		spec.Current.Identity() != spec.Scope.Current {
		return errors.New(
			"job output: transaction current differs from scope",
		)
	}
	if spec.Successor != nil &&
		spec.Successor.Identity() != spec.Scope.Successor {
		return errors.New(
			"job output: transaction successor differs from scope",
		)
	}
	if spec.UnusedPermit.Valid() &&
		(spec.UnusedPermit.Owner() != spec.Scope.Successor ||
			spec.UnusedPermit.Class() != lifecycle.LongLivedJob) {
		return errors.New(
			"job output: transaction unused permit differs from scope",
		)
	}
	if spec.Successor != nil && spec.UnusedPermit.Valid() {
		return errors.New(
			"job output: transaction owns both successor and unused permit",
		)
	}
	switch spec.Disposition {
	case lifecycle.ResourceTransactionUnchanged:
		if spec.Successor != nil ||
			(spec.Current == nil) != !spec.Scope.Current.Valid() {
			return errors.New(
				"job output: invalid unchanged transaction",
			)
		}
		if spec.Scope.Successor.Valid() != spec.UnusedPermit.Valid() {
			return errors.New(
				"job output: unchanged transaction lost successor permit",
			)
		}
	case lifecycle.ResourceTransactionRemoved:
		if spec.Current == nil ||
			!spec.Scope.Current.Valid() ||
			spec.Successor != nil ||
			spec.UnusedPermit.Valid() ||
			spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid remove transaction")
		}
	case lifecycle.ResourceTransactionInstalled:
		if spec.Current != nil ||
			spec.Scope.Current.Valid() ||
			spec.Successor == nil ||
			spec.UnusedPermit.Valid() ||
			!spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid install transaction")
		}
	case lifecycle.ResourceTransactionReplaced:
		if spec.Current == nil ||
			!spec.Scope.Current.Valid() ||
			spec.Successor == nil ||
			spec.UnusedPermit.Valid() ||
			!spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid replace transaction")
		}
	default:
		return errors.New(
			"job output: unknown resource transaction disposition",
		)
	}
	return nil
}
