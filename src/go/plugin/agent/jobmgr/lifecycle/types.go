// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var ErrFunctionResultTooLarge = errors.New("jobmgr lifecycle: Function result exceeds bound")

const (
	TaskStartServiceQuantum             = 4
	RetainedTimeoutFailStopThreshold    = 4
	InheritedCancellationServiceQuantum = 4
	ControlFrameBytes                   = 64 * 1024
	FunctionPayloadBytes                = 100 * 1024 * 1024
	FunctionEnvelopeBytes               = 64 * 1024
	MaximumFunctionFrameBytes           = FunctionPayloadBytes + FunctionEnvelopeBytes
	MaximumOtherFrameBytes              = 100 * 1024 * 1024
	FunctionTaskPhases                  = 4
	TransactionTaskPhases               = 6
)

type SealedResult struct {
	status      int    // HTTP-ish status (100-599)
	contentType string // payload MIME type
	payload     []byte // owned encoded payload
}

func NewSealedResult(status int, contentType string, payload []byte) (SealedResult, error) {
	result := SealedResult{status: status, contentType: contentType, payload: payload}
	if err := result.validate(); err != nil {
		return SealedResult{}, err
	}
	result.payload = slices.Clone(payload)
	return result, nil
}

func validateFunctionPayloadSize(size int) error {
	if size < 0 {
		return fmt.Errorf("%w: negative payload size", ErrFunctionResultTooLarge)
	}
	deferred := size
	if size > 0 {
		if size == int(^uint(0)>>1) {
			return fmt.Errorf("%w: payload size overflow", ErrFunctionResultTooLarge)
		}
		deferred++
	}
	if deferred > FunctionPayloadBytes {
		return fmt.Errorf("%w: deferred payload is %d bytes", ErrFunctionResultTooLarge, deferred)
	}
	return nil
}

func (sr SealedResult) validate() error {
	if sr.status < 100 || sr.status > 599 {
		return errors.New("jobmgr lifecycle: invalid result status")
	}
	if sr.contentType == "" || strings.ContainsAny(sr.contentType, " \t\r\n\x00") {
		return errors.New("jobmgr lifecycle: invalid result content type")
	}
	return validateFunctionPayloadSize(len(sr.payload))
}

type TaskWork func(context.Context) (TaskOutcome, error)

type TaskCleanup func() error

type PreparedResourceTransactionWork func(
	context.Context,
	ReadyResource,
	ResourceTransactionScope,
	LongLivedPermit,
) (PreparedResourceTransaction, error)

func canonicalCancellationCause(cause error) (canonical error, deadline, ok bool) {
	if stopping, ok := cause.(*StoppingRejection); ok &&
		stopping.Generation != 0 {
		return stopping, false, true
	}
	cancelled := errors.Is(cause, context.Canceled)
	deadline = errors.Is(cause, context.DeadlineExceeded)
	if cancelled == deadline {
		return nil, false, false
	}
	if deadline {
		return context.DeadlineExceeded, true, true
	}
	return context.Canceled, false, true
}

const strictErrorTreeLimit = 32

func allErrorLeavesMatch(
	err error,
	match func(error) bool,
) bool {
	if err == nil || match == nil {
		return false
	}
	pending := [strictErrorTreeLimit]error{err}
	count := 1
	leaves := 0
	visited := 0
	for count > 0 {
		visited++
		if visited > strictErrorTreeLimit {
			return false
		}
		count--
		current := pending[count]
		if current == nil {
			continue
		}
		if joined, ok := current.(interface {
			Unwrap() []error
		}); ok {
			children := joined.Unwrap()
			if len(children) == 0 ||
				count+len(children) >
					strictErrorTreeLimit {
				return false
			}
			for _, child := range children {
				if child == nil {
					continue
				}
				pending[count] = child
				count++
			}
			continue
		}
		if wrapped, ok := current.(interface {
			Unwrap() error
		}); ok {
			child := wrapped.Unwrap()
			if child == nil ||
				count == strictErrorTreeLimit {
				return false
			}
			pending[count] = child
			count++
			continue
		}
		if !match(current) {
			return false
		}
		leaves++
	}
	return leaves > 0
}

type TaskPlan struct {
	Source                 Source                          // ingress origin; selects the default phase bound
	Deadline               time.Time                       // absolute child deadline; zero = none
	InitialCancellation    error                           // pre-arm the child context as cancelled/deadline
	MaxPhaseTransitions    uint8                           // phase-action ceiling; 0 = source default
	Work                   TaskWork                        // one-shot work closure (one work source)
	Cleanup                TaskCleanup                     // post-disposal cleanup
	permitOwner            ResourceIdentity                // owning resource identity for the permit
	permitPlan             LongLivedPlan                   // long-lived plan terms
	transactionWork        PreparedResourceTransactionWork // permit-bound transaction work (one variant)
	transactionScope       ResourceTransactionScope        // current/successor identities a transaction may touch
	transactionScopeSet    bool                            // distinguishes a zero scope from unset
	initialReady           ReadyResource                   // pre-existing ready resource threaded in as the work source
	initialIdentity        ResourceIdentity                // identity of initialReady
	taskContext            context.Context                 // shutdown-budget context substituted for the dispatch parent
	preserveDisposeContext bool                            // dispose under the live context
	drainDependent         bool                            // task owns a ready resource that must drain before terminal
}

func NewResourceTransactionTaskPlan(
	source Source,
	deadline time.Time,
	maxPhaseTransitions uint8,
	current ReadyResource,
	scope ResourceTransactionScope,
	work PreparedResourceTransactionWork,
) (TaskPlan, error) {
	plan := TaskPlan{
		Source:              source,
		Deadline:            deadline,
		MaxPhaseTransitions: maxPhaseTransitions,
		transactionWork:     work,
		transactionScope:    scope,
		transactionScopeSet: true,
		initialReady:        current,
		initialIdentity:     scope.Current,
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func NewResourceTransactionPermitTaskPlan(
	source Source,
	deadline time.Time,
	maxPhaseTransitions uint8,
	current ReadyResource,
	scope ResourceTransactionScope,
	permitPlan LongLivedPlan,
	work PreparedResourceTransactionWork,
) (TaskPlan, error) {
	if scope.Current.Valid() {
		if err := permitPlan.validateReplacementClass(); err != nil {
			return TaskPlan{}, err
		}
	}
	plan := TaskPlan{
		Source:              source,
		Deadline:            deadline,
		MaxPhaseTransitions: maxPhaseTransitions,
		permitOwner:         scope.Successor,
		permitPlan:          permitPlan,
		transactionWork:     work,
		transactionScope:    scope,
		transactionScopeSet: true,
		initialReady:        current,
		initialIdentity:     scope.Current,
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func NewShutdownReadyResourceTaskPlan(source Source, budget *ShutdownBudget, maxPhaseTransitions uint8, resource ReadyResource, identity ResourceIdentity) (TaskPlan, error) {
	if budget == nil || budget.Context() == nil {
		return TaskPlan{}, errors.New("jobmgr lifecycle: nil shutdown budget")
	}
	plan := TaskPlan{
		Source: source, MaxPhaseTransitions: maxPhaseTransitions,
		initialReady: resource, initialIdentity: identity,
		taskContext: budget.Context(), preserveDisposeContext: true,
		drainDependent: true,
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func NewShutdownWorkTaskPlan(source Source, budget *ShutdownBudget, maxPhaseTransitions uint8, work TaskWork) (TaskPlan, error) {
	if budget == nil || budget.Context() == nil {
		return TaskPlan{}, errors.New("jobmgr lifecycle: nil shutdown budget")
	}
	plan := TaskPlan{
		Source: source, MaxPhaseTransitions: maxPhaseTransitions, Work: work,
		taskContext: budget.Context(),
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func (tp TaskPlan) Validate() error {
	if !tp.Source.Valid() {
		return errors.New("jobmgr lifecycle: invalid task source")
	}
	if tp.drainDependent && tp.initialReady == nil {
		return errors.New("jobmgr lifecycle: drain-dependent task has no ready resource")
	}
	workSources := 0
	if tp.Work != nil {
		workSources++
	}
	if tp.transactionWork != nil {
		workSources++
	} else if tp.initialReady != nil {
		workSources++
	}
	if workSources != 1 {
		return errors.New("jobmgr lifecycle: task must have exactly one work source")
	}
	if tp.InitialCancellation != nil {
		_, deadline, ok := canonicalCancellationCause(tp.InitialCancellation)
		if !ok {
			return errors.New("jobmgr lifecycle: invalid initial task cancellation")
		}
		if deadline && tp.Deadline.IsZero() {
			return errors.New("jobmgr lifecycle: initial deadline cancellation has no deadline")
		}
	}
	if tp.initialReady != nil && !tp.initialIdentity.Valid() {
		return errors.New("jobmgr lifecycle: initial ready resource has invalid identity")
	}
	if tp.Work != nil && tp.initialIdentity.Valid() {
		return errors.New("jobmgr lifecycle: work task has an unexpected resource identity")
	}
	if tp.transactionWork != nil {
		if !tp.transactionScopeSet ||
			!tp.transactionScope.Valid() ||
			(tp.initialReady == nil) != !tp.transactionScope.Current.Valid() ||
			tp.initialIdentity != tp.transactionScope.Current {
			return errors.New("jobmgr lifecycle: invalid resource transaction scope")
		}
	} else if tp.transactionScopeSet ||
		tp.transactionScope != (ResourceTransactionScope{}) {
		return errors.New("jobmgr lifecycle: unexpected resource transaction scope")
	}
	if tp.taskContext != nil {
		if !tp.Deadline.IsZero() || tp.InitialCancellation != nil || (tp.initialReady == nil) == tp.preserveDisposeContext {
			return errors.New("jobmgr lifecycle: invalid shutdown task context")
		}
	} else if tp.preserveDisposeContext {
		return errors.New("jobmgr lifecycle: unexpected preserved disposal context")
	}
	if tp.transactionWork != nil && tp.transactionScope.Successor.Valid() {
		if !tp.permitOwner.Valid() {
			return errors.New("jobmgr lifecycle: incomplete prepared-resource permit work")
		}
		if err := tp.permitPlan.Validate(); err != nil {
			return err
		}
		if tp.transactionWork != nil &&
			tp.permitOwner != tp.transactionScope.Successor {
			return errors.New("jobmgr lifecycle: transaction permit owner differs from successor")
		}
	} else if tp.permitOwner.Valid() || tp.permitPlan.class != 0 {
		return errors.New("jobmgr lifecycle: unexpected prepared-resource permit terms")
	}
	limit := tp.phaseLimit()
	if limit < 3 || limit > TransactionTaskPhases {
		return errors.New("jobmgr lifecycle: invalid task phase bound")
	}
	return nil
}

func (tp TaskPlan) phaseLimit() uint8 {
	if tp.MaxPhaseTransitions != 0 {
		return tp.MaxPhaseTransitions
	}
	if tp.Source == SourceFunction {
		return FunctionTaskPhases
	}
	return TransactionTaskPhases
}
