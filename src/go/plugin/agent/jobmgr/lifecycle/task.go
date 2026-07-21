// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrTaskPanic = errors.New("jobmgr task child: panic")

type TaskRef struct {
	Slot       uint32
	Generation uint64
}

type TaskRequestRef struct {
	Slot       uint32
	Generation uint32
}

func (ref TaskRequestRef) Valid() bool {
	return ref.Slot > 0 && ref.Generation > 0
}

type TaskStart struct {
	Request TaskRequestRef
	Task    TaskRef
	Outcome TaskOutcome
	Err     error
}

func (ref TaskRef) Valid() bool {
	return ref.Generation > 0
}

type TaskCompletion struct {
	Ref      TaskRef
	Sequence uint8
	Kind     TaskOutcomeKind
	Err      error
}

type TaskActionKind uint8

const (
	TaskActionEncodeWrite TaskActionKind = iota + 1
	TaskActionAcceptStart
	TaskActionPublishResource
	TaskActionCommitCapability
	TaskActionStopResource
	TaskActionFinalizeResource
	TaskActionApplyResourceTransaction
	TaskActionDispose
	TaskActionCleanup
	TaskActionTerminate
)

type TaskAction struct {
	Ref                TaskRef        // target task ref
	Sequence           uint8          // phase-action sequence number
	Kind               TaskActionKind // action kind (accept-start / commit / apply / dispose)
	UID                string         // operation UID for correlation
	Expiry             int64          // result expiry stamp
	ExpectedGeneration uint64         // generation the action expects the resource to match
}

type TaskAcknowledgement struct {
	Ref                   TaskRef
	Sequence              uint8
	Kind                  TaskActionKind
	CapabilityDisposition CapabilityDisposition
	Err                   error
}

type taskSlot struct {
	generation             uint64                  // ABA guard for the TaskRef
	freeNext               uint32                  // freelist link
	active                 bool                    // slot is in use
	joined                 bool                    // child goroutine has exited its action loop
	actionPending          bool                    // an action is queued but unacknowledged
	sequence               uint8                   // last acknowledged phase sequence
	maxPhaseTransitions    uint8                   // phase-action ceiling (from the plan)
	outcome                TaskOutcome             // current in-flight outcome (result/resource/capability/transaction)
	cancel                 context.CancelCauseFunc // child context canceller
	cleanup                TaskCleanup             // deferred cleanup closure
	preserveDisposeContext bool                    // dispose under the live (not cancel-stripped) context
	retainedTimeout        bool                    // child kept alive past a timeout (feeds the fail-stop)
	action                 chan TaskAction         // 1-deep phase-action mailbox
}

type taskRequest struct {
	slot       uint32      // slot index in the request storage
	generation uint32      // ABA guard for the request ref
	freeNext   uint32      // freelist link
	previous   uint32      // previous request in its class queue
	next       uint32      // next request in its class queue
	active     bool        // request slot is in use
	class      TaskClass   // task class (framework-control / generic Function)
	plan       TaskPlan    // the task plan to run
	initial    TaskOutcome // seed outcome carried into the child
	queuedAt   time.Time   // enqueue time (oldest-wait metric)
}

type taskRequestQueue struct {
	head  uint32
	tail  uint32
	count int
}

type TaskSupervisor struct {
	frame       *FrameOwner              // frame owner used as the collector/output sink
	inherited   inheritedTaskRegistry    // registry of long-lived inherited tasks
	longLived   longLivedRegistry        // registry of long-lived permits
	slots       []*taskSlot              // transient task slot storage (freelist-backed)
	freeSlot    uint32                   // head of the task slot freelist
	requests    []*taskRequest           // task request storage (freelist-backed)
	pending     [2]taskRequestQueue      // per-class pending request queues
	freeRequest uint32                   // head of the request freelist
	nextClass   TaskClass                // class to service first (round-robin fairness)
	completions chan TaskCompletion      // inbound task completion channel
	acks        chan TaskAcknowledgement // inbound task acknowledgement channel
	active      int                      // count of running task children
	retained    int                      // count of retained (timed-out) children
	saturated   bool                     // retained-timeout fail-stop threshold reached
	observer    RuntimeObserver          // sink for task runtime counters
	run         *RunSupervisor           // bound run supervisor
	wake        func()                   // kernel wake callback

	admissionReadyMu      sync.Mutex // guards the admission-ready notifier
	onAdmissionReady      func()     // callback fired when admission can make progress
	admissionReadyPending bool       // an admission-ready notification is pending
}

func (ts *TaskSupervisor) BindRun(
	run *RunSupervisor,
	wake func(),
) error {
	if ts == nil || run == nil || wake == nil {
		return errors.New(
			"jobmgr task supervisor: invalid run binding",
		)
	}
	if ts.run != nil ||
		ts.wake != nil ||
		ts.active != 0 ||
		ts.Pending() != 0 ||
		ts.InheritedActive() != 0 {
		return errors.New(
			"jobmgr task supervisor: run bound after activation",
		)
	}
	ts.run = run
	ts.wake = wake
	return nil
}

func NewTaskSupervisor(frame *FrameOwner) (*TaskSupervisor, error) {
	if frame == nil {
		return nil, errors.New("jobmgr task supervisor: nil FrameOwner")
	}
	supervisor := &TaskSupervisor{
		frame:       frame,
		requests:    []*taskRequest{nil},
		completions: make(chan TaskCompletion, TaskStartServiceQuantum),
		acks:        make(chan TaskAcknowledgement, TaskStartServiceQuantum),
	}
	supervisor.inherited.initialize()
	supervisor.longLived.initialize()
	supervisor.nextClass = TaskClassFrameworkControl
	return supervisor, nil
}

func (ts *TaskSupervisor) BindRuntimeObserver(
	observer RuntimeObserver,
) error {
	if ts == nil || observer == nil {
		return errors.New("jobmgr task supervisor: invalid runtime observer")
	}
	if ts.observer != nil || ts.active != 0 ||
		ts.Pending() != 0 {
		return errors.New("jobmgr task supervisor: runtime observer bound after activation")
	}
	ts.observer = observer
	ts.observeRuntimeState()
	return nil
}

func (ts *TaskSupervisor) BindAdmissionReady(notify func()) error {
	if ts == nil || notify == nil {
		return errors.New("jobmgr task supervisor: invalid admission-ready binding")
	}
	ts.admissionReadyMu.Lock()
	if ts.onAdmissionReady != nil {
		ts.admissionReadyMu.Unlock()
		return errors.New("jobmgr task supervisor: admission-ready notifier already bound")
	}
	ts.onAdmissionReady = notify
	pending := ts.admissionReadyPending
	ts.admissionReadyPending = false
	ts.admissionReadyMu.Unlock()
	if pending {
		notify()
	}
	return nil
}

func (ts *TaskSupervisor) notifyAdmissionReady() {
	ts.admissionReadyMu.Lock()
	notify := ts.onAdmissionReady
	if notify == nil {
		ts.admissionReadyPending = true
	}
	ts.admissionReadyMu.Unlock()
	if notify != nil {
		notify()
	}
}

func (ts *TaskSupervisor) Enqueue(class TaskClass, plan TaskPlan) (TaskRequestRef, error) {
	if !class.Valid() {
		return TaskRequestRef{}, errors.New("jobmgr task supervisor: invalid scheduling class")
	}
	if err := plan.Validate(); err != nil {
		return TaskRequestRef{}, err
	}
	if plan.InitialCancellation != nil {
		plan.InitialCancellation, _, _ = canonicalCancellationCause(
			plan.InitialCancellation,
		)
	}
	var initial TaskOutcome
	if plan.initialReady != nil {
		var err error
		initial, err = readyResourceOutcome(plan.initialReady, plan.initialIdentity)
		if err != nil {
			return TaskRequestRef{}, err
		}
		plan.initialReady = nil
		plan.initialIdentity = ResourceIdentity{}
	}
	var slot uint32
	var record *taskRequest
	var generation uint32
	for generation == 0 {
		slot = ts.freeRequest
		if slot == 0 {
			if uint64(len(ts.requests)) > uint64(^uint32(0)) {
				return TaskRequestRef{}, errors.New("jobmgr task supervisor: request reference space exhausted")
			}
			slot = uint32(len(ts.requests))
			record = &taskRequest{slot: slot}
			ts.requests = append(ts.requests, record)
		} else {
			record = ts.requests[slot]
			ts.freeRequest = record.freeNext
		}
		generation = record.generation + 1
		if generation == 0 {
			*record = taskRequest{slot: slot, generation: record.generation}
		}
	}
	*record = taskRequest{
		slot: slot, generation: generation, active: true,
		class: class, plan: plan, initial: initial,
		queuedAt: time.Now(),
	}
	queue := &ts.pending[taskClassIndex(class)]
	record.previous = queue.tail
	if queue.tail != 0 {
		ts.requests[queue.tail].next = slot
	} else {
		queue.head = slot
	}
	queue.tail = slot
	queue.count++
	ts.observeRuntimeState()
	return TaskRequestRef{Slot: slot, Generation: generation}, nil
}

func (ts *TaskSupervisor) CancelPending(ref TaskRequestRef) error {
	record, err := ts.request(ref)
	if err != nil {
		return err
	}
	if !record.initial.empty() {
		return errors.New("jobmgr task supervisor: pending ready resource requires transfer-aware cancellation")
	}
	ts.removeRequest(record)
	return nil
}

func (ts *TaskSupervisor) SetPendingCancellation(ref TaskRequestRef, cause error) error {
	record, err := ts.request(ref)
	if err != nil {
		return err
	}
	cause, deadline, ok := canonicalCancellationCause(cause)
	if !ok {
		return errors.New("jobmgr task supervisor: invalid pending cancellation")
	}
	if record.plan.InitialCancellation != nil {
		return errors.New("jobmgr task supervisor: pending cancellation already set")
	}
	if deadline && record.plan.Deadline.IsZero() {
		return errors.New("jobmgr task supervisor: pending deadline cancellation has no deadline")
	}
	record.plan.InitialCancellation = cause
	return nil
}

func (ts *TaskSupervisor) CancelPendingOutcome(ref TaskRequestRef) (TaskOutcome, error) {
	record, err := ts.request(ref)
	if err != nil {
		return TaskOutcome{}, err
	}
	var outcome TaskOutcome
	if !record.initial.empty() {
		outcome = record.initial
		record.initial = TaskOutcome{}
	}
	ts.removeRequest(record)
	return outcome, nil
}

func (ts *TaskSupervisor) Dispatch(parent context.Context, quantum int, started *[TaskStartServiceQuantum]TaskStart) (int, bool, error) {
	if parent == nil || started == nil || quantum < 0 || quantum > len(started) {
		return 0, ts.Pending() > 0, errors.New("jobmgr task supervisor: invalid dispatch")
	}
	count := 0
	for count < quantum {
		first := taskClassIndex(ts.nextClass)
		second := 1 - first
		selected := first
		if ts.pending[selected].head == 0 {
			selected = second
		}
		queue := &ts.pending[selected]
		if queue.head == 0 {
			break
		}
		record := ts.requests[queue.head]
		requestRef := TaskRequestRef{Slot: record.slot, Generation: record.generation}
		taskRef, err := ts.start(parent, record.plan, record.initial)
		if err != nil {
			if errors.Is(err, ErrLongLivedRecordCapacity) {
				outcome := record.initial
				record.initial = TaskOutcome{}
				ts.removeRequest(record)
				started[count] = TaskStart{
					Request: requestRef,
					Outcome: outcome,
					Err:     err,
				}
				count++
				ts.nextClass = otherTaskClass(classForTaskIndex(selected))
				continue
			}
			return count, true, err
		}
		ts.removeRequest(record)
		started[count] = TaskStart{Request: requestRef, Task: taskRef}
		count++
		ts.nextClass = otherTaskClass(classForTaskIndex(selected))
	}
	return count, ts.Pending() > 0, nil
}

func (ts *TaskSupervisor) Pending() int {
	return ts.pending[0].count + ts.pending[1].count
}

func (ts *TaskSupervisor) start(parent context.Context, plan TaskPlan, initial TaskOutcome) (TaskRef, error) {
	hasDirectWork := plan.Work != nil ||
		plan.Runner != nil ||
		plan.permitWork != nil ||
		plan.capabilityPermitWork != nil
	if plan.transactionWork == nil {
		if (hasDirectWork != initial.empty()) ||
			plan.initialReady != nil ||
			plan.initialIdentity.Valid() {
			return TaskRef{}, errors.New("jobmgr task supervisor: invalid sealed task request")
		}
	} else if hasDirectWork ||
		plan.initialReady != nil ||
		plan.initialIdentity.Valid() ||
		!plan.transactionScopeSet ||
		!plan.transactionScope.Valid() ||
		(initial.empty() == plan.transactionScope.Current.Valid()) {
		return TaskRef{}, errors.New("jobmgr task supervisor: invalid sealed task request")
	}
	index, slot, err := ts.allocateSlot()
	if err != nil {
		return TaskRef{}, err
	}
	launched := false
	defer func() {
		if !launched {
			ts.recycleUnusedSlot(index, slot)
		}
	}()
	if plan.permitWork != nil {
		permit, err := ts.IssueLongLivedPermit(plan.permitAdmission, plan.permitAdmissionRef, plan.permitOwner, plan.permitPlan)
		if err != nil {
			return TaskRef{}, err
		}
		permitWork := plan.permitWork
		permitOwner := plan.permitOwner
		plan.Work = func(
			ctx context.Context,
		) (outcome TaskOutcome, resultErr error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					outcome = TaskOutcome{}
					resultErr = errors.Join(
						fmt.Errorf(
							"%w in prepared-resource work: %v",
							ErrTaskPanic,
							recovered,
						),
						permit.AbortUnused(),
					)
				}
			}()
			resource, workErr := permitWork(ctx, permit)
			if resource == nil {
				return TaskOutcome{}, errors.Join(workErr, permit.AbortUnused())
			}
			identity, identityErr := preparedResourceIdentity(resource)
			if identityErr != nil || identity != permitOwner {
				outcome, outcomeErr := preparedResourceOutcome(resource, permitOwner)
				return outcome, errors.Join(workErr, identityErr, outcomeErr, errors.New("jobmgr task supervisor: prepared resource identity differs from permit owner"))
			}
			outcome, outcomeErr := preparedResourceOutcome(resource, identity)
			return outcome, errors.Join(workErr, outcomeErr)
		}
		plan.permitAdmission = nil
		plan.permitAdmissionRef = AdmissionRef{}
		plan.permitOwner = ResourceIdentity{}
		plan.permitPlan = LongLivedPlan{}
		plan.permitWork = nil
	}
	if plan.capabilityPermitWork != nil {
		permit, err := ts.IssueLongLivedPermit(plan.permitAdmission, plan.permitAdmissionRef, plan.permitOwner, plan.permitPlan)
		if err != nil {
			return TaskRef{}, err
		}
		permitWork := plan.capabilityPermitWork
		permitOwner := plan.permitOwner
		plan.Work = func(
			ctx context.Context,
		) (outcome TaskOutcome, resultErr error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					outcome = TaskOutcome{}
					resultErr = errors.Join(
						fmt.Errorf(
							"%w in prepared-capability work: %v",
							ErrTaskPanic,
							recovered,
						),
						permit.AbortUnused(),
					)
				}
			}()
			capability, workErr := permitWork(ctx, permit)
			if capability == nil {
				return TaskOutcome{}, errors.Join(workErr, permit.AbortUnused())
			}
			identity, identityErr := preparedCapabilityIdentity(capability)
			if identityErr != nil || identity != permitOwner {
				outcome, outcomeErr := preparedCapabilityOutcome(capability, permitOwner)
				return outcome, errors.Join(workErr, identityErr, outcomeErr, errors.New("jobmgr task supervisor: prepared capability identity differs from permit owner"))
			}
			outcome, outcomeErr := preparedCapabilityOutcome(capability, identity)
			return outcome, errors.Join(workErr, outcomeErr)
		}
		plan.permitAdmission = nil
		plan.permitAdmissionRef = AdmissionRef{}
		plan.permitOwner = ResourceIdentity{}
		plan.permitPlan = LongLivedPlan{}
		plan.capabilityPermitWork = nil
	}
	if plan.transactionWork != nil {
		var current ReadyResource
		if plan.transactionScope.Current.Valid() {
			var ok bool
			current, ok = initial.ReadyResource()
			identity, identityOK := initial.ResourceIdentity()
			if !ok || !identityOK || identity != plan.transactionScope.Current {
				return TaskRef{}, errors.New(
					"jobmgr task supervisor: transaction current differs from sealed scope",
				)
			}
		} else if !initial.empty() {
			return TaskRef{}, errors.New(
				"jobmgr task supervisor: graph-only transaction has a current resource",
			)
		}
		initial = TaskOutcome{}
		var permit LongLivedPermit
		if plan.transactionScope.Successor.Valid() {
			issued, err := ts.IssueLongLivedPermit(
				plan.permitAdmission,
				plan.permitAdmissionRef,
				plan.permitOwner,
				plan.permitPlan,
			)
			if err != nil {
				return TaskRef{}, err
			}
			permit = issued
		}
		work := plan.transactionWork
		scope := plan.transactionScope
		plan.Work = func(
			ctx context.Context,
		) (outcome TaskOutcome, resultErr error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					var abortErr error
					if permit.Valid() {
						abortErr = permit.AbortUnused()
					}
					var outcomeErr error
					if current != nil {
						outcome, outcomeErr = readyResourceOutcome(
							current,
							scope.Current,
						)
					}
					resultErr = errors.Join(
						fmt.Errorf(
							"%w in resource-transaction work: %v",
							ErrTaskPanic,
							recovered,
						),
						abortErr,
						outcomeErr,
					)
				}
			}()
			transaction, workErr := work(ctx, current, scope, permit)
			if transaction == nil {
				var abortErr error
				if permit.Valid() {
					abortErr = permit.AbortUnused()
				}
				if current != nil {
					outcome, outcomeErr := readyResourceOutcome(current, scope.Current)
					return outcome, errors.Join(workErr, abortErr, outcomeErr)
				}
				return TaskOutcome{}, errors.Join(workErr, abortErr)
			}
			transactionScope, scopeErr := preparedResourceTransactionScope(transaction)
			if workErr != nil || scopeErr != nil || transactionScope != scope {
				restored, disposeErr := runPreparedResourceTransactionDispose(
					context.WithoutCancel(ctx),
					transaction,
				)
				current = nil
				permit = LongLivedPermit{}
				if restored != nil {
					outcome, outcomeErr := readyResourceOutcome(restored, scope.Current)
					return outcome, errors.Join(
						workErr,
						scopeErr,
						disposeErr,
						outcomeErr,
					)
				}
				return TaskOutcome{}, errors.Join(
					workErr,
					scopeErr,
					disposeErr,
				)
			}
			current = nil
			permit = LongLivedPermit{}
			return preparedResourceTransactionOutcome(transaction, scope)
		}
		plan.transactionWork = nil
		plan.transactionScope = ResourceTransactionScope{}
		plan.transactionScopeSet = false
		plan.permitAdmission = nil
		plan.permitAdmissionRef = AdmissionRef{}
		plan.permitOwner = ResourceIdentity{}
		plan.permitPlan = LongLivedPlan{}
	}
	var parentCtx context.Context
	if plan.taskContext != nil {
		parentCtx = plan.taskContext
	} else {
		parentCtx = parent
	}
	ctx, cancel := newTaskChildContext(parentCtx, plan.Deadline)
	if plan.InitialCancellation != nil {
		cancel(plan.InitialCancellation)
	}
	slot.active = true
	slot.cancel = cancel
	slot.cleanup = plan.Cleanup
	slot.preserveDisposeContext = plan.preserveDisposeContext
	slot.maxPhaseTransitions = plan.phaseLimit()
	ref := TaskRef{Slot: index, Generation: slot.generation}
	ts.active++
	ts.observeRuntimeState()
	launched = true
	go ts.runChild(ctx, ref, slot, plan, initial)
	return ref, nil
}

func (ts *TaskSupervisor) allocateSlot() (uint32, *taskSlot, error) {
	var index uint32
	var slot *taskSlot
	if ts.freeSlot == 0 {
		if uint64(len(ts.slots)) >= uint64(^uint32(0)) {
			return 0, nil, errors.New("jobmgr task supervisor: reference space exhausted")
		}
		index = uint32(len(ts.slots))
		slot = &taskSlot{action: make(chan TaskAction, 1)}
		ts.slots = append(ts.slots, slot)
	} else {
		index = ts.freeSlot - 1
		slot = ts.slots[index]
		if slot == nil {
			return 0, nil, errors.New("jobmgr task supervisor: nil reusable slot")
		}
		ts.freeSlot = slot.freeNext
		slot.freeNext = 0
	}
	if slot.active || slot.cancel != nil || slot.cleanup != nil ||
		len(slot.action) != 0 || slot.actionPending || !slot.outcome.empty() ||
		slot.sequence != 0 || slot.maxPhaseTransitions != 0 || slot.joined ||
		slot.preserveDisposeContext || slot.retainedTimeout {
		return 0, nil, errors.New("jobmgr task supervisor: dirty reusable slot")
	}
	slot.generation++
	if slot.generation == 0 {
		return 0, nil, errors.New("jobmgr task supervisor: slot generation wrapped")
	}
	return index, slot, nil
}

func (ts *TaskSupervisor) recycleUnusedSlot(index uint32, slot *taskSlot) {
	if slot == nil || slot.active || uint64(index) >= uint64(len(ts.slots)) ||
		ts.slots[index] != slot {
		panic("jobmgr task supervisor: invalid unused slot recycle")
	}
	slot.freeNext = ts.freeSlot
	ts.freeSlot = index + 1
}

type taskChildContext struct {
	context.Context
	deadline time.Time
}

func (ctx taskChildContext) Deadline() (time.Time, bool) {
	if parentDeadline, ok := ctx.Context.Deadline(); ok && parentDeadline.Before(ctx.deadline) {
		return parentDeadline, true
	}
	return ctx.deadline, true
}

func (ctx taskChildContext) Err() error {
	if ctx.Context.Err() == nil {
		return nil
	}
	if errors.Is(context.Cause(ctx.Context), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return context.Canceled
}

func newTaskChildContext(parent context.Context, deadline time.Time) (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	if deadline.IsZero() {
		return ctx, cancel
	}
	return taskChildContext{Context: ctx, deadline: deadline}, cancel
}

func (ts *TaskSupervisor) CompletionCh() <-chan TaskCompletion {
	return ts.completions
}

func (ts *TaskSupervisor) AcknowledgementCh() <-chan TaskAcknowledgement {
	return ts.acks
}

func (ts *TaskSupervisor) SendAction(action TaskAction) error {
	if !action.Ref.Valid() || action.Kind < TaskActionEncodeWrite || action.Kind > TaskActionTerminate {
		return errors.New("jobmgr task supervisor: invalid task action")
	}
	slot, err := ts.slot(action.Ref)
	if err != nil {
		return err
	}
	if slot.joined || slot.actionPending || action.Sequence != slot.sequence+1 || action.Sequence > slot.maxPhaseTransitions {
		return errors.New("jobmgr task supervisor: stale, duplicate, or wrong-sequence phase action")
	}
	if action.Kind == TaskActionEncodeWrite && (action.UID == "" || action.Expiry <= 0 || action.ExpectedGeneration != 0) {
		return errors.New("jobmgr task supervisor: invalid encode/write action")
	}
	if (action.Kind == TaskActionAcceptStart ||
		action.Kind == TaskActionCommitCapability) &&
		(action.ExpectedGeneration == 0 || action.UID != "" || action.Expiry != 0) {
		return errors.New("jobmgr task supervisor: invalid generation-bound action")
	}
	if action.Kind != TaskActionEncodeWrite &&
		action.Kind != TaskActionAcceptStart &&
		action.Kind != TaskActionCommitCapability &&
		(action.UID != "" || action.Expiry != 0 || action.ExpectedGeneration != 0) {
		return errors.New("jobmgr task supervisor: unexpected action payload")
	}
	slot.actionPending = true
	select {
	case slot.action <- action:
		return nil
	default:
		slot.actionPending = false
		return errors.New("jobmgr task supervisor: full phase mailbox")
	}
}

func (ts *TaskSupervisor) Cancel(ref TaskRef) error {
	return ts.CancelWithCause(ref, context.Canceled)
}

func (ts *TaskSupervisor) CancelWithCause(ref TaskRef, cause error) error {
	cause, _, ok := canonicalCancellationCause(cause)
	if !ok {
		return errors.New("jobmgr task supervisor: invalid cancellation cause")
	}
	slot, err := ts.slot(ref)
	if err != nil {
		return err
	}
	slot.cancel(cause)
	return nil
}

func (ts *TaskSupervisor) Release(ref TaskRef) error {
	slot, err := ts.slot(ref)
	if err != nil {
		return err
	}
	if !slot.joined || slot.actionPending || len(slot.action) != 0 || !slot.outcome.empty() || slot.retainedTimeout {
		return errors.New("jobmgr task supervisor: release before empty joined acknowledgement")
	}
	slot.cancel(context.Canceled)
	generation := slot.generation
	action := slot.action
	*slot = taskSlot{
		generation: generation,
		freeNext:   ts.freeSlot,
		action:     action,
	}
	ts.freeSlot = ref.Slot + 1
	ts.active--
	ts.observeRuntimeState()
	return nil
}

func (ts *TaskSupervisor) Active() int {
	return ts.active
}

func (ts *TaskSupervisor) MarkRetainedTimeout(ref TaskRef) (bool, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return false, err
	}
	if slot.retainedTimeout {
		return false, errors.New("jobmgr task supervisor: duplicate retained timeout")
	}
	slot.retainedTimeout = true
	ts.retained++
	justSaturated := ts.retained == RetainedTimeoutFailStopThreshold && !ts.saturated
	if justSaturated {
		ts.saturated = true
	}
	return justSaturated, nil
}

func (ts *TaskSupervisor) ClearRetainedTimeout(ref TaskRef) (bool, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return false, err
	}
	if !slot.retainedTimeout {
		return false, nil
	}
	slot.retainedTimeout = false
	ts.retained--
	if ts.retained < 0 {
		return false, errors.New("jobmgr task supervisor: negative retained-timeout count")
	}
	return true, nil
}

func (ts *TaskSupervisor) RetainedTimeouts() (int, bool) {
	return ts.retained, ts.saturated
}

type ResultPreflight struct {
	PlanBytes  int64
	FrameBytes int64
}

func (ts *TaskSupervisor) PreflightResult(ref TaskRef, uid string, expiry int64) (ResultPreflight, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return ResultPreflight{}, err
	}
	if slot.outcome.kind != TaskOutcomeFrame {
		return ResultPreflight{}, errors.New("jobmgr task supervisor: result is unavailable for preflight")
	}
	frame, err := PrepareFrame(uid, slot.outcome.frame, expiry)
	if err != nil {
		return ResultPreflight{}, err
	}
	encodedBytes, err := frame.encodedSize()
	if err != nil {
		return ResultPreflight{}, err
	}
	return ResultPreflight{PlanBytes: int64(len(slot.outcome.frame.payload)), FrameBytes: int64(encodedBytes)}, nil
}

func (ts *TaskSupervisor) TakePublishedReadyResource(ref TaskRef, sequence uint8, expected ResourceIdentity) (ReadyResource, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return nil, err
	}
	if slot.actionPending || slot.sequence != sequence || slot.outcome.kind != TaskOutcomeReadyResource || slot.outcome.ready == nil {
		return nil, errors.New("jobmgr task supervisor: ready resource is unavailable for publication")
	}
	resource := slot.outcome.ready
	if !expected.Valid() || slot.outcome.identity != expected {
		return nil, errors.New("jobmgr task supervisor: ready resource identity differs")
	}
	slot.outcome = TaskOutcome{}
	return resource, nil
}

func (ts *TaskSupervisor) TakeAppliedResourceTransaction(
	ref TaskRef,
	sequence uint8,
	expected ResourceTransactionScope,
) (ResourceTransactionDisposition, ReadyResource, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return 0, nil, err
	}
	outcome := slot.outcome
	if slot.actionPending ||
		slot.sequence != sequence ||
		outcome.kind != TaskOutcomeAppliedResourceTransaction ||
		!outcome.scopeSet ||
		outcome.scope != expected {
		return 0, nil, errors.New(
			"jobmgr task supervisor: applied resource transaction is unavailable",
		)
	}
	if err := outcome.validate(); err != nil {
		return 0, nil, err
	}
	slot.outcome = TaskOutcome{kind: TaskOutcomeFrame, frame: outcome.frame}
	return outcome.disposition, outcome.ready, nil
}

func (ts *TaskSupervisor) TakeDisposedResourceTransaction(
	ref TaskRef,
	sequence uint8,
	expected ResourceTransactionScope,
) (ReadyResource, error) {
	slot, err := ts.slot(ref)
	if err != nil {
		return nil, err
	}
	if slot.actionPending || slot.sequence != sequence {
		return nil, errors.New(
			"jobmgr task supervisor: disposed resource transaction is unavailable",
		)
	}
	if expected.Current.Valid() {
		if slot.outcome.kind != TaskOutcomeReadyResource ||
			slot.outcome.ready == nil ||
			slot.outcome.identity != expected.Current {
			return nil, errors.New(
				"jobmgr task supervisor: disposed transaction lost current resource",
			)
		}
		current := slot.outcome.ready
		slot.outcome = TaskOutcome{}
		return current, nil
	}
	if !slot.outcome.empty() {
		return nil, errors.New(
			"jobmgr task supervisor: empty disposed transaction retained an outcome",
		)
	}
	return nil, nil
}

func (ts *TaskSupervisor) runChild(ctx context.Context, ref TaskRef, slot *taskSlot, plan TaskPlan, outcome TaskOutcome) {
	var err error
	if outcome.empty() {
		if plan.Runner != nil {
			outcome, err = runTaskRunner(ctx, plan.Runner)
		} else {
			outcome, err = runTaskWork(ctx, plan.Work)
		}
	}
	if err != nil {
		err = normalizeStoppingCancellation(
			err,
			context.Cause(ctx),
		)
	}
	if publishErr := outcome.validate(); publishErr != nil {
		err = errors.Join(err, publishErr)
	} else if !outcome.empty() {
		slot.outcome = outcome
	}
	outcome = TaskOutcome{}
	slot.sequence = 1
	ts.completions <- TaskCompletion{
		Ref:      ref,
		Sequence: 1,
		Kind:     slot.outcome.kind,
		Err:      err,
	}
	ts.observeTaskPanic(err)
	for {
		action := <-slot.action
		ack := TaskAcknowledgement{Ref: ref, Sequence: action.Sequence, Kind: action.Kind}
		if action.Ref != ref || action.Sequence != slot.sequence+1 || action.Sequence > slot.maxPhaseTransitions {
			ack.Err = errors.New("jobmgr task child: stale or wrong-sequence phase action")
			slot.actionPending = false
			slot.joined = true
			ts.acks <- ack
			return
		}
		switch action.Kind {
		case TaskActionEncodeWrite:
			if slot.outcome.kind != TaskOutcomeFrame {
				ack.Err = errors.New("jobmgr task child: encode/write without published result")
			} else {
				frame, prepareErr := PrepareFrame(action.UID, slot.outcome.frame, action.Expiry)
				if prepareErr != nil {
					ack.Err = prepareErr
				} else {
					ack.Err = ts.frame.Commit(frame)
				}
			}
			slot.outcome = TaskOutcome{}
		case TaskActionAcceptStart:
			if slot.outcome.kind != TaskOutcomePreparedResource || slot.outcome.prepared == nil {
				ack.Err = errors.New("jobmgr task child: accept/start without prepared resource")
			} else {
				prepared := slot.outcome.prepared
				identity := slot.outcome.identity
				ready, acceptErr, panicked := runPreparedAcceptStart(
					context.WithoutCancel(ctx),
					prepared,
					action.ExpectedGeneration,
				)
				if acceptErr != nil {
					ack.Err = acceptErr
					if ready != nil {
						slot.outcome, ack.Err = readyResourceOutcome(ready, identity)
						ack.Err = errors.Join(acceptErr, ack.Err)
					} else if !panicked {
						slot.outcome = TaskOutcome{}
					}
				} else {
					slot.outcome, ack.Err = readyResourceOutcome(ready, identity)
				}
			}
		case TaskActionPublishResource:
			if slot.outcome.kind != TaskOutcomeReadyResource || slot.outcome.ready == nil {
				ack.Err = errors.New("jobmgr task child: publish without ready resource")
			} else {
				ack.Err = callReadyResource("publish", slot.outcome.ready.Publish)
			}
		case TaskActionCommitCapability:
			if slot.outcome.kind != TaskOutcomePreparedCapability || slot.outcome.capability == nil {
				ack.Err = errors.New("jobmgr task child: commit without prepared capability")
			} else {
				disposition, commitErr, panicked := runPreparedCapabilityCommit(
					context.WithoutCancel(ctx),
					slot.outcome.capability,
					action.ExpectedGeneration,
				)
				ack.CapabilityDisposition = disposition
				ack.Err = commitErr
				switch disposition {
				case CapabilityApplied:
					slot.outcome = TaskOutcome{}
				case CapabilityDisposed:
					slot.outcome = TaskOutcome{}
					if ack.Err == nil {
						ack.Err = errors.New("jobmgr task child: capability commit disposed without an error")
					}
				case CapabilityRetained:
					if ack.Err == nil {
						ack.Err = errors.New("jobmgr task child: capability commit retained without an error")
					}
				default:
					ack.CapabilityDisposition = CapabilityRetained
					ack.Err = errors.Join(ack.Err, errors.New("jobmgr task child: invalid capability disposition"))
				}
				if panicked {
					ack.CapabilityDisposition = CapabilityRetained
				}
			}
		case TaskActionStopResource:
			if slot.outcome.kind != TaskOutcomeReadyResource || slot.outcome.ready == nil {
				ack.Err = errors.New("jobmgr task child: stop without ready resource")
			} else {
				ack.Err = callReadyResource("stop", func() error {
					return slot.outcome.ready.Stop(
						context.WithoutCancel(ctx),
					)
				})
			}
		case TaskActionFinalizeResource:
			if slot.outcome.kind != TaskOutcomeReadyResource || slot.outcome.ready == nil {
				ack.Err = errors.New("jobmgr task child: finalize without ready resource")
			} else {
				ack.Err = callReadyResource("finalize", slot.outcome.ready.Finalize)
				if ack.Err == nil {
					slot.outcome = TaskOutcome{}
				}
			}
		case TaskActionApplyResourceTransaction:
			if slot.outcome.kind != TaskOutcomePreparedResourceTransaction ||
				slot.outcome.transaction == nil ||
				!slot.outcome.scopeSet {
				ack.Err = errors.New(
					"jobmgr task child: apply without prepared resource transaction",
				)
			} else {
				transaction := slot.outcome.transaction
				scope := slot.outcome.scope
				applied, applyErr := runPreparedResourceTransactionApply(
					context.WithoutCancel(ctx),
					transaction,
				)
				if applyErr != nil {
					ack.Err = applyErr
				} else if applied.scope != scope {
					ack.Err = errors.New(
						"jobmgr task child: applied resource transaction changed scope",
					)
				} else {
					next, outcomeErr := appliedResourceTransactionOutcome(applied)
					ack.Err = outcomeErr
					if outcomeErr == nil {
						slot.outcome = next
						slot.cleanup = applied.cleanup
					}
				}
			}
			slot.sequence = action.Sequence
			slot.actionPending = false
			ts.completions <- TaskCompletion{
				Ref:      ref,
				Sequence: action.Sequence,
				Kind:     slot.outcome.kind,
				Err:      ack.Err,
			}
			ts.observeTaskPanic(ack.Err)
			continue
		case TaskActionDispose:
			if slot.outcome.kind == TaskOutcomePreparedResourceTransaction &&
				slot.outcome.transaction != nil {
				transaction := slot.outcome.transaction
				scope := slot.outcome.scope
				current, disposeErr := runPreparedResourceTransactionDispose(
					disposeContext(ctx, slot.preserveDisposeContext),
					transaction,
				)
				ack.Err = disposeErr
				if disposeErr == nil {
					if scope.Current.Valid() {
						slot.outcome, ack.Err = readyResourceOutcome(
							current,
							scope.Current,
						)
					} else if current != nil {
						ack.Err = errors.New(
							"jobmgr task child: graph-only transaction disposal returned a resource",
						)
					} else {
						slot.outcome = TaskOutcome{}
					}
				}
			} else {
				ack.Err = disposeTaskOutcome(
					ctx,
					slot.outcome,
					slot.preserveDisposeContext,
				)
				if ack.Err == nil {
					slot.outcome = TaskOutcome{}
				}
			}
		case TaskActionCleanup:
			if !slot.outcome.empty() {
				ack.Err = errors.New("jobmgr task child: cleanup before result disposition")
			} else if slot.cleanup == nil {
				ack.Err = errors.New("jobmgr task child: cleanup phase is unavailable")
			} else {
				ack.Err = runTaskCleanup(slot.cleanup)
				slot.cleanup = nil
			}
		case TaskActionTerminate:
			if !slot.outcome.empty() {
				ack.Err = errors.New("jobmgr task child: terminate with published result")
			}
		default:
			ack.Err = errors.New("jobmgr task child: unsupported phase action")
		}
		slot.sequence = action.Sequence
		slot.actionPending = false
		if action.Kind == TaskActionTerminate {
			slot.joined = true
			ts.acks <- ack
			return
		}
		if action.Kind == TaskActionDispose &&
			ack.Err == nil &&
			ts.observer != nil {
			ts.observer.AddRuntimeCounter(
				RuntimeCounterResultsDisposed,
				1,
			)
		}
		ts.observeTaskPanic(ack.Err)
		ts.acks <- ack
	}
}

func normalizeStoppingCancellation(
	err error,
	cause error,
) error {
	stopping, ok := cause.(*StoppingRejection)
	if !ok {
		return err
	}
	if !allErrorLeavesMatch(err, func(leaf error) bool {
		return leaf == context.Canceled
	}) {
		return err
	}
	return stopping
}

func (ts *TaskSupervisor) observeTaskPanic(err error) {
	if errors.Is(err, ErrTaskPanic) && ts.observer != nil {
		ts.observer.AddRuntimeCounter(RuntimeCounterTaskPanics, 1)
	}
}

func runTaskWork(ctx context.Context, work TaskWork) (result TaskOutcome, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = TaskOutcome{}
			err = fmt.Errorf("%w: %v", ErrTaskPanic, recovered)
		}
	}()
	return work(ctx)
}

func runTaskRunner(ctx context.Context, runner TaskRunner) (result TaskOutcome, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = TaskOutcome{}
			err = fmt.Errorf("%w: %v", ErrTaskPanic, recovered)
		}
	}()
	return runner.RunTask(ctx)
}

func runPreparedAcceptStart(ctx context.Context, prepared PreparedResource, expected uint64) (ready ReadyResource, err error, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			ready = nil
			err = fmt.Errorf("%w in prepared-resource accept/start: %v", ErrTaskPanic, recovered)
			panicked = true
		}
	}()
	ready, err = prepared.AcceptStart(ctx, expected)
	return ready, err, false
}

func runPreparedCapabilityCommit(ctx context.Context, capability PreparedCapability, expected uint64) (disposition CapabilityDisposition, err error, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			disposition = CapabilityRetained
			err = fmt.Errorf("%w in prepared-capability commit: %v", ErrTaskPanic, recovered)
			panicked = true
		}
	}()
	disposition, err = capability.Commit(ctx, expected)
	return disposition, err, false
}

func runPreparedResourceTransactionApply(
	ctx context.Context,
	transaction PreparedResourceTransaction,
) (applied AppliedResourceTransaction, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			applied = AppliedResourceTransaction{}
			err = fmt.Errorf(
				"%w in prepared resource transaction apply: %v",
				ErrTaskPanic,
				recovered,
			)
		}
	}()
	return transaction.Apply(ctx)
}

func runPreparedResourceTransactionDispose(
	ctx context.Context,
	transaction PreparedResourceTransaction,
) (current ReadyResource, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			current = nil
			err = fmt.Errorf(
				"%w in prepared resource transaction dispose: %v",
				ErrTaskPanic,
				recovered,
			)
		}
	}()
	return transaction.Dispose(ctx)
}

func disposeContext(ctx context.Context, preserve bool) context.Context {
	if preserve {
		return ctx
	}
	return context.WithoutCancel(ctx)
}

func disposeTaskOutcome(ctx context.Context, outcome TaskOutcome, preserveContext bool) error {
	cleanupCtx := disposeContext(ctx, preserveContext)
	switch outcome.kind {
	case TaskOutcomeNone, TaskOutcomeFrame:
		return nil
	case TaskOutcomePreparedResource:
		return callReadyResource("dispose prepared", func() error { return outcome.prepared.Dispose(cleanupCtx) })
	case TaskOutcomeReadyResource:
		return callReadyResource("abort ready", func() error { return outcome.ready.AbortReady(cleanupCtx) })
	case TaskOutcomePreparedCapability:
		return callReadyResource("dispose prepared capability", func() error { return outcome.capability.Dispose(cleanupCtx) })
	case TaskOutcomePreparedResourceTransaction:
		return errors.New(
			"jobmgr task child: resource transaction requires transfer-aware disposal",
		)
	case TaskOutcomeAppliedResourceTransaction:
		return errors.New(
			"jobmgr task child: applied resource transaction requires kernel transfer",
		)
	default:
		return errors.New("jobmgr task child: cannot dispose unknown outcome")
	}
}

func callReadyResource(operation string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in resource %s: %v", ErrTaskPanic, operation, recovered)
		}
	}()
	return call()
}

func runTaskCleanup(cleanup TaskCleanup) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in cleanup: %v", ErrTaskPanic, recovered)
		}
	}()
	return cleanup()
}

func (ts *TaskSupervisor) slot(ref TaskRef) (*taskSlot, error) {
	if !ref.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid task reference")
	}
	if uint64(ref.Slot) >= uint64(len(ts.slots)) ||
		ts.slots[ref.Slot] == nil {
		return nil, errors.New("jobmgr task supervisor: stale task reference")
	}
	slot := ts.slots[ref.Slot]
	if !slot.active || slot.generation != ref.Generation {
		return nil, errors.New("jobmgr task supervisor: stale task reference")
	}
	return slot, nil
}

func (ts *TaskSupervisor) request(ref TaskRequestRef) (*taskRequest, error) {
	if !ref.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid request reference")
	}
	if uint64(ref.Slot) >= uint64(len(ts.requests)) ||
		ts.requests[ref.Slot] == nil {
		return nil, errors.New("jobmgr task supervisor: stale request reference")
	}
	record := ts.requests[ref.Slot]
	if !record.active || record.generation != ref.Generation {
		return nil, errors.New("jobmgr task supervisor: stale request reference")
	}
	return record, nil
}

func (ts *TaskSupervisor) removeRequest(record *taskRequest) {
	queue := &ts.pending[taskClassIndex(record.class)]
	if record.previous != 0 {
		ts.requests[record.previous].next = record.next
	} else {
		queue.head = record.next
	}
	if record.next != 0 {
		ts.requests[record.next].previous = record.previous
	} else {
		queue.tail = record.previous
	}
	queue.count--
	slot := record.slot
	generation := record.generation
	*record = taskRequest{slot: slot, generation: generation, freeNext: ts.freeRequest}
	ts.freeRequest = slot
	ts.observeRuntimeState()
}

func (ts *TaskSupervisor) observeRuntimeState() {
	if ts == nil || ts.observer == nil {
		return
	}
	ts.observer.SetRuntimeGauge(
		RuntimeGaugeTasksActive,
		ts.active,
	)
	ts.observer.SetRuntimeGauge(
		RuntimeGaugeTasksQueued,
		ts.Pending(),
	)
	var oldest time.Time
	for index := range ts.pending {
		head := ts.pending[index].head
		if head == 0 {
			continue
		}
		queuedAt := ts.requests[head].queuedAt
		if oldest.IsZero() || queuedAt.Before(oldest) {
			oldest = queuedAt
		}
	}
	ts.observer.SetRuntimeTimestamp(
		RuntimeTimestampOldestTaskWait,
		oldest,
	)
}

func taskClassIndex(class TaskClass) int {
	if class == TaskClassFrameworkControl {
		return 0
	}
	return 1
}

func classForTaskIndex(index int) TaskClass {
	if index == 0 {
		return TaskClassFrameworkControl
	}
	return TaskClassGenericFunction
}

func otherTaskClass(class TaskClass) TaskClass {
	if class == TaskClassFrameworkControl {
		return TaskClassGenericFunction
	}
	return TaskClassFrameworkControl
}

func emptySealedResult(result SealedResult) bool {
	return result.status == 0 && result.contentType == "" && len(result.payload) == 0
}
