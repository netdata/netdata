// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
)

type TaskOutcomeKind uint8

const (
	TaskOutcomeNone TaskOutcomeKind = iota
	TaskOutcomeFrame
	TaskOutcomeReadyResource
	TaskOutcomePreparedResourceTransaction
	TaskOutcomeAppliedResourceTransaction
)

type TaskOutcome struct {
	kind        TaskOutcomeKind                // discriminant selecting which payload field is set
	frame       SealedResult                   // sealed result (Frame / applied transaction) payload
	ready       ReadyResource                  // ready-resource payload
	transaction PreparedResourceTransaction    // prepared-resource-transaction payload
	scope       ResourceTransactionScope       // transaction scope (transaction kinds)
	scopeSet    bool                           // distinguishes a zero scope from unset
	disposition ResourceTransactionDisposition // transaction disposition (applied variant)
	identity    ResourceIdentity               // resource identity for kinds that carry one
}

func NoValueOutcome() TaskOutcome {
	return TaskOutcome{}
}

func NewFrameOutcome(result SealedResult) (TaskOutcome, error) {
	if err := result.validate(); err != nil {
		return TaskOutcome{}, err
	}
	return TaskOutcome{kind: TaskOutcomeFrame, frame: result}, nil
}

func preparedResourceTransactionOutcome(
	transaction PreparedResourceTransaction,
	scope ResourceTransactionScope,
) (TaskOutcome, error) {
	outcome := TaskOutcome{
		kind:        TaskOutcomePreparedResourceTransaction,
		transaction: transaction,
		scope:       scope,
		scopeSet:    true,
	}
	return outcome, outcome.validate()
}

func appliedResourceTransactionOutcome(
	applied AppliedResourceTransaction,
) (TaskOutcome, error) {
	outcome := TaskOutcome{
		kind:        TaskOutcomeAppliedResourceTransaction,
		frame:       applied.result,
		ready:       applied.current,
		scope:       applied.scope,
		scopeSet:    true,
		disposition: applied.disposition,
	}
	if applied.current != nil {
		identity, err := readyResourceIdentity(applied.current)
		if err != nil {
			return TaskOutcome{}, err
		}
		outcome.identity = identity
	}
	return outcome, outcome.validate()
}

func readyResourceOutcome(resource ReadyResource, identity ResourceIdentity) (TaskOutcome, error) {
	if resource == nil || !identity.Valid() {
		return TaskOutcome{}, errors.New("jobmgr lifecycle: invalid known ready resource")
	}
	return TaskOutcome{kind: TaskOutcomeReadyResource, ready: resource, identity: identity}, nil
}

func (to TaskOutcome) Kind() TaskOutcomeKind {
	return to.kind
}

func (to TaskOutcome) ReadyResource() (ReadyResource, bool) {
	return to.ready, to.kind == TaskOutcomeReadyResource && to.ready != nil
}

func (to TaskOutcome) ResourceIdentity() (ResourceIdentity, bool) {
	return to.identity, (to.kind == TaskOutcomeReadyResource ||
		to.kind == TaskOutcomeAppliedResourceTransaction) &&
		to.identity.Valid()
}

func (to TaskOutcome) validate() error {
	switch to.kind {
	case TaskOutcomeNone:
		if !emptySealedResult(to.frame) ||
			to.ready != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: nonempty no-value outcome")
		}
	case TaskOutcomeFrame:
		if to.ready != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: mixed frame outcome")
		}
		return to.frame.validate()
	case TaskOutcomeReadyResource:
		if !emptySealedResult(to.frame) ||
			to.ready == nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			!to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid ready resource outcome")
		}
	case TaskOutcomePreparedResourceTransaction:
		if !emptySealedResult(to.frame) ||
			to.ready != nil ||
			to.transaction == nil ||
			!to.scopeSet ||
			!to.scope.Valid() ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid prepared resource transaction outcome")
		}
	case TaskOutcomeAppliedResourceTransaction:
		if to.transaction != nil ||
			!to.scopeSet ||
			!to.scope.Valid() ||
			!to.disposition.Valid() {
			return errors.New("jobmgr lifecycle: invalid applied resource transaction outcome")
		}
		if err := to.frame.validate(); err != nil {
			return err
		}
		switch to.disposition {
		case ResourceTransactionUnchanged:
			if to.scope.Current.Valid() {
				if to.ready == nil || to.identity != to.scope.Current {
					return errors.New("jobmgr lifecycle: unchanged transaction lost current resource")
				}
			} else if to.ready != nil || to.identity.Valid() {
				return errors.New("jobmgr lifecycle: empty unchanged transaction installed a resource")
			}
		case ResourceTransactionRemoved:
			if !to.scope.Current.Valid() ||
				to.ready != nil ||
				to.identity.Valid() {
				return errors.New("jobmgr lifecycle: invalid removed transaction result")
			}
		case ResourceTransactionInstalled:
			if to.scope.Current.Valid() ||
				!to.scope.Successor.Valid() ||
				to.ready == nil ||
				to.identity != to.scope.Successor {
				return errors.New("jobmgr lifecycle: invalid installed transaction result")
			}
		case ResourceTransactionReplaced:
			if !to.scope.Current.Valid() ||
				!to.scope.Successor.Valid() ||
				to.ready == nil ||
				to.identity != to.scope.Successor {
				return errors.New("jobmgr lifecycle: invalid replaced transaction result")
			}
		}
	default:
		return errors.New("jobmgr lifecycle: unknown task outcome")
	}
	return nil
}

func (to TaskOutcome) empty() bool {
	return to.kind == TaskOutcomeNone &&
		emptySealedResult(to.frame) &&
		to.ready == nil &&
		to.transaction == nil &&
		!to.scopeSet &&
		to.disposition == 0 &&
		!to.identity.Valid()
}

func readyResourceIdentity(resource ReadyResource) (identity ResourceIdentity, err error) {
	if resource == nil {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: nil ready resource")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			identity = ResourceIdentity{}
			err = fmt.Errorf(
				"%w in ready resource identity: %v",
				ErrTaskPanic,
				recovered,
			)
		}
	}()
	identity = resource.Identity()
	if !identity.Valid() {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: invalid ready resource identity")
	}
	return identity, nil
}
