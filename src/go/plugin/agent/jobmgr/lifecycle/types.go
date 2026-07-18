// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrFunctionResultTooLarge = errors.New("jobmgr lifecycle: Function result exceeds bound")

const (
	ProcessBudgetBytes      = 256 * 1024 * 1024
	CleanupBudgetBytes      = 100 * 1024 * 1024
	OrdinaryBudgetBytes     = ProcessBudgetBytes - CleanupBudgetBytes
	TaskStartServiceQuantum = 4
	// TaskChildExecutionBytes is the conservative admission charge for one
	// live task child. It accounts for the initial goroutine stack and the
	// task's runtime/heap bookkeeping; it does not limit concurrency.
	TaskChildExecutionBytes             = int64(4 * 1024)
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
	status       int
	contentType  string
	payloadKind  sealedPayloadKind
	payload      []byte
	value        Value
	payloadBytes int
	planBytes    int64
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
	result.payload = append([]byte(nil), payload...)
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

func (result SealedResult) validate() error {
	if result.status < 100 || result.status > 599 {
		return errors.New("jobmgr lifecycle: invalid result status")
	}
	if result.contentType == "" || strings.ContainsAny(result.contentType, " \t\r\n\x00") {
		return errors.New("jobmgr lifecycle: invalid result content type")
	}
	switch result.payloadKind {
	case sealedPayloadRaw:
		if len(result.payload) != result.payloadBytes || result.planBytes != int64(len(result.payload)) {
			return errors.New("jobmgr lifecycle: raw result size differs")
		}
	case sealedPayloadValue:
		if result.value.kind == valueInvalid || result.payload != nil || result.planBytes != result.value.charge {
			return errors.New("jobmgr lifecycle: invalid closed Value result")
		}
	default:
		return errors.New("jobmgr lifecycle: unknown sealed payload variant")
	}
	return validateFunctionPayloadSize(result.payloadBytes)
}

func (result SealedResult) appendPayload(dst []byte) ([]byte, error) {
	switch result.payloadKind {
	case sealedPayloadRaw:
		return append(dst, result.payload...), nil
	case sealedPayloadValue:
		return appendValueJSON(dst, result.value, 0)
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

func PreparedCapabilityTaskWork(work func(context.Context) (PreparedCapability, error)) TaskWork {
	return func(ctx context.Context) (TaskOutcome, error) {
		capability, err := work(ctx)
		if capability == nil {
			return TaskOutcome{}, err
		}
		outcome, outcomeErr := PreparedCapabilityOutcome(capability)
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

type TaskPlan struct {
	Source                 Source
	Deadline               time.Time
	InitialCancellation    error
	MaxPhaseTransitions    uint8
	Work                   TaskWork
	Runner                 TaskRunner
	Cleanup                TaskCleanup
	permitAdmission        *AdmissionLedger
	permitAdmissionRef     AdmissionRef
	permitOwner            ResourceIdentity
	permitPlan             LongLivedPlan
	permitWork             PreparedResourcePermitWork
	capabilityPermitWork   PreparedCapabilityPermitWork
	transactionWork        PreparedResourceTransactionWork
	transactionScope       ResourceTransactionScope
	transactionScopeSet    bool
	initialReady           ReadyResource
	initialIdentity        ResourceIdentity
	taskContext            context.Context
	preserveDisposeContext bool
	drainDependent         bool
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

func (plan TaskPlan) Validate() error {
	if !plan.Source.Valid() {
		return errors.New("jobmgr lifecycle: invalid task source")
	}
	if plan.drainDependent && plan.initialReady == nil {
		return errors.New("jobmgr lifecycle: drain-dependent task has no ready resource")
	}
	workSources := 0
	if plan.Work != nil {
		workSources++
	}
	if plan.Runner != nil {
		workSources++
	}
	if plan.permitWork != nil {
		workSources++
	}
	if plan.capabilityPermitWork != nil {
		workSources++
	}
	if plan.transactionWork != nil {
		workSources++
	} else if plan.initialReady != nil {
		workSources++
	}
	if workSources != 1 {
		return errors.New("jobmgr lifecycle: task must have exactly one work source")
	}
	if plan.InitialCancellation != nil {
		if plan.InitialCancellation != context.Canceled && plan.InitialCancellation != context.DeadlineExceeded {
			return errors.New("jobmgr lifecycle: invalid initial task cancellation")
		}
		if plan.InitialCancellation == context.DeadlineExceeded && plan.Deadline.IsZero() {
			return errors.New("jobmgr lifecycle: initial deadline cancellation has no deadline")
		}
	}
	if plan.initialReady != nil && !plan.initialIdentity.Valid() {
		return errors.New("jobmgr lifecycle: initial ready resource has invalid identity")
	}
	if (plan.Work != nil ||
		plan.Runner != nil ||
		plan.permitWork != nil ||
		plan.capabilityPermitWork != nil) &&
		plan.initialIdentity.Valid() {
		return errors.New("jobmgr lifecycle: work task has an unexpected resource identity")
	}
	if plan.transactionWork != nil {
		if !plan.transactionScopeSet ||
			!plan.transactionScope.Valid() ||
			(plan.initialReady == nil) != !plan.transactionScope.Current.Valid() ||
			plan.initialIdentity != plan.transactionScope.Current {
			return errors.New("jobmgr lifecycle: invalid resource transaction scope")
		}
	} else if plan.transactionScopeSet ||
		plan.transactionScope != (ResourceTransactionScope{}) {
		return errors.New("jobmgr lifecycle: unexpected resource transaction scope")
	}
	if plan.taskContext != nil {
		if !plan.Deadline.IsZero() || plan.InitialCancellation != nil || (plan.initialReady == nil) == plan.preserveDisposeContext {
			return errors.New("jobmgr lifecycle: invalid shutdown task context")
		}
	} else if plan.preserveDisposeContext {
		return errors.New("jobmgr lifecycle: unexpected preserved disposal context")
	}
	if plan.permitWork != nil ||
		plan.capabilityPermitWork != nil ||
		(plan.transactionWork != nil && plan.transactionScope.Successor.Valid()) {
		if plan.permitAdmission == nil || !plan.permitAdmissionRef.Valid() || !plan.permitOwner.Valid() {
			return errors.New("jobmgr lifecycle: incomplete prepared-resource permit work")
		}
		if err := plan.permitPlan.Validate(); err != nil {
			return err
		}
		if plan.transactionWork != nil &&
			plan.permitOwner != plan.transactionScope.Successor {
			return errors.New("jobmgr lifecycle: transaction permit owner differs from successor")
		}
	} else if plan.permitAdmission != nil || plan.permitAdmissionRef.Valid() || plan.permitOwner.Valid() || plan.permitPlan.class != 0 {
		return errors.New("jobmgr lifecycle: unexpected prepared-resource permit terms")
	}
	limit := plan.MaxPhaseTransitions
	if limit == 0 {
		if plan.Source == SourceFunction {
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

func (plan TaskPlan) phaseLimit() uint8 {
	if plan.MaxPhaseTransitions != 0 {
		return plan.MaxPhaseTransitions
	}
	if plan.Source == SourceFunction {
		return FunctionTaskPhases
	}
	return TransactionTaskPhases
}
