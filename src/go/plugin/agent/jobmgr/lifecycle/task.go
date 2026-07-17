// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrTaskPanic = errors.New("jobmgr task child: panic")

type TaskRef struct {
	Slot       uint8
	Generation uint64
}

type TaskRequestRef struct {
	Slot       uint16
	Generation uint32
}

func (ref TaskRequestRef) Valid() bool {
	return ref.Slot > 0 && ref.Slot <= MaximumAdmissionRecords && ref.Generation > 0
}

type TaskStart struct {
	Request TaskRequestRef
	Task    TaskRef
	Err     error
}

func (ref TaskRef) Valid() bool {
	return int(ref.Slot) < TransientTaskSlots && ref.Generation > 0
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
	TaskActionCommitCapability
	TaskActionStopResource
	TaskActionFinalizeResource
	TaskActionDispose
	TaskActionCleanup
	TaskActionTerminate
)

type TaskAction struct {
	Ref                TaskRef
	Sequence           uint8
	Kind               TaskActionKind
	UID                string
	Expiry             int64
	ExpectedGeneration uint64
}

type TaskAcknowledgement struct {
	Ref                   TaskRef
	Sequence              uint8
	Kind                  TaskActionKind
	CapabilityDisposition CapabilityDisposition
	Err                   error
}

type taskSlot struct {
	generation             uint64
	active                 bool
	joined                 bool
	actionPending          bool
	sequence               uint8
	maxPhaseTransitions    uint8
	outcome                TaskOutcome
	cancel                 context.CancelCauseFunc
	cleanup                TaskCleanup
	preserveDisposeContext bool
	retainedTimeout        bool
	action                 chan TaskAction
}

type taskRequest struct {
	slot       uint16
	generation uint32
	freeNext   uint16
	previous   uint16
	next       uint16
	active     bool
	plan       TaskPlan
	initial    TaskOutcome
}

type taskRequestQueue struct {
	head  uint16
	tail  uint16
	count int
}

type TaskSupervisor struct {
	frame       *FrameOwner
	inherited   inheritedTaskRegistry
	longLived   longLivedRegistry
	slots       [TransientTaskSlots]taskSlot
	requests    [MaximumAdmissionRecords + 1]taskRequest
	pending     [2]taskRequestQueue
	freeRequest uint16
	nextSource  Source
	completions chan TaskCompletion
	acks        chan TaskAcknowledgement
	active      int
	retained    int
	saturated   bool
}

func NewTaskSupervisor(frame *FrameOwner) (*TaskSupervisor, error) {
	if frame == nil {
		return nil, errors.New("jobmgr task supervisor: nil FrameOwner")
	}
	supervisor := &TaskSupervisor{frame: frame, completions: make(chan TaskCompletion, TransientTaskSlots), acks: make(chan TaskAcknowledgement, TransientTaskSlots)}
	supervisor.inherited.initialize()
	supervisor.longLived.initialize()
	for index := range supervisor.slots {
		supervisor.slots[index].action = make(chan TaskAction, 1)
	}
	for index := 1; index <= MaximumAdmissionRecords; index++ {
		supervisor.requests[index].slot = uint16(index)
		supervisor.requests[index].freeNext = uint16(index + 1)
	}
	supervisor.requests[MaximumAdmissionRecords].freeNext = 0
	supervisor.freeRequest = 1
	supervisor.nextSource = SourceJobManager
	return supervisor, nil
}

func (supervisor *TaskSupervisor) Enqueue(plan TaskPlan) (TaskRequestRef, error) {
	if err := plan.Validate(); err != nil {
		return TaskRequestRef{}, err
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
	slot := supervisor.freeRequest
	if slot == 0 {
		return TaskRequestRef{}, errors.New("jobmgr task supervisor: pending request capacity exhausted")
	}
	record := &supervisor.requests[slot]
	supervisor.freeRequest = record.freeNext
	generation := record.generation + 1
	if generation == 0 {
		record.freeNext = supervisor.freeRequest
		supervisor.freeRequest = slot
		return TaskRequestRef{}, errors.New("jobmgr task supervisor: request generation wrapped")
	}
	*record = taskRequest{slot: slot, generation: generation, active: true, plan: plan, initial: initial}
	queue := &supervisor.pending[taskSourceIndex(plan.Source)]
	record.previous = queue.tail
	if queue.tail != 0 {
		supervisor.requests[queue.tail].next = slot
	} else {
		queue.head = slot
	}
	queue.tail = slot
	queue.count++
	return TaskRequestRef{Slot: slot, Generation: generation}, nil
}

func (supervisor *TaskSupervisor) CancelPending(ref TaskRequestRef) error {
	record, err := supervisor.request(ref)
	if err != nil {
		return err
	}
	if !record.initial.empty() {
		return errors.New("jobmgr task supervisor: pending ready resource requires transfer-aware cancellation")
	}
	supervisor.removeRequest(record)
	return nil
}

func (supervisor *TaskSupervisor) SetPendingCancellation(ref TaskRequestRef, cause error) error {
	record, err := supervisor.request(ref)
	if err != nil {
		return err
	}
	if cause != context.Canceled && cause != context.DeadlineExceeded {
		return errors.New("jobmgr task supervisor: invalid pending cancellation")
	}
	if record.plan.InitialCancellation != nil {
		return errors.New("jobmgr task supervisor: pending cancellation already set")
	}
	if cause == context.DeadlineExceeded && record.plan.Deadline.IsZero() {
		return errors.New("jobmgr task supervisor: pending deadline cancellation has no deadline")
	}
	record.plan.InitialCancellation = cause
	return nil
}

func (supervisor *TaskSupervisor) CancelPendingOutcome(ref TaskRequestRef) (TaskOutcome, error) {
	record, err := supervisor.request(ref)
	if err != nil {
		return TaskOutcome{}, err
	}
	var outcome TaskOutcome
	if !record.initial.empty() {
		outcome = record.initial
		record.initial = TaskOutcome{}
	}
	supervisor.removeRequest(record)
	return outcome, nil
}

func (supervisor *TaskSupervisor) Dispatch(parent context.Context, quantum int, started *[TransientTaskSlots]TaskStart) (int, bool, error) {
	if parent == nil || started == nil || quantum < 0 || quantum > len(started) {
		return 0, supervisor.Pending() > 0, errors.New("jobmgr task supervisor: invalid dispatch")
	}
	count := 0
	for count < quantum && supervisor.active < TransientTaskSlots {
		first := taskSourceIndex(supervisor.nextSource)
		second := 1 - first
		selected := first
		if supervisor.pending[selected].head == 0 {
			selected = second
		}
		queue := &supervisor.pending[selected]
		if queue.head == 0 {
			break
		}
		record := &supervisor.requests[queue.head]
		requestRef := TaskRequestRef{Slot: record.slot, Generation: record.generation}
		taskRef, err := supervisor.start(parent, record.plan, record.initial)
		if err != nil {
			if errors.Is(err, ErrLongLivedRecordCapacity) {
				supervisor.removeRequest(record)
				started[count] = TaskStart{Request: requestRef, Err: err}
				count++
				supervisor.nextSource = otherTaskSource(sourceForTaskIndex(selected))
				continue
			}
			return count, true, err
		}
		supervisor.removeRequest(record)
		started[count] = TaskStart{Request: requestRef, Task: taskRef}
		count++
		supervisor.nextSource = otherTaskSource(sourceForTaskIndex(selected))
	}
	return count, supervisor.Pending() > 0, nil
}

func (supervisor *TaskSupervisor) Pending() int {
	return supervisor.pending[0].count + supervisor.pending[1].count
}

func (supervisor *TaskSupervisor) start(parent context.Context, plan TaskPlan, initial TaskOutcome) (TaskRef, error) {
	if ((plan.Work == nil && plan.permitWork == nil && plan.capabilityPermitWork == nil) == initial.empty()) || plan.initialReady != nil || plan.initialIdentity.Valid() {
		return TaskRef{}, errors.New("jobmgr task supervisor: invalid sealed task request")
	}
	for index := range supervisor.slots {
		slot := &supervisor.slots[index]
		if slot.active {
			continue
		}
		if len(slot.action) != 0 || slot.actionPending || !slot.outcome.empty() || slot.sequence != 0 || slot.joined || slot.retainedTimeout {
			return TaskRef{}, errors.New("jobmgr task supervisor: dirty reusable slot")
		}
		slot.generation++
		if slot.generation == 0 {
			return TaskRef{}, errors.New("jobmgr task supervisor: slot generation wrapped")
		}
		if plan.permitWork != nil {
			permit, err := supervisor.IssueLongLivedPermit(plan.permitAdmission, plan.permitAdmissionRef, plan.permitOwner, plan.permitPlan)
			if err != nil {
				return TaskRef{}, err
			}
			permitWork := plan.permitWork
			permitOwner := plan.permitOwner
			plan.Work = func(ctx context.Context) (TaskOutcome, error) {
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
			permit, err := supervisor.IssueLongLivedPermit(plan.permitAdmission, plan.permitAdmissionRef, plan.permitOwner, plan.permitPlan)
			if err != nil {
				return TaskRef{}, err
			}
			permitWork := plan.capabilityPermitWork
			permitOwner := plan.permitOwner
			plan.Work = func(ctx context.Context) (TaskOutcome, error) {
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
		ref := TaskRef{Slot: uint8(index), Generation: slot.generation}
		supervisor.active++
		go supervisor.runChild(ctx, ref, plan, initial)
		return ref, nil
	}
	return TaskRef{}, errors.New("jobmgr task supervisor: no transient slot")
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

func (supervisor *TaskSupervisor) CompletionCh() <-chan TaskCompletion {
	return supervisor.completions
}

func (supervisor *TaskSupervisor) AcknowledgementCh() <-chan TaskAcknowledgement {
	return supervisor.acks
}

func (supervisor *TaskSupervisor) SendAction(action TaskAction) error {
	if !action.Ref.Valid() || action.Kind < TaskActionEncodeWrite || action.Kind > TaskActionTerminate {
		return errors.New("jobmgr task supervisor: invalid task action")
	}
	slot, err := supervisor.slot(action.Ref)
	if err != nil {
		return err
	}
	if slot.joined || slot.actionPending || action.Sequence != slot.sequence+1 || action.Sequence > slot.maxPhaseTransitions {
		return errors.New("jobmgr task supervisor: stale, duplicate, or wrong-sequence phase action")
	}
	if action.Kind == TaskActionEncodeWrite && (action.UID == "" || action.Expiry <= 0 || action.ExpectedGeneration != 0) {
		return errors.New("jobmgr task supervisor: invalid encode/write action")
	}
	if (action.Kind == TaskActionAcceptStart || action.Kind == TaskActionCommitCapability) && (action.ExpectedGeneration == 0 || action.UID != "" || action.Expiry != 0) {
		return errors.New("jobmgr task supervisor: invalid generation-bound action")
	}
	if action.Kind != TaskActionEncodeWrite && action.Kind != TaskActionAcceptStart && action.Kind != TaskActionCommitCapability && (action.UID != "" || action.Expiry != 0 || action.ExpectedGeneration != 0) {
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

func (supervisor *TaskSupervisor) Cancel(ref TaskRef) error {
	return supervisor.CancelWithCause(ref, context.Canceled)
}

func (supervisor *TaskSupervisor) CancelWithCause(ref TaskRef, cause error) error {
	if cause == nil {
		return errors.New("jobmgr task supervisor: nil cancellation cause")
	}
	slot, err := supervisor.slot(ref)
	if err != nil {
		return err
	}
	slot.cancel(cause)
	return nil
}

func (supervisor *TaskSupervisor) Release(ref TaskRef) error {
	slot, err := supervisor.slot(ref)
	if err != nil {
		return err
	}
	if !slot.joined || slot.actionPending || len(slot.action) != 0 || !slot.outcome.empty() || slot.retainedTimeout {
		return errors.New("jobmgr task supervisor: release before empty joined acknowledgement")
	}
	slot.cancel(context.Canceled)
	slot.active = false
	slot.cancel = nil
	slot.cleanup = nil
	slot.joined = false
	slot.sequence = 0
	slot.maxPhaseTransitions = 0
	supervisor.active--
	return nil
}

func (supervisor *TaskSupervisor) Active() int {
	return supervisor.active
}

func (supervisor *TaskSupervisor) MarkRetainedTimeout(ref TaskRef) (bool, error) {
	slot, err := supervisor.slot(ref)
	if err != nil {
		return false, err
	}
	if slot.retainedTimeout {
		return false, errors.New("jobmgr task supervisor: duplicate retained timeout")
	}
	slot.retainedTimeout = true
	supervisor.retained++
	if supervisor.retained > TransientTaskSlots {
		return false, errors.New("jobmgr task supervisor: retained-timeout count exceeded task slots")
	}
	justSaturated := supervisor.retained == TransientTaskSlots && !supervisor.saturated
	if justSaturated {
		supervisor.saturated = true
	}
	return justSaturated, nil
}

func (supervisor *TaskSupervisor) ClearRetainedTimeout(ref TaskRef) (bool, error) {
	slot, err := supervisor.slot(ref)
	if err != nil {
		return false, err
	}
	if !slot.retainedTimeout {
		return false, nil
	}
	slot.retainedTimeout = false
	supervisor.retained--
	if supervisor.retained < 0 {
		return false, errors.New("jobmgr task supervisor: negative retained-timeout count")
	}
	return true, nil
}

func (supervisor *TaskSupervisor) RetainedTimeouts() (int, bool) {
	return supervisor.retained, supervisor.saturated
}

type ResultPreflight struct {
	PlanBytes  int64
	FrameBytes int64
}

func (supervisor *TaskSupervisor) PreflightResult(ref TaskRef, uid string, expiry int64) (ResultPreflight, error) {
	slot, err := supervisor.slot(ref)
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
	return ResultPreflight{PlanBytes: slot.outcome.frame.planBytes, FrameBytes: int64(encodedBytes)}, nil
}

func (supervisor *TaskSupervisor) PublishAndTakeReadyResource(ref TaskRef, sequence uint8, expected ResourceIdentity) (ReadyResource, error) {
	slot, err := supervisor.slot(ref)
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
	if err := callReadyResource("publish", resource.Publish); err != nil {
		return nil, err
	}
	slot.outcome = TaskOutcome{}
	return resource, nil
}

func (supervisor *TaskSupervisor) runChild(ctx context.Context, ref TaskRef, plan TaskPlan, outcome TaskOutcome) {
	var err error
	if outcome.empty() {
		outcome, err = runTaskWork(ctx, plan.Work)
	}
	slot := &supervisor.slots[ref.Slot]
	if publishErr := outcome.validate(); publishErr != nil {
		err = errors.Join(err, publishErr)
	} else if !outcome.empty() {
		slot.outcome = outcome
	}
	outcome = TaskOutcome{}
	slot.sequence = 1
	supervisor.completions <- TaskCompletion{Ref: ref, Sequence: 1, Kind: slot.outcome.kind, Err: err}
	for {
		action := <-slot.action
		ack := TaskAcknowledgement{Ref: ref, Sequence: action.Sequence, Kind: action.Kind}
		if action.Ref != ref || action.Sequence != slot.sequence+1 || action.Sequence > slot.maxPhaseTransitions {
			ack.Err = errors.New("jobmgr task child: stale or wrong-sequence phase action")
			slot.actionPending = false
			slot.joined = true
			supervisor.acks <- ack
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
					ack.Err = supervisor.frame.Commit(frame)
				}
			}
			slot.outcome = TaskOutcome{}
		case TaskActionAcceptStart:
			if slot.outcome.kind != TaskOutcomePreparedResource || slot.outcome.prepared == nil {
				ack.Err = errors.New("jobmgr task child: accept/start without prepared resource")
			} else {
				prepared := slot.outcome.prepared
				identity := slot.outcome.identity
				ready, acceptErr, panicked := runPreparedAcceptStart(ctx, prepared, action.ExpectedGeneration)
				if acceptErr != nil {
					ack.Err = acceptErr
					if ready != nil {
						slot.outcome, ack.Err = readyResourceOutcome(ready, identity)
						ack.Err = errors.Join(acceptErr, ack.Err)
					} else if !panicked {
						slot.outcome = TaskOutcome{}
					}
				} else {
					slot.outcome = TaskOutcome{}
					slot.outcome, ack.Err = readyResourceOutcome(ready, identity)
				}
			}
		case TaskActionCommitCapability:
			if slot.outcome.kind != TaskOutcomePreparedCapability || slot.outcome.capability == nil {
				ack.Err = errors.New("jobmgr task child: commit without prepared capability")
			} else {
				disposition, commitErr, panicked := runPreparedCapabilityCommit(ctx, slot.outcome.capability, action.ExpectedGeneration)
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
				ack.Err = callReadyResource("stop", func() error { return slot.outcome.ready.Stop(ctx) })
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
		case TaskActionDispose:
			ack.Err = disposeTaskOutcome(ctx, slot.outcome, slot.preserveDisposeContext)
			if ack.Err == nil {
				slot.outcome = TaskOutcome{}
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
			supervisor.acks <- ack
			return
		}
		supervisor.acks <- ack
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

func disposeTaskOutcome(ctx context.Context, outcome TaskOutcome, preserveContext bool) error {
	cleanupCtx := ctx
	if !preserveContext {
		cleanupCtx = context.WithoutCancel(ctx)
	}
	switch outcome.kind {
	case TaskOutcomeNone, TaskOutcomeFrame:
		return nil
	case TaskOutcomePreparedResource:
		return callReadyResource("dispose prepared", func() error { return outcome.prepared.Dispose(cleanupCtx) })
	case TaskOutcomeReadyResource:
		return callReadyResource("abort ready", func() error { return outcome.ready.AbortReady(cleanupCtx) })
	case TaskOutcomePreparedCapability:
		return callReadyResource("dispose prepared capability", func() error { return outcome.capability.Dispose(cleanupCtx) })
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

func (supervisor *TaskSupervisor) slot(ref TaskRef) (*taskSlot, error) {
	if !ref.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid task reference")
	}
	slot := &supervisor.slots[ref.Slot]
	if !slot.active || slot.generation != ref.Generation {
		return nil, errors.New("jobmgr task supervisor: stale task reference")
	}
	return slot, nil
}

func (supervisor *TaskSupervisor) request(ref TaskRequestRef) (*taskRequest, error) {
	if !ref.Valid() {
		return nil, errors.New("jobmgr task supervisor: invalid request reference")
	}
	record := &supervisor.requests[ref.Slot]
	if !record.active || record.generation != ref.Generation {
		return nil, errors.New("jobmgr task supervisor: stale request reference")
	}
	return record, nil
}

func (supervisor *TaskSupervisor) removeRequest(record *taskRequest) {
	queue := &supervisor.pending[taskSourceIndex(record.plan.Source)]
	if record.previous != 0 {
		supervisor.requests[record.previous].next = record.next
	} else {
		queue.head = record.next
	}
	if record.next != 0 {
		supervisor.requests[record.next].previous = record.previous
	} else {
		queue.tail = record.previous
	}
	queue.count--
	slot := record.slot
	generation := record.generation
	*record = taskRequest{slot: slot, generation: generation, freeNext: supervisor.freeRequest}
	supervisor.freeRequest = slot
}

func taskSourceIndex(source Source) int {
	if source == SourceJobManager {
		return 0
	}
	return 1
}

func sourceForTaskIndex(index int) Source {
	if index == 0 {
		return SourceJobManager
	}
	return SourceFunction
}

func otherTaskSource(source Source) Source {
	if source == SourceJobManager {
		return SourceFunction
	}
	return SourceJobManager
}

func emptySealedResult(result SealedResult) bool {
	return result.status == 0 && result.contentType == "" && result.payloadKind == 0 && len(result.payload) == 0 && result.value.kind == valueInvalid && result.payloadBytes == 0 && result.planBytes == 0
}
