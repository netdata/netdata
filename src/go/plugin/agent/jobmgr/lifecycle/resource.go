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
	kind                   TaskOutcomeKind
	frame                  SealedResult
	prepared               PreparedResource
	ready                  ReadyResource
	capability             PreparedCapability
	transaction            PreparedResourceTransaction
	scope                  ResourceTransactionScope
	scopeSet               bool
	disposition            ResourceTransactionDisposition
	identity               ResourceIdentity
	longLivedResourceBytes int64
}

type PreparedLongLivedResourceSizer interface {
	LongLivedResourceBytes() (int64, error)
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
	var longLivedResourceBytes int64
	if sizer, ok := transaction.(PreparedLongLivedResourceSizer); ok {
		var err error
		longLivedResourceBytes, err =
			sizer.LongLivedResourceBytes()
		if err != nil {
			return TaskOutcome{}, err
		}
		if longLivedResourceBytes < 0 ||
			longLivedResourceBytes >=
				OrdinaryBudgetBytes-
					TaskChildExecutionBytes {
			return TaskOutcome{}, errors.New(
				"jobmgr lifecycle: invalid transaction retained bytes",
			)
		}
	}
	outcome := TaskOutcome{
		kind:                   TaskOutcomePreparedResourceTransaction,
		transaction:            transaction,
		scope:                  scope,
		scopeSet:               true,
		longLivedResourceBytes: longLivedResourceBytes,
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

func (outcome TaskOutcome) Kind() TaskOutcomeKind {
	return outcome.kind
}

func (outcome TaskOutcome) ReadyResource() (ReadyResource, bool) {
	return outcome.ready, outcome.kind == TaskOutcomeReadyResource && outcome.ready != nil
}

func (outcome TaskOutcome) ResourceIdentity() (ResourceIdentity, bool) {
	return outcome.identity, (outcome.kind == TaskOutcomePreparedResource ||
		outcome.kind == TaskOutcomeReadyResource ||
		outcome.kind == TaskOutcomePreparedCapability ||
		outcome.kind == TaskOutcomeAppliedResourceTransaction) &&
		outcome.identity.Valid()
}

func (outcome TaskOutcome) validate() error {
	switch outcome.kind {
	case TaskOutcomeNone:
		if !emptySealedResult(outcome.frame) ||
			outcome.prepared != nil ||
			outcome.ready != nil ||
			outcome.capability != nil ||
			outcome.transaction != nil ||
			outcome.scopeSet ||
			outcome.disposition != 0 ||
			outcome.identity.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: nonempty no-value outcome")
		}
	case TaskOutcomeFrame:
		if outcome.prepared != nil ||
			outcome.ready != nil ||
			outcome.capability != nil ||
			outcome.transaction != nil ||
			outcome.scopeSet ||
			outcome.disposition != 0 ||
			outcome.identity.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: mixed frame outcome")
		}
		return outcome.frame.validate()
	case TaskOutcomePreparedResource:
		if !emptySealedResult(outcome.frame) ||
			outcome.prepared == nil ||
			outcome.ready != nil ||
			outcome.capability != nil ||
			outcome.transaction != nil ||
			outcome.scopeSet ||
			outcome.disposition != 0 ||
			!outcome.identity.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: invalid prepared resource outcome")
		}
	case TaskOutcomeReadyResource:
		if !emptySealedResult(outcome.frame) ||
			outcome.prepared != nil ||
			outcome.ready == nil ||
			outcome.capability != nil ||
			outcome.transaction != nil ||
			outcome.scopeSet ||
			outcome.disposition != 0 ||
			!outcome.identity.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: invalid ready resource outcome")
		}
	case TaskOutcomePreparedCapability:
		if !emptySealedResult(outcome.frame) ||
			outcome.prepared != nil ||
			outcome.ready != nil ||
			outcome.capability == nil ||
			outcome.transaction != nil ||
			outcome.scopeSet ||
			outcome.disposition != 0 ||
			!outcome.identity.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: invalid prepared capability outcome")
		}
	case TaskOutcomePreparedResourceTransaction:
		if !emptySealedResult(outcome.frame) ||
			outcome.prepared != nil ||
			outcome.ready != nil ||
			outcome.capability != nil ||
			outcome.transaction == nil ||
			!outcome.scopeSet ||
			!outcome.scope.Valid() ||
			outcome.disposition != 0 ||
			outcome.identity.Valid() {
			return errors.New("jobmgr lifecycle: invalid prepared resource transaction outcome")
		}
	case TaskOutcomeAppliedResourceTransaction:
		if outcome.prepared != nil ||
			outcome.capability != nil ||
			outcome.transaction != nil ||
			!outcome.scopeSet ||
			!outcome.scope.Valid() ||
			!outcome.disposition.Valid() ||
			outcome.longLivedResourceBytes != 0 {
			return errors.New("jobmgr lifecycle: invalid applied resource transaction outcome")
		}
		if err := outcome.frame.validate(); err != nil {
			return err
		}
		switch outcome.disposition {
		case ResourceTransactionUnchanged:
			if outcome.scope.Current.Valid() {
				if outcome.ready == nil || outcome.identity != outcome.scope.Current {
					return errors.New("jobmgr lifecycle: unchanged transaction lost current resource")
				}
			} else if outcome.ready != nil || outcome.identity.Valid() {
				return errors.New("jobmgr lifecycle: empty unchanged transaction installed a resource")
			}
		case ResourceTransactionRemoved:
			if !outcome.scope.Current.Valid() ||
				outcome.scope.Successor.Valid() ||
				outcome.ready != nil ||
				outcome.identity.Valid() {
				return errors.New("jobmgr lifecycle: invalid removed transaction result")
			}
		case ResourceTransactionInstalled:
			if outcome.scope.Current.Valid() ||
				!outcome.scope.Successor.Valid() ||
				outcome.ready == nil ||
				outcome.identity != outcome.scope.Successor {
				return errors.New("jobmgr lifecycle: invalid installed transaction result")
			}
		case ResourceTransactionReplaced:
			if !outcome.scope.Current.Valid() ||
				!outcome.scope.Successor.Valid() ||
				outcome.ready == nil ||
				outcome.identity != outcome.scope.Successor {
				return errors.New("jobmgr lifecycle: invalid replaced transaction result")
			}
		}
	default:
		return errors.New("jobmgr lifecycle: unknown task outcome")
	}
	return nil
}

func (outcome TaskOutcome) empty() bool {
	return outcome.kind == TaskOutcomeNone &&
		emptySealedResult(outcome.frame) &&
		outcome.prepared == nil &&
		outcome.ready == nil &&
		outcome.capability == nil &&
		outcome.transaction == nil &&
		!outcome.scopeSet &&
		outcome.disposition == 0 &&
		!outcome.identity.Valid() &&
		outcome.longLivedResourceBytes == 0
}

func preparedCapabilityIdentity(capability PreparedCapability) (identity ResourceIdentity, err error) {
	if capability == nil {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: nil prepared capability")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			identity = ResourceIdentity{}
			err = fmt.Errorf("jobmgr lifecycle: prepared capability identity panic: %v", recovered)
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
			err = fmt.Errorf("jobmgr lifecycle: prepared resource identity panic: %v", recovered)
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
			err = fmt.Errorf("jobmgr lifecycle: ready resource identity panic: %v", recovered)
		}
	}()
	identity = resource.Identity()
	if !identity.Valid() {
		return ResourceIdentity{}, errors.New("jobmgr lifecycle: invalid ready resource identity")
	}
	return identity, nil
}
