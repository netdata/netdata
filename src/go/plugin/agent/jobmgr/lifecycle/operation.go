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

func (og *OperationGeneration) Advance(state OperationState) error {
	if state <= og.State || state > OperationDisposedTerminal {
		return errors.New("jobmgr operation: non-monotonic operation transition")
	}
	og.State = state
	return nil
}

func (og *OperationGeneration) StartChild(ref TaskRef) error {
	if (og.Child != ChildNotStarted && og.Child != ChildDeadlineStartPending) || !ref.Valid() {
		return errors.New("jobmgr operation: invalid child start")
	}
	og.Task = ref
	og.Child = ChildExecuting
	return nil
}

func (og *OperationGeneration) RequireDeadlineStart() error {
	if og.Child != ChildNotStarted {
		return errors.New("jobmgr operation: invalid required deadline start")
	}
	og.Child = ChildDeadlineStartPending
	return nil
}

func (og *OperationGeneration) AbandonDeadlineStart() error {
	if og.Child != ChildDeadlineStartPending {
		return errors.New("jobmgr operation: invalid deadline-start abandonment")
	}
	og.Child = ChildAbandonedBeforeStart
	return nil
}

func (og *OperationGeneration) ResultReady(ref TaskRef, sequence uint8) error {
	if og.Child != ChildExecuting || og.Task != ref || sequence != 1 || og.childPhase != 0 {
		return errors.New("jobmgr operation: stale child result")
	}
	og.Child = ChildResultReady
	og.childPhase = sequence
	return nil
}

func (og *OperationGeneration) PhaseResultReady(
	ref TaskRef,
	sequence uint8,
) error {
	if og.Child != ChildActionPending ||
		og.Task != ref ||
		sequence != og.childPhase {
		return errors.New("jobmgr operation: stale child phase result")
	}
	og.Child = ChildResultReady
	return nil
}

func (og *OperationGeneration) ActionPending(ref TaskRef, sequence uint8) error {
	if (og.Child != ChildResultReady && og.Child != ChildActionAcknowledged) || og.Task != ref || sequence != og.childPhase+1 {
		return errors.New("jobmgr operation: action without result")
	}
	og.Child = ChildActionPending
	og.childPhase = sequence
	return nil
}

func (og *OperationGeneration) ActionAcknowledged(ref TaskRef, sequence uint8) error {
	if og.Child != ChildActionPending || og.Task != ref || sequence != og.childPhase {
		return errors.New("jobmgr operation: stale phase acknowledgement")
	}
	og.Child = ChildActionAcknowledged
	return nil
}

func (og *OperationGeneration) TerminationPending(ref TaskRef, sequence uint8) error {
	if og.Child != ChildActionAcknowledged || og.Task != ref || sequence != og.childPhase+1 {
		return errors.New("jobmgr operation: invalid termination action")
	}
	og.Child = ChildTerminationPending
	og.childPhase = sequence
	return nil
}

func (og *OperationGeneration) ChildExited(ref TaskRef, sequence uint8) error {
	if og.Child != ChildTerminationPending || og.Task != ref || sequence != og.childPhase {
		return errors.New("jobmgr operation: stale child exit")
	}
	og.Child = ChildExitAcknowledged
	return nil
}

func (og *OperationGeneration) MarkResponsePending() error {
	if og.Response != ResponseOpen {
		return errors.New("jobmgr operation: response is not open")
	}
	og.Response = ResponsePending
	return nil
}

func (og *OperationGeneration) CommitResponse() error {
	if og.Response != ResponseOpen && og.Response != ResponsePending {
		return errors.New("jobmgr operation: response cannot commit")
	}
	og.Response = ResponseCommitted
	return nil
}

func (og *OperationGeneration) PoisonResponse() {
	og.Response = ResponsePoisoned
}

func (og *OperationGeneration) MarkTimedOut() {
	og.timedOut = true
}

func (og *OperationGeneration) TimedOut() bool {
	return og.timedOut
}

func (og *OperationGeneration) CanDisposeTerminal() bool {
	responseTerminal := og.Response == ResponseNotRequired || og.Response == ResponseCommitted || og.Response == ResponsePoisoned
	childTerminal := og.Child == ChildNotStarted || og.Child == ChildAbandonedBeforeStart || og.Child == ChildExitAcknowledged
	return responseTerminal && childTerminal
}
