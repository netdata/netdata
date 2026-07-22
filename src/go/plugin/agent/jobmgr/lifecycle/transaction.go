// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
)

// ResourceTransactionScope seals the exact current and optional successor
// generations that one operation may transition.
type ResourceTransactionScope struct {
	ID        string
	Current   ResourceIdentity
	Successor ResourceIdentity
}

func (rts ResourceTransactionScope) Valid() bool {
	if rts.ID == "" || !rts.Current.Valid() && rts.Current != (ResourceIdentity{}) {
		return false
	}
	if !rts.Successor.Valid() && rts.Successor != (ResourceIdentity{}) {
		return false
	}
	if rts.Current.Valid() && rts.Current.ID != rts.ID {
		return false
	}
	if rts.Successor.Valid() && rts.Successor.ID != rts.ID {
		return false
	}
	return true
}

type ResourceTransactionDisposition uint8

const (
	ResourceTransactionUnchanged ResourceTransactionDisposition = iota + 1
	ResourceTransactionRemoved
	ResourceTransactionInstalled
	ResourceTransactionReplaced
)

func (rtd ResourceTransactionDisposition) Valid() bool {
	switch rtd {
	case ResourceTransactionUnchanged,
		ResourceTransactionRemoved,
		ResourceTransactionInstalled,
		ResourceTransactionReplaced:
		return true
	default:
		return false
	}
}

// PreparedResourceTransaction owns an unpublished graph/resource postimage.
// Dispose must return the exact untouched current resource, if one existed.
type PreparedResourceTransaction interface {
	Scope() ResourceTransactionScope
	Apply(context.Context) (AppliedResourceTransaction, error)
	Dispose(context.Context) (ReadyResource, error)
}

type AppliedResourceTransaction struct {
	scope       ResourceTransactionScope
	disposition ResourceTransactionDisposition
	current     ReadyResource
	result      SealedResult
	cleanup     TaskCleanup
}

// ResultStatus exposes only the response status needed for operational
// classification; the sealed response body remains owned by the transaction.
func (art AppliedResourceTransaction) ResultStatus() int {
	return art.result.status
}

func NewAppliedResourceTransaction(
	scope ResourceTransactionScope,
	disposition ResourceTransactionDisposition,
	current ReadyResource,
	result SealedResult,
	cleanup TaskCleanup,
) (AppliedResourceTransaction, error) {
	applied := AppliedResourceTransaction{
		scope:       scope,
		disposition: disposition,
		current:     current,
		result:      result,
		cleanup:     cleanup,
	}
	if !scope.Valid() || !disposition.Valid() || cleanup == nil {
		return AppliedResourceTransaction{}, errors.New("jobmgr lifecycle: invalid applied resource transaction")
	}
	outcome, err := appliedResourceTransactionOutcome(applied)
	if err != nil {
		return AppliedResourceTransaction{}, err
	}
	if outcome.kind != TaskOutcomeAppliedResourceTransaction {
		return AppliedResourceTransaction{}, errors.New(
			"jobmgr lifecycle: applied resource transaction did not seal",
		)
	}
	return applied, nil
}

func preparedResourceTransactionScope(
	transaction PreparedResourceTransaction,
) (scope ResourceTransactionScope, err error) {
	if transaction == nil {
		return ResourceTransactionScope{}, errors.New("jobmgr lifecycle: nil prepared resource transaction")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			scope = ResourceTransactionScope{}
			err = fmt.Errorf("%w in prepared resource transaction scope: %v", ErrTaskPanic, recovered)
		}
	}()
	scope = transaction.Scope()
	if !scope.Valid() {
		return ResourceTransactionScope{}, errors.New(
			"jobmgr lifecycle: invalid prepared resource transaction scope",
		)
	}
	return scope, nil
}
