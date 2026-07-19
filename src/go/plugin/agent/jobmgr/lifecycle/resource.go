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
	TaskOutcomePreparedResource
	TaskOutcomeReadyResource
	TaskOutcomePreparedCapability
	TaskOutcomePreparedResourceTransaction
	TaskOutcomeAppliedResourceTransaction
)

type TaskOutcome struct {
	kind        TaskOutcomeKind
	frame       SealedResult
	prepared    PreparedResource
	ready       ReadyResource
	capability  PreparedCapability
	transaction PreparedResourceTransaction
	scope       ResourceTransactionScope
	scopeSet    bool
	disposition ResourceTransactionDisposition
	identity    ResourceIdentity
}

func PreparedCapabilityOutcome(capability PreparedCapability) (TaskOutcome, error) {
	identity, err := preparedCapabilityIdentity(capability)
	if err != nil {
		return TaskOutcome{}, err
	}
	return preparedCapabilityOutcome(capability, identity)
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

func PreparedResourceOutcome(resource PreparedResource) (TaskOutcome, error) {
	identity, err := preparedResourceIdentity(resource)
	if err != nil {
		return TaskOutcome{}, err
	}
	return preparedResourceOutcome(resource, identity)
}

func preparedResourceOutcome(resource PreparedResource, identity ResourceIdentity) (TaskOutcome, error) {
	outcome := TaskOutcome{kind: TaskOutcomePreparedResource, prepared: resource, identity: identity}
	return outcome, outcome.validate()
}

func preparedCapabilityOutcome(capability PreparedCapability, identity ResourceIdentity) (TaskOutcome, error) {
	outcome := TaskOutcome{kind: TaskOutcomePreparedCapability, capability: capability, identity: identity}
	return outcome, outcome.validate()
}

func ReadyResourceOutcome(resource ReadyResource) (TaskOutcome, error) {
	identity, err := readyResourceIdentity(resource)
	if err != nil {
		return TaskOutcome{}, err
	}
	return readyResourceOutcome(resource, identity)
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
	return to.identity, (to.kind == TaskOutcomePreparedResource ||
		to.kind == TaskOutcomeReadyResource ||
		to.kind == TaskOutcomePreparedCapability ||
		to.kind == TaskOutcomeAppliedResourceTransaction) &&
		to.identity.Valid()
}

func (to TaskOutcome) validate() error {
	switch to.kind {
	case TaskOutcomeNone:
		if !emptySealedResult(to.frame) ||
			to.prepared != nil ||
			to.ready != nil ||
			to.capability != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: nonempty no-value outcome")
		}
	case TaskOutcomeFrame:
		if to.prepared != nil ||
			to.ready != nil ||
			to.capability != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: mixed frame outcome")
		}
		return to.frame.validate()
	case TaskOutcomePreparedResource:
		if !emptySealedResult(to.frame) ||
			to.prepared == nil ||
			to.ready != nil ||
			to.capability != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			!to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid prepared resource outcome")
		}
	case TaskOutcomeReadyResource:
		if !emptySealedResult(to.frame) ||
			to.prepared != nil ||
			to.ready == nil ||
			to.capability != nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			!to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid ready resource outcome")
		}
	case TaskOutcomePreparedCapability:
		if !emptySealedResult(to.frame) ||
			to.prepared != nil ||
			to.ready != nil ||
			to.capability == nil ||
			to.transaction != nil ||
			to.scopeSet ||
			to.disposition != 0 ||
			!to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid prepared capability outcome")
		}
	case TaskOutcomePreparedResourceTransaction:
		if !emptySealedResult(to.frame) ||
			to.prepared != nil ||
			to.ready != nil ||
			to.capability != nil ||
			to.transaction == nil ||
			!to.scopeSet ||
			!to.scope.Valid() ||
			to.disposition != 0 ||
			to.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid prepared resource transaction outcome")
		}
	case TaskOutcomeAppliedResourceTransaction:
		if to.prepared != nil ||
			to.capability != nil ||
			to.transaction != nil ||
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
				to.scope.Successor.Valid() ||
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
		to.prepared == nil &&
		to.ready == nil &&
		to.capability == nil &&
		to.transaction == nil &&
		!to.scopeSet &&
		to.disposition == 0 &&
		!to.identity.Valid()
}

func preparedCapabilityIdentity(capability PreparedCapability) (identity ResourceIdentity, err error) {
	if capability == nil {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: nil prepared capability")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			identity = ResourceIdentity{}
			err = fmt.Errorf(
				"%w: prepared capability identity panic: %v",
				ErrTaskPanic,
				recovered,
			)
		}
	}()
	identity = capability.Identity()
	if !identity.Valid() {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: invalid prepared capability identity")
	}
	return identity, nil
}

func preparedResourceIdentity(resource PreparedResource) (identity ResourceIdentity, err error) {
	if resource == nil {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: nil prepared resource")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			identity = ResourceIdentity{}
			err = fmt.Errorf(
				"%w in prepared resource identity: %v",
				ErrTaskPanic,
				recovered,
			)
		}
	}()
	identity = resource.Identity()
	if !identity.Valid() {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: invalid prepared resource identity")
	}
	return identity, nil
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
