// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const externalSourceQueueDepth = lifecycle.MaximumLaneDepth

const maximumFunctionCleanupBacklog = lifecycle.MaximumAdmissionRecords + MaximumFunctionMutationChanges

const (
	maximumPlanClaims     = 1_024
	maximumClaimKeyBytes  = maximumRequestArgumentBytes
	maximumPlanClaimBytes = lifecycle.ControlFrameBytes
)

var ErrStopped = errors.New("jobmgr kernel: stopped")

type WorkPlan struct {
	Claims              []string
	ReadClaims          []string
	OwnedBytes          int64
	Work                lifecycle.TaskWork
	Runner              lifecycle.TaskRunner
	Resource            *ResourcePlan
	Transaction         *ResourceTransactionPlan
	Capability          *CapabilityPlan
	NoResponse          bool
	Cleanup             lifecycle.TaskCleanup
	CooperativeCancel   bool
	CooperativeDeadline bool
}

type ResourceAction uint8

const (
	ResourceInstall ResourceAction = iota + 1
	ResourceStop
)

type ResourcePlan struct {
	Action  ResourceAction
	ID      string
	Permit  lifecycle.LongLivedPlan
	Prepare func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedResource, error)
}

type ResourceTransactionPlan struct {
	ID                string
	AllocateSuccessor bool
	Permit            lifecycle.LongLivedPlan
	Prepare           lifecycle.PreparedResourceTransactionWork
}

type CapabilityPlan struct {
	ID      string
	Permit  lifecycle.LongLivedPlan
	Prepare func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error)
}

func (plan WorkPlan) validate() error {
	if plan.OwnedBytes < 0 {
		return errors.New("jobmgr kernel: negative plan-owned bytes")
	}
	if len(plan.Claims) > maximumPlanClaims-len(plan.ReadClaims) {
		return errors.New("jobmgr kernel: too many plan claims")
	}
	claimBytes := 0
	for _, claims := range [][]string{plan.Claims, plan.ReadClaims} {
		for _, key := range claims {
			if key == "" || len(key) > maximumClaimKeyBytes ||
				len(key) > maximumPlanClaimBytes-claimBytes {
				return errors.New("jobmgr kernel: invalid or oversized claim key")
			}
			claimBytes += len(key)
		}
	}
	workKinds := 0
	if plan.Work != nil {
		workKinds++
	}
	if plan.Runner != nil {
		workKinds++
	}
	if plan.Resource != nil {
		workKinds++
	}
	if plan.Transaction != nil {
		workKinds++
	}
	if plan.Capability != nil {
		workKinds++
	}
	if workKinds != 1 {
		return errors.New("jobmgr kernel: plan must have exactly one work kind")
	}
	if plan.Work != nil || plan.Runner != nil {
		if plan.NoResponse {
			return errors.New("jobmgr kernel: frame work cannot suppress its response")
		}
		return nil
	}
	if plan.Transaction != nil {
		if plan.Cleanup != nil {
			return errors.New("jobmgr kernel: resource transaction cleanup must be sealed by apply")
		}
		if plan.Transaction.ID == "" || plan.Transaction.Prepare == nil {
			return errors.New("jobmgr kernel: invalid resource transaction plan")
		}
		if plan.Transaction.AllocateSuccessor {
			if err := plan.Transaction.Permit.Validate(); err != nil {
				return errors.Join(
					errors.New("jobmgr kernel: transaction successor has no long-lived permit"),
					err,
				)
			}
		} else if plan.Transaction.Permit.Class() != 0 ||
			plan.Transaction.Permit.Bytes() != 0 {
			return errors.New("jobmgr kernel: transaction without successor has a permit")
		}
		return nil
	}
	if !plan.NoResponse {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	if plan.Cleanup != nil {
		return errors.New("jobmgr kernel: resource plan cannot add an unrelated task cleanup")
	}
	if plan.Capability != nil {
		if plan.Capability.ID == "" || plan.Capability.Prepare == nil {
			return errors.New("jobmgr kernel: invalid prepared capability plan")
		}
		if err := plan.Capability.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: capability plan has no long-lived permit"), err)
		}
		return nil
	}
	if plan.Resource.ID == "" {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	switch plan.Resource.Action {
	case ResourceInstall:
		if plan.Resource.Prepare == nil {
			return errors.New("jobmgr kernel: install resource plan has no factory")
		}
		if err := plan.Resource.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: install resource plan has no long-lived permit"), err)
		}
	case ResourceStop:
		if plan.Resource.Prepare != nil || plan.Resource.Permit.Class() != 0 || plan.Resource.Permit.Bytes() != 0 {
			return errors.New("jobmgr kernel: stop resource plan has a factory")
		}
	default:
		return errors.New("jobmgr kernel: unknown resource action")
	}
	return nil
}

type Planner interface {
	Plan(Request) (WorkPlan, error)
}

type RunFinalizer interface {
	FinalizeRun(context.Context, uint64) error
}

// RunShutdownBarrier performs blocking external withdrawal before the
// loop-owned Function catalog begins close. It runs as one supervised shutdown
// task, never on KernelLoop.
type RunShutdownBarrier interface {
	BeforeFunctionCatalogClose(context.Context, uint64) error
}

type RunShutdownBarrierFunc func(context.Context, uint64) error

func (fn RunShutdownBarrierFunc) BeforeFunctionCatalogClose(
	ctx context.Context,
	generation uint64,
) error {
	return fn(ctx, generation)
}

type RunFinalizerFunc func(context.Context, uint64) error

func (fn RunFinalizerFunc) FinalizeRun(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}

type noopRunFinalizer struct{}

func (noopRunFinalizer) FinalizeRun(context.Context, uint64) error { return nil }

func newNoopRunFinalizer() RunFinalizer { return noopRunFinalizer{} }

type noopRunShutdownBarrier struct{}

func (noopRunShutdownBarrier) BeforeFunctionCatalogClose(context.Context, uint64) error {
	return nil
}

func newNoopRunShutdownBarrier() RunShutdownBarrier { return noopRunShutdownBarrier{} }

type submission struct {
	request       Request
	plan          WorkPlan
	context       context.Context
	controlStatus lifecycle.ControlStatus
	result        chan error
	terminal      chan error
}

type commandLaneKey struct {
	source        lifecycle.Source
	functionRoute uint64
	key           string
}

type functionCleanupTask struct {
	ref FunctionCleanupRef
	err error
}

type functionCleanupQueue struct {
	plans [maximumFunctionCleanupBacklog]FunctionCleanupPlan
	head  int
	count int
}

func (queue *functionCleanupQueue) free() int {
	return len(queue.plans) - queue.count
}

func (queue *functionCleanupQueue) push(plan FunctionCleanupPlan) error {
	if err := plan.validate(); err != nil {
		return err
	}
	if !plan.Ref.Valid() {
		return nil
	}
	if queue.count == len(queue.plans) {
		return errors.New("jobmgr kernel: Function cleanup backlog exhausted")
	}
	index := (queue.head + queue.count) % len(queue.plans)
	queue.plans[index] = plan
	queue.count++
	return nil
}

func (queue *functionCleanupQueue) front() FunctionCleanupPlan {
	if queue.count == 0 {
		return FunctionCleanupPlan{}
	}
	return queue.plans[queue.head]
}

func (queue *functionCleanupQueue) pop() {
	if queue.count == 0 {
		return
	}
	queue.plans[queue.head] = FunctionCleanupPlan{}
	queue.head = (queue.head + 1) % len(queue.plans)
	queue.count--
	if queue.count == 0 {
		queue.head = 0
	}
}

type functionMutationSubmission struct {
	mutation FunctionCatalogMutation
	result   chan functionMutationResult
}

type functionMutationResult struct {
	version uint64
	err     error
}

type commandOperation struct {
	*lifecycle.OperationGeneration
	request             Request
	plan                WorkPlan
	claims              []authorityClaim
	authorityClaimEdges []authorityClaimEdge
	claimCursor         int
	claimTicket         uint64
	claimPrepared       bool
	claimRegistered     bool
	claimWaiting        bool
	claimsHeld          bool
	lane                *commandLane
	previous            *commandOperation
	next                *commandOperation
	control             lifecycle.ControlStatus
	controlQueued       bool
	cleanupDone         bool
	uidCompleted        bool
	cancelled           bool
	functionInvocation  FunctionInvocationRef
	resourceGeneration  uint64
	transactionScope    lifecycle.ResourceTransactionScope
	transactionApplied  bool
	transactionRestored bool
	deadline            deadlineEntry
	admission           lifecycle.AdmissionRef
	admissionBase       int64
	admitted            bool
	resultGrowthWaiting bool
	resultExpiry        int64
	taskRequest         lifecycle.TaskRequestRef
	submissionContext   context.Context
	submissionResult    chan error
	terminalResult      chan error
	terminalErr         error
}

type commandLane struct {
	slot               uint16
	generation         uint32
	mapKey             commandLaneKey
	owners             int
	freeNext           uint16
	key                string
	source             lifecycle.Source
	head               *commandOperation
	tail               *commandOperation
	active             *commandOperation
	ready              bool
	readyPrev          *commandLane
	readyNext          *commandLane
	resourceGeneration uint64
	currentIdentity    lifecycle.ResourceIdentity
	current            lifecycle.ReadyResource
	currentStopping    bool
	retiringIdentity   lifecycle.ResourceIdentity
	installPlanned     bool
	stopPlanned        bool
	transactionPlanned int
	shutdownRequest    lifecycle.TaskRequestRef
	shutdownTask       lifecycle.TaskRef
	shutdownAction     lifecycle.TaskActionKind
}

type readyRing struct {
	head *commandLane
	tail *commandLane
	len  int
}

func (ring *readyRing) push(lane *commandLane) {
	if lane.ready {
		return
	}
	lane.ready = true
	lane.readyPrev = ring.tail
	if ring.tail != nil {
		ring.tail.readyNext = lane
	} else {
		ring.head = lane
	}
	ring.tail = lane
	ring.len++
}

func (ring *readyRing) pop() *commandLane {
	lane := ring.head
	if lane == nil {
		return nil
	}
	ring.head = lane.readyNext
	if ring.head != nil {
		ring.head.readyPrev = nil
	} else {
		ring.tail = nil
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	ring.len--
	return lane
}

func (ring *readyRing) remove(lane *commandLane) {
	if !lane.ready {
		return
	}
	if lane.readyPrev != nil {
		lane.readyPrev.readyNext = lane.readyNext
	} else {
		ring.head = lane.readyNext
	}
	if lane.readyNext != nil {
		lane.readyNext.readyPrev = lane.readyPrev
	} else {
		ring.tail = lane.readyPrev
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	ring.len--
}

type deadlineEntry struct {
	when      time.Time
	operation *commandOperation
	index     int
}

type deadlineHeap []*deadlineEntry

func (entries deadlineHeap) Len() int           { return len(entries) }
func (entries deadlineHeap) Less(i, j int) bool { return entries[i].when.Before(entries[j].when) }
func (entries deadlineHeap) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
	entries[i].index = i
	entries[j].index = j
}
func (entries *deadlineHeap) Push(value any) {
	entry := value.(*deadlineEntry)
	entry.index = len(*entries)
	*entries = append(*entries, entry)
}
func (entries *deadlineHeap) Pop() any {
	old := *entries
	last := old[len(old)-1]
	old[len(old)-1] = nil
	last.index = -1
	*entries = old[:len(old)-1]
	return last
}

type CommandKernel struct {
	run                      *lifecycle.RunSupervisor
	admission                *lifecycle.AdmissionLedger
	uids                     *lifecycle.UIDLedger
	tasks                    *lifecycle.TaskSupervisor
	frames                   *lifecycle.FrameOwner
	clock                    lifecycle.Clock
	claims                   *claimAuthority
	submissions              [2]chan submission
	submissionSpace          [2]chan struct{}
	submissionStopped        chan struct{}
	submissionMu             sync.Mutex
	submissionClosed         bool
	blockedSubmissions       [2]submission
	blockedSubmission        [2]bool
	cancel                   chan string
	wake                     chan struct{}
	stop                     chan struct{}
	done                     chan struct{}
	doneErr                  error
	startOnce                sync.Once
	startErr                 error
	stopOnce                 sync.Once
	shutdownStarted          chan struct{}
	shutdownStartOnce        sync.Once
	operations               map[string]*commandOperation
	tasksByRef               map[lifecycle.TaskRef]*commandOperation
	tasksByRequest           map[lifecycle.TaskRequestRef]*commandOperation
	functionCleanupTasks     map[lifecycle.TaskRef]functionCleanupTask
	functionCleanupRequests  map[lifecycle.TaskRequestRef]FunctionCleanupRef
	functionCleanupBacklog   functionCleanupQueue
	functionMutations        chan functionMutationSubmission
	functionMutation         functionMutationSubmission
	functionMutationActive   bool
	functionCatalogClosing   bool
	functionCatalogCloseMore bool
	shutdownRequests         [lifecycle.MaximumAdmissionRecords + 1]*commandLane
	shutdownTasks            [lifecycle.TransientTaskSlots]*commandLane
	shutdownRequestCount     int
	shutdownTaskCount        int
	shutdownBarrier          RunShutdownBarrier
	shutdownBarrierRequest   lifecycle.TaskRequestRef
	shutdownBarrierTask      lifecycle.TaskRef
	shutdownBarrierAction    lifecycle.TaskActionKind
	shutdownBarrierDone      bool
	shutdownBarrierFailed    bool
	finalizer                RunFinalizer
	finalizerRequest         lifecycle.TaskRequestRef
	finalizerTask            lifecycle.TaskRef
	finalizerAction          lifecycle.TaskActionKind
	finalizerDone            bool
	finalizerFailed          bool
	byAdmission              map[lifecycle.AdmissionRef]*commandOperation
	lanes                    map[commandLaneKey]*commandLane
	laneSlots                [lifecycle.MaximumAdmissionRecords + 1]commandLane
	freeLane                 uint16
	ready                    [2]readyRing
	nextID                   lifecycle.OperationID
	nextResourceGeneration   uint64
	nextSource               lifecycle.Source
	nextExternalSource       lifecycle.Source
	nextAsyncEvent           uint8
	deadlines                deadlineHeap
	controls                 []*commandOperation
	jobPlanner               Planner
	functionCatalog          FunctionCatalogPort
	inputBodyGrants          chan<- lifecycle.AdmissionGrant
	admissionServiceGate     <-chan struct{}
}

func NewCommandKernel(run *lifecycle.RunSupervisor, admission *lifecycle.AdmissionLedger, uids *lifecycle.UIDLedger, tasks *lifecycle.TaskSupervisor, frames *lifecycle.FrameOwner, clock lifecycle.Clock, inputBodyGrants chan<- lifecycle.AdmissionGrant, admissionServiceGate <-chan struct{}, shutdownBarrier RunShutdownBarrier, finalizer RunFinalizer, jobPlanner Planner, functionCatalog FunctionCatalogPort) (*CommandKernel, error) {
	if run == nil || admission == nil || uids == nil || tasks == nil || frames == nil || clock == nil || inputBodyGrants == nil || shutdownBarrier == nil || finalizer == nil {
		return nil, errors.New("jobmgr kernel: incomplete lifecycle capabilities")
	}
	if jobPlanner == nil || functionCatalog == nil {
		return nil, errors.New("jobmgr kernel: incomplete command planning ports")
	}
	kernel := &CommandKernel{
		run: run, admission: admission, uids: uids, tasks: tasks, frames: frames, clock: clock, claims: newClaimAuthority(),
		cancel: make(chan string), wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), shutdownStarted: make(chan struct{}),
		submissionStopped: make(chan struct{}),
		operations:        make(map[string]*commandOperation), tasksByRef: make(map[lifecycle.TaskRef]*commandOperation),
		tasksByRequest:          make(map[lifecycle.TaskRequestRef]*commandOperation),
		functionCleanupTasks:    make(map[lifecycle.TaskRef]functionCleanupTask),
		functionCleanupRequests: make(map[lifecycle.TaskRequestRef]FunctionCleanupRef),
		functionMutations:       make(chan functionMutationSubmission),
		byAdmission:             make(map[lifecycle.AdmissionRef]*commandOperation), lanes: make(map[commandLaneKey]*commandLane),
		nextSource: lifecycle.SourceJobManager, nextExternalSource: lifecycle.SourceJobManager,
		inputBodyGrants:      inputBodyGrants,
		admissionServiceGate: admissionServiceGate,
		shutdownBarrier:      shutdownBarrier,
		shutdownBarrierDone:  isNoopRunShutdownBarrier(shutdownBarrier),
		finalizer:            finalizer,
		finalizerDone:        isNoopRunFinalizer(finalizer),
		jobPlanner:           jobPlanner,
		functionCatalog:      functionCatalog,
	}
	for index := range kernel.submissions {
		kernel.submissions[index] = make(chan submission, externalSourceQueueDepth)
		kernel.submissionSpace[index] = make(chan struct{}, 1)
	}
	for index := 1; index <= lifecycle.MaximumAdmissionRecords; index++ {
		kernel.laneSlots[index].freeNext = uint16(index + 1)
	}
	kernel.laneSlots[lifecycle.MaximumAdmissionRecords].freeNext = 0
	kernel.freeLane = 1
	heap.Init(&kernel.deadlines)
	return kernel, nil
}

type KernelLoop struct {
	kernel *CommandKernel
}

func NewKernelLoop(kernel *CommandKernel) (*KernelLoop, error) {
	if kernel == nil {
		return nil, errors.New("jobmgr kernel loop: nil command kernel")
	}
	return &KernelLoop{kernel: kernel}, nil
}

func (loop *KernelLoop) Start(ctx context.Context) error {
	if loop == nil || ctx == nil {
		return errors.New("jobmgr kernel loop: invalid start")
	}
	started := false
	loop.kernel.startOnce.Do(func() {
		started = true
		loop.kernel.startErr = loop.kernel.bindRunNotifications()
		if loop.kernel.startErr != nil {
			return
		}
		go loop.kernel.runLoop(ctx)
	})
	if !started {
		return errors.New("jobmgr kernel loop: already started")
	}
	return loop.kernel.startErr
}

func (kernel *CommandKernel) bindRunNotifications() error {
	if err := kernel.frames.BindRunNotifications(
		kernel.run.Generation(),
		kernel.NotifyControlReady,
		func(err error) {
			_ = kernel.run.Dirty(err)
			kernel.NotifyControlReady()
		},
	); err != nil {
		return err
	}
	if err := kernel.tasks.BindAdmissionReady(kernel.NotifyControlReady); err != nil {
		return errors.Join(
			err,
			kernel.frames.ReleaseRunNotifications(kernel.run.Generation()),
		)
	}
	return nil
}

func (kernel *CommandKernel) Submit(ctx context.Context, request Request) error {
	return kernel.submit(ctx, request, nil)
}

func (kernel *CommandKernel) SubmitAndWait(ctx context.Context, request Request) error {
	terminal := make(chan error, 1)
	if err := kernel.submit(ctx, request, terminal); err != nil {
		return err
	}
	select {
	case err := <-terminal:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-kernel.done:
		return kernel.Wait(context.Background())
	}
}

func (kernel *CommandKernel) MutateFunctions(ctx context.Context, mutation FunctionCatalogMutation) (uint64, error) {
	if ctx == nil || mutation == nil {
		return 0, errors.New("jobmgr kernel: invalid Function mutation")
	}
	result := make(chan functionMutationResult, 1)
	submission := functionMutationSubmission{mutation: mutation, result: result}
	select {
	case kernel.functionMutations <- submission:
		kernel.NotifyControlReady()
	case <-kernel.submissionStopped:
		return 0, ErrStopped
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-kernel.done:
		return 0, ErrStopped
	}
	select {
	case completed := <-result:
		return completed.version, completed.err
	case <-kernel.done:
		select {
		case completed := <-result:
			return completed.version, completed.err
		default:
			return 0, errors.Join(ErrStopped, kernel.doneErr)
		}
	}
}

func (kernel *CommandKernel) SubmitPrepared(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	return kernel.submitPrepared(ctx, request, plan, nil)
}

func (kernel *CommandKernel) SubmitPreparedAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	terminal := make(chan error, 1)
	if err := kernel.submitPrepared(ctx, request, plan, terminal); err != nil {
		return err
	}
	select {
	case err := <-terminal:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-kernel.done:
		return kernel.Wait(context.Background())
	}
}

func (kernel *CommandKernel) submit(
	ctx context.Context,
	request Request,
	terminal chan error,
) error {
	return kernel.submitWithPlan(ctx, request, WorkPlan{}, false, terminal)
}

func (kernel *CommandKernel) submitPrepared(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	terminal chan error,
) error {
	return kernel.submitWithPlan(ctx, request, plan, true, terminal)
}

func (kernel *CommandKernel) submitWithPlan(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	prepared bool,
	terminal chan error,
) error {
	if ctx == nil {
		return errors.Join(errors.New("jobmgr kernel: nil submission context"), kernel.abortRequestInputBody(request))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, kernel.abortRequestInputBody(request))
	}
	if err := request.Validate(); err != nil {
		return errors.Join(err, kernel.abortRequestInputBody(request))
	}
	if prepared && request.Source != lifecycle.SourceJobManager {
		return errors.Join(
			errors.New("jobmgr kernel: only Job Manager commands accept prepared plans"),
			kernel.abortRequestInputBody(request),
		)
	}
	request.Args = append([]string(nil), request.Args...)
	if prepared {
		var err error
		plan, err = prepareOwnedJobPlan(request, plan)
		if err != nil {
			return errors.Join(err, kernel.abortRequestInputBody(request))
		}
	} else if request.Source == lifecycle.SourceJobManager {
		var err error
		plan, err = kernel.prepareJobPlan(request)
		if err != nil {
			return errors.Join(err, kernel.abortRequestInputBody(request))
		}
	}
	result := make(chan error, 1)
	if err := kernel.enqueueSubmission(ctx, request.Source, submission{
		request:  request,
		plan:     plan,
		context:  ctx,
		result:   result,
		terminal: terminal,
	}); err != nil {
		return errors.Join(err, kernel.abortRequestInputBody(request))
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		select {
		case kernel.cancel <- request.UID:
		case err := <-result:
			return err
		case <-kernel.done:
			return ErrStopped
		}
		select {
		case err := <-result:
			return err
		case <-kernel.done:
			return ErrStopped
		}
	case <-kernel.done:
		return ErrStopped
	}
}

func (kernel *CommandKernel) enqueueSubmission(ctx context.Context, source lifecycle.Source, submitted submission) error {
	index := sourceIndex(source)
	for {
		kernel.submissionMu.Lock()
		if kernel.submissionClosed {
			kernel.submissionMu.Unlock()
			return ErrStopped
		}
		select {
		case kernel.submissions[index] <- submitted:
			kernel.submissionMu.Unlock()
			kernel.NotifyControlReady()
			return nil
		default:
			kernel.submissionMu.Unlock()
		}
		select {
		case <-kernel.submissionSpace[index]:
		case <-kernel.submissionStopped:
			return ErrStopped
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (kernel *CommandKernel) closeSubmissionIngress() {
	kernel.submissionMu.Lock()
	if !kernel.submissionClosed {
		kernel.submissionClosed = true
		close(kernel.submissionStopped)
	}
	kernel.submissionMu.Unlock()
}

func (kernel *CommandKernel) notifySubmissionSpace(source int) {
	select {
	case kernel.submissionSpace[source] <- struct{}{}:
	default:
	}
}

func (kernel *CommandKernel) Reject(ctx context.Context, uid string, status lifecycle.ControlStatus) error {
	if ctx == nil {
		return errors.New("jobmgr kernel: nil submission context")
	}
	if err := (lifecycle.ControlFramePlan{UID: uid, Status: status, Expiry: 1}).Validate(); err != nil {
		return err
	}
	if status != lifecycle.ControlBadRequest && status != lifecycle.ControlPayloadTooLarge && status != lifecycle.ControlCancelled {
		return errors.New("jobmgr kernel: invalid pre-admission control status")
	}
	result := make(chan error, 1)
	if err := kernel.enqueueSubmission(ctx, lifecycle.SourceFunction, submission{
		controlStatus: status,
		request:       Request{UID: uid, Source: lifecycle.SourceFunction},
		result:        result,
	}); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-kernel.done:
		return ErrStopped
	}
}

func (kernel *CommandKernel) Cancel(ctx context.Context, uid string) error {
	select {
	case kernel.cancel <- uid:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-kernel.done:
		return ErrStopped
	}
}

func (kernel *CommandKernel) NotifyControlReady() {
	select {
	case kernel.wake <- struct{}{}:
	default:
	}
}

func (kernel *CommandKernel) Stop() {
	kernel.stopOnce.Do(func() {
		kernel.closeSubmissionIngress()
		close(kernel.stop)
	})
}

func (kernel *CommandKernel) Done() <-chan struct{} {
	return kernel.done
}

func (kernel *CommandKernel) Wait(ctx context.Context) error {
	select {
	case <-kernel.done:
		return kernel.doneErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (kernel *CommandKernel) WaitShutdownStarted(ctx context.Context) error {
	select {
	case <-kernel.shutdownStarted:
		return nil
	default:
	}
	select {
	case <-kernel.shutdownStarted:
		return nil
	case <-kernel.done:
		select {
		case <-kernel.shutdownStarted:
			return nil
		default:
			return ErrStopped
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (kernel *CommandKernel) runLoop(ctx context.Context) {
	var terminal error
	var shutdownBudget *lifecycle.ShutdownBudget
	var shutdownC <-chan struct{}
	var deadlineTimer lifecycle.ReusableTimer
	if clock, ok := kernel.clock.(lifecycle.ReusableTimerClock); ok {
		deadlineTimer = clock.NewTimer(lifecycle.TimerKindDeadline)
	}
	var deadlineC <-chan time.Time
	var cancelDeadline func()
	var armedDeadline time.Time
	stopDeadline := func() {
		if cancelDeadline != nil {
			cancelDeadline()
		}
		deadlineC = nil
		cancelDeadline = nil
		armedDeadline = time.Time{}
	}
	armDeadline := func() {
		deadline := kernel.nextDeadline()
		if deadline.IsZero() {
			stopDeadline()
			return
		}
		if deadlineC != nil && deadline.Equal(armedDeadline) {
			return
		}
		stopDeadline()
		delay := deadline.Sub(kernel.clock.Now())
		if delay < 0 {
			delay = 0
		}
		if deadlineTimer != nil {
			deadlineC = deadlineTimer.Arm(delay)
			cancelDeadline = deadlineTimer.Stop
		} else {
			deadlineC, cancelDeadline = kernel.clock.Arm(lifecycle.TimerKindDeadline, delay)
		}
		armedDeadline = deadline
	}
	stopC := (<-chan struct{})(kernel.stop)
	contextC := ctx.Done()
	shuttingDown := false
	beginShutdown := func(cause error) {
		if shuttingDown {
			terminal = errors.Join(terminal, cause)
			return
		}
		kernel.closeSubmissionIngress()
		shuttingDown = true
		stopDeadline()
		terminal = errors.Join(terminal, cause)
		budget, err := kernel.run.BeginShutdown()
		if err != nil {
			kernel.run.Dirty(err)
			terminal = errors.Join(terminal, err)
			return
		}
		shutdownBudget = budget
		if err := kernel.beginShutdown(budget.Deadline()); err != nil {
			kernel.run.Dirty(err)
		}
		kernel.shutdownStartOnce.Do(func() { close(kernel.shutdownStarted) })
		shutdownC = budget.Context().Done()
		stopC = nil
		contextC = nil
	}
	defer func() {
		stopDeadline()
		if err := kernel.frames.ReleaseRunNotifications(
			kernel.run.Generation(),
		); err != nil {
			terminal = errors.Join(terminal, err)
		}
		kernel.doneErr = terminal
		close(kernel.done)
	}()
	for {
		if !shuttingDown {
			if cause := kernel.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), kernel.run.Terminal(kernel.runCensus()))
				return
			}
		}
		moreDeadlines := false
		if !shuttingDown {
			if deadline := kernel.nextDeadline(); !deadline.IsZero() && !deadline.After(kernel.clock.Now()) {
				moreDeadlines = kernel.serviceDeadlines(kernel.clock.Now(), 4)
			}
		}
		moreControls := kernel.serviceControls(4)
		moreSubmissions := kernel.serviceSubmissions(4)
		moreFunctionCleanups := kernel.serviceFunctionCleanupBacklog(4)
		moreFunctionMutation := false
		moreFunctionClose := false
		if !shuttingDown {
			moreFunctionMutation = kernel.serviceFunctionMutation(16)
			if cause := kernel.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		} else if kernel.shutdownBarrierDone {
			moreFunctionClose = kernel.serviceFunctionCatalogClose(MaximumFunctionCloseQuantum)
		}
		moreFunctionCleanups = moreFunctionCleanups || kernel.functionCleanupBacklog.count != 0
		moreAdmissions := false
		moreTasks := false
		if !shuttingDown {
			moreAdmissions = kernel.serviceAdmissions(4)
			moreTasks = kernel.scheduleTasks(4)
			kernel.serviceTaskStarts(4)
			if cause := kernel.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		} else {
			if err := kernel.advanceShutdownAdmission(); err != nil {
				kernel.run.Dirty(err)
			}
			if err := kernel.advanceShutdownBarrier(); err != nil {
				kernel.run.Dirty(err)
			}
			if kernel.shutdownBarrierDone {
				if err := kernel.enqueueShutdownStops(); err != nil {
					kernel.run.Dirty(err)
				}
			}
			if err := kernel.advanceRunFinalizer(); err != nil {
				kernel.run.Dirty(err)
			}
			kernel.serviceTaskStarts(4)
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), kernel.run.Terminal(kernel.runCensus()))
				return
			}
			if kernel.shutdownQuiescent() ||
				kernel.runShutdownBarrierFailedTerminal() ||
				kernel.runFinalizerFailedTerminal() {
				terminal = errors.Join(terminal, kernel.run.Terminal(kernel.runCensus()))
				return
			}
		}
		if moreDeadlines || moreControls || moreSubmissions || moreFunctionCleanups ||
			moreFunctionMutation || moreFunctionClose || moreAdmissions || moreTasks ||
			kernel.hasRunnableSubmissions() {
			if !shuttingDown {
				select {
				case <-stopC:
					beginShutdown(nil)
					continue
				default:
				}
				select {
				case <-contextC:
					beginShutdown(ctx.Err())
					continue
				default:
				}
			} else {
				if shutdownBudget.ExpireIfDue() {
					terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), kernel.run.Terminal(kernel.runCensus()))
					return
				}
			}
			kernel.serviceOneAsyncEvent()
			continue
		}
		if !shuttingDown {
			armDeadline()
		}
		functionMutationC := (<-chan functionMutationSubmission)(kernel.functionMutations)
		if shuttingDown || kernel.functionMutationActive {
			functionMutationC = nil
		}
		select {
		case uid := <-kernel.cancel:
			kernel.cancelOperation(uid)
		case submitted := <-functionMutationC:
			kernel.beginFunctionMutation(submitted)
		case completion := <-kernel.tasks.CompletionCh():
			kernel.completeTask(completion)
		case acknowledgement := <-kernel.tasks.AcknowledgementCh():
			kernel.acknowledgeTask(acknowledgement)
		case <-deadlineC:
			deadlineC = nil
			cancelDeadline = nil
			armedDeadline = time.Time{}
			kernel.serviceDeadlines(kernel.clock.Now(), 4)
		case <-kernel.wake:
		case <-stopC:
			beginShutdown(nil)
		case <-contextC:
			beginShutdown(ctx.Err())
		case <-shutdownC:
			terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), kernel.run.Terminal(kernel.runCensus()))
			return
		}
	}
}

func (kernel *CommandKernel) serviceOneAsyncEvent() bool {
	const sources = 4
	for offset := 0; offset < sources; offset++ {
		source := (int(kernel.nextAsyncEvent) + offset) % sources
		switch source {
		case 0:
			select {
			case uid := <-kernel.cancel:
				kernel.cancelOperation(uid)
				kernel.nextAsyncEvent = 1
				return true
			default:
			}
		case 1:
			select {
			case completion := <-kernel.tasks.CompletionCh():
				kernel.completeTask(completion)
				kernel.nextAsyncEvent = 2
				return true
			default:
			}
		case 2:
			select {
			case acknowledgement := <-kernel.tasks.AcknowledgementCh():
				kernel.acknowledgeTask(acknowledgement)
				kernel.nextAsyncEvent = 3
				return true
			default:
			}
		case 3:
			if kernel.functionMutationActive || kernel.functionCatalogClosing {
				continue
			}
			select {
			case submitted := <-kernel.functionMutations:
				kernel.beginFunctionMutation(submitted)
				kernel.nextAsyncEvent = 0
				return true
			default:
			}
		}
	}
	return false
}

func (kernel *CommandKernel) serviceSubmissions(quantum int) bool {
	for quantum > 0 {
		first := sourceIndex(kernel.nextExternalSource)
		second := 1 - first
		var submitted submission
		selected := -1
		wasBlocked := false
		dequeued := false
		if kernel.blockedSubmission[first] {
			submitted = kernel.blockedSubmissions[first]
			selected = first
			wasBlocked = true
		} else {
			select {
			case submitted = <-kernel.submissions[first]:
				selected = first
				dequeued = true
			default:
			}
		}
		if selected < 0 {
			if kernel.blockedSubmission[second] {
				submitted = kernel.blockedSubmissions[second]
				selected = second
				wasBlocked = true
			} else {
				select {
				case submitted = <-kernel.submissions[second]:
					selected = second
					dequeued = true
				default:
					return kernel.hasRunnableSubmissions()
				}
			}
		}
		if dequeued {
			kernel.notifySubmissionSpace(selected)
		}
		var err error
		if submitted.controlStatus != 0 {
			err = kernel.frames.TryCommitControl(lifecycle.ControlFramePlan{
				UID: submitted.request.UID, Status: submitted.controlStatus, Expiry: lifecycle.ExpiryAt(kernel.clock.Now()),
			})
			if err != nil && !errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
				kernel.run.Dirty(err)
			}
		} else {
			if submitted.context != nil && submitted.context.Err() != nil {
				err = errors.Join(context.Cause(submitted.context), kernel.abortRequestInputBody(submitted.request))
			} else {
				err = kernel.admit(submitted.request, submitted.plan, submitted.context, submitted.result, submitted.terminal)
			}
		}
		if errors.Is(err, lifecycle.ErrAdmissionRecordCapacity) || errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			if !wasBlocked {
				kernel.blockedSubmissions[selected] = submitted
				kernel.blockedSubmission[selected] = true
			}
		} else {
			if wasBlocked {
				kernel.blockedSubmissions[selected] = submission{}
				kernel.blockedSubmission[selected] = false
			}
			if submitted.controlStatus != 0 || err != nil {
				submitted.result <- err
			}
		}
		kernel.nextExternalSource = otherSource(sourceForIndex(selected))
		quantum--
	}
	return kernel.hasRunnableSubmissions()
}

func (kernel *CommandKernel) hasRunnableSubmissions() bool {
	for source := range kernel.submissions {
		if !kernel.blockedSubmission[source] && len(kernel.submissions[source]) != 0 {
			return true
		}
	}
	if kernel.run.Admitting() {
		return false
	}
	for source := range kernel.blockedSubmission {
		if kernel.blockedSubmission[source] && kernel.blockedSubmissions[source].controlStatus == 0 {
			return true
		}
	}
	return false
}

func (kernel *CommandKernel) prepareJobPlan(request Request) (WorkPlan, error) {
	plan, err := kernel.jobPlanner.Plan(request)
	if err != nil {
		return WorkPlan{}, err
	}
	return prepareOwnedJobPlan(request, plan)
}

func prepareOwnedJobPlan(request Request, plan WorkPlan) (WorkPlan, error) {
	plan.Claims = append([]string(nil), plan.Claims...)
	plan.ReadClaims = append([]string(nil), plan.ReadClaims...)
	if plan.Resource != nil {
		resource := *plan.Resource
		plan.Resource = &resource
	}
	if plan.Transaction != nil {
		transaction := *plan.Transaction
		plan.Transaction = &transaction
	}
	if plan.Capability != nil {
		capability := *plan.Capability
		plan.Capability = &capability
	}
	if err := plan.validate(); err != nil {
		return WorkPlan{}, err
	}
	if plan.Resource != nil && plan.Resource.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: resource identity differs from lane")
	}
	if plan.Transaction != nil && plan.Transaction.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: transaction identity differs from lane")
	}
	if plan.Capability != nil && plan.Capability.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: capability identity differs from lane")
	}
	return plan, nil
}

func (kernel *CommandKernel) admit(request Request, plan WorkPlan, submissionContext context.Context, submissionResult, terminalResult chan error) error {
	if !kernel.run.Admitting() {
		return kernel.rejectClosedAdmission(request)
	}
	now := kernel.clock.Now()
	if err := kernel.uids.Admit(request.UID, now); err != nil {
		return errors.Join(err, kernel.abortRequestInputBody(request))
	}
	var functionInvocation FunctionInvocationRef
	var laneID commandLaneKey
	if request.Source == lifecycle.SourceFunction {
		decision, err := kernel.functionCatalog.ResolveAndAcquire(FunctionLookup{
			UID: request.UID, Route: request.Route, Args: request.Args,
			Payload: request.Payload, ContentType: request.ContentType,
			Permissions: request.Permissions, CallerSource: request.CallerSource,
			Timeout: request.Timeout, HasPayload: request.HasPayload,
		})
		if err != nil {
			_ = kernel.uids.Complete(request.UID, false, now)
			_ = kernel.abortRequestInputBody(request)
			return err
		}
		if err := decision.validate(); err != nil {
			if decision.Lease.Valid() {
				err = errors.Join(err, kernel.releaseFunctionInvocation(decision.Lease))
			}
			_ = kernel.uids.Complete(request.UID, false, now)
			_ = kernel.abortRequestInputBody(request)
			return err
		}
		request.LaneKey = decision.Lane.Scope
		if request.LaneKey == "" {
			request.LaneKey = request.Route
		}
		laneID = commandLaneKey{
			source: request.Source, functionRoute: decision.Lane.Route, key: decision.Lane.Scope,
		}
		if decision.Rejected != 0 {
			plan = RejectionPlan(decision.Rejected, "function:"+request.Route)
		} else {
			plan = decision.Plan
			plan.Claims = append([]string(nil), plan.Claims...)
			plan.ReadClaims = append([]string(nil), plan.ReadClaims...)
			functionInvocation = decision.Lease
		}
	}
	releaseFunctionInvocation := functionInvocation.Valid()
	defer func() {
		if releaseFunctionInvocation {
			if err := kernel.releaseFunctionInvocation(functionInvocation); err != nil {
				kernel.run.Dirty(err)
			}
		}
	}()
	claims, err := normalizeAuthorityClaimModes(plan.Claims, plan.ReadClaims)
	if err != nil {
		_ = kernel.uids.Complete(request.UID, false, now)
		_ = kernel.abortRequestInputBody(request)
		return err
	}
	if request.Source == lifecycle.SourceJobManager {
		laneID = commandLaneKey{source: request.Source, key: request.LaneKey}
	}
	lane := kernel.lanes[laneID]
	if resource := plan.Resource; resource != nil {
		if lane != nil && lane.transactionPlanned != 0 {
			_ = kernel.uids.Complete(request.UID, false, now)
			_ = kernel.abortRequestInputBody(request)
			return errors.New(
				"jobmgr kernel: internal resource plan overlaps a public resource transaction",
			)
		}
		switch resource.Action {
		case ResourceInstall:
			if lane != nil && (lane.installPlanned || ((lane.current != nil || lane.currentIdentity.Valid()) && !lane.stopPlanned && !lane.currentStopping && !lane.retiringIdentity.Valid())) {
				_ = kernel.uids.Complete(request.UID, false, now)
				_ = kernel.abortRequestInputBody(request)
				return errors.New("jobmgr kernel: install is not sequenced after an exact stop")
			}
		case ResourceStop:
			if lane == nil || lane.stopPlanned || lane.currentStopping || lane.retiringIdentity.Valid() ||
				(lane.current == nil && !lane.currentIdentity.Valid() && !lane.installPlanned) {
				err := errors.Join(kernel.uids.Complete(request.UID, false, now), kernel.abortRequestInputBody(request))
				if err == nil {
					if submissionResult != nil {
						submissionResult <- nil
					}
					if terminalResult != nil {
						terminalResult <- nil
					}
				}
				return err
			}
		}
	}
	if plan.Transaction != nil && lane != nil &&
		(lane.installPlanned ||
			lane.stopPlanned ||
			lane.currentStopping ||
			lane.retiringIdentity.Valid()) {
		_ = kernel.uids.Complete(request.UID, false, now)
		_ = kernel.abortRequestInputBody(request)
		return errors.New(
			"jobmgr kernel: resource transaction overlaps internal resource authority",
		)
	}
	if lane == nil {
		lane, err = kernel.allocateLane(laneID, request)
		if err != nil {
			_ = kernel.uids.Complete(request.UID, false, now)
			_ = kernel.abortRequestInputBody(request)
			return err
		}
	}
	if lane.owners >= lifecycle.MaximumLaneDepth {
		_ = kernel.uids.Complete(request.UID, false, now)
		_ = kernel.abortRequestInputBody(request)
		return errors.New("jobmgr kernel: lane depth exhausted")
	}
	kernel.nextID++
	operationGeneration, err := lifecycle.NewOperation(kernel.nextID, request.UID, request.Source, request.LaneKey, !plan.NoResponse)
	if err != nil {
		_ = kernel.uids.Complete(request.UID, false, now)
		kernel.releaseUnusedLane(lane)
		_ = kernel.abortRequestInputBody(request)
		return err
	}
	charge, err := operationAdmissionBytes(request, plan)
	if err != nil {
		_ = kernel.uids.Complete(request.UID, false, now)
		kernel.releaseUnusedLane(lane)
		_ = kernel.abortRequestInputBody(request)
		return err
	}
	admissionLane := lifecycle.AdmissionLaneRef{Slot: lane.slot, Generation: lane.generation}
	requested := lifecycle.AdmissionRequestResult{}
	if request.InputBodyToken != 0 {
		requested = kernel.admission.TransferInputBody(kernel.run.Generation(), request.InputBodyToken, admissionLane, charge, request.PayloadCapacity)
	} else {
		requested = kernel.admission.RequestOrdinary(kernel.run.Generation(), admissionLane, charge)
	}
	if requested.Rejected != nil {
		_ = kernel.uids.Complete(request.UID, false, now)
		kernel.releaseUnusedLane(lane)
		if !errors.Is(requested.Rejected, lifecycle.ErrAdmissionRecordCapacity) {
			_ = kernel.abortRequestInputBody(request)
		}
		return requested.Rejected
	}
	request.InputBodyToken = 0
	operation := &commandOperation{
		OperationGeneration: operationGeneration, request: request, plan: plan, claims: claims,
		functionInvocation: functionInvocation,
		admission:          requested.Ref, admissionBase: charge, deadline: deadlineEntry{index: -1},
		submissionContext: submissionContext, submissionResult: submissionResult, terminalResult: terminalResult,
	}
	prepareClaimEdges(operation, claims)
	if err := kernel.claims.Register(operation); err != nil {
		_ = kernel.admission.CancelWaiting(requested.Ref)
		_ = kernel.uids.Complete(request.UID, false, now)
		kernel.releaseUnusedLane(lane)
		return err
	}
	operation.lane = lane
	lane.owners++
	if lane.tail != nil {
		lane.tail.next = operation
		operation.previous = lane.tail
	} else {
		lane.head = operation
	}
	lane.tail = operation
	_ = operation.Advance(lifecycle.OperationQueued)
	kernel.operations[request.UID] = operation
	kernel.byAdmission[requested.Ref] = operation
	releaseFunctionInvocation = false
	if resource := plan.Resource; resource != nil {
		switch resource.Action {
		case ResourceInstall:
			lane.installPlanned = true
		case ResourceStop:
			lane.stopPlanned = true
		}
	}
	if plan.Transaction != nil {
		lane.transactionPlanned++
	}
	if !request.Deadline.IsZero() {
		operation.deadline = deadlineEntry{when: request.Deadline, operation: operation, index: -1}
		heap.Push(&kernel.deadlines, &operation.deadline)
	}
	if operation.admitted && lane.active == nil && lane.head == operation {
		kernel.markReady(lane)
	}
	return nil
}

func (kernel *CommandKernel) rejectClosedAdmission(request Request) error {
	closedErr := errors.New("jobmgr kernel: admission closed")
	if err := kernel.abortRequestInputBody(request); err != nil {
		kernel.run.Dirty(err)
		return errors.Join(closedErr, err)
	}
	select {
	case <-kernel.shutdownStarted:
		return errors.Join(ErrStopped, closedErr)
	default:
		return closedErr
	}
}

func (kernel *CommandKernel) serviceAdmissions(quantum int) bool {
	if kernel.admissionServiceGate != nil {
		select {
		case <-kernel.admissionServiceGate:
		default:
			return false
		}
	}
	var grants [4]lifecycle.AdmissionGrant
	count, more, err := kernel.admission.TakeGrants(quantum, &grants)
	if err != nil {
		kernel.run.Dirty(err)
		return false
	}
	for _, grant := range grants[:count] {
		if grant.Kind == lifecycle.ReservationInputBodyGrowth {
			select {
			case kernel.inputBodyGrants <- grant:
			default:
				kernel.run.Dirty(errors.New("jobmgr kernel: input body grant gate is full"))
				return false
			}
			continue
		}
		operation := kernel.byAdmission[grant.Ref]
		if operation == nil || operation.admission != grant.Ref {
			kernel.run.Dirty(errors.New("jobmgr kernel: invalid admission grant"))
			return false
		}
		switch grant.Kind {
		case lifecycle.ReservationOrdinary:
			if operation.admitted {
				kernel.run.Dirty(errors.New("jobmgr kernel: duplicate initial admission grant"))
				return false
			}
			operation.admitted = true
			kernel.settleSubmission(operation, nil)
			if operation.lane.active == nil && operation.lane.head == operation {
				kernel.markReady(operation.lane)
			}
		case lifecycle.ReservationOrdinaryGrowth:
			if !operation.admitted || !operation.resultGrowthWaiting || operation.Child != lifecycle.ChildResultReady {
				kernel.run.Dirty(errors.New("jobmgr kernel: invalid result growth grant"))
				return false
			}
			operation.resultGrowthWaiting = false
			if err := kernel.sendEncodeAction(operation); err != nil {
				kernel.run.Dirty(err)
				return false
			}
		default:
			kernel.run.Dirty(errors.New("jobmgr kernel: unexpected admission grant kind"))
			return false
		}
	}
	return more
}

func (kernel *CommandKernel) settleSubmission(operation *commandOperation, err error) {
	if operation == nil || operation.submissionResult == nil {
		return
	}
	operation.submissionResult <- err
	operation.submissionResult = nil
	operation.submissionContext = nil
}

func (kernel *CommandKernel) scheduleTasks(quantum int) bool {
	for quantum > 0 {
		lane := kernel.nextReadyLane()
		if lane == nil {
			return false
		}
		quantum--
		operation := lane.head
		if operation == nil || !operation.admitted || lane.active != nil {
			kernel.run.Dirty(errors.New("jobmgr kernel: invalid ready lane"))
			return false
		}
		if !operation.claimsHeld {
			if operation.State < lifecycle.OperationAcquiringClaims {
				_ = operation.Advance(lifecycle.OperationAcquiringClaims)
			}
			granted, err := kernel.claims.Acquire(operation)
			if err != nil {
				kernel.run.Dirty(err)
				return false
			}
			if !granted {
				continue
			}
		}
		if operation.State < lifecycle.OperationReady {
			_ = operation.Advance(lifecycle.OperationReady)
		}
		phaseLimit := uint8(lifecycle.TransactionTaskPhases)
		if operation.Source == lifecycle.SourceFunction {
			phaseLimit = lifecycle.FunctionTaskPhases
		}
		taskPlan := lifecycle.TaskPlan{
			Source: operation.Source, Deadline: operation.request.Deadline,
			MaxPhaseTransitions: phaseLimit, Work: operation.plan.Work, Cleanup: operation.plan.Cleanup,
			Runner: operation.plan.Runner,
		}
		if operation.Child == lifecycle.ChildDeadlineStartPending {
			taskPlan.InitialCancellation = context.DeadlineExceeded
		}
		if resource := operation.plan.Resource; resource != nil {
			switch resource.Action {
			case ResourceInstall:
				if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() {
					kernel.run.Dirty(errors.New("jobmgr kernel: install encountered a live or retiring resource"))
					return false
				}
				kernel.nextResourceGeneration++
				generation := kernel.nextResourceGeneration
				if generation == 0 {
					kernel.run.Dirty(errors.New("jobmgr kernel: resource generation wrapped"))
					return false
				}
				operation.resourceGeneration = generation
				prepare := resource.Prepare
				identity := lifecycle.ResourceIdentity{ID: resource.ID, Generation: generation}
				permitTaskPlan, err := lifecycle.NewPreparedResourcePermitTaskPlan(
					operation.Source, operation.request.Deadline, phaseLimit,
					kernel.admission, operation.admission, identity, resource.Permit,
					func(ctx context.Context, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResource, error) {
						return prepare(ctx, generation, permit)
					},
				)
				if err != nil {
					kernel.run.Dirty(err)
					return false
				}
				taskPlan = permitTaskPlan
			case ResourceStop:
				if lane.current == nil || !lane.currentIdentity.Valid() || lane.currentIdentity.ID != resource.ID || lane.currentStopping || lane.retiringIdentity.Valid() {
					kernel.run.Dirty(errors.New("jobmgr kernel: stop encountered no exact current resource"))
					return false
				}
				operation.resourceGeneration = lane.currentIdentity.Generation
				readyPlan, err := lifecycle.NewReadyResourceTaskPlan(
					operation.Source, operation.request.Deadline, phaseLimit, lane.current, lane.currentIdentity,
				)
				if err != nil {
					kernel.run.Dirty(err)
					return false
				}
				taskPlan = readyPlan
			default:
				kernel.run.Dirty(errors.New("jobmgr kernel: unknown resource action at dispatch"))
				return false
			}
		}
		if transaction := operation.plan.Transaction; transaction != nil {
			if lane.currentStopping ||
				lane.retiringIdentity.Valid() ||
				(lane.current == nil) != !lane.currentIdentity.Valid() {
				kernel.run.Dirty(
					errors.New(
						"jobmgr kernel: transaction encountered an invalid current resource slot",
					),
				)
				return false
			}
			scope := lifecycle.ResourceTransactionScope{
				ID:      transaction.ID,
				Current: lane.currentIdentity,
			}
			if transaction.AllocateSuccessor {
				kernel.nextResourceGeneration++
				generation := kernel.nextResourceGeneration
				if generation == 0 {
					kernel.run.Dirty(
						errors.New(
							"jobmgr kernel: transaction successor generation wrapped",
						),
					)
					return false
				}
				scope.Successor = lifecycle.ResourceIdentity{
					ID:         transaction.ID,
					Generation: generation,
				}
				operation.resourceGeneration = generation
			}
			operation.transactionScope = scope
			var err error
			if scope.Successor.Valid() {
				taskPlan, err = lifecycle.NewResourceTransactionPermitTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					kernel.admission,
					operation.admission,
					lane.current,
					scope,
					transaction.Permit,
					transaction.Prepare,
				)
			} else {
				taskPlan, err = lifecycle.NewResourceTransactionTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					lane.current,
					scope,
					transaction.Prepare,
				)
			}
			if err != nil {
				kernel.run.Dirty(err)
				return false
			}
		}
		if capability := operation.plan.Capability; capability != nil {
			kernel.nextResourceGeneration++
			generation := kernel.nextResourceGeneration
			if generation == 0 {
				kernel.run.Dirty(errors.New("jobmgr kernel: capability generation wrapped"))
				return false
			}
			operation.resourceGeneration = generation
			identity := lifecycle.ResourceIdentity{ID: capability.ID, Generation: generation}
			prepare := capability.Prepare
			capabilityPlan, err := lifecycle.NewPreparedCapabilityPermitTaskPlan(
				operation.Source, operation.request.Deadline, phaseLimit,
				kernel.admission, operation.admission, identity, capability.Permit,
				func(ctx context.Context, permit lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					return prepare(ctx, generation, permit)
				},
			)
			if err != nil {
				kernel.run.Dirty(err)
				return false
			}
			taskPlan = capabilityPlan
		}
		requestRef, err := kernel.tasks.Enqueue(taskPlan)
		if err != nil {
			for _, grantedOperation := range kernel.releaseClaims(operation) {
				kernel.markReady(grantedOperation.lane)
			}
			kernel.markReady(lane)
			return false
		}
		if operation.plan.Resource != nil && operation.plan.Resource.Action == ResourceStop {
			lane.current = nil
			lane.currentStopping = true
		}
		if operation.plan.Transaction != nil && operation.transactionScope.Current.Valid() {
			lane.current = nil
			lane.currentStopping = true
		}
		operation.taskRequest = requestRef
		lane.active = operation
		kernel.tasksByRequest[requestRef] = operation
	}
	return kernel.ready[0].len != 0 || kernel.ready[1].len != 0
}

func (kernel *CommandKernel) serviceTaskStarts(quantum int) {
	var started [lifecycle.TransientTaskSlots]lifecycle.TaskStart
	count, _, dispatchErr := kernel.tasks.Dispatch(context.Background(), quantum, &started)
	for _, start := range started[:count] {
		if start.Err != nil {
			kernel.rejectTaskStart(start)
			continue
		}
		if cleanupRef, ok := kernel.functionCleanupRequests[start.Request]; ok {
			if _, exists := kernel.functionCleanupTasks[start.Task]; exists {
				kernel.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup task slot"))
				return
			}
			delete(kernel.functionCleanupRequests, start.Request)
			kernel.functionCleanupTasks[start.Task] = functionCleanupTask{ref: cleanupRef}
			continue
		}
		if kernel.shutdownBarrierRequest.Valid() &&
			start.Request == kernel.shutdownBarrierRequest {
			if kernel.shutdownBarrierTask.Valid() ||
				kernel.shutdownBarrierDone ||
				kernel.shutdownBarrierFailed {
				kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier start acknowledgement"))
				return
			}
			kernel.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
			kernel.shutdownBarrierTask = start.Task
			continue
		}
		if kernel.finalizerRequest.Valid() && start.Request == kernel.finalizerRequest {
			if kernel.finalizerTask.Valid() || kernel.finalizerDone || kernel.finalizerFailed {
				kernel.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer start acknowledgement"))
				return
			}
			kernel.finalizerRequest = lifecycle.TaskRequestRef{}
			kernel.finalizerTask = start.Task
			continue
		}
		operation := kernel.tasksByRequest[start.Request]
		if operation == nil {
			lane := kernel.shutdownRequests[start.Request.Slot]
			if lane == nil || lane.shutdownRequest != start.Request || lane.shutdownTask.Valid() || kernel.shutdownTasks[start.Task.Slot] != nil {
				kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task start acknowledgement"))
				return
			}
			kernel.shutdownRequests[start.Request.Slot] = nil
			kernel.shutdownRequestCount--
			lane.shutdownRequest = lifecycle.TaskRequestRef{}
			lane.shutdownTask = start.Task
			kernel.shutdownTasks[start.Task.Slot] = lane
			kernel.shutdownTaskCount++
			continue
		}
		if operation == nil || operation.taskRequest != start.Request ||
			(operation.Child != lifecycle.ChildNotStarted && operation.Child != lifecycle.ChildDeadlineStartPending) {
			kernel.run.Dirty(errors.New("jobmgr kernel: invalid task start acknowledgement"))
			return
		}
		delete(kernel.tasksByRequest, start.Request)
		operation.taskRequest = lifecycle.TaskRequestRef{}
		if err := operation.Advance(lifecycle.OperationRunning); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := operation.StartChild(start.Task); err != nil {
			kernel.run.Dirty(err)
			return
		}
		kernel.tasksByRef[start.Task] = operation
	}
	if dispatchErr != nil {
		kernel.run.Dirty(dispatchErr)
	}
}

func (kernel *CommandKernel) rejectTaskStart(start lifecycle.TaskStart) {
	if cleanupRef, ok := kernel.functionCleanupRequests[start.Request]; ok {
		delete(kernel.functionCleanupRequests, start.Request)
		completeErr := errors.Join(errors.New("jobmgr kernel: Function cleanup task start rejected"), start.Err)
		kernel.run.Dirty(errors.Join(completeErr, kernel.functionCatalog.CompleteCleanup(cleanupRef, completeErr)))
		return
	}
	if kernel.shutdownBarrierRequest.Valid() &&
		start.Request == kernel.shutdownBarrierRequest {
		kernel.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
		kernel.shutdownBarrierFailed = true
		kernel.run.Dirty(errors.Join(
			errors.New("jobmgr kernel: shutdown barrier task start rejected"),
			start.Err,
		))
		return
	}
	operation := kernel.tasksByRequest[start.Request]
	if !errors.Is(start.Err, lifecycle.ErrLongLivedRecordCapacity) || operation == nil ||
		operation.taskRequest != start.Request || operation.Child != lifecycle.ChildNotStarted {
		kernel.run.Dirty(errors.Join(errors.New("jobmgr kernel: invalid task start rejection"), start.Err))
		return
	}
	if operation.plan.Transaction != nil {
		if err := kernel.restoreTransactionOutcome(operation, start.Outcome); err != nil {
			kernel.run.Dirty(errors.Join(
				errors.New("jobmgr kernel: rejected transaction start lost current resource"),
				err,
			))
			return
		}
	} else if start.Outcome.Kind() != lifecycle.TaskOutcomeNone {
		kernel.run.Dirty(
			errors.New("jobmgr kernel: rejected non-transaction start returned an outcome"),
		)
		return
	}
	delete(kernel.tasksByRequest, start.Request)
	operation.taskRequest = lifecycle.TaskRequestRef{}
	operation.terminalErr = errors.Join(operation.terminalErr, start.Err)
	kernel.unlinkQueued(operation, start.Err)
	if operation.Response != lifecycle.ResponseNotRequired {
		kernel.enqueueControl(operation, lifecycle.ControlUnavailable)
	}
	kernel.tryDispose(operation)
}

func (kernel *CommandKernel) completeTask(completion lifecycle.TaskCompletion) {
	if cleanup, ok := kernel.functionCleanupTasks[completion.Ref]; ok {
		if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone {
			completion.Err = errors.Join(completion.Err, errors.New("jobmgr kernel: invalid Function cleanup completion"))
		}
		cleanup.err = completion.Err
		kernel.functionCleanupTasks[completion.Ref] = cleanup
		if err := kernel.tasks.SendAction(lifecycle.TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate,
		}); err != nil {
			kernel.run.Dirty(err)
		}
		return
	}
	if kernel.finalizerTask.Valid() && completion.Ref == kernel.finalizerTask {
		kernel.completeRunFinalizer(completion)
		return
	}
	if kernel.shutdownBarrierTask.Valid() &&
		completion.Ref == kernel.shutdownBarrierTask {
		kernel.completeShutdownBarrier(completion)
		return
	}
	operation := kernel.tasksByRef[completion.Ref]
	if operation == nil {
		kernel.completeShutdownTask(completion)
		return
	}
	if _, err := kernel.tasks.ClearRetainedTimeout(operation.Task); err != nil {
		kernel.run.Dirty(err)
		return
	}
	var resultErr error
	if operation.plan.Transaction != nil && completion.Sequence > 1 {
		resultErr = operation.PhaseResultReady(
			completion.Ref,
			completion.Sequence,
		)
	} else {
		resultErr = operation.ResultReady(
			completion.Ref,
			completion.Sequence,
		)
	}
	if resultErr != nil {
		kernel.run.Dirty(resultErr)
		return
	}
	kernel.markOperationDeadlineIfDue(operation)
	if operation.plan.Transaction != nil {
		kernel.completeResourceTransactionTask(operation, completion)
		return
	}
	if operation.plan.Resource != nil {
		kernel.completeResourceTask(operation, completion)
		return
	}
	if operation.plan.Capability != nil {
		kernel.completeCapabilityTask(operation, completion)
		return
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: lifecycle.TaskActionDispose}
	if completion.Err == nil && operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
		expiry := lifecycle.ExpiryAt(kernel.clock.Now())
		preflight, err := kernel.tasks.PreflightResult(completion.Ref, operation.UID, expiry)
		if err != nil {
			status := lifecycle.ControlInternal
			if errors.Is(err, lifecycle.ErrFunctionResultTooLarge) {
				status = lifecycle.ControlPayloadTooLarge
			}
			kernel.enqueueControl(operation, status)
		} else {
			total, sizeErr := operationResultAdmissionBytes(operation.admissionBase, preflight)
			if sizeErr != nil {
				kernel.enqueueControl(operation, lifecycle.ControlPayloadTooLarge)
			} else {
				ready, _, resizeErr := kernel.admission.ResizeOrdinary(operation.admission, total)
				if resizeErr != nil {
					kernel.run.Dirty(resizeErr)
					return
				}
				operation.resultExpiry = expiry
				if !ready {
					operation.resultGrowthWaiting = true
					return
				}
				if err := kernel.sendEncodeAction(operation); err != nil {
					kernel.run.Dirty(err)
				}
				return
			}
		}
	}
	if completion.Err != nil && operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
		status := lifecycle.ControlUnavailable
		if errors.Is(completion.Err, lifecycle.ErrFunctionResultTooLarge) {
			status = lifecycle.ControlPayloadTooLarge
		} else if errors.Is(completion.Err, lifecycle.ErrTaskPanic) {
			status = lifecycle.ControlInternal
		}
		kernel.enqueueControl(operation, status)
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(action); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) completeCapabilityTask(operation *commandOperation, completion lifecycle.TaskCompletion) {
	kind := lifecycle.TaskActionDispose
	if completion.Err == nil && !operation.cancelled && !operation.TimedOut() {
		if completion.Kind != lifecycle.TaskOutcomePreparedCapability {
			kernel.dirtyCapability(operation, errors.New("jobmgr kernel: capability task returned the wrong outcome"))
			return
		}
		kind = lifecycle.TaskActionCommitCapability
	}
	if completion.Err != nil {
		kernel.dirtyCapability(operation, completion.Err)
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: kind}
	if kind == lifecycle.TaskActionCommitCapability {
		action.ExpectedGeneration = operation.resourceGeneration
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(action); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) completeResourceTask(operation *commandOperation, completion lifecycle.TaskCompletion) {
	kind := lifecycle.TaskActionDispose
	if completion.Err == nil && !operation.TimedOut() {
		switch operation.plan.Resource.Action {
		case ResourceInstall:
			if completion.Kind != lifecycle.TaskOutcomePreparedResource {
				kernel.run.Dirty(errors.New("jobmgr kernel: install task returned the wrong outcome"))
				return
			}
			kind = lifecycle.TaskActionAcceptStart
		case ResourceStop:
			if completion.Kind != lifecycle.TaskOutcomeReadyResource {
				kernel.run.Dirty(errors.New("jobmgr kernel: stop task returned the wrong outcome"))
				return
			}
			kind = lifecycle.TaskActionStopResource
		default:
			kernel.run.Dirty(errors.New("jobmgr kernel: unknown resource completion"))
			return
		}
	}
	if completion.Err != nil {
		kernel.run.Dirty(completion.Err)
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: kind}
	if kind == lifecycle.TaskActionAcceptStart {
		action.ExpectedGeneration = operation.resourceGeneration
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(action); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) completeResourceTransactionTask(
	operation *commandOperation,
	completion lifecycle.TaskCompletion,
) {
	switch completion.Sequence {
	case 1:
		if completion.Err != nil {
			current, err := kernel.tasks.TakeDisposedResourceTransaction(
				completion.Ref,
				completion.Sequence,
				operation.transactionScope,
			)
			if err != nil {
				kernel.run.Dirty(errors.Join(
					errors.New(
						"jobmgr kernel: failed transaction preparation lost current resource",
					),
					completion.Err,
					err,
				))
				return
			}
			if err := kernel.restoreTransactionCurrent(operation, current); err != nil {
				kernel.run.Dirty(errors.Join(completion.Err, err))
				return
			}
			status := lifecycle.ControlUnavailable
			if errors.Is(completion.Err, lifecycle.ErrTaskPanic) {
				status = lifecycle.ControlInternal
			}
			if operation.Response == lifecycle.ResponseOpen &&
				!operation.controlQueued {
				kernel.enqueueControl(operation, status)
			}
			kernel.sendTransactionAction(
				operation,
				completion.Ref,
				completion.Sequence+1,
				lifecycle.TaskActionDispose,
			)
			return
		}
		if completion.Kind != lifecycle.TaskOutcomePreparedResourceTransaction {
			kernel.run.Dirty(
				errors.New(
					"jobmgr kernel: transaction preparation returned the wrong outcome",
				),
			)
			return
		}
		action := lifecycle.TaskActionApplyResourceTransaction
		if operation.cancelled ||
			operation.TimedOut() ||
			(operation.Response != lifecycle.ResponseOpen &&
				operation.Response != lifecycle.ResponseNotRequired) ||
			operation.controlQueued {
			action = lifecycle.TaskActionDispose
		}
		kernel.sendTransactionAction(
			operation,
			completion.Ref,
			completion.Sequence+1,
			action,
		)
	case 2:
		if completion.Err != nil {
			operation.terminalErr = errors.Join(
				operation.terminalErr,
				completion.Err,
			)
			kernel.run.Dirty(errors.Join(
				errors.New(
					"jobmgr kernel: resource transaction apply left unprovable state",
				),
				completion.Err,
			))
			return
		}
		if completion.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction {
			kernel.run.Dirty(
				errors.New(
					"jobmgr kernel: transaction apply returned the wrong outcome",
				),
			)
			return
		}
		disposition, current, err :=
			kernel.tasks.TakeAppliedResourceTransaction(
				completion.Ref,
				completion.Sequence,
				operation.transactionScope,
			)
		if err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.applyTransactionDisposition(
			operation,
			disposition,
			current,
		); err != nil {
			kernel.run.Dirty(err)
			return
		}
		operation.transactionApplied = true

		if operation.Response == lifecycle.ResponseOpen &&
			!operation.controlQueued {
			expiry := lifecycle.ExpiryAt(kernel.clock.Now())
			preflight, preflightErr := kernel.tasks.PreflightResult(
				completion.Ref,
				operation.UID,
				expiry,
			)
			if preflightErr != nil {
				status := lifecycle.ControlInternal
				if errors.Is(
					preflightErr,
					lifecycle.ErrFunctionResultTooLarge,
				) {
					status = lifecycle.ControlPayloadTooLarge
				}
				kernel.enqueueControl(operation, status)
			} else {
				total, sizeErr := operationResultAdmissionBytes(
					operation.admissionBase,
					preflight,
				)
				if sizeErr != nil {
					kernel.enqueueControl(
						operation,
						lifecycle.ControlPayloadTooLarge,
					)
				} else {
					ready, _, resizeErr := kernel.admission.ResizeOrdinary(
						operation.admission,
						total,
					)
					if resizeErr != nil {
						kernel.run.Dirty(resizeErr)
						return
					}
					operation.resultExpiry = expiry
					if !ready {
						operation.resultGrowthWaiting = true
						return
					}
					if err := kernel.sendEncodeAction(operation); err != nil {
						kernel.run.Dirty(err)
					}
					return
				}
			}
		}
		kernel.sendTransactionAction(
			operation,
			completion.Ref,
			completion.Sequence+1,
			lifecycle.TaskActionDispose,
		)
	default:
		kernel.run.Dirty(
			errors.New("jobmgr kernel: unexpected transaction completion"),
		)
	}
}

func (kernel *CommandKernel) sendTransactionAction(
	operation *commandOperation,
	ref lifecycle.TaskRef,
	sequence uint8,
	kind lifecycle.TaskActionKind,
) {
	action := lifecycle.TaskAction{
		Ref: ref, Sequence: sequence, Kind: kind,
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(action); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) restoreTransactionOutcome(
	operation *commandOperation,
	outcome lifecycle.TaskOutcome,
) error {
	scope := operation.transactionScope
	if scope.Current.Valid() {
		current, ok := outcome.ReadyResource()
		identity, identityOK := outcome.ResourceIdentity()
		if !ok || !identityOK || identity != scope.Current {
			return errors.New(
				"jobmgr kernel: transaction outcome differs from exact current resource",
			)
		}
		return kernel.restoreTransactionCurrent(operation, current)
	}
	if outcome.Kind() != lifecycle.TaskOutcomeNone {
		return errors.New(
			"jobmgr kernel: graph-only transaction returned a current resource",
		)
	}
	return kernel.restoreTransactionCurrent(operation, nil)
}

func (kernel *CommandKernel) restoreTransactionCurrent(
	operation *commandOperation,
	current lifecycle.ReadyResource,
) error {
	if operation == nil ||
		operation.plan.Transaction == nil ||
		operation.transactionRestored ||
		operation.transactionApplied {
		return errors.New("jobmgr kernel: invalid transaction restoration")
	}
	lane := operation.lane
	scope := operation.transactionScope
	if lane == nil || lane.retiringIdentity.Valid() {
		return errors.New("jobmgr kernel: transaction restoration lost its lane")
	}
	if scope.Current.Valid() {
		if current == nil ||
			lane.current != nil ||
			!lane.currentStopping ||
			lane.currentIdentity != scope.Current {
			return errors.New(
				"jobmgr kernel: transaction restoration differs from detached current",
			)
		}
		lane.current = current
		lane.currentStopping = false
	} else {
		if current != nil ||
			lane.current != nil ||
			lane.currentIdentity.Valid() ||
			lane.currentStopping {
			return errors.New(
				"jobmgr kernel: graph-only transaction restoration found resource state",
			)
		}
	}
	operation.transactionRestored = true
	return nil
}

func (kernel *CommandKernel) applyTransactionDisposition(
	operation *commandOperation,
	disposition lifecycle.ResourceTransactionDisposition,
	current lifecycle.ReadyResource,
) error {
	if operation == nil ||
		operation.plan.Transaction == nil ||
		operation.transactionRestored ||
		operation.transactionApplied {
		return errors.New("jobmgr kernel: invalid transaction disposition")
	}
	lane := operation.lane
	scope := operation.transactionScope
	if lane == nil ||
		lane.retiringIdentity.Valid() ||
		lane.current != nil ||
		lane.currentStopping != scope.Current.Valid() ||
		lane.currentIdentity != scope.Current {
		return errors.New(
			"jobmgr kernel: transaction disposition differs from detached lane",
		)
	}

	var identity lifecycle.ResourceIdentity
	switch disposition {
	case lifecycle.ResourceTransactionUnchanged:
		identity = scope.Current
		if identity.Valid() != (current != nil) {
			return errors.New(
				"jobmgr kernel: unchanged transaction returned the wrong current resource",
			)
		}
	case lifecycle.ResourceTransactionRemoved:
		if current != nil {
			return errors.New(
				"jobmgr kernel: removed transaction returned a resource",
			)
		}
	case lifecycle.ResourceTransactionInstalled,
		lifecycle.ResourceTransactionReplaced:
		identity = scope.Successor
		if current == nil || !identity.Valid() {
			return errors.New(
				"jobmgr kernel: installed transaction lost its successor",
			)
		}
	default:
		return errors.New(
			"jobmgr kernel: unknown resource transaction disposition",
		)
	}
	lane.current = current
	lane.currentIdentity = identity
	lane.currentStopping = false
	if identity.Valid() {
		lane.resourceGeneration = identity.Generation
	} else {
		lane.resourceGeneration = 0
	}
	return nil
}

func (kernel *CommandKernel) completeShutdownTask(completion lifecycle.TaskCompletion) {
	if !completion.Ref.Valid() {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task completion"))
		return
	}
	lane := kernel.shutdownTasks[completion.Ref.Slot]
	if lane == nil || lane.shutdownTask != completion.Ref || lane.shutdownAction != 0 || completion.Sequence != 1 ||
		!lane.currentStopping || lane.current != nil || !lane.currentIdentity.Valid() || lane.retiringIdentity.Valid() {
		kernel.run.Dirty(errors.New("jobmgr kernel: completion for unknown or invalid shutdown task"))
		return
	}
	if completion.Err != nil {
		kernel.run.Dirty(completion.Err)
		return
	}
	if completion.Kind != lifecycle.TaskOutcomeReadyResource {
		kernel.run.Dirty(errors.New("jobmgr kernel: shutdown Stop task returned the wrong outcome"))
		return
	}
	kernel.sendShutdownAction(lane, lifecycle.TaskActionStopResource, 2)
}

func (kernel *CommandKernel) acknowledgeShutdownTask(ack lifecycle.TaskAcknowledgement) {
	if !ack.Ref.Valid() {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task acknowledgement"))
		return
	}
	lane := kernel.shutdownTasks[ack.Ref.Slot]
	if lane == nil || lane.shutdownTask != ack.Ref || lane.shutdownAction != ack.Kind {
		kernel.run.Dirty(errors.New("jobmgr kernel: acknowledgement for unknown or invalid shutdown task"))
		return
	}
	wantSequence := uint8(0)
	switch ack.Kind {
	case lifecycle.TaskActionStopResource:
		wantSequence = 2
	case lifecycle.TaskActionFinalizeResource:
		wantSequence = 3
	case lifecycle.TaskActionTerminate:
		wantSequence = 4
	default:
		kernel.run.Dirty(errors.New("jobmgr kernel: unexpected shutdown task action"))
		return
	}
	if ack.Sequence != wantSequence {
		kernel.run.Dirty(errors.New("jobmgr kernel: stale shutdown task acknowledgement"))
		return
	}
	if ack.Err != nil && ack.Kind != lifecycle.TaskActionTerminate {
		kernel.run.Dirty(ack.Err)
		return
	}
	lane.shutdownAction = 0
	switch ack.Kind {
	case lifecycle.TaskActionStopResource:
		identity := lane.currentIdentity
		if !lane.currentStopping || lane.current != nil || !identity.Valid() || lane.retiringIdentity.Valid() {
			kernel.run.Dirty(errors.New("jobmgr kernel: shutdown stopped resource differs from current slot"))
			return
		}
		lane.currentIdentity = lifecycle.ResourceIdentity{}
		lane.currentStopping = false
		lane.retiringIdentity = identity
		kernel.sendShutdownAction(lane, lifecycle.TaskActionFinalizeResource, 3)
	case lifecycle.TaskActionFinalizeResource:
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || !lane.retiringIdentity.Valid() {
			kernel.run.Dirty(errors.New("jobmgr kernel: shutdown finalized resource differs from retiring slot"))
			return
		}
		lane.retiringIdentity = lifecycle.ResourceIdentity{}
		kernel.sendShutdownAction(lane, lifecycle.TaskActionTerminate, 4)
	case lifecycle.TaskActionTerminate:
		if ack.Err != nil {
			kernel.run.Dirty(ack.Err)
		}
		if err := kernel.tasks.Release(ack.Ref); err != nil {
			kernel.run.Dirty(err)
			return
		}
		kernel.shutdownTasks[ack.Ref.Slot] = nil
		kernel.shutdownTaskCount--
		lane.shutdownTask = lifecycle.TaskRef{}
		lane.shutdownAction = 0
		kernel.releaseUnusedLane(lane)
	}
}

func (kernel *CommandKernel) sendShutdownAction(lane *commandLane, kind lifecycle.TaskActionKind, sequence uint8) {
	if lane == nil || !lane.shutdownTask.Valid() || lane.shutdownAction != 0 {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown action transition"))
		return
	}
	lane.shutdownAction = kind
	if err := kernel.tasks.SendAction(lifecycle.TaskAction{Ref: lane.shutdownTask, Sequence: sequence, Kind: kind}); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) sendEncodeAction(operation *commandOperation) error {
	if operation == nil || operation.Child != lifecycle.ChildResultReady || operation.resultExpiry <= 0 {
		return errors.New("jobmgr kernel: invalid result encode transition")
	}
	if err := operation.MarkResponsePending(); err != nil {
		return err
	}
	sequence := uint8(2)
	if operation.plan.Transaction != nil {
		if !operation.transactionApplied {
			return errors.New(
				"jobmgr kernel: transaction result is not applied",
			)
		}
		sequence = 3
	}
	action := lifecycle.TaskAction{
		Ref: operation.Task, Sequence: sequence, Kind: lifecycle.TaskActionEncodeWrite,
		UID: operation.UID, Expiry: operation.resultExpiry,
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		return err
	}
	return kernel.tasks.SendAction(action)
}

func (kernel *CommandKernel) acknowledgeTask(ack lifecycle.TaskAcknowledgement) {
	if cleanup, ok := kernel.functionCleanupTasks[ack.Ref]; ok {
		if ack.Kind != lifecycle.TaskActionTerminate || ack.Sequence != 2 {
			kernel.run.Dirty(errors.New("jobmgr kernel: invalid Function cleanup acknowledgement"))
			return
		}
		if err := kernel.tasks.Release(ack.Ref); err != nil {
			kernel.run.Dirty(err)
			return
		}
		delete(kernel.functionCleanupTasks, ack.Ref)
		completeErr := errors.Join(cleanup.err, ack.Err)
		catalogErr := kernel.functionCatalog.CompleteCleanup(cleanup.ref, completeErr)
		if completeErr != nil || catalogErr != nil {
			kernel.run.Dirty(errors.Join(completeErr, catalogErr))
		}
		return
	}
	if kernel.finalizerTask.Valid() && ack.Ref == kernel.finalizerTask {
		kernel.acknowledgeRunFinalizer(ack)
		return
	}
	if kernel.shutdownBarrierTask.Valid() &&
		ack.Ref == kernel.shutdownBarrierTask {
		kernel.acknowledgeShutdownBarrier(ack)
		return
	}
	operation := kernel.tasksByRef[ack.Ref]
	if operation == nil {
		kernel.acknowledgeShutdownTask(ack)
		return
	}
	if ack.Kind == lifecycle.TaskActionTerminate {
		if err := operation.ChildExited(ack.Ref, ack.Sequence); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if ack.Err != nil {
			operation.PoisonResponse()
			kernel.run.Dirty(ack.Err)
		}
		if err := kernel.tasks.Release(ack.Ref); err != nil {
			kernel.run.Dirty(err)
			return
		}
		delete(kernel.tasksByRef, ack.Ref)
		kernel.tryDispose(operation)
		return
	}
	kernel.markOperationDeadlineIfDue(operation)
	if err := operation.ActionAcknowledged(ack.Ref, ack.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if operation.plan.Transaction != nil {
		kernel.acknowledgeResourceTransactionTask(operation, ack)
		return
	}
	if operation.plan.Resource != nil {
		kernel.acknowledgeResourceTask(operation, ack)
		return
	}
	if operation.plan.Capability != nil {
		kernel.acknowledgeCapabilityTask(operation, ack)
		return
	}
	if ack.Err != nil {
		operation.PoisonResponse()
		kernel.run.Dirty(ack.Err)
	} else if ack.Kind == lifecycle.TaskActionEncodeWrite {
		if err := operation.CommitResponse(); err != nil {
			kernel.run.Dirty(err)
		}
		if _, _, err := kernel.admission.ResizeOrdinary(operation.admission, operation.admissionBase); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.completeOperationUID(operation, false); err != nil {
			kernel.run.Dirty(err)
			return
		}
	}
	if ack.Kind == lifecycle.TaskActionCleanup {
		operation.cleanupDone = true
	}
	if operation.plan.Cleanup != nil && !operation.cleanupDone {
		cleanup := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionCleanup}
		if err := operation.ActionPending(cleanup.Ref, cleanup.Sequence); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.tasks.SendAction(cleanup); err != nil {
			kernel.run.Dirty(err)
		}
		return
	}
	termination := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionTerminate}
	if err := operation.TerminationPending(termination.Ref, termination.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(termination); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) acknowledgeResourceTransactionTask(
	operation *commandOperation,
	ack lifecycle.TaskAcknowledgement,
) {
	if ack.Err != nil {
		operation.terminalErr = errors.Join(operation.terminalErr, ack.Err)
		if ack.Kind == lifecycle.TaskActionEncodeWrite {
			operation.PoisonResponse()
		}
		kernel.run.Dirty(ack.Err)
		if ack.Kind == lifecycle.TaskActionDispose &&
			!operation.transactionApplied {
			return
		}
	}

	switch ack.Kind {
	case lifecycle.TaskActionDispose:
		if !operation.transactionApplied {
			if ack.Err != nil {
				return
			}
			if !operation.transactionRestored {
				current, err := kernel.tasks.TakeDisposedResourceTransaction(
					ack.Ref,
					ack.Sequence,
					operation.transactionScope,
				)
				if err != nil {
					kernel.run.Dirty(err)
					return
				}
				if err := kernel.restoreTransactionCurrent(
					operation,
					current,
				); err != nil {
					kernel.run.Dirty(err)
					return
				}
			}
			kernel.sendResourceTermination(
				operation,
				ack.Ref,
				ack.Sequence+1,
			)
			return
		}
	case lifecycle.TaskActionEncodeWrite:
		if ack.Err == nil {
			if err := operation.CommitResponse(); err != nil {
				kernel.run.Dirty(err)
				return
			}
			if _, _, err := kernel.admission.ResizeOrdinary(
				operation.admission,
				operation.admissionBase,
			); err != nil {
				kernel.run.Dirty(err)
				return
			}
			if err := kernel.completeOperationUID(
				operation,
				false,
			); err != nil {
				kernel.run.Dirty(err)
				return
			}
		}
	case lifecycle.TaskActionCleanup:
		operation.cleanupDone = true
	default:
		kernel.run.Dirty(
			errors.New(
				"jobmgr kernel: unexpected resource transaction acknowledgement",
			),
		)
		return
	}

	if operation.transactionApplied && !operation.cleanupDone {
		cleanup := lifecycle.TaskAction{
			Ref:      ack.Ref,
			Sequence: ack.Sequence + 1,
			Kind:     lifecycle.TaskActionCleanup,
		}
		if err := operation.ActionPending(
			cleanup.Ref,
			cleanup.Sequence,
		); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.tasks.SendAction(cleanup); err != nil {
			kernel.run.Dirty(err)
		}
		return
	}
	kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
}

func (kernel *CommandKernel) acknowledgeCapabilityTask(operation *commandOperation, ack lifecycle.TaskAcknowledgement) {
	switch ack.Kind {
	case lifecycle.TaskActionCommitCapability:
		switch ack.CapabilityDisposition {
		case lifecycle.CapabilityApplied:
			if ack.Err != nil {
				kernel.dirtyCapability(operation, ack.Err)
			}
		case lifecycle.CapabilityDisposed:
			if ack.Err == nil {
				kernel.dirtyCapability(operation, errors.New("jobmgr kernel: capability was disposed without a commit error"))
			} else {
				kernel.dirtyCapability(operation, ack.Err)
			}
		case lifecycle.CapabilityRetained:
			kernel.dirtyCapability(operation, errors.Join(errors.New("jobmgr kernel: capability commit retained ambiguous ownership"), ack.Err))
			return
		default:
			kernel.dirtyCapability(operation, errors.New("jobmgr kernel: invalid capability commit disposition"))
			return
		}
	case lifecycle.TaskActionDispose:
		if ack.CapabilityDisposition != 0 {
			kernel.dirtyCapability(operation, errors.New("jobmgr kernel: dispose acknowledged a capability disposition"))
			return
		}
		if ack.Err != nil {
			kernel.dirtyCapability(operation, ack.Err)
			return
		}
	default:
		kernel.dirtyCapability(operation, errors.New("jobmgr kernel: unexpected capability acknowledgement"))
		return
	}
	kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
}

func (kernel *CommandKernel) dirtyCapability(operation *commandOperation, err error) {
	if err == nil {
		err = errors.New("jobmgr kernel: unspecified capability failure")
	}
	operation.terminalErr = errors.Join(operation.terminalErr, err)
	kernel.run.Dirty(err)
}

func (kernel *CommandKernel) acknowledgeResourceTask(operation *commandOperation, ack lifecycle.TaskAcknowledgement) {
	if ack.Err != nil {
		kernel.run.Dirty(ack.Err)
		if ack.Kind == lifecycle.TaskActionStopResource || ack.Kind == lifecycle.TaskActionFinalizeResource || ack.Kind == lifecycle.TaskActionDispose {
			return
		}
		if ack.Kind == lifecycle.TaskActionAcceptStart || ack.Kind == lifecycle.TaskActionPublishResource {
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
				kernel.run.Dirty(err)
				return
			}
			if err := kernel.tasks.SendAction(action); err != nil {
				kernel.run.Dirty(err)
			}
			return
		}
		kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
		return
	}
	lane := operation.lane
	switch ack.Kind {
	case lifecycle.TaskActionAcceptStart:
		if operation.cancelled || operation.TimedOut() {
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
				kernel.run.Dirty(err)
				return
			}
			if err := kernel.tasks.SendAction(action); err != nil {
				kernel.run.Dirty(err)
			}
			return
		}
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() {
			kernel.run.Dirty(errors.New("jobmgr kernel: resource publication found a nonempty current slot"))
			return
		}
		action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionPublishResource}
		if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.tasks.SendAction(action); err != nil {
			kernel.run.Dirty(err)
		}
	case lifecycle.TaskActionPublishResource:
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() {
			kernel.run.Dirty(errors.New("jobmgr kernel: resource publication found a nonempty current slot"))
			return
		}
		expected := lifecycle.ResourceIdentity{ID: operation.plan.Resource.ID, Generation: operation.resourceGeneration}
		resource, err := kernel.tasks.TakePublishedReadyResource(ack.Ref, ack.Sequence, expected)
		if err != nil {
			kernel.run.Dirty(err)
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if actionErr := operation.ActionPending(action.Ref, action.Sequence); actionErr != nil {
				kernel.run.Dirty(actionErr)
				return
			}
			if actionErr := kernel.tasks.SendAction(action); actionErr != nil {
				kernel.run.Dirty(actionErr)
			}
			return
		}
		identity := expected
		lane.current = resource
		lane.currentIdentity = identity
		lane.resourceGeneration = identity.Generation
		kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	case lifecycle.TaskActionStopResource:
		identity := lane.currentIdentity
		if !lane.currentStopping || lane.current != nil || identity.ID != operation.plan.Resource.ID || identity.Generation != operation.resourceGeneration || lane.retiringIdentity.Valid() {
			kernel.run.Dirty(errors.New("jobmgr kernel: stopped resource differs from current slot"))
			return
		}
		lane.currentIdentity = lifecycle.ResourceIdentity{}
		lane.currentStopping = false
		lane.retiringIdentity = identity
		action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionFinalizeResource}
		if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
			kernel.run.Dirty(err)
			return
		}
		if err := kernel.tasks.SendAction(action); err != nil {
			kernel.run.Dirty(err)
		}
	case lifecycle.TaskActionFinalizeResource:
		identity := lane.retiringIdentity
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || identity.ID != operation.plan.Resource.ID || identity.Generation != operation.resourceGeneration {
			kernel.run.Dirty(errors.New("jobmgr kernel: finalized resource differs from retiring slot"))
			return
		}
		lane.retiringIdentity = lifecycle.ResourceIdentity{}
		kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	case lifecycle.TaskActionDispose:
		kernel.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	default:
		kernel.run.Dirty(errors.New("jobmgr kernel: unexpected resource acknowledgement"))
	}
}

func (kernel *CommandKernel) sendResourceTermination(operation *commandOperation, ref lifecycle.TaskRef, sequence uint8) {
	termination := lifecycle.TaskAction{Ref: ref, Sequence: sequence, Kind: lifecycle.TaskActionTerminate}
	if err := operation.TerminationPending(termination.Ref, termination.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(termination); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) cancelOperation(uid string) {
	operation := kernel.operations[uid]
	if operation == nil || operation.Response == lifecycle.ResponseCommitted || operation.Response == lifecycle.ResponsePoisoned {
		return
	}
	if operation.TimedOut() {
		return
	}
	operation.cancelled = true
	if operation.Child == lifecycle.ChildExecuting {
		_ = kernel.tasks.Cancel(operation.Task)
		if operation.Response != lifecycle.ResponseNotRequired && !operation.plan.CooperativeCancel {
			kernel.enqueueControl(operation, lifecycle.ControlCancelled)
		}
		return
	}
	if operation.Child == lifecycle.ChildDeadlineStartPending {
		return
	}
	if operation.Child == lifecycle.ChildNotStarted {
		var cause error
		if operation.submissionContext != nil {
			cause = context.Cause(operation.submissionContext)
		}
		if cause == nil {
			cause = context.Canceled
		}
		kernel.unlinkQueued(operation, cause)
		if operation.Response != lifecycle.ResponseNotRequired {
			kernel.enqueueControl(operation, lifecycle.ControlCancelled)
		} else {
			kernel.tryDispose(operation)
		}
		return
	}
	if operation.Child == lifecycle.ChildResultReady && operation.resultGrowthWaiting {
		if err := kernel.admission.CancelWaiting(operation.admission); err != nil {
			kernel.run.Dirty(err)
			return
		}
		operation.resultGrowthWaiting = false
		kernel.enqueueControl(operation, lifecycle.ControlCancelled)
		kernel.sendDisposeAction(operation)
		return
	}
	if operation.Child == lifecycle.ChildActionPending && cancellablePendingAction(operation) {
		_ = kernel.tasks.Cancel(operation.Task)
	}
}

func (kernel *CommandKernel) sendDisposeAction(operation *commandOperation) {
	sequence := uint8(2)
	if operation.plan.Transaction != nil && operation.transactionApplied {
		sequence = 3
	}
	action := lifecycle.TaskAction{
		Ref:      operation.Task,
		Sequence: sequence,
		Kind:     lifecycle.TaskActionDispose,
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		kernel.run.Dirty(err)
		return
	}
	if err := kernel.tasks.SendAction(action); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) serviceDeadlines(now time.Time, quantum int) bool {
	for quantum > 0 && kernel.deadlines.Len() > 0 {
		entry := kernel.deadlines[0]
		if entry.when.After(now) {
			return false
		}
		heap.Pop(&kernel.deadlines)
		quantum--
		operation := entry.operation
		if operation.State == lifecycle.OperationDisposedTerminal ||
			(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
			continue
		}
		operation.MarkTimedOut()
		deferControl := requiresCooperativeDeadlineStart(operation) &&
			(operation.Child == lifecycle.ChildNotStarted || operation.Child == lifecycle.ChildExecuting)
		if operation.Child == lifecycle.ChildExecuting {
			_ = kernel.tasks.CancelWithCause(operation.Task, context.DeadlineExceeded)
			if operation.Response == lifecycle.ResponseNotRequired {
				if err := kernel.markRetainedTimeout(operation, true); err != nil {
					kernel.run.Dirty(err)
				}
			}
		} else if operation.Child == lifecycle.ChildNotStarted {
			if requiresCooperativeDeadlineStart(operation) {
				if err := operation.RequireDeadlineStart(); err != nil {
					kernel.run.Dirty(err)
					return false
				}
				if operation.taskRequest.Valid() {
					if err := kernel.tasks.SetPendingCancellation(operation.taskRequest, context.DeadlineExceeded); err != nil {
						kernel.run.Dirty(err)
						return false
					}
				}
			} else {
				kernel.unlinkQueued(operation, context.DeadlineExceeded)
				if operation.Response == lifecycle.ResponseNotRequired {
					kernel.tryDispose(operation)
				}
			}
		} else if operation.Child == lifecycle.ChildResultReady && operation.resultGrowthWaiting {
			if err := kernel.admission.CancelWaiting(operation.admission); err != nil {
				kernel.run.Dirty(err)
				return false
			}
			operation.resultGrowthWaiting = false
			kernel.sendDisposeAction(operation)
		} else if operation.Child == lifecycle.ChildActionPending && cancellablePendingAction(operation) {
			_ = kernel.tasks.CancelWithCause(operation.Task, context.DeadlineExceeded)
		}
		if operation.Response != lifecycle.ResponseNotRequired && !deferControl {
			kernel.enqueueControl(operation, lifecycle.ControlDeadline)
		}
	}
	return kernel.deadlines.Len() > 0 && !kernel.deadlines[0].when.After(now)
}

func cancellablePendingAction(operation *commandOperation) bool {
	return operation != nil &&
		(operation.plan.Capability != nil ||
			operation.plan.Resource != nil ||
			operation.plan.Transaction != nil)
}

func requiresCooperativeDeadlineStart(operation *commandOperation) bool {
	return operation != nil && operation.plan.Work != nil && operation.plan.CooperativeDeadline
}

func (kernel *CommandKernel) markOperationDeadlineIfDue(operation *commandOperation) {
	if operation == nil || operation.request.Deadline.IsZero() ||
		(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
		return
	}
	if operation.TimedOut() {
		if operation.Response == lifecycle.ResponseOpen {
			kernel.enqueueControl(operation, lifecycle.ControlDeadline)
		}
		return
	}
	if operation.request.Deadline.After(kernel.clock.Now()) {
		return
	}
	operation.MarkTimedOut()
	if operation.Response == lifecycle.ResponseOpen {
		kernel.enqueueControl(operation, lifecycle.ControlDeadline)
	}
}

func (kernel *CommandKernel) enqueueControl(operation *commandOperation, status lifecycle.ControlStatus) {
	if operation == nil || operation.Response != lifecycle.ResponseOpen || operation.controlQueued {
		return
	}
	operation.control = status
	operation.controlQueued = true
	kernel.controls = append(kernel.controls, operation)
}

func (kernel *CommandKernel) serviceControls(quantum int) bool {
	for quantum > 0 && len(kernel.controls) > 0 {
		operation := kernel.controls[0]
		if operation.Response == lifecycle.ResponseOpen {
			_ = operation.MarkResponsePending()
		}
		err := kernel.frames.TryCommitControl(lifecycle.ControlFramePlan{UID: operation.UID, Status: operation.control, Expiry: lifecycle.ExpiryAt(kernel.clock.Now())})
		if errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			return false
		}
		kernel.controls = kernel.controls[1:]
		operation.controlQueued = false
		quantum--
		if err != nil {
			operation.PoisonResponse()
			kernel.run.Dirty(err)
			kernel.tryDispose(operation)
			continue
		}
		if err := operation.CommitResponse(); err != nil {
			kernel.run.Dirty(err)
			continue
		}
		if err := kernel.completeOperationUID(operation, true); err != nil {
			kernel.run.Dirty(err)
			return false
		}
		if operation.TimedOut() && operation.Child == lifecycle.ChildExecuting {
			if err := kernel.markRetainedTimeout(operation, false); err != nil {
				kernel.run.Dirty(err)
			}
		}
		kernel.tryDispose(operation)
	}
	return len(kernel.controls) > 0
}

func (kernel *CommandKernel) markRetainedTimeout(operation *commandOperation, background bool) error {
	if operation == nil || !operation.TimedOut() || operation.Child != lifecycle.ChildExecuting || !operation.Task.Valid() {
		return errors.New("jobmgr kernel: invalid retained-timeout mark")
	}
	saturated, err := kernel.tasks.MarkRetainedTimeout(operation.Task)
	if err != nil {
		return err
	}
	if saturated && background {
		return errors.New("jobmgr kernel: fourth background timeout saturated all task slots")
	}
	if saturated {
		return errors.New("jobmgr kernel: fourth retained timeout saturated all task slots")
	}
	return nil
}

func (kernel *CommandKernel) tryDispose(operation *commandOperation) {
	if !operation.CanDisposeTerminal() {
		return
	}
	if operation.taskRequest.Valid() {
		kernel.run.Dirty(errors.New("jobmgr kernel: terminal operation retained a pending task request"))
		return
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if operation.Response == lifecycle.ResponseNotRequired {
		if err := kernel.completeOperationUID(operation, false); err != nil {
			kernel.run.Dirty(err)
			return
		}
	}
	if operation.functionInvocation.Valid() {
		ref := operation.functionInvocation
		operation.functionInvocation = FunctionInvocationRef{}
		if err := kernel.releaseFunctionInvocation(ref); err != nil {
			kernel.run.Dirty(err)
		}
	}
	if operation.deadline.index >= 0 {
		heap.Remove(&kernel.deadlines, operation.deadline.index)
	}
	for _, granted := range kernel.releaseClaims(operation) {
		kernel.markReady(granted.lane)
	}
	lane := operation.lane
	if lane.active == operation {
		lane.active = nil
	}
	if lane.head == operation {
		kernel.removeHead(lane)
	} else if operation.previous != nil || operation.next != nil {
		kernel.unlink(operation)
	}
	if operation.admission.Valid() {
		if !operation.admitted {
			kernel.run.Dirty(errors.New("jobmgr kernel: terminal operation retained an ungranted admission"))
			return
		}
		if _, err := kernel.admission.ReleaseOrdinary(operation.admission); err != nil {
			kernel.run.Dirty(err)
			return
		}
		delete(kernel.byAdmission, operation.admission)
		operation.admission = lifecycle.AdmissionRef{}
	}
	if resource := operation.plan.Resource; resource != nil {
		switch resource.Action {
		case ResourceInstall:
			if !lane.installPlanned {
				kernel.run.Dirty(errors.New("jobmgr kernel: install plan marker cleared twice"))
				return
			}
			lane.installPlanned = false
		case ResourceStop:
			if !lane.stopPlanned {
				kernel.run.Dirty(errors.New("jobmgr kernel: stop plan marker cleared twice"))
				return
			}
			lane.stopPlanned = false
		}
	}
	if operation.plan.Transaction != nil {
		if lane.transactionPlanned <= 0 {
			kernel.run.Dirty(
				errors.New(
					"jobmgr kernel: transaction plan marker cleared twice",
				),
			)
			return
		}
		lane.transactionPlanned--
	}
	lane.owners--
	if lane.owners < 0 {
		kernel.run.Dirty(errors.New("jobmgr kernel: negative lane ownership"))
		return
	}
	_ = operation.Advance(lifecycle.OperationDisposedTerminal)
	delete(kernel.operations, operation.UID)
	if operation.terminalResult != nil {
		operation.terminalResult <- operation.terminalErr
		operation.terminalResult = nil
	}
	if lane.active == nil && lane.head != nil {
		kernel.markReady(lane)
	}
	kernel.releaseUnusedLane(lane)
}

func (kernel *CommandKernel) completeOperationUID(operation *commandOperation, tombstone bool) error {
	if operation.uidCompleted {
		return errors.New("jobmgr kernel: operation UID completed twice")
	}
	if err := kernel.uids.Complete(operation.UID, tombstone, kernel.clock.Now()); err != nil {
		return err
	}
	operation.uidCompleted = true
	return nil
}

func (kernel *CommandKernel) unlinkQueued(operation *commandOperation, submissionErr error) {
	if operation.Child == lifecycle.ChildDeadlineStartPending {
		kernel.run.Dirty(errors.New("jobmgr kernel: required deadline start was unlinked without abandonment"))
		return
	}
	lane := operation.lane
	if lane.ready {
		kernel.ready[sourceIndex(lane.source)].remove(lane)
	}
	if operation.admission.Valid() && !operation.admitted {
		if err := kernel.admission.CancelWaiting(operation.admission); err != nil {
			kernel.run.Dirty(err)
		} else {
			delete(kernel.byAdmission, operation.admission)
			operation.admission = lifecycle.AdmissionRef{}
			operation.request.Args = nil
			operation.request.Payload = nil
			operation.plan.Claims = nil
			operation.plan.ReadClaims = nil
			operation.plan.Work = nil
			operation.plan.Runner = nil
			operation.plan.Cleanup = nil
			operation.claims = nil
			operation.authorityClaimEdges = nil
			kernel.settleSubmission(operation, submissionErr)
		}
	}
	if operation.taskRequest.Valid() {
		var err error
		if operation.plan.Resource != nil && operation.plan.Resource.Action == ResourceStop {
			var outcome lifecycle.TaskOutcome
			outcome, err = kernel.tasks.CancelPendingOutcome(operation.taskRequest)
			if err == nil {
				resource, ok := outcome.ReadyResource()
				identity, identityOK := outcome.ResourceIdentity()
				if !ok || !identityOK || !operation.lane.currentStopping || operation.lane.current != nil || identity != operation.lane.currentIdentity {
					err = errors.New("jobmgr kernel: cancelled stop did not return the exact current resource")
				} else {
					operation.lane.current = resource
					operation.lane.currentStopping = false
				}
			}
		} else if operation.plan.Transaction != nil {
			var outcome lifecycle.TaskOutcome
			outcome, err = kernel.tasks.CancelPendingOutcome(
				operation.taskRequest,
			)
			if err == nil {
				err = kernel.restoreTransactionOutcome(
					operation,
					outcome,
				)
			}
		} else {
			err = kernel.tasks.CancelPending(operation.taskRequest)
		}
		if err != nil {
			kernel.run.Dirty(err)
		} else {
			delete(kernel.tasksByRequest, operation.taskRequest)
			operation.taskRequest = lifecycle.TaskRequestRef{}
		}
	}
	if operation.claimsHeld {
		for _, granted := range kernel.releaseClaims(operation) {
			kernel.markReady(granted.lane)
		}
	} else if kernel.claims.Waiting(operation) {
		granted, err := kernel.claims.Cancel(operation)
		if err != nil {
			kernel.run.Dirty(err)
		}
		for _, grantedOperation := range granted {
			kernel.markReady(grantedOperation.lane)
		}
	} else if operation.claimRegistered {
		if err := kernel.claims.Abandon(operation); err != nil {
			kernel.run.Dirty(err)
		}
	}
	if lane.head == operation {
		kernel.removeHead(lane)
	} else {
		kernel.unlink(operation)
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if lane.active == nil && lane.head != nil {
		kernel.markReady(lane)
	}
}

func (kernel *CommandKernel) releaseClaims(operation *commandOperation) []*commandOperation {
	if !operation.claimsHeld {
		return nil
	}
	granted, err := kernel.claims.Release(operation)
	if err != nil {
		kernel.run.Dirty(err)
		return nil
	}
	return granted
}

func (kernel *CommandKernel) removeHead(lane *commandLane) {
	operation := lane.head
	if operation == nil {
		return
	}
	lane.head = operation.next
	if lane.head != nil {
		lane.head.previous = nil
	} else {
		lane.tail = nil
	}
	operation.previous = nil
	operation.next = nil
}

func (kernel *CommandKernel) unlink(operation *commandOperation) {
	if operation.previous != nil {
		operation.previous.next = operation.next
	}
	if operation.next != nil {
		operation.next.previous = operation.previous
	}
	if operation.lane.tail == operation {
		operation.lane.tail = operation.previous
	}
	operation.previous = nil
	operation.next = nil
}

func (kernel *CommandKernel) markReady(lane *commandLane) {
	if lane == nil || lane.active != nil || lane.head == nil || !lane.head.admitted {
		return
	}
	index := sourceIndex(lane.source)
	kernel.ready[index].push(lane)
}

func (kernel *CommandKernel) nextReadyLane() *commandLane {
	first := sourceIndex(kernel.nextSource)
	second := 1 - first
	if lane := kernel.ready[first].pop(); lane != nil {
		kernel.nextSource = otherSource(kernel.nextSource)
		return lane
	}
	if lane := kernel.ready[second].pop(); lane != nil {
		kernel.nextSource = otherSource(lane.source)
		return lane
	}
	return nil
}

func (kernel *CommandKernel) nextDeadline() time.Time {
	if kernel.deadlines.Len() == 0 {
		return time.Time{}
	}
	return kernel.deadlines[0].when
}

func (kernel *CommandKernel) beginShutdown(deadline time.Time) error {
	if deadline.IsZero() {
		return errors.New("jobmgr kernel: zero shutdown deadline")
	}
	if kernel.functionMutationActive {
		var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
		count, abortErr := kernel.functionCatalog.AbortMutation(&cleanups)
		pushErr := kernel.pushFunctionCleanupBatch(&cleanups, count)
		terminalErr := errors.Join(ErrStopped, abortErr, pushErr)
		kernel.functionMutation.result <- functionMutationResult{err: terminalErr}
		kernel.functionMutation = functionMutationSubmission{}
		kernel.functionMutationActive = false
		if abortErr != nil || pushErr != nil {
			return errors.Join(abortErr, pushErr)
		}
	}
	if _, err := kernel.tasks.SealAndCancelInherited(); err != nil {
		return err
	}
	for _, operation := range kernel.operations {
		operation.cancelled = true
		switch operation.Child {
		case lifecycle.ChildExecuting:
			if operation.plan.Resource == nil || operation.plan.Resource.Action != ResourceStop {
				_ = kernel.tasks.Cancel(operation.Task)
			}
			if operation.Response != lifecycle.ResponseNotRequired && !operation.plan.CooperativeCancel &&
				!(operation.TimedOut() && requiresCooperativeDeadlineStart(operation)) {
				kernel.enqueueControl(operation, cancellationControl(operation))
			}
		case lifecycle.ChildNotStarted:
			kernel.unlinkQueued(operation, ErrStopped)
			if operation.Response != lifecycle.ResponseNotRequired {
				kernel.enqueueControl(operation, cancellationControl(operation))
			} else {
				kernel.tryDispose(operation)
			}
		case lifecycle.ChildDeadlineStartPending:
			if !operation.taskRequest.Valid() {
				if err := operation.AbandonDeadlineStart(); err != nil {
					return err
				}
				kernel.unlinkQueued(operation, ErrStopped)
				if operation.Response == lifecycle.ResponseOpen {
					kernel.enqueueControl(operation, lifecycle.ControlDeadline)
				} else {
					kernel.tryDispose(operation)
				}
			}
		case lifecycle.ChildResultReady:
			if operation.resultGrowthWaiting {
				if err := kernel.admission.CancelWaiting(operation.admission); err != nil {
					return err
				}
				operation.resultGrowthWaiting = false
				kernel.sendDisposeAction(operation)
				kernel.enqueueControl(operation, cancellationControl(operation))
			}
		case lifecycle.ChildActionPending:
			if shutdownCancellablePendingAction(operation) {
				_ = kernel.tasks.Cancel(operation.Task)
			}
		}
	}
	if err := kernel.advanceShutdownAdmission(); err != nil {
		return err
	}
	return kernel.advanceShutdownBarrier()
}

func shutdownCancellablePendingAction(operation *commandOperation) bool {
	return operation != nil &&
		(operation.plan.Capability != nil ||
			operation.plan.Transaction != nil ||
			operation.plan.Resource != nil &&
				operation.plan.Resource.Action == ResourceInstall)
}

func cancellationControl(operation *commandOperation) lifecycle.ControlStatus {
	if operation != nil && operation.TimedOut() {
		return lifecycle.ControlDeadline
	}
	return lifecycle.ControlCancelled
}

func (kernel *CommandKernel) advanceShutdownAdmission() error {
	grant, waiting, err := kernel.admission.TakeShutdownInputBodyGrant(kernel.run.Generation())
	if err != nil {
		return err
	}
	if grant.Kind == lifecycle.ReservationInputBodyGrowth {
		select {
		case kernel.inputBodyGrants <- grant:
		default:
			return errors.New("jobmgr kernel: input body grant gate is full during shutdown")
		}
	}
	if waiting {
		return nil
	}
	return kernel.admission.BeginCleanupOnly(kernel.run.Generation())
}

func (kernel *CommandKernel) enqueueShutdownStops() error {
	for _, lane := range kernel.lanes {
		if lane.currentStopping {
			if lane.current != nil || !lane.currentIdentity.Valid() || lane.retiringIdentity.Valid() {
				return errors.New("jobmgr kernel: shutdown found an invalid stopping resource")
			}
			continue
		}
		if lane.retiringIdentity.Valid() {
			if lane.current != nil || lane.currentIdentity.Valid() {
				return errors.New("jobmgr kernel: shutdown found an invalid retiring resource")
			}
			continue
		}
		if lane.current == nil {
			if lane.currentIdentity.Valid() {
				return errors.New("jobmgr kernel: shutdown found a detached current identity")
			}
			continue
		}
		if lane.owners != 0 {
			continue
		}
		if lane.head != nil || lane.tail != nil || lane.active != nil || lane.ready {
			return errors.New("jobmgr kernel: owner-free resource lane retains operation state")
		}
		identity := lane.currentIdentity
		if !identity.Valid() || identity.ID != lane.key {
			return errors.New("jobmgr kernel: shutdown found an invalid current resource")
		}
		budget, err := kernel.run.BeginShutdown()
		if err != nil {
			return err
		}
		plan, err := lifecycle.NewShutdownReadyResourceTaskPlan(
			lane.source, budget, lifecycle.TransactionTaskPhases, lane.current, identity,
		)
		if err != nil {
			return err
		}
		request, err := kernel.tasks.Enqueue(plan)
		if err != nil {
			return err
		}
		if owner := kernel.shutdownRequests[request.Slot]; owner != nil {
			outcome, cancelErr := kernel.tasks.CancelPendingOutcome(request)
			_, ok := outcome.ReadyResource()
			returnedIdentity, identityOK := outcome.ResourceIdentity()
			if cancelErr != nil || !ok || !identityOK || returnedIdentity != identity {
				return errors.Join(errors.New("jobmgr kernel: occupied shutdown request owner"), cancelErr)
			}
			return errors.New("jobmgr kernel: occupied shutdown request owner")
		}
		lane.current = nil
		lane.currentStopping = true
		lane.shutdownRequest = request
		kernel.shutdownRequests[request.Slot] = lane
		kernel.shutdownRequestCount++
	}
	return nil
}

func (kernel *CommandKernel) advanceShutdownBarrier() error {
	if kernel.shutdownBarrier == nil ||
		kernel.shutdownBarrierDone ||
		kernel.shutdownBarrierFailed ||
		kernel.shutdownBarrierRequest.Valid() ||
		kernel.shutdownBarrierTask.Valid() {
		return nil
	}
	budget, err := kernel.run.BeginShutdown()
	if err != nil {
		return err
	}
	plan, err := lifecycle.NewShutdownWorkTaskPlan(
		lifecycle.SourceFunction,
		budget,
		3,
		func(ctx context.Context) (lifecycle.TaskOutcome, error) {
			return lifecycle.NoValueOutcome(),
				kernel.shutdownBarrier.BeforeFunctionCatalogClose(
					ctx,
					kernel.run.Generation(),
				)
		},
	)
	if err != nil {
		return err
	}
	request, err := kernel.tasks.Enqueue(plan)
	if err != nil {
		return err
	}
	kernel.shutdownBarrierRequest = request
	return nil
}

func isNoopRunShutdownBarrier(barrier RunShutdownBarrier) bool {
	_, ok := barrier.(noopRunShutdownBarrier)
	return ok
}

func (kernel *CommandKernel) completeShutdownBarrier(
	completion lifecycle.TaskCompletion,
) {
	if completion.Sequence != 1 ||
		completion.Kind != lifecycle.TaskOutcomeNone ||
		kernel.shutdownBarrierAction != 0 ||
		kernel.shutdownBarrierDone ||
		kernel.shutdownBarrierFailed {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier completion"))
		return
	}
	if completion.Err != nil {
		kernel.shutdownBarrierFailed = true
		kernel.run.Dirty(completion.Err)
	}
	kernel.shutdownBarrierAction = lifecycle.TaskActionTerminate
	if err := kernel.tasks.SendAction(lifecycle.TaskAction{
		Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate,
	}); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) acknowledgeShutdownBarrier(
	ack lifecycle.TaskAcknowledgement,
) {
	if ack.Sequence != 2 ||
		ack.Kind != lifecycle.TaskActionTerminate ||
		kernel.shutdownBarrierAction != lifecycle.TaskActionTerminate ||
		kernel.shutdownBarrierDone {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier acknowledgement"))
		return
	}
	if ack.Err != nil {
		kernel.shutdownBarrierFailed = true
		kernel.run.Dirty(ack.Err)
	}
	if err := kernel.tasks.Release(ack.Ref); err != nil {
		kernel.shutdownBarrierFailed = true
		kernel.run.Dirty(err)
		return
	}
	kernel.shutdownBarrierTask = lifecycle.TaskRef{}
	kernel.shutdownBarrierAction = 0
	if !kernel.shutdownBarrierFailed {
		kernel.shutdownBarrierDone = true
	}
}

func (kernel *CommandKernel) runShutdownBarrierFailedTerminal() bool {
	return kernel.shutdownBarrierFailed &&
		!kernel.shutdownBarrierRequest.Valid() &&
		!kernel.shutdownBarrierTask.Valid() &&
		kernel.shutdownBarrierAction == 0
}

func (kernel *CommandKernel) advanceRunFinalizer() error {
	if kernel.finalizer == nil || kernel.finalizerDone || kernel.finalizerFailed || kernel.finalizerRequest.Valid() || kernel.finalizerTask.Valid() {
		return nil
	}
	if !kernel.shutdownReadyForFinalizer() {
		return nil
	}
	budget, err := kernel.run.BeginShutdown()
	if err != nil {
		return err
	}
	plan, err := lifecycle.NewShutdownWorkTaskPlan(
		lifecycle.SourceJobManager, budget, 3,
		func(ctx context.Context) (lifecycle.TaskOutcome, error) {
			return lifecycle.NoValueOutcome(), kernel.finalizer.FinalizeRun(ctx, kernel.run.Generation())
		},
	)
	if err != nil {
		return err
	}
	request, err := kernel.tasks.Enqueue(plan)
	if err != nil {
		return err
	}
	kernel.finalizerRequest = request
	return nil
}

func (kernel *CommandKernel) shutdownReadyForFinalizer() bool {
	if !kernel.shutdownBarrierDone || kernel.shutdownBarrierFailed ||
		kernel.shutdownBarrierRequest.Valid() ||
		kernel.shutdownBarrierTask.Valid() ||
		kernel.shutdownBarrierAction != 0 {
		return false
	}
	inherited := kernel.tasks.InheritedCensus()
	longLived := kernel.tasks.LongLivedCensus()
	if len(kernel.operations) != 0 || len(kernel.tasksByRef) != 0 || len(kernel.tasksByRequest) != 0 ||
		len(kernel.functionCleanupTasks) != 0 || len(kernel.functionCleanupRequests) != 0 ||
		kernel.functionCleanupBacklog.count != 0 || kernel.functionMutationActive ||
		len(kernel.byAdmission) != 0 || len(kernel.lanes) != 0 ||
		kernel.tasks.Active() != 0 || kernel.tasks.Pending() != 0 || inherited.Active != 0 ||
		kernel.shutdownRequestCount != 0 || kernel.shutdownTaskCount != 0 || len(kernel.controls) != 0 || kernel.deadlines.Len() != 0 ||
		len(kernel.submissions[0]) != 0 || len(kernel.submissions[1]) != 0 || kernel.blockedSubmission[0] || kernel.blockedSubmission[1] ||
		kernel.ready[0].len != 0 || kernel.ready[1].len != 0 || kernel.claims.WaitingCount() != 0 || len(kernel.claims.keys) != 0 {
		return false
	}
	if !kernel.functionCatalogDrained() {
		return false
	}
	if longLived.Active != longLived.FinalizerOwnedActive || longLived.Bytes != longLived.FinalizerOwnedBytes {
		return false
	}
	return kernel.admission.RunFinalizerReady(kernel.run.Generation(), longLived.FinalizerOwnedRecords, longLived.FinalizerOwnedBytes)
}

func isNoopRunFinalizer(finalizer RunFinalizer) bool {
	_, ok := finalizer.(noopRunFinalizer)
	return ok
}

func (kernel *CommandKernel) completeRunFinalizer(completion lifecycle.TaskCompletion) {
	if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone || kernel.finalizerAction != 0 || kernel.finalizerDone || kernel.finalizerFailed {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer completion"))
		return
	}
	if completion.Err != nil {
		kernel.finalizerFailed = true
		kernel.run.Dirty(completion.Err)
	}
	kernel.finalizerAction = lifecycle.TaskActionTerminate
	if err := kernel.tasks.SendAction(lifecycle.TaskAction{Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate}); err != nil {
		kernel.run.Dirty(err)
	}
}

func (kernel *CommandKernel) acknowledgeRunFinalizer(ack lifecycle.TaskAcknowledgement) {
	if ack.Sequence != 2 || ack.Kind != lifecycle.TaskActionTerminate || kernel.finalizerAction != lifecycle.TaskActionTerminate || kernel.finalizerDone {
		kernel.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer acknowledgement"))
		return
	}
	if ack.Err != nil {
		kernel.finalizerFailed = true
		kernel.run.Dirty(ack.Err)
	}
	if err := kernel.tasks.Release(ack.Ref); err != nil {
		kernel.finalizerFailed = true
		kernel.run.Dirty(err)
		return
	}
	kernel.finalizerTask = lifecycle.TaskRef{}
	kernel.finalizerAction = 0
	if !kernel.finalizerFailed {
		kernel.finalizerDone = true
	}
}

func (kernel *CommandKernel) runFinalizerFailedTerminal() bool {
	return kernel.finalizerFailed && !kernel.finalizerRequest.Valid() && !kernel.finalizerTask.Valid() && kernel.finalizerAction == 0
}

func (kernel *CommandKernel) shutdownQuiescent() bool {
	if !kernel.shutdownBarrierDone || kernel.shutdownBarrierFailed ||
		kernel.shutdownBarrierRequest.Valid() ||
		kernel.shutdownBarrierTask.Valid() ||
		kernel.shutdownBarrierAction != 0 {
		return false
	}
	inherited := kernel.tasks.InheritedCensus()
	longLived := kernel.tasks.LongLivedCensus()
	frame := kernel.frames.Census()
	if len(kernel.operations) != 0 || len(kernel.tasksByRef) != 0 || len(kernel.tasksByRequest) != 0 ||
		len(kernel.functionCleanupTasks) != 0 || len(kernel.functionCleanupRequests) != 0 ||
		kernel.functionCleanupBacklog.count != 0 || kernel.functionMutationActive ||
		len(kernel.byAdmission) != 0 || len(kernel.lanes) != 0 ||
		kernel.tasks.Active() != 0 || kernel.tasks.Pending() != 0 || kernel.shutdownRequestCount != 0 || kernel.shutdownTaskCount != 0 || len(kernel.controls) != 0 || kernel.deadlines.Len() != 0 ||
		len(kernel.submissions[0]) != 0 || len(kernel.submissions[1]) != 0 || kernel.blockedSubmission[0] || kernel.blockedSubmission[1] ||
		kernel.ready[0].len != 0 || kernel.ready[1].len != 0 || kernel.claims.WaitingCount() != 0 || len(kernel.claims.keys) != 0 {
		return false
	}
	return kernel.functionCatalogDrained() &&
		kernel.admission.RunDrained(kernel.run.Generation()) && inherited.Active == 0 &&
		longLived == (lifecycle.LongLivedCensus{}) && !frame.Poisoned &&
		!frame.Busy && !frame.PendingControl && frame.RetainedBytes == 0 &&
		kernel.finalizerDone && !kernel.finalizerFailed
}

func (kernel *CommandKernel) functionCatalogDrained() bool {
	census := kernel.functionCatalog.LifecycleCensus()
	return census.Closed && !census.MutationActive && census.Routes == 0 &&
		census.CloseRoutesPending == 0 && census.InvocationLeases == 0 &&
		census.PendingCleanups == 0
}

func (kernel *CommandKernel) runCensus() lifecycle.RunCensus {
	return lifecycle.RunCensus{
		AdmissionRunDrained: kernel.admission.RunDrained(kernel.run.Generation()),
		Admission:           kernel.admission.Census(), TransientActive: kernel.tasks.Active(), TransientPending: kernel.tasks.Pending(),
		Inherited: kernel.tasks.InheritedCensus(), LongLived: kernel.tasks.LongLivedCensus(),
		Frame:                kernel.frames.Census(),
		RunFinalizerComplete: kernel.finalizerDone && !kernel.finalizerFailed,
	}
}

func (kernel *CommandKernel) releaseFunctionInvocation(ref FunctionInvocationRef) error {
	cleanup, err := kernel.functionCatalog.ReleaseInvocation(ref)
	if err != nil {
		return err
	}
	return kernel.functionCleanupBacklog.push(cleanup)
}

func (kernel *CommandKernel) pushFunctionCleanupBatch(cleanups *[MaximumFunctionCleanupBatch]FunctionCleanupPlan, count int) error {
	if cleanups == nil || count < 0 || count > len(cleanups) ||
		count > kernel.functionCleanupBacklog.free() {
		return errors.New("jobmgr kernel: invalid Function cleanup batch")
	}
	for index := 0; index < count; index++ {
		if err := kernel.functionCleanupBacklog.push(cleanups[index]); err != nil {
			return err
		}
		cleanups[index] = FunctionCleanupPlan{}
	}
	return nil
}

func (kernel *CommandKernel) serviceFunctionCleanupBacklog(quantum int) bool {
	if quantum <= 0 {
		return kernel.functionCleanupBacklog.count != 0
	}
	for quantum > 0 && kernel.functionCleanupBacklog.count != 0 {
		if kernel.tasks.Pending() >= lifecycle.MaximumAdmissionRecords {
			return true
		}
		cleanup := kernel.functionCleanupBacklog.front()
		request, err := kernel.tasks.Enqueue(lifecycle.TaskPlan{
			Source: lifecycle.SourceFunction, Work: cleanup.Work, Runner: cleanup.Runner,
		})
		if err != nil {
			kernel.run.Dirty(err)
			return true
		}
		if _, exists := kernel.functionCleanupRequests[request]; exists {
			kernel.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup request"))
			return true
		}
		kernel.functionCleanupRequests[request] = cleanup.Ref
		kernel.functionCleanupBacklog.pop()
		quantum--
	}
	return kernel.functionCleanupBacklog.count != 0
}

func (kernel *CommandKernel) beginFunctionMutation(submitted functionMutationSubmission) {
	if submitted.mutation == nil || submitted.result == nil || kernel.functionMutationActive ||
		kernel.functionCatalogClosing || !kernel.run.Admitting() {
		if submitted.result != nil {
			submitted.result <- functionMutationResult{err: errors.New("jobmgr kernel: Function mutation admission closed")}
		}
		return
	}
	if err := kernel.functionCatalog.BeginMutation(submitted.mutation); err != nil {
		submitted.result <- functionMutationResult{err: err}
		return
	}
	kernel.functionMutation = submitted
	kernel.functionMutationActive = true
}

func (kernel *CommandKernel) serviceFunctionMutation(quantum int) bool {
	if !kernel.functionMutationActive {
		return false
	}
	if kernel.functionCleanupBacklog.free() < MaximumFunctionMutationChanges {
		return true
	}
	var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
	progress, count, err := kernel.functionCatalog.AdvanceMutation(quantum, &cleanups)
	if err != nil {
		abortCount, abortErr := kernel.functionCatalog.AbortMutation(&cleanups)
		if pushErr := kernel.pushFunctionCleanupBatch(&cleanups, abortCount); pushErr != nil {
			abortErr = errors.Join(abortErr, pushErr)
		}
		kernel.functionMutation.result <- functionMutationResult{err: errors.Join(err, abortErr)}
		kernel.functionMutation = functionMutationSubmission{}
		kernel.functionMutationActive = false
		if abortErr != nil {
			kernel.run.Dirty(abortErr)
		}
		return false
	}
	if err := kernel.pushFunctionCleanupBatch(&cleanups, count); err != nil {
		kernel.functionMutation.result <- functionMutationResult{err: err}
		kernel.functionMutation = functionMutationSubmission{}
		kernel.functionMutationActive = false
		kernel.run.Dirty(err)
		return false
	}
	if !progress.Done {
		return true
	}
	kernel.functionMutation.result <- functionMutationResult{version: progress.Version}
	kernel.functionMutation = functionMutationSubmission{}
	kernel.functionMutationActive = false
	return false
}

func (kernel *CommandKernel) serviceFunctionCatalogClose(quantum int) bool {
	if !kernel.shutdownBarrierDone || kernel.shutdownBarrierFailed {
		return false
	}
	if !kernel.functionCatalogClosing {
		if err := kernel.functionCatalog.BeginClose(); err != nil {
			kernel.run.Dirty(err)
			return false
		}
		kernel.functionCatalogClosing = true
		kernel.functionCatalogCloseMore = true
	}
	if !kernel.functionCatalogCloseMore {
		return false
	}
	if kernel.functionCleanupBacklog.free() < MaximumFunctionCloseQuantum {
		return true
	}
	var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
	count, more, err := kernel.functionCatalog.CloseStep(quantum, &cleanups)
	if err != nil {
		kernel.run.Dirty(err)
		return false
	}
	if err := kernel.pushFunctionCleanupBatch(&cleanups, count); err != nil {
		kernel.run.Dirty(err)
		return false
	}
	kernel.functionCatalogCloseMore = more
	return more
}

func (kernel *CommandKernel) allocateLane(mapKey commandLaneKey, request Request) (*commandLane, error) {
	slot := kernel.freeLane
	if slot == 0 {
		return nil, errors.New("jobmgr kernel: lane capacity exhausted")
	}
	lane := &kernel.laneSlots[slot]
	kernel.freeLane = lane.freeNext
	generation := lane.generation + 1
	if generation == 0 {
		lane.freeNext = kernel.freeLane
		kernel.freeLane = slot
		return nil, errors.New("jobmgr kernel: lane generation wrapped")
	}
	*lane = commandLane{
		slot: slot, generation: generation, mapKey: mapKey,
		key: request.LaneKey, source: request.Source,
	}
	kernel.lanes[mapKey] = lane
	return lane, nil
}

func (kernel *CommandKernel) releaseUnusedLane(lane *commandLane) {
	if lane == nil || lane.owners != 0 || lane.head != nil || lane.tail != nil || lane.active != nil || lane.ready ||
		lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() ||
		lane.installPlanned || lane.stopPlanned ||
		lane.shutdownRequest.Valid() || lane.shutdownTask.Valid() || lane.shutdownAction != 0 {
		return
	}
	delete(kernel.lanes, lane.mapKey)
	slot := lane.slot
	generation := lane.generation
	*lane = commandLane{slot: slot, generation: generation, freeNext: kernel.freeLane}
	kernel.freeLane = slot
}

func operationAdmissionBytes(request Request, plan WorkPlan) (int64, error) {
	bytes := int64(512)
	if request.PayloadCapacity < 0 || request.PayloadCapacity > lifecycle.MaximumInputBodyBytes || request.PayloadCapacity > lifecycle.OrdinaryBudgetBytes-bytes {
		return 0, errors.New("jobmgr kernel: input body does not self-fit admission")
	}
	bytes += request.PayloadCapacity
	if plan.OwnedBytes > lifecycle.OrdinaryBudgetBytes-bytes {
		return 0, errors.New("jobmgr kernel: plan-owned bytes do not self-fit admission")
	}
	bytes += plan.OwnedBytes
	if plan.Resource != nil && plan.Resource.Action == ResourceInstall {
		persistent := plan.Resource.Permit.Bytes()
		if persistent <= 0 || persistent > lifecycle.OrdinaryBudgetBytes-bytes {
			return 0, errors.New("jobmgr kernel: long-lived resource does not self-fit admission")
		}
		bytes += persistent
	}
	if plan.Transaction != nil && plan.Transaction.AllocateSuccessor {
		persistent := plan.Transaction.Permit.Bytes()
		if persistent <= 0 ||
			persistent > lifecycle.OrdinaryBudgetBytes-bytes {
			return 0, errors.New(
				"jobmgr kernel: transaction successor does not self-fit admission",
			)
		}
		bytes += persistent
	}
	if plan.Capability != nil {
		persistent := plan.Capability.Permit.Bytes()
		if persistent <= 0 || persistent > lifecycle.OrdinaryBudgetBytes-bytes {
			return 0, errors.New("jobmgr kernel: long-lived capability does not self-fit admission")
		}
		bytes += persistent
	}
	fields := []string{
		request.UID,
		request.LaneKey,
		request.Route,
		request.ContentType,
		request.Permissions,
		request.CallerSource,
	}
	fields = append(fields, request.Args...)
	fields = append(fields, plan.Claims...)
	fields = append(fields, plan.ReadClaims...)
	for _, field := range fields {
		if int64(len(field)) > lifecycle.OrdinaryBudgetBytes-bytes {
			return 0, errors.New("jobmgr kernel: operation does not self-fit admission")
		}
		bytes += int64(len(field))
	}
	const requestArgumentAdmissionBytes = int64(16)
	arguments := int64(len(request.Args))
	if arguments > (lifecycle.OrdinaryBudgetBytes-bytes)/requestArgumentAdmissionBytes {
		return 0, errors.New("jobmgr kernel: request arguments do not self-fit admission")
	}
	bytes += arguments * requestArgumentAdmissionBytes
	const authorityClaimEdgeAdmissionBytes = int64(96)
	authorityClaimEdges := int64(len(plan.Claims) + len(plan.ReadClaims))
	if authorityClaimEdges > (lifecycle.OrdinaryBudgetBytes-bytes)/authorityClaimEdgeAdmissionBytes {
		return 0, errors.New("jobmgr kernel: claim edges do not self-fit admission")
	}
	bytes += authorityClaimEdges * authorityClaimEdgeAdmissionBytes
	return bytes, nil
}

func AdmissionFootprint(request Request, plan WorkPlan) (int64, error) {
	if err := request.Validate(); err != nil {
		return 0, err
	}
	if err := plan.validate(); err != nil {
		return 0, err
	}
	if plan.Resource != nil && plan.Resource.ID != request.LaneKey ||
		plan.Transaction != nil && plan.Transaction.ID != request.LaneKey ||
		plan.Capability != nil && plan.Capability.ID != request.LaneKey {
		return 0, errors.New("jobmgr kernel: admission footprint identity differs")
	}
	return operationAdmissionBytes(request, plan)
}

func (kernel *CommandKernel) abortRequestInputBody(request Request) error {
	if request.InputBodyToken == 0 {
		return nil
	}
	wake, err := kernel.admission.AbortInputBody(request.InputBodyToken)
	if wake {
		kernel.NotifyControlReady()
	}
	return err
}

func operationResultAdmissionBytes(base int64, result lifecycle.ResultPreflight) (int64, error) {
	if base <= 0 || result.PlanBytes < 0 || result.FrameBytes <= 0 {
		return 0, errors.New("jobmgr kernel: invalid result admission terms")
	}
	total := base
	for _, term := range []int64{result.PlanBytes, result.FrameBytes} {
		if term > lifecycle.OrdinaryBudgetBytes-total {
			return 0, fmt.Errorf("%w: result does not self-fit ordinary budget", lifecycle.ErrFunctionResultTooLarge)
		}
		total += term
	}
	return total, nil
}

func sourceIndex(source lifecycle.Source) int {
	if source == lifecycle.SourceJobManager {
		return 0
	}
	return 1
}

func sourceForIndex(index int) lifecycle.Source {
	if index == 0 {
		return lifecycle.SourceJobManager
	}
	return lifecycle.SourceFunction
}

func otherSource(source lifecycle.Source) lifecycle.Source {
	if source == lifecycle.SourceJobManager {
		return lifecycle.SourceFunction
	}
	return lifecycle.SourceJobManager
}
