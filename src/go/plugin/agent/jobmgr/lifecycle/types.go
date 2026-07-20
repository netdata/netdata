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
	ProcessBudgetBytes                  = 256 * 1024 * 1024
	CleanupBudgetBytes                  = 100 * 1024 * 1024
	OrdinaryBudgetBytes                 = ProcessBudgetBytes - CleanupBudgetBytes
	TaskStartServiceQuantum             = 4
	RetainedTimeoutFailStopThreshold    = 4
	InheritedCancellationServiceQuantum = 4
	ControlFrameBytes                   = 64 * 1024
	FunctionPayloadBytes                = 100 * 1024 * 1024
	FunctionEnvelopeBytes               = 64 * 1024
	MaximumFunctionFrameBytes           = FunctionPayloadBytes + FunctionEnvelopeBytes
	MaximumOtherFrameBytes              = 100 * 1024 * 1024
	MaximumInputBodyBytes               = 20 * 1024 * 1024
	FunctionTaskPhases                  = 4
	TransactionTaskPhases               = 6
)

type SealedResult struct {
	status       int               // HTTP-ish status (100-599)
	contentType  string            // payload MIME type
	payloadKind  sealedPayloadKind // raw bytes vs a closed Value
	payload      []byte            // owned raw bytes (raw kind)
	value        Value             // closed Value (value kind)
	payloadBytes int               // encoded payload length for framing
	planBytes    int64             // accounting charge (raw: byte length; value: value.charge)
}

type sealedPayloadKind uint8

const (
	sealedPayloadRaw sealedPayloadKind = iota + 1
	sealedPayloadValue
)

func NewSealedResult(status int, contentType string, payload []byte) (SealedResult, error) {
	result := SealedResult{status: status, contentType: contentType, payloadKind: sealedPayloadRaw, payload: payload, payloadBytes: len(payload), planBytes: int64(len(payload))}
	if err := result.validate(); err != nil {
		return SealedResult{}, err
	}
	result.payload = slices.Clone(payload)
	return result, nil
}

func newOwnedSealedResult(status int, contentType string, ownedPayload []byte) (SealedResult, error) {
	result := SealedResult{status: status, contentType: contentType, payloadKind: sealedPayloadRaw, payload: ownedPayload, payloadBytes: len(ownedPayload), planBytes: int64(len(ownedPayload))}
	if err := result.validate(); err != nil {
		return SealedResult{}, err
	}
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
	switch sr.payloadKind {
	case sealedPayloadRaw:
		if len(sr.payload) != sr.payloadBytes || sr.planBytes != int64(len(sr.payload)) {
			return errors.New("jobmgr lifecycle: raw result size differs")
		}
	case sealedPayloadValue:
		if sr.value.kind == valueInvalid || sr.payload != nil || sr.planBytes != sr.value.charge {
			return errors.New("jobmgr lifecycle: invalid closed Value result")
		}
	default:
		return errors.New("jobmgr lifecycle: unknown sealed payload variant")
	}
	return validateFunctionPayloadSize(sr.payloadBytes)
}

func (sr SealedResult) appendPayload(dst []byte) ([]byte, error) {
	switch sr.payloadKind {
	case sealedPayloadRaw:
		return append(dst, sr.payload...), nil
	case sealedPayloadValue:
		return appendValueJSON(dst, sr.value, 0)
	default:
		return nil, errors.New("jobmgr lifecycle: unknown sealed payload variant")
	}
}

type TaskWork func(context.Context) (TaskOutcome, error)

// TaskRunner permits a supervisor-owned reusable record to provide work
// without allocating a per-dispatch closure.
type TaskRunner interface {
	RunTask(context.Context) (TaskOutcome, error)
}

func FrameTaskWork(work func(context.Context) (SealedResult, error)) TaskWork {
	return func(ctx context.Context) (TaskOutcome, error) {
		result, err := work(ctx)
		if err != nil {
			return TaskOutcome{}, err
		}
		return NewFrameOutcome(result)
	}
}

func PreparedResourceTaskWork(work func(context.Context) (PreparedResource, error)) TaskWork {
	return func(ctx context.Context) (TaskOutcome, error) {
		resource, err := work(ctx)
		if resource == nil {
			return TaskOutcome{}, err
		}
		outcome, outcomeErr := PreparedResourceOutcome(resource)
		return outcome, errors.Join(err, outcomeErr)
	}
}

type TaskCleanup func() error

type PreparedResourcePermitWork func(context.Context, LongLivedPermit) (PreparedResource, error)

type PreparedCapabilityPermitWork func(context.Context, LongLivedPermit) (PreparedCapability, error)

type PreparedResourceTransactionWork func(
	context.Context,
	ReadyResource,
	ResourceTransactionScope,
	LongLivedPermit,
) (PreparedResourceTransaction, error)

func canonicalCancellationCause(cause error) (error, bool, bool) {
	if stopping, ok := cause.(*StoppingRejection); ok &&
		stopping.Generation != 0 {
		return stopping, false, true
	}
	cancelled := errors.Is(cause, context.Canceled)
	deadline := errors.Is(cause, context.DeadlineExceeded)
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
	Runner                 TaskRunner                      // reusable work runner (one work source)
	Cleanup                TaskCleanup                     // post-disposal cleanup
	permitAdmission        *AdmissionLedger                // admission ledger for a long-lived permit
	permitAdmissionRef     AdmissionRef                    // admission record backing the permit
	permitOwner            ResourceIdentity                // owning resource identity for the permit
	permitPlan             LongLivedPlan                   // long-lived plan terms
	permitWork             PreparedResourcePermitWork      // permit-bound resource work (one variant)
	capabilityPermitWork   PreparedCapabilityPermitWork    // permit-bound capability work (one variant)
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
	admission *AdmissionLedger,
	admissionRef AdmissionRef,
	current ReadyResource,
	scope ResourceTransactionScope,
	permitPlan LongLivedPlan,
	work PreparedResourceTransactionWork,
) (TaskPlan, error) {
	if scope.Current.Valid() {
		var err error
		permitPlan, err = permitPlan.forReplacement()
		if err != nil {
			return TaskPlan{}, err
		}
	}
	plan := TaskPlan{
		Source:              source,
		Deadline:            deadline,
		MaxPhaseTransitions: maxPhaseTransitions,
		permitAdmission:     admission,
		permitAdmissionRef:  admissionRef,
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

func NewPreparedResourcePermitTaskPlan(source Source, deadline time.Time, maxPhaseTransitions uint8, admission *AdmissionLedger, admissionRef AdmissionRef, owner ResourceIdentity, permitPlan LongLivedPlan, work PreparedResourcePermitWork) (TaskPlan, error) {
	plan := TaskPlan{
		Source: source, Deadline: deadline, MaxPhaseTransitions: maxPhaseTransitions,
		permitAdmission: admission, permitAdmissionRef: admissionRef, permitOwner: owner, permitPlan: permitPlan, permitWork: work,
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func NewPreparedCapabilityPermitTaskPlan(source Source, deadline time.Time, maxPhaseTransitions uint8, admission *AdmissionLedger, admissionRef AdmissionRef, owner ResourceIdentity, permitPlan LongLivedPlan, work PreparedCapabilityPermitWork) (TaskPlan, error) {
	plan := TaskPlan{
		Source: source, Deadline: deadline, MaxPhaseTransitions: maxPhaseTransitions,
		permitAdmission: admission, permitAdmissionRef: admissionRef, permitOwner: owner, permitPlan: permitPlan, capabilityPermitWork: work,
	}
	if err := plan.Validate(); err != nil {
		return TaskPlan{}, err
	}
	return plan, nil
}

func NewReadyResourceTaskPlan(source Source, deadline time.Time, maxPhaseTransitions uint8, resource ReadyResource, identity ResourceIdentity) (TaskPlan, error) {
	plan := TaskPlan{
		Source: source, Deadline: deadline, MaxPhaseTransitions: maxPhaseTransitions,
		initialReady: resource, initialIdentity: identity,
		drainDependent: true,
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
	if tp.Runner != nil {
		workSources++
	}
	if tp.permitWork != nil {
		workSources++
	}
	if tp.capabilityPermitWork != nil {
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
	if (tp.Work != nil ||
		tp.Runner != nil ||
		tp.permitWork != nil ||
		tp.capabilityPermitWork != nil) &&
		tp.initialIdentity.Valid() {
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
	if tp.permitWork != nil ||
		tp.capabilityPermitWork != nil ||
		(tp.transactionWork != nil && tp.transactionScope.Successor.Valid()) {
		if tp.permitAdmission == nil || !tp.permitAdmissionRef.Valid() || !tp.permitOwner.Valid() {
			return errors.New("jobmgr lifecycle: incomplete prepared-resource permit work")
		}
		if err := tp.permitPlan.Validate(); err != nil {
			return err
		}
		if tp.transactionWork != nil &&
			tp.permitOwner != tp.transactionScope.Successor {
			return errors.New("jobmgr lifecycle: transaction permit owner differs from successor")
		}
	} else if tp.permitAdmission != nil || tp.permitAdmissionRef.Valid() || tp.permitOwner.Valid() || tp.permitPlan.class != 0 {
		return errors.New("jobmgr lifecycle: unexpected prepared-resource permit terms")
	}
	limit := tp.MaxPhaseTransitions
	if limit == 0 {
		if tp.Source == SourceFunction {
			limit = FunctionTaskPhases
		} else {
			limit = TransactionTaskPhases
		}
	}
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
