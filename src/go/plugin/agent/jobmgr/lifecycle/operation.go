// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "errors"

type OperationState uint8

const (
	OperationAdmitted OperationState = iota + 1
	OperationQueued
	OperationAcquiringClaims
	OperationReady
	OperationRunning
	OperationDisposing
	OperationDisposedTerminal
)

type ResponseState uint8

const (
	ResponseOpen ResponseState = iota + 1
	ResponsePending
	ResponseCommitted
	ResponsePoisoned
	ResponseNotRequired
)

type ChildState uint8

const (
	ChildNotStarted ChildState = iota + 1
	ChildDeadlineStartPending
	ChildAbandonedBeforeStart
	ChildExecuting
	ChildResultReady
	ChildActionPending
	ChildActionAcknowledged
	ChildTerminationPending
	ChildExitAcknowledged
)

type OperationGeneration struct {
	ID         OperationID
	UID        string
	Source     Source
	LaneKey    string
	State      OperationState
	Response   ResponseState
	Child      ChildState
	Task       TaskRef
	timedOut   bool
	childPhase uint8
}

func NewOperation(id OperationID, uid string, source Source, laneKey string, responseRequired bool) (*OperationGeneration, error) {
	if id == 0 || uid == "" || laneKey == "" || !source.Valid() {
		return nil, errors.New("jobmgr operation: invalid identity")
	}
	response := ResponseNotRequired
	if responseRequired {
		response = ResponseOpen
	}
	return &OperationGeneration{ID: id, UID: uid, Source: source, LaneKey: laneKey, State: OperationAdmitted, Response: response, Child: ChildNotStarted}, nil
}

func (operation *OperationGeneration) Advance(state OperationState) error {
	if state <= operation.State || state > OperationDisposedTerminal {
		return errors.New("jobmgr operation: non-monotonic operation transition")
	}
	operation.State = state
	return nil
}

func (operation *OperationGeneration) StartChild(ref TaskRef) error {
	if (operation.Child != ChildNotStarted && operation.Child != ChildDeadlineStartPending) || !ref.Valid() {
		return errors.New("jobmgr operation: invalid child start")
	}
	operation.Task = ref
	operation.Child = ChildExecuting
	return nil
}

func (operation *OperationGeneration) RequireDeadlineStart() error {
	if operation.Child != ChildNotStarted {
		return errors.New("jobmgr operation: invalid required deadline start")
	}
	operation.Child = ChildDeadlineStartPending
	return nil
}

func (operation *OperationGeneration) AbandonDeadlineStart() error {
	if operation.Child != ChildDeadlineStartPending {
		return errors.New("jobmgr operation: invalid deadline-start abandonment")
	}
	operation.Child = ChildAbandonedBeforeStart
	return nil
}

func (operation *OperationGeneration) ResultReady(ref TaskRef, sequence uint8) error {
	if operation.Child != ChildExecuting || operation.Task != ref || sequence != 1 || operation.childPhase != 0 {
		return errors.New("jobmgr operation: stale child result")
	}
	operation.Child = ChildResultReady
	operation.childPhase = sequence
	return nil
}

func (operation *OperationGeneration) PhaseResultReady(
	ref TaskRef,
	sequence uint8,
) error {
	if operation.Child != ChildActionPending ||
		operation.Task != ref ||
		sequence != operation.childPhase {
		return errors.New("jobmgr operation: stale child phase result")
	}
	operation.Child = ChildResultReady
	return nil
}

func (operation *OperationGeneration) ActionPending(ref TaskRef, sequence uint8) error {
	if (operation.Child != ChildResultReady && operation.Child != ChildActionAcknowledged) || operation.Task != ref || sequence != operation.childPhase+1 {
		return errors.New("jobmgr operation: action without result")
	}
	operation.Child = ChildActionPending
	operation.childPhase = sequence
	return nil
}

func (operation *OperationGeneration) ActionAcknowledged(ref TaskRef, sequence uint8) error {
	if operation.Child != ChildActionPending || operation.Task != ref || sequence != operation.childPhase {
		return errors.New("jobmgr operation: stale phase acknowledgement")
	}
	operation.Child = ChildActionAcknowledged
	return nil
}

func (operation *OperationGeneration) TerminationPending(ref TaskRef, sequence uint8) error {
	if operation.Child != ChildActionAcknowledged || operation.Task != ref || sequence != operation.childPhase+1 {
		return errors.New("jobmgr operation: invalid termination action")
	}
	operation.Child = ChildTerminationPending
	operation.childPhase = sequence
	return nil
}

func (operation *OperationGeneration) ChildExited(ref TaskRef, sequence uint8) error {
	if operation.Child != ChildTerminationPending || operation.Task != ref || sequence != operation.childPhase {
		return errors.New("jobmgr operation: stale child exit")
	}
	operation.Child = ChildExitAcknowledged
	return nil
}

func (operation *OperationGeneration) MarkResponsePending() error {
	if operation.Response != ResponseOpen {
		return errors.New("jobmgr operation: response is not open")
	}
	operation.Response = ResponsePending
	return nil
}

func (operation *OperationGeneration) CommitResponse() error {
	if operation.Response != ResponseOpen && operation.Response != ResponsePending {
		return errors.New("jobmgr operation: response cannot commit")
	}
	operation.Response = ResponseCommitted
	return nil
}

func (operation *OperationGeneration) PoisonResponse() {
	operation.Response = ResponsePoisoned
}

func (operation *OperationGeneration) MarkTimedOut() {
	operation.timedOut = true
}

func (operation *OperationGeneration) TimedOut() bool {
	return operation.timedOut
}

func (operation *OperationGeneration) CanDisposeTerminal() bool {
	responseTerminal := operation.Response == ResponseNotRequired || operation.Response == ResponseCommitted || operation.Response == ResponsePoisoned
	childTerminal := operation.Child == ChildNotStarted || operation.Child == ChildAbandonedBeforeStart || operation.Child == ChildExitAcknowledged
	return responseTerminal && childTerminal
}
