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

const externalSourceQueueDepth = 32

const (
	maximumPlanClaims     = 1_024
	maximumClaimKeyBytes  = maximumRequestArgumentBytes
	maximumPlanClaimBytes = lifecycle.ControlFrameBytes
	// Keep lifecycle-event service capacity at least equal to the maximum
	// phase work introduced by one task-start quantum.
	asyncEventServiceQuantum = lifecycle.TaskStartServiceQuantum *
		lifecycle.TransactionTaskPhases
)

var ErrStopped = errors.New("jobmgr kernel: stopped")

type WorkPlan struct {
	Runner              lifecycle.TaskRunner
	Work                lifecycle.TaskWork
	Resource            *ResourcePlan
	Transaction         *ResourceTransactionPlan
	Capability          *CapabilityPlan
	Cleanup             lifecycle.TaskCleanup
	Claims              []string
	ReadClaims          []string
	OwnedBytes          int64
	NoResponse          bool
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
	PrepareComposite  CompositeResourceTransactionWork
}

type CapabilityPlan struct {
	ID      string
	Permit  lifecycle.LongLivedPlan
	Prepare func(context.Context, uint64, lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error)
}

func (wp WorkPlan) validate() error {
	if wp.OwnedBytes < 0 {
		return errors.New("jobmgr kernel: negative plan-owned bytes")
	}
	if len(wp.Claims) > maximumPlanClaims-len(wp.ReadClaims) {
		return errors.New("jobmgr kernel: too many plan claims")
	}
	claimBytes := 0
	for _, claims := range [][]string{wp.Claims, wp.ReadClaims} {
		for _, key := range claims {
			if key == "" || len(key) > maximumClaimKeyBytes ||
				len(key) > maximumPlanClaimBytes-claimBytes {
				return errors.New("jobmgr kernel: invalid or oversized claim key")
			}
			claimBytes += len(key)
		}
	}
	workKinds := 0
	if wp.Work != nil {
		workKinds++
	}
	if wp.Runner != nil {
		workKinds++
	}
	if wp.Resource != nil {
		workKinds++
	}
	if wp.Transaction != nil {
		workKinds++
	}
	if wp.Capability != nil {
		workKinds++
	}
	if workKinds != 1 {
		return errors.New("jobmgr kernel: plan must have exactly one work kind")
	}
	if wp.Work != nil || wp.Runner != nil {
		if wp.NoResponse {
			return errors.New("jobmgr kernel: frame work cannot suppress its response")
		}
		return nil
	}
	if wp.Transaction != nil {
		if wp.Cleanup != nil {
			return errors.New("jobmgr kernel: resource transaction cleanup must be sealed by apply")
		}
		if wp.Transaction.ID == "" ||
			(wp.Transaction.Prepare == nil) ==
				(wp.Transaction.PrepareComposite == nil) {
			return errors.New("jobmgr kernel: invalid resource transaction plan")
		}
		if wp.Transaction.AllocateSuccessor {
			if err := wp.Transaction.Permit.Validate(); err != nil {
				return errors.Join(
					errors.New("jobmgr kernel: transaction successor has no long-lived permit"),
					err,
				)
			}
		} else if wp.Transaction.Permit.Class() != 0 ||
			wp.Transaction.Permit.Bytes() != 0 {
			return errors.New("jobmgr kernel: transaction without successor has a permit")
		}
		return nil
	}
	if !wp.NoResponse {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	if wp.Cleanup != nil {
		return errors.New("jobmgr kernel: resource plan cannot add an unrelated task cleanup")
	}
	if wp.Capability != nil {
		if wp.Capability.ID == "" || wp.Capability.Prepare == nil {
			return errors.New("jobmgr kernel: invalid prepared capability plan")
		}
		if err := wp.Capability.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: capability plan has no long-lived permit"), err)
		}
		return nil
	}
	if wp.Resource.ID == "" {
		return errors.New("jobmgr kernel: invalid internal resource plan")
	}
	switch wp.Resource.Action {
	case ResourceInstall:
		if wp.Resource.Prepare == nil {
			return errors.New("jobmgr kernel: install resource plan has no factory")
		}
		if err := wp.Resource.Permit.Validate(); err != nil {
			return errors.Join(errors.New("jobmgr kernel: install resource plan has no long-lived permit"), err)
		}
	case ResourceStop:
		if wp.Resource.Prepare != nil || wp.Resource.Permit.Class() != 0 || wp.Resource.Permit.Bytes() != 0 {
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
	composite     *kernelCompositeScope
	rollback      bool
	controlStatus lifecycle.ControlStatus
	result        chan error
	terminal      chan error
}

type commandLaneKey struct {
	key                string
	functionInvocation FunctionInvocationRef
	source             lifecycle.Source
	resource           bool
}

type functionCleanupTask struct {
	ref FunctionCleanupRef
	err error
}

// Fixed chunks keep each KernelLoop queue operation worst-case O(1).
const functionCleanupChunkCapacity = 64

type functionCleanupChunk struct {
	plans [functionCleanupChunkCapacity]FunctionCleanupPlan
	head  int
	tail  int
	next  *functionCleanupChunk
}

type functionCleanupQueue struct {
	head  *functionCleanupChunk
	tail  *functionCleanupChunk
	count int
}

func (fcq *functionCleanupQueue) push(plan FunctionCleanupPlan) error {
	if err := plan.validate(); err != nil {
		return err
	}
	if !plan.Ref.Valid() {
		return nil
	}
	if fcq.tail == nil ||
		fcq.tail.tail == functionCleanupChunkCapacity {
		chunk := &functionCleanupChunk{}
		if fcq.tail == nil {
			fcq.head = chunk
		} else {
			fcq.tail.next = chunk
		}
		fcq.tail = chunk
	}
	fcq.tail.plans[fcq.tail.tail] = plan
	fcq.tail.tail++
	fcq.count++
	return nil
}

func (fcq *functionCleanupQueue) front() FunctionCleanupPlan {
	if fcq.count == 0 {
		return FunctionCleanupPlan{}
	}
	return fcq.head.plans[fcq.head.head]
}

func (fcq *functionCleanupQueue) pop() {
	if fcq.count == 0 {
		return
	}
	chunk := fcq.head
	chunk.plans[chunk.head] = FunctionCleanupPlan{}
	chunk.head++
	fcq.count--
	if chunk.head == chunk.tail {
		fcq.head = chunk.next
		chunk.next = nil
		if fcq.head == nil {
			fcq.tail = nil
		}
	}
}

type functionMutationSubmission struct {
	mutation FunctionCatalogMutation
	result   chan functionMutationResult
	action   functionMutationAction
}

type functionMutationResult struct {
	version uint64
	err     error
}

type functionMutationAction uint8

const (
	functionMutationQuiesce functionMutationAction = iota + 1
	functionMutationCommit
	functionMutationAbort
)

type preAdmissionControl struct {
	status lifecycle.ControlStatus
	cause  error
}

func (pac preAdmissionControl) Error() string {
	return pac.cause.Error()
}

func (pac preAdmissionControl) Unwrap() error {
	return pac.cause
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
	claimWaitStarted    time.Time
	claimWaitPrevious   *commandOperation
	claimWaitNext       *commandOperation
	claimWaitListed     bool
	lane                *commandLane
	previous            *commandOperation
	next                *commandOperation
	allPrevious         *commandOperation
	allNext             *commandOperation
	allListed           bool
	runtimeStarted      time.Time
	runtimePrevious     *commandOperation
	runtimeNext         *commandOperation
	runtimeListed       bool
	shutdownVisited     bool
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
	parent              *commandOperation
	composite           *kernelCompositeScope
	activeChild         *commandOperation
	fencePrevious       *commandOperation
	fenceNext           *commandOperation
	fenceBlocked        bool
	fenceChecked        uint64
	deferredCompletion  *lifecycle.TaskCompletion
	claimsInherited     bool
	compositeRollback   bool
}

type commandLane struct {
	slot               uint32
	generation         uint32
	mapKey             commandLaneKey
	owners             int
	freeNext           uint32
	key                string
	source             lifecycle.Source
	head               *commandOperation
	tail               *commandOperation
	active             *commandOperation
	continuationTail   *commandOperation
	ready              bool
	readyPrev          *commandLane
	readyNext          *commandLane
	allPrevious        *commandLane
	allNext            *commandLane
	allListed          bool
	shutdownVisited    bool
	resourceGeneration uint64
	resourceSource     lifecycle.Source
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

type readyQueue struct {
	head *commandLane
	tail *commandLane
	len  int
}

func (rq *readyQueue) push(lane *commandLane) {
	if lane.ready {
		return
	}
	lane.ready = true
	lane.readyPrev = rq.tail
	if rq.tail != nil {
		rq.tail.readyNext = lane
	} else {
		rq.head = lane
	}
	rq.tail = lane
	rq.len++
}

func (rq *readyQueue) pop() *commandLane {
	lane := rq.head
	if lane == nil {
		return nil
	}
	rq.head = lane.readyNext
	if rq.head != nil {
		rq.head.readyPrev = nil
	} else {
		rq.tail = nil
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	rq.len--
	return lane
}

func (rq *readyQueue) remove(lane *commandLane) {
	if !lane.ready {
		return
	}
	if lane.readyPrev != nil {
		lane.readyPrev.readyNext = lane.readyNext
	} else {
		rq.head = lane.readyNext
	}
	if lane.readyNext != nil {
		lane.readyNext.readyPrev = lane.readyPrev
	} else {
		rq.tail = lane.readyPrev
	}
	lane.ready = false
	lane.readyPrev = nil
	lane.readyNext = nil
	rq.len--
}

type deadlineEntry struct {
	when      time.Time
	operation *commandOperation
	index     int
}

type deadlineHeap []*deadlineEntry

func (dh *deadlineHeap) Len() int           { return len(*dh) }
func (dh *deadlineHeap) Less(i, j int) bool { return (*dh)[i].when.Before((*dh)[j].when) }
func (dh *deadlineHeap) Swap(i, j int) {
	(*dh)[i], (*dh)[j] = (*dh)[j], (*dh)[i]
	(*dh)[i].index = i
	(*dh)[j].index = j
}
func (dh *deadlineHeap) Push(value any) {
	entry := value.(*deadlineEntry)
	entry.index = len(*dh)
	*dh = append(*dh, entry)
}
func (dh *deadlineHeap) Pop() any {
	old := *dh
	last := old[len(old)-1]
	old[len(old)-1] = nil
	last.index = -1
	*dh = old[:len(old)-1]
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
	functionMutationPaused   bool
	functionCatalogClosing   bool
	functionCatalogCloseMore bool
	shutdownRequests         map[lifecycle.TaskRequestRef]*commandLane
	shutdownTasks            map[lifecycle.TaskRef]*commandLane
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
	laneSlots                []*commandLane
	freeLane                 uint32
	ready                    [2]readyQueue
	nextID                   lifecycle.OperationID
	nextResourceGeneration   uint64
	nextSource               lifecycle.Source
	nextExternalSource       lifecycle.Source
	nextAsyncEvent           uint8
	deadlines                deadlineHeap
	controls                 []*commandOperation
	operationHead            *commandOperation
	operationTail            *commandOperation
	compositeFenceClaims     map[string]compositeFenceClaimUse
	compositeFenceHead       *commandOperation
	compositeFenceTail       *commandOperation
	compositeFenceCount      int
	compositeFenceGeneration uint64
	compositeFenceRecheck    bool
	laneHead                 *commandLane
	laneTail                 *commandLane
	shutdownActive           bool
	shutdownOperationCursor  *commandOperation
	shutdownOperationsDone   bool
	shutdownLaneCursor       *commandLane
	shutdownLanesDone        bool
	jobPlanner               Planner
	functionCatalog          FunctionCatalogPort
	inputBodyGrants          chan<- lifecycle.AdmissionGrant
	admissionServiceGate     <-chan struct{}
	runtimeObserver          lifecycle.RuntimeObserver
	runtimeHead              *commandOperation
	runtimeTail              *commandOperation
	functionOperations       int
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
		shutdownRequests:        make(map[lifecycle.TaskRequestRef]*commandLane),
		shutdownTasks:           make(map[lifecycle.TaskRef]*commandLane),
		functionMutations:       make(chan functionMutationSubmission),
		byAdmission:             make(map[lifecycle.AdmissionRef]*commandOperation), lanes: make(map[commandLaneKey]*commandLane),
		compositeFenceClaims: make(map[string]compositeFenceClaimUse),
		nextSource:           lifecycle.SourceJobManager, nextExternalSource: lifecycle.SourceJobManager,
		inputBodyGrants:      inputBodyGrants,
		admissionServiceGate: admissionServiceGate,
		shutdownBarrier:      shutdownBarrier,
		shutdownBarrierDone:  isNoopRunShutdownBarrier(shutdownBarrier),
		finalizer:            finalizer,
		finalizerDone:        isNoopRunFinalizer(finalizer),
		jobPlanner:           jobPlanner,
		functionCatalog:      functionCatalog,
		laneSlots:            []*commandLane{nil},
	}
	for index := range kernel.submissions {
		kernel.submissions[index] = make(chan submission, externalSourceQueueDepth)
		kernel.submissionSpace[index] = make(chan struct{}, 1)
	}
	if err := tasks.BindRun(run, kernel.NotifyControlReady); err != nil {
		return nil, err
	}
	heap.Init(&kernel.deadlines)
	return kernel, nil
}

func (ck *CommandKernel) BindRuntimeObserver(
	observer lifecycle.RuntimeObserver,
) error {
	if ck == nil || observer == nil {
		return errors.New("jobmgr kernel: invalid runtime observer")
	}
	if ck.runtimeObserver != nil {
		return errors.New("jobmgr kernel: runtime observer already bound")
	}
	select {
	case <-ck.done:
		return errors.New("jobmgr kernel: runtime observer bound after terminal")
	default:
	}
	if err := ck.run.BindRuntimeObserver(observer); err != nil {
		return err
	}
	if err := ck.tasks.BindRuntimeObserver(observer); err != nil {
		return err
	}
	if err := ck.claims.bindRuntimeObserver(
		observer,
		ck.clock.Now,
	); err != nil {
		return err
	}
	ck.runtimeObserver = observer
	ck.observeRuntimeOperations()
	return nil
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

func (kl *KernelLoop) Start(ctx context.Context) error {
	if kl == nil || ctx == nil {
		return errors.New("jobmgr kernel loop: invalid start")
	}
	started := false
	kl.kernel.startOnce.Do(func() {
		started = true
		kl.kernel.startErr = kl.kernel.bindRunNotifications()
		if kl.kernel.startErr != nil {
			return
		}
		go kl.kernel.runLoop(ctx)
	})
	if !started {
		return errors.New("jobmgr kernel loop: already started")
	}
	return kl.kernel.startErr
}

func (ck *CommandKernel) bindRunNotifications() error {
	if err := ck.frames.BindRunNotifications(
		ck.run.Generation(),
		ck.NotifyControlReady,
		func(err error) {
			ck.run.Dirty(err)
			ck.NotifyControlReady()
		},
		ck.runtimeObserver,
	); err != nil {
		return err
	}
	if err := ck.tasks.BindAdmissionReady(ck.NotifyControlReady); err != nil {
		return errors.Join(
			err,
			ck.frames.ReleaseRunNotifications(ck.run.Generation()),
		)
	}
	return nil
}

func (ck *CommandKernel) Submit(ctx context.Context, request Request) error {
	return ck.submit(ctx, request, nil)
}

func (ck *CommandKernel) SubmitAndWait(ctx context.Context, request Request) error {
	terminal := make(chan error, 1)
	if err := ck.submit(ctx, request, terminal); err != nil {
		return err
	}
	select {
	case err := <-terminal:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-ck.done:
		return ck.Wait(context.Background())
	}
}

func (ck *CommandKernel) QuiesceFunctions(
	ctx context.Context,
	mutation FunctionCatalogMutation,
) error {
	_, err := ck.submitFunctionMutation(
		ctx,
		functionMutationQuiesce,
		mutation,
	)
	return err
}

func (ck *CommandKernel) CommitFunctions(
	ctx context.Context,
	mutation FunctionCatalogMutation,
) (uint64, error) {
	return ck.submitFunctionMutation(ctx, functionMutationCommit, mutation)
}

func (ck *CommandKernel) AbortFunctions(
	ctx context.Context,
	mutation FunctionCatalogMutation,
) error {
	_, err := ck.submitFunctionMutation(ctx, functionMutationAbort, mutation)
	return err
}

func (ck *CommandKernel) submitFunctionMutation(
	ctx context.Context,
	action functionMutationAction,
	mutation FunctionCatalogMutation,
) (uint64, error) {
	if ctx == nil || mutation == nil ||
		action < functionMutationQuiesce || action > functionMutationAbort {
		return 0, errors.New("jobmgr kernel: invalid Function mutation")
	}
	if ck.run.IsStopping() {
		return 0, ck.run.StoppingCause()
	}
	result := make(chan functionMutationResult, 1)
	submission := functionMutationSubmission{
		mutation: mutation,
		result:   result,
		action:   action,
	}
	select {
	case ck.functionMutations <- submission:
		ck.NotifyControlReady()
	case <-ck.submissionStopped:
		return 0, ck.stoppingError()
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-ck.done:
		return 0, ck.stoppingError()
	}
	select {
	case completed := <-result:
		return completed.version, completed.err
	case <-ck.done:
		select {
		case completed := <-result:
			return completed.version, completed.err
		default:
			return 0, errors.Join(
				ck.stoppingError(),
				ck.doneErr,
			)
		}
	}
}

func (ck *CommandKernel) SubmitPrepared(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	return ck.submitPrepared(ctx, request, plan, nil)
}

func (ck *CommandKernel) SubmitPreparedAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	terminal := make(chan error, 1)
	if err := ck.submitPrepared(ctx, request, plan, terminal); err != nil {
		return err
	}
	var cancellation error
	done := ctx.Done()
	for {
		select {
		case err := <-terminal:
			return errors.Join(cancellation, err)
		case <-done:
			cancellation = context.Cause(ctx)
			done = nil
			select {
			case ck.cancel <- request.UID:
			case err := <-terminal:
				return errors.Join(cancellation, err)
			case <-ck.done:
				return errors.Join(
					cancellation,
					ck.Wait(context.Background()),
				)
			}
		case <-ck.done:
			return errors.Join(
				cancellation,
				ck.Wait(context.Background()),
			)
		}
	}
}

func (ck *CommandKernel) submit(
	ctx context.Context,
	request Request,
	terminal chan error,
) error {
	return ck.submitWithPlan(ctx, request, WorkPlan{}, false, terminal)
}

func (ck *CommandKernel) submitPrepared(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	terminal chan error,
) error {
	return ck.submitWithPlan(ctx, request, plan, true, terminal)
}

func (ck *CommandKernel) submitWithPlan(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	prepared bool,
	terminal chan error,
) error {
	if ctx == nil {
		return errors.Join(errors.New("jobmgr kernel: nil submission context"), ck.abortRequestInputBody(request))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, ck.abortRequestInputBody(request))
	}
	if err := request.Validate(); err != nil {
		return errors.Join(err, ck.abortRequestInputBody(request))
	}
	if prepared && request.Source != lifecycle.SourceJobManager {
		return errors.Join(
			errors.New("jobmgr kernel: only Job Manager commands accept prepared plans"),
			ck.abortRequestInputBody(request),
		)
	}
	request.Args = append([]string(nil), request.Args...)
	if prepared {
		var err error
		plan, err = prepareOwnedJobPlan(request, plan)
		if err != nil {
			return errors.Join(err, ck.abortRequestInputBody(request))
		}
	} else if request.Source == lifecycle.SourceJobManager {
		var err error
		plan, err = ck.prepareJobPlan(request)
		if err != nil {
			return errors.Join(err, ck.abortRequestInputBody(request))
		}
	}
	result := make(chan error, 1)
	if err := ck.enqueueSubmission(ctx, request.Source, submission{
		request:  request,
		plan:     plan,
		context:  ctx,
		result:   result,
		terminal: terminal,
	}); err != nil {
		return errors.Join(err, ck.abortRequestInputBody(request))
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		select {
		case ck.cancel <- request.UID:
		case err := <-result:
			return err
		case <-ck.done:
			return ck.stoppingError()
		}
		select {
		case err := <-result:
			return err
		case <-ck.done:
			return ck.stoppingError()
		}
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) enqueueSubmission(ctx context.Context, source lifecycle.Source, submitted submission) error {
	index := sourceIndex(source)
	for {
		ck.submissionMu.Lock()
		if ck.submissionClosed {
			ck.submissionMu.Unlock()
			return ck.stoppingError()
		}
		select {
		case ck.submissions[index] <- submitted:
			ck.submissionMu.Unlock()
			ck.NotifyControlReady()
			return nil
		default:
			ck.submissionMu.Unlock()
		}
		select {
		case <-ck.submissionSpace[index]:
		case <-ck.submissionStopped:
			return ck.stoppingError()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (ck *CommandKernel) closeSubmissionIngress() {
	ck.submissionMu.Lock()
	if !ck.submissionClosed {
		ck.submissionClosed = true
		close(ck.submissionStopped)
	}
	ck.submissionMu.Unlock()
}

func (ck *CommandKernel) notifySubmissionSpace(source int) {
	select {
	case ck.submissionSpace[source] <- struct{}{}:
	default:
	}
}

func (ck *CommandKernel) Reject(ctx context.Context, uid string, status lifecycle.ControlStatus) error {
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
	if err := ck.enqueueSubmission(ctx, lifecycle.SourceFunction, submission{
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
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) Cancel(ctx context.Context, uid string) error {
	if ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	select {
	case ck.cancel <- uid:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-ck.done:
		return ck.stoppingError()
	}
}

func (ck *CommandKernel) NotifyControlReady() {
	select {
	case ck.wake <- struct{}{}:
	default:
	}
}

func (ck *CommandKernel) Stop() {
	ck.stopOnce.Do(func() {
		ck.run.BeginStopping()
		ck.closeSubmissionIngress()
		close(ck.stop)
	})
}

func (ck *CommandKernel) stoppingError() error {
	if ck != nil && ck.run != nil && ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	return ErrStopped
}

func (ck *CommandKernel) Done() <-chan struct{} {
	return ck.done
}

func (ck *CommandKernel) Wait(ctx context.Context) error {
	select {
	case <-ck.done:
		return ck.doneErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ck *CommandKernel) WaitShutdownStarted(ctx context.Context) error {
	select {
	case <-ck.shutdownStarted:
		return nil
	default:
	}
	select {
	case <-ck.shutdownStarted:
		return nil
	case <-ck.done:
		select {
		case <-ck.shutdownStarted:
			return nil
		default:
			return ck.stoppingError()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ck *CommandKernel) runLoop(ctx context.Context) {
	var terminal error
	var shutdownBudget *lifecycle.ShutdownBudget
	var shutdownC <-chan struct{}
	var deadlineTimer lifecycle.ReusableTimer
	if clock, ok := ck.clock.(lifecycle.ReusableTimerClock); ok {
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
		deadline := ck.nextDeadline()
		if deadline.IsZero() {
			stopDeadline()
			return
		}
		if deadlineC != nil && deadline.Equal(armedDeadline) {
			return
		}
		stopDeadline()
		delay := max(deadline.Sub(ck.clock.Now()), 0)
		if deadlineTimer != nil {
			deadlineC = deadlineTimer.Arm(delay)
			cancelDeadline = deadlineTimer.Stop
		} else {
			deadlineC, cancelDeadline = ck.clock.Arm(lifecycle.TimerKindDeadline, delay)
		}
		armedDeadline = deadline
	}
	stopC := (<-chan struct{})(ck.stop)
	contextC := ctx.Done()
	shuttingDown := false
	beginShutdown := func(cause error) {
		if shuttingDown {
			terminal = errors.Join(terminal, cause)
			return
		}
		ck.run.BeginStopping()
		ck.closeSubmissionIngress()
		shuttingDown = true
		stopDeadline()
		terminal = errors.Join(terminal, cause)
		budget, err := ck.run.BeginShutdown()
		if err != nil {
			ck.run.Dirty(err)
			terminal = errors.Join(terminal, err)
			return
		}
		shutdownBudget = budget
		if err := ck.beginShutdown(budget.Deadline()); err != nil {
			ck.run.Dirty(err)
		}
		ck.shutdownStartOnce.Do(func() { close(ck.shutdownStarted) })
		shutdownC = budget.Context().Done()
		stopC = nil
		contextC = nil
	}
	defer func() {
		stopDeadline()
		if err := ck.frames.ReleaseRunNotifications(
			ck.run.Generation(),
		); err != nil {
			terminal = errors.Join(terminal, err)
		}
		ck.doneErr = terminal
		close(ck.done)
	}()
	for {
		if !shuttingDown {
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), ck.run.Terminal(ck.runCensus()))
				return
			}
		}
		moreDeadlines := false
		if !shuttingDown {
			if deadline := ck.nextDeadline(); !deadline.IsZero() && !deadline.After(ck.clock.Now()) {
				moreDeadlines = ck.serviceDeadlines(ck.clock.Now(), 4)
			}
		}
		moreControls := ck.serviceControls(4)
		moreSubmissions := ck.serviceSubmissions(4)
		moreFunctionCleanups := ck.serviceFunctionCleanupBacklog(4)
		moreFunctionMutation := false
		moreFunctionClose := false
		if !shuttingDown {
			moreFunctionMutation = ck.serviceFunctionMutation(16)
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		} else if ck.shutdownBarrierDone {
			moreFunctionClose = ck.serviceFunctionCatalogClose(MaximumFunctionCloseQuantum)
		}
		moreFunctionCleanups = moreFunctionCleanups || ck.functionCleanupBacklog.count != 0
		moreClaimSettlements := ck.serviceClaimSettlements(
			maximumClaimSettlementQuantum,
		)
		moreAdmissions := false
		moreCompositeFenceRechecks := false
		moreTasks := false
		moreTaskStarts := false
		moreShutdownOperations := false
		moreInheritedCancellations := false
		moreShutdownLanes := false
		if !shuttingDown {
			moreCompositeFenceRechecks =
				ck.serviceCompositeFenceBlocked(4)
			moreAdmissions = ck.serviceAdmissions(4)
			moreTasks = ck.scheduleTasks(4)
		}
		servicedAsyncEvents := ck.serviceAsyncEvents(
			asyncEventServiceQuantum,
		)
		if !shuttingDown {
			moreTaskStarts = ck.serviceTaskStarts(
				lifecycle.TaskStartServiceQuantum,
			)
			if cause := ck.run.DirtyCause(); cause != nil {
				beginShutdown(cause)
			}
		} else {
			if err := ck.advanceShutdownAdmission(); err != nil {
				ck.run.Dirty(err)
			}
			var shutdownErr error
			moreShutdownOperations, shutdownErr =
				ck.serviceShutdownOperations(4)
			if shutdownErr != nil {
				ck.run.Dirty(shutdownErr)
			}
			_, moreInheritedCancellations, shutdownErr =
				ck.tasks.CancelInheritedBatch(
					lifecycle.InheritedCancellationServiceQuantum,
				)
			if shutdownErr != nil {
				ck.run.Dirty(shutdownErr)
			}
			if err := ck.advanceShutdownBarrier(); err != nil {
				ck.run.Dirty(err)
			}
			if ck.shutdownBarrierDone {
				moreShutdownLanes, shutdownErr =
					ck.serviceShutdownStops(4)
				if shutdownErr != nil {
					ck.run.Dirty(shutdownErr)
				}
			}
			if err := ck.advanceRunFinalizer(); err != nil {
				ck.run.Dirty(err)
			}
			if ck.shutdownOperationsDone {
				moreTaskStarts = ck.serviceTaskStarts(
					lifecycle.TaskStartServiceQuantum,
				)
			}
		}
		if shuttingDown {
			if shutdownBudget.ExpireIfDue() {
				terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), ck.run.Terminal(ck.runCensus()))
				return
			}
			if ck.shutdownQuiescent() ||
				ck.runShutdownBarrierFailedTerminal() ||
				ck.runFinalizerFailedTerminal() {
				terminal = errors.Join(terminal, ck.run.Terminal(ck.runCensus()))
				return
			}
		}
		if moreDeadlines || moreControls || moreSubmissions || moreFunctionCleanups ||
			moreFunctionMutation || moreFunctionClose || moreClaimSettlements ||
			moreCompositeFenceRechecks || moreAdmissions ||
			moreTasks || moreTaskStarts ||
			servicedAsyncEvents > 0 || moreShutdownOperations ||
			moreInheritedCancellations || moreShutdownLanes ||
			ck.claims.pendingSettlements() ||
			ck.hasRunnableSubmissions() {
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
					terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), ck.run.Terminal(ck.runCensus()))
					return
				}
			}
			continue
		}
		if !shuttingDown {
			armDeadline()
		}
		functionMutationC := (<-chan functionMutationSubmission)(ck.functionMutations)
		if shuttingDown || ck.functionMutationActive {
			functionMutationC = nil
		}
		select {
		case uid := <-ck.cancel:
			ck.cancelOperation(uid)
		case submitted := <-functionMutationC:
			ck.beginFunctionMutation(submitted)
		case completion := <-ck.tasks.CompletionCh():
			ck.completeTask(completion)
		case acknowledgement := <-ck.tasks.AcknowledgementCh():
			ck.acknowledgeTask(acknowledgement)
		case <-deadlineC:
			deadlineC = nil
			cancelDeadline = nil
			armedDeadline = time.Time{}
			ck.serviceDeadlines(ck.clock.Now(), 4)
		case <-ck.wake:
		case <-stopC:
			beginShutdown(nil)
		case <-contextC:
			beginShutdown(ctx.Err())
		case <-shutdownC:
			terminal = errors.Join(terminal, errors.New("jobmgr kernel: shutdown deadline exceeded"), ck.run.Terminal(ck.runCensus()))
			return
		}
	}
}

func (ck *CommandKernel) serviceClaimSettlements(quantum int) bool {
	granted, more, err := ck.claims.serviceSettlements(quantum)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	for _, operation := range granted {
		ck.markReady(operation.lane)
	}
	return more
}

func (ck *CommandKernel) serviceOneAsyncEvent() bool {
	const sources = 4
	for offset := range sources {
		source := (int(ck.nextAsyncEvent) + offset) % sources
		switch source {
		case 0:
			select {
			case uid := <-ck.cancel:
				ck.cancelOperation(uid)
				ck.nextAsyncEvent = 1
				return true
			default:
			}
		case 1:
			select {
			case completion := <-ck.tasks.CompletionCh():
				ck.completeTask(completion)
				ck.nextAsyncEvent = 2
				return true
			default:
			}
		case 2:
			select {
			case acknowledgement := <-ck.tasks.AcknowledgementCh():
				ck.acknowledgeTask(acknowledgement)
				ck.nextAsyncEvent = 3
				return true
			default:
			}
		case 3:
			if ck.functionMutationActive || ck.functionCatalogClosing {
				continue
			}
			select {
			case submitted := <-ck.functionMutations:
				ck.beginFunctionMutation(submitted)
				ck.nextAsyncEvent = 0
				return true
			default:
			}
		}
	}
	return false
}

func (ck *CommandKernel) serviceAsyncEvents(
	quantum int,
) int {
	count := 0
	for count < quantum && ck.serviceOneAsyncEvent() {
		count++
	}
	return count
}

func (ck *CommandKernel) serviceSubmissions(quantum int) bool {
	for quantum > 0 {
		first := sourceIndex(ck.nextExternalSource)
		second := 1 - first
		var submitted submission
		selected := -1
		wasBlocked := false
		dequeued := false
		if ck.blockedSubmission[first] {
			submitted = ck.blockedSubmissions[first]
			selected = first
			wasBlocked = true
		} else {
			select {
			case submitted = <-ck.submissions[first]:
				selected = first
				dequeued = true
			default:
			}
		}
		if selected < 0 {
			if ck.blockedSubmission[second] {
				submitted = ck.blockedSubmissions[second]
				selected = second
				wasBlocked = true
			} else {
				select {
				case submitted = <-ck.submissions[second]:
					selected = second
					dequeued = true
				default:
					return ck.hasRunnableSubmissions()
				}
			}
		}
		if dequeued {
			ck.notifySubmissionSpace(selected)
		}
		var err error
		if submitted.controlStatus != 0 {
			err = ck.frames.TryCommitControl(lifecycle.ControlFramePlan{
				UID: submitted.request.UID, Status: submitted.controlStatus, Expiry: lifecycle.ExpiryAt(ck.clock.Now()),
			})
			if err != nil && !errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
				ck.run.Dirty(err)
			}
		} else {
			if submitted.context != nil && submitted.context.Err() != nil {
				err = errors.Join(context.Cause(submitted.context), ck.abortRequestInputBody(submitted.request))
			} else {
				err = ck.admitSubmission(
					submitted.request,
					submitted.plan,
					submitted.context,
					submitted.result,
					submitted.terminal,
					submitted.composite,
					submitted.rollback,
				)
			}
		}
		var control preAdmissionControl
		if errors.As(err, &control) {
			submitted.controlStatus = control.status
			err = ck.frames.TryCommitControl(lifecycle.ControlFramePlan{
				UID:    submitted.request.UID,
				Status: control.status,
				Expiry: lifecycle.ExpiryAt(ck.clock.Now()),
			})
			if err != nil && !errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
				ck.run.Dirty(err)
			}
		}
		if errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			if !wasBlocked {
				ck.blockedSubmissions[selected] = submitted
				ck.blockedSubmission[selected] = true
			}
		} else {
			if wasBlocked {
				ck.blockedSubmissions[selected] = submission{}
				ck.blockedSubmission[selected] = false
			}
			if submitted.controlStatus != 0 || err != nil {
				if ck.runtimeObserver != nil {
					ck.runtimeObserver.AddRuntimeCounter(
						lifecycle.RuntimeCounterOperationsRejected,
						1,
					)
				}
				submitted.result <- err
				if submitted.controlStatus != 0 && submitted.terminal != nil {
					submitted.terminal <- err
				}
			}
		}
		ck.nextExternalSource = otherSource(sourceForIndex(selected))
		quantum--
	}
	return ck.hasRunnableSubmissions()
}

func (ck *CommandKernel) hasRunnableSubmissions() bool {
	for source := range ck.submissions {
		if !ck.blockedSubmission[source] && len(ck.submissions[source]) != 0 {
			return true
		}
	}
	return false
}

func (ck *CommandKernel) prepareJobPlan(request Request) (WorkPlan, error) {
	plan, err := ck.jobPlanner.Plan(request)
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

func (ck *CommandKernel) admit(
	request Request,
	plan WorkPlan,
	submissionContext context.Context,
	submissionResult,
	terminalResult chan error,
) error {
	return ck.admitSubmission(
		request,
		plan,
		submissionContext,
		submissionResult,
		terminalResult,
		nil,
		false,
	)
}

func (ck *CommandKernel) admitSubmission(
	request Request,
	plan WorkPlan,
	submissionContext context.Context,
	submissionResult,
	terminalResult chan error,
	composite *kernelCompositeScope,
	rollback bool,
) error {
	if !ck.run.Admitting() {
		return ck.rejectClosedAdmission(request)
	}
	var parent *commandOperation
	if composite != nil {
		var err error
		parent, err = ck.validateCompositeAdmission(
			composite,
			plan,
			rollback,
		)
		if err != nil {
			return errors.Join(
				err,
				ck.abortRequestInputBody(request),
			)
		}
		if request.InputBodyToken != 0 {
			return errors.Join(
				errors.New(
					"jobmgr composite: child cannot own parser input",
				),
				ck.abortRequestInputBody(request),
			)
		}
	} else if rollback {
		return errors.Join(
			errors.New("jobmgr kernel: rollback child has no parent"),
			ck.abortRequestInputBody(request),
		)
	}
	now := ck.clock.Now()
	if err := ck.uids.Admit(request.UID, now); err != nil {
		if ck.runtimeObserver != nil {
			ck.runtimeObserver.AddRuntimeCounter(
				lifecycle.RuntimeCounterDuplicateUIDRejected,
				1,
			)
		}
		return errors.Join(err, ck.abortRequestInputBody(request))
	}
	var functionInvocation FunctionInvocationRef
	var functionResourceID string
	var laneID commandLaneKey
	if request.Source == lifecycle.SourceFunction {
		decision, err := ck.functionCatalog.ResolveAndAcquire(FunctionLookup{
			UID: request.UID, Route: request.Route, Args: request.Args,
			Payload: request.Payload, ContentType: request.ContentType,
			Permissions: request.Permissions, CallerSource: request.CallerSource,
			Timeout: request.Timeout, HasPayload: request.HasPayload,
		})
		if err != nil {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
		if err := decision.validate(); err != nil {
			if decision.Lease.Valid() {
				err = errors.Join(err, ck.releaseFunctionInvocation(decision.Lease))
			}
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
		if decision.Rejected != 0 {
			if err := errors.Join(
				ck.uids.Complete(request.UID, false, now),
				ck.abortRequestInputBody(request),
			); err != nil {
				return err
			}
			return preAdmissionControl{
				status: decision.Rejected,
				cause: fmt.Errorf(
					"jobmgr kernel: Function catalog rejected route %q",
					request.Route,
				),
			}
		}
		plan = decision.Plan
		plan.Claims = append([]string(nil), plan.Claims...)
		plan.ReadClaims = append([]string(nil), plan.ReadClaims...)
		functionInvocation = decision.Lease
		functionResourceID = decision.ResourceID
		request.LaneKey = request.Route
		if functionResourceID != "" {
			request.LaneKey = functionResourceID
		}
	}
	releaseFunctionInvocation := functionInvocation.Valid()
	defer func() {
		if releaseFunctionInvocation {
			if err := ck.releaseFunctionInvocation(functionInvocation); err != nil {
				ck.run.Dirty(err)
			}
		}
	}()
	claims, err := normalizeAuthorityClaimModes(plan.Claims, plan.ReadClaims)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	if request.Source == lifecycle.SourceJobManager {
		laneID = commandLaneKey{source: request.Source, key: request.LaneKey}
	} else if functionResourceID != "" {
		laneID = resourceCommandLaneKey(functionResourceID)
	} else {
		laneID = commandLaneKey{
			source:             request.Source,
			functionInvocation: functionInvocation,
		}
	}
	if plan.Resource != nil || plan.Transaction != nil {
		laneID = resourceCommandLaneKey(request.LaneKey)
	}
	if parent != nil &&
		parent.lane != nil &&
		parent.lane.mapKey == laneID {
		return errors.Join(
			errors.New(
				"jobmgr composite: child cannot use its active parent lane",
			),
			ck.uids.Complete(request.UID, false, now),
			ck.abortRequestInputBody(request),
		)
	}
	lane := ck.lanes[laneID]
	if resource := plan.Resource; resource != nil {
		if lane != nil && lane.transactionPlanned != 0 {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return errors.New(
				"jobmgr kernel: internal resource plan overlaps a public resource transaction",
			)
		}
		switch resource.Action {
		case ResourceInstall:
			if lane != nil && (lane.installPlanned || ((lane.current != nil || lane.currentIdentity.Valid()) && !lane.stopPlanned && !lane.currentStopping && !lane.retiringIdentity.Valid())) {
				_ = ck.uids.Complete(request.UID, false, now)
				_ = ck.abortRequestInputBody(request)
				return errors.New("jobmgr kernel: install is not sequenced after an exact stop")
			}
		case ResourceStop:
			if lane == nil || lane.stopPlanned || lane.currentStopping || lane.retiringIdentity.Valid() ||
				(lane.current == nil && !lane.currentIdentity.Valid() && !lane.installPlanned) {
				err := errors.Join(ck.uids.Complete(request.UID, false, now), ck.abortRequestInputBody(request))
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
		_ = ck.uids.Complete(request.UID, false, now)
		_ = ck.abortRequestInputBody(request)
		return errors.New(
			"jobmgr kernel: resource transaction overlaps internal resource authority",
		)
	}
	if lane == nil {
		lane, err = ck.allocateLane(laneID, request)
		if err != nil {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
	}
	ck.nextID++
	operationGeneration, err := lifecycle.NewOperation(ck.nextID, request.UID, request.Source, request.LaneKey, !plan.NoResponse)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		ck.releaseUnusedLane(lane)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	charge, err := operationAdmissionBytes(request, plan)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		ck.releaseUnusedLane(lane)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	admissionLane := lifecycle.AdmissionLaneRef{Slot: lane.slot, Generation: lane.generation}
	requested := lifecycle.AdmissionRequestResult{}
	if request.InputBodyToken != 0 {
		requested = ck.admission.TransferInputBody(ck.run.Generation(), request.InputBodyToken, admissionLane, charge, request.PayloadCapacity)
	} else if parent != nil {
		if err := ck.beginCompositeFence(parent); err != nil {
			ck.releaseUnusedLane(lane)
			return errors.Join(
				err,
				ck.uids.Complete(request.UID, false, now),
				ck.abortRequestInputBody(request),
			)
		}
		requested = ck.admission.GrantCompositeProgress(
			ck.run.Generation(),
			parent.admission,
			admissionLane,
			charge,
		)
	} else {
		requested = ck.admission.RequestOrdinary(ck.run.Generation(), admissionLane, charge)
	}
	if requested.Rejected != nil {
		ck.releaseUnusedLane(lane)
		return errors.Join(
			requested.Rejected,
			ck.uids.Complete(request.UID, false, now),
			ck.abortRequestInputBody(request),
		)
	}
	request.InputBodyToken = 0
	operation := &commandOperation{
		OperationGeneration: operationGeneration, request: request, plan: plan, claims: claims,
		functionInvocation: functionInvocation,
		admission:          requested.Ref, admissionBase: charge, deadline: deadlineEntry{index: -1},
		submissionContext: submissionContext, submissionResult: submissionResult, terminalResult: terminalResult,
		parent: parent, claimsInherited: parent != nil,
		compositeRollback: rollback,
		runtimeStarted:    now,
	}
	if parent == nil {
		prepareClaimEdges(operation, claims)
		if err := ck.claims.register(operation); err != nil {
			_ = ck.admission.CancelWaiting(requested.Ref)
			_ = ck.uids.Complete(request.UID, false, now)
			ck.releaseUnusedLane(lane)
			return err
		}
	}
	operation.lane = lane
	lane.owners++
	if parent != nil {
		ck.insertCompositeOperation(lane, operation)
		parent.activeChild = operation
	} else if lane.tail != nil {
		lane.tail.next = operation
		operation.previous = lane.tail
		lane.tail = operation
	} else {
		lane.head = operation
		lane.tail = operation
	}
	_ = operation.Advance(lifecycle.OperationQueued)
	ck.operations[request.UID] = operation
	ck.appendOperation(operation)
	ck.appendRuntimeOperation(operation)
	if request.Source == lifecycle.SourceFunction {
		ck.functionOperations++
	}
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(
			lifecycle.RuntimeCounterOperationsAdmitted,
			1,
		)
	}
	ck.observeRuntimeOperations()
	ck.byAdmission[requested.Ref] = operation
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
	if plan.Transaction != nil &&
		plan.Transaction.PrepareComposite != nil {
		operation.composite = newKernelCompositeScope(ck, operation)
	}
	if !request.Deadline.IsZero() {
		operation.deadline = deadlineEntry{when: request.Deadline, operation: operation, index: -1}
		heap.Push(&ck.deadlines, &operation.deadline)
	}
	if parent != nil {
		operation.admitted = true
		ck.settleSubmission(operation, nil)
	}
	if operation.admitted && lane.active == nil && lane.head == operation {
		ck.markReady(lane)
	}
	return nil
}

func (ck *CommandKernel) rejectClosedAdmission(request Request) error {
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(
			lifecycle.RuntimeCounterShutdownRejected,
			1,
		)
	}
	closedErr := errors.New("jobmgr kernel: admission closed")
	if err := ck.abortRequestInputBody(request); err != nil {
		ck.run.Dirty(err)
		return errors.Join(ck.stoppingError(), err)
	}
	if ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	return closedErr
}

func (ck *CommandKernel) serviceAdmissions(quantum int) bool {
	if ck.admissionServiceGate != nil {
		select {
		case <-ck.admissionServiceGate:
		default:
			return false
		}
	}
	var grants [4]lifecycle.AdmissionGrant
	count, more, err := ck.admission.TakeGrants(quantum, &grants)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	for _, grant := range grants[:count] {
		if grant.Kind == lifecycle.ReservationInputBodyGrowth {
			select {
			case ck.inputBodyGrants <- grant:
			default:
				ck.run.Dirty(errors.New("jobmgr kernel: input body grant gate is full"))
				return false
			}
			continue
		}
		operation := ck.byAdmission[grant.Ref]
		if operation == nil || operation.admission != grant.Ref {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid admission grant"))
			return false
		}
		switch grant.Kind {
		case lifecycle.ReservationOrdinary:
			if operation.admitted {
				ck.run.Dirty(errors.New("jobmgr kernel: duplicate initial admission grant"))
				return false
			}
			if ck.compositeFenceConflicts(operation.claims) {
				wake, suspendErr :=
					ck.admission.SuspendOrdinary(
						grant.Ref,
						operation.request.PayloadCapacity,
					)
				if suspendErr != nil {
					ck.run.Dirty(suspendErr)
					return false
				}
				if err := ck.blockOnCompositeFence(
					operation,
				); err != nil {
					ck.run.Dirty(err)
					return false
				}
				more = more || wake
				continue
			}
			operation.admitted = true
			ck.settleSubmission(operation, nil)
			if operation.lane.active == nil && operation.lane.head == operation {
				ck.markReady(operation.lane)
			}
		case lifecycle.ReservationOrdinaryGrowth:
			if !operation.admitted ||
				operation.Child !=
					lifecycle.ChildResultReady {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid result growth grant"))
				return false
			}
			if !operation.resultGrowthWaiting {
				ck.run.Dirty(
					errors.New(
						"jobmgr kernel: unowned ordinary growth grant",
					),
				)
				return false
			}
			operation.resultGrowthWaiting = false
			if err := ck.sendEncodeAction(operation); err != nil {
				ck.run.Dirty(err)
				return false
			}
		default:
			ck.run.Dirty(errors.New("jobmgr kernel: unexpected admission grant kind"))
			return false
		}
	}
	return more
}

func (ck *CommandKernel) settleSubmission(operation *commandOperation, err error) {
	if operation == nil || operation.submissionResult == nil {
		return
	}
	operation.submissionResult <- err
	operation.submissionResult = nil
	operation.submissionContext = nil
}

func taskClassForOperation(
	operation *commandOperation,
	lane *commandLane,
) lifecycle.TaskClass {
	if operation.Source == lifecycle.SourceJobManager || lane.mapKey.resource {
		return lifecycle.TaskClassFrameworkControl
	}
	return lifecycle.TaskClassGenericFunction
}

func (ck *CommandKernel) scheduleTasks(quantum int) bool {
	for quantum > 0 {
		lane := ck.nextReadyLane()
		if lane == nil {
			return false
		}
		quantum--
		operation := lane.head
		if operation == nil || !operation.admitted || lane.active != nil {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid ready lane"))
			return false
		}
		if !operation.claimsHeld {
			if operation.claimsInherited {
				operation.claimsHeld = true
			} else {
				if operation.State < lifecycle.OperationAcquiringClaims {
					_ = operation.Advance(lifecycle.OperationAcquiringClaims)
				}
				granted, err := ck.claims.acquire(operation)
				if err != nil {
					ck.run.Dirty(err)
					return false
				}
				if !granted {
					continue
				}
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
					ck.run.Dirty(errors.New("jobmgr kernel: install encountered a live or retiring resource"))
					return false
				}
				ck.nextResourceGeneration++
				generation := ck.nextResourceGeneration
				if generation == 0 {
					ck.run.Dirty(errors.New("jobmgr kernel: resource generation wrapped"))
					return false
				}
				operation.resourceGeneration = generation
				prepare := resource.Prepare
				identity := lifecycle.ResourceIdentity{ID: resource.ID, Generation: generation}
				permitTaskPlan, err := lifecycle.NewPreparedResourcePermitTaskPlan(
					operation.Source, operation.request.Deadline, phaseLimit,
					ck.admission, operation.admission, identity, resource.Permit,
					func(ctx context.Context, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResource, error) {
						return prepare(ctx, generation, permit)
					},
				)
				if err != nil {
					ck.run.Dirty(err)
					return false
				}
				taskPlan = permitTaskPlan
			case ResourceStop:
				if lane.current == nil || !lane.currentIdentity.Valid() || lane.currentIdentity.ID != resource.ID || lane.currentStopping || lane.retiringIdentity.Valid() {
					ck.run.Dirty(errors.New("jobmgr kernel: stop encountered no exact current resource"))
					return false
				}
				operation.resourceGeneration = lane.currentIdentity.Generation
				readyPlan, err := lifecycle.NewReadyResourceTaskPlan(
					operation.Source, operation.request.Deadline, phaseLimit, lane.current, lane.currentIdentity,
				)
				if err != nil {
					ck.run.Dirty(err)
					return false
				}
				taskPlan = readyPlan
			default:
				ck.run.Dirty(errors.New("jobmgr kernel: unknown resource action at dispatch"))
				return false
			}
		}
		if transaction := operation.plan.Transaction; transaction != nil {
			if lane.currentStopping ||
				lane.retiringIdentity.Valid() ||
				(lane.current == nil) != !lane.currentIdentity.Valid() {
				ck.run.Dirty(
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
				ck.nextResourceGeneration++
				generation := ck.nextResourceGeneration
				if generation == 0 {
					ck.run.Dirty(
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
			transactionWork := transaction.Prepare
			if transaction.PrepareComposite != nil {
				prepare := transaction.PrepareComposite
				composite := operation.composite
				transactionWork = func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					prepared, prepareErr := prepare(
						ctx,
						current,
						scope,
						permit,
					)
					if prepared == nil {
						return nil, prepareErr
					}
					return &preparedCompositeBridge{
						transaction: prepared,
						scope:       composite,
					}, prepareErr
				}
			}
			var err error
			if scope.Successor.Valid() {
				taskPlan, err = lifecycle.NewResourceTransactionPermitTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					ck.admission,
					operation.admission,
					lane.current,
					scope,
					transaction.Permit,
					transactionWork,
				)
			} else {
				taskPlan, err = lifecycle.NewResourceTransactionTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					lane.current,
					scope,
					transactionWork,
				)
			}
			if err != nil {
				ck.run.Dirty(err)
				return false
			}
		}
		if capability := operation.plan.Capability; capability != nil {
			ck.nextResourceGeneration++
			generation := ck.nextResourceGeneration
			if generation == 0 {
				ck.run.Dirty(errors.New("jobmgr kernel: capability generation wrapped"))
				return false
			}
			operation.resourceGeneration = generation
			identity := lifecycle.ResourceIdentity{ID: capability.ID, Generation: generation}
			prepare := capability.Prepare
			capabilityPlan, err := lifecycle.NewPreparedCapabilityPermitTaskPlan(
				operation.Source, operation.request.Deadline, phaseLimit,
				ck.admission, operation.admission, identity, capability.Permit,
				func(ctx context.Context, permit lifecycle.LongLivedPermit) (lifecycle.PreparedCapability, error) {
					return prepare(ctx, generation, permit)
				},
			)
			if err != nil {
				ck.run.Dirty(err)
				return false
			}
			taskPlan = capabilityPlan
		}
		requestRef, err := ck.tasks.Enqueue(
			taskClassForOperation(operation, lane),
			taskPlan,
		)
		if err != nil {
			for _, grantedOperation := range ck.releaseClaims(operation) {
				ck.markReady(grantedOperation.lane)
			}
			ck.markReady(lane)
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
		ck.tasksByRequest[requestRef] = operation
	}
	return ck.ready[0].len != 0 || ck.ready[1].len != 0
}

func (ck *CommandKernel) serviceTaskStarts(quantum int) bool {
	var started [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, more, dispatchErr := ck.tasks.Dispatch(
		context.Background(),
		quantum,
		&started,
	)
	for _, start := range started[:count] {
		if start.Err != nil {
			ck.rejectTaskStart(start)
			continue
		}
		if cleanupRef, ok := ck.functionCleanupRequests[start.Request]; ok {
			if _, exists := ck.functionCleanupTasks[start.Task]; exists {
				ck.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup task slot"))
				return more
			}
			delete(ck.functionCleanupRequests, start.Request)
			ck.functionCleanupTasks[start.Task] = functionCleanupTask{ref: cleanupRef}
			continue
		}
		if ck.shutdownBarrierRequest.Valid() &&
			start.Request == ck.shutdownBarrierRequest {
			if ck.shutdownBarrierTask.Valid() ||
				ck.shutdownBarrierDone ||
				ck.shutdownBarrierFailed {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier start acknowledgement"))
				return more
			}
			ck.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
			ck.shutdownBarrierTask = start.Task
			continue
		}
		if ck.finalizerRequest.Valid() && start.Request == ck.finalizerRequest {
			if ck.finalizerTask.Valid() || ck.finalizerDone || ck.finalizerFailed {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer start acknowledgement"))
				return more
			}
			ck.finalizerRequest = lifecycle.TaskRequestRef{}
			ck.finalizerTask = start.Task
			continue
		}
		operation := ck.tasksByRequest[start.Request]
		if operation == nil {
			lane := ck.shutdownRequests[start.Request]
			if lane == nil || lane.shutdownRequest != start.Request ||
				lane.shutdownTask.Valid() || ck.shutdownTasks[start.Task] != nil {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task start acknowledgement"))
				return more
			}
			delete(ck.shutdownRequests, start.Request)
			lane.shutdownRequest = lifecycle.TaskRequestRef{}
			lane.shutdownTask = start.Task
			ck.shutdownTasks[start.Task] = lane
			continue
		}
		if operation == nil || operation.taskRequest != start.Request ||
			(operation.Child != lifecycle.ChildNotStarted && operation.Child != lifecycle.ChildDeadlineStartPending) {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid task start acknowledgement"))
			return more
		}
		delete(ck.tasksByRequest, start.Request)
		operation.taskRequest = lifecycle.TaskRequestRef{}
		if err := operation.Advance(lifecycle.OperationRunning); err != nil {
			ck.run.Dirty(err)
			return more
		}
		if err := operation.StartChild(start.Task); err != nil {
			ck.run.Dirty(err)
			return more
		}
		ck.tasksByRef[start.Task] = operation
	}
	if dispatchErr != nil {
		ck.run.Dirty(dispatchErr)
	}
	return more
}

func (ck *CommandKernel) rejectTaskStart(start lifecycle.TaskStart) {
	if cleanupRef, ok := ck.functionCleanupRequests[start.Request]; ok {
		delete(ck.functionCleanupRequests, start.Request)
		completeErr := errors.Join(errors.New("jobmgr kernel: Function cleanup task start rejected"), start.Err)
		ck.run.Dirty(errors.Join(completeErr, ck.functionCatalog.CompleteCleanup(cleanupRef, completeErr)))
		return
	}
	if ck.shutdownBarrierRequest.Valid() &&
		start.Request == ck.shutdownBarrierRequest {
		ck.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
		ck.shutdownBarrierFailed = true
		ck.run.Dirty(errors.Join(
			errors.New("jobmgr kernel: shutdown barrier task start rejected"),
			start.Err,
		))
		return
	}
	operation := ck.tasksByRequest[start.Request]
	if !errors.Is(start.Err, lifecycle.ErrLongLivedRecordCapacity) || operation == nil ||
		operation.taskRequest != start.Request || operation.Child != lifecycle.ChildNotStarted {
		ck.run.Dirty(errors.Join(errors.New("jobmgr kernel: invalid task start rejection"), start.Err))
		return
	}
	if operation.plan.Transaction != nil {
		if err := ck.restoreTransactionOutcome(operation, start.Outcome); err != nil {
			ck.run.Dirty(errors.Join(
				errors.New("jobmgr kernel: rejected transaction start lost current resource"),
				err,
			))
			return
		}
	} else if start.Outcome.Kind() != lifecycle.TaskOutcomeNone {
		ck.run.Dirty(
			errors.New("jobmgr kernel: rejected non-transaction start returned an outcome"),
		)
		return
	}
	delete(ck.tasksByRequest, start.Request)
	operation.taskRequest = lifecycle.TaskRequestRef{}
	operation.terminalErr = errors.Join(operation.terminalErr, start.Err)
	ck.unlinkQueued(operation, start.Err)
	if operation.Response != lifecycle.ResponseNotRequired {
		ck.enqueueControl(operation, lifecycle.ControlUnavailable)
	}
	ck.tryDispose(operation)
}

func (ck *CommandKernel) completeTask(completion lifecycle.TaskCompletion) {
	if cleanup, ok := ck.functionCleanupTasks[completion.Ref]; ok {
		if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone {
			completion.Err = errors.Join(completion.Err, errors.New("jobmgr kernel: invalid Function cleanup completion"))
		}
		cleanup.err = completion.Err
		ck.functionCleanupTasks[completion.Ref] = cleanup
		if err := ck.tasks.SendAction(lifecycle.TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate,
		}); err != nil {
			ck.run.Dirty(err)
		}
		return
	}
	if ck.finalizerTask.Valid() && completion.Ref == ck.finalizerTask {
		ck.completeRunFinalizer(completion)
		return
	}
	if ck.shutdownBarrierTask.Valid() &&
		completion.Ref == ck.shutdownBarrierTask {
		ck.completeShutdownBarrier(completion)
		return
	}
	operation := ck.tasksByRef[completion.Ref]
	if operation == nil {
		ck.completeShutdownTask(completion)
		return
	}
	if operation.composite != nil &&
		operation.activeChild != nil {
		if operation.deferredCompletion != nil {
			ck.run.Dirty(
				errors.New(
					"jobmgr composite: parent completed twice with a live child",
				),
			)
			return
		}
		pending := completion
		operation.deferredCompletion = &pending
		return
	}
	if _, err := ck.tasks.ClearRetainedTimeout(operation.Task); err != nil {
		ck.run.Dirty(err)
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
		ck.run.Dirty(resultErr)
		return
	}
	ck.markOperationDeadlineIfDue(operation)
	if operation.plan.Transaction != nil {
		ck.completeResourceTransactionTask(operation, completion)
		return
	}
	if operation.plan.Resource != nil {
		ck.completeResourceTask(operation, completion)
		return
	}
	if operation.plan.Capability != nil {
		ck.completeCapabilityTask(operation, completion)
		return
	}
	if operation.cancelled &&
		operation.Response == lifecycle.ResponseOpen &&
		!operation.controlQueued {
		ck.enqueueControl(
			operation,
			cancellationControl(operation),
		)
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: lifecycle.TaskActionDispose}
	if completion.Err == nil && operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
		expiry := lifecycle.ExpiryAt(ck.clock.Now())
		preflight, err := ck.tasks.PreflightResult(completion.Ref, operation.UID, expiry)
		if err != nil {
			status := lifecycle.ControlInternal
			if errors.Is(err, lifecycle.ErrFunctionResultTooLarge) {
				status = lifecycle.ControlPayloadTooLarge
			}
			ck.enqueueControl(operation, status)
		} else {
			total, sizeErr := operationResultAdmissionBytes(operation.admissionBase, preflight)
			if sizeErr != nil {
				ck.enqueueControl(operation, lifecycle.ControlPayloadTooLarge)
			} else {
				ready, _, resizeErr := ck.admission.ResizeOrdinary(operation.admission, total)
				if resizeErr != nil {
					ck.run.Dirty(resizeErr)
					return
				}
				operation.resultExpiry = expiry
				if !ready {
					operation.resultGrowthWaiting = true
					return
				}
				if err := ck.sendEncodeAction(operation); err != nil {
					ck.run.Dirty(err)
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
		ck.enqueueControl(operation, status)
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) completeCapabilityTask(operation *commandOperation, completion lifecycle.TaskCompletion) {
	kind := lifecycle.TaskActionDispose
	if completion.Err == nil && !operation.cancelled && !operation.TimedOut() {
		if completion.Kind != lifecycle.TaskOutcomePreparedCapability {
			ck.dirtyCapability(operation, errors.New("jobmgr kernel: capability task returned the wrong outcome"))
			return
		}
		kind = lifecycle.TaskActionCommitCapability
	}
	if completion.Err != nil {
		ck.dirtyCapability(operation, completion.Err)
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: kind}
	if kind == lifecycle.TaskActionCommitCapability {
		action.ExpectedGeneration = operation.resourceGeneration
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) completeResourceTask(operation *commandOperation, completion lifecycle.TaskCompletion) {
	kind := lifecycle.TaskActionDispose
	if completion.Err == nil && !operation.TimedOut() {
		switch operation.plan.Resource.Action {
		case ResourceInstall:
			if completion.Kind != lifecycle.TaskOutcomePreparedResource {
				ck.run.Dirty(errors.New("jobmgr kernel: install task returned the wrong outcome"))
				return
			}
			kind = lifecycle.TaskActionAcceptStart
		case ResourceStop:
			if completion.Kind != lifecycle.TaskOutcomeReadyResource {
				ck.run.Dirty(errors.New("jobmgr kernel: stop task returned the wrong outcome"))
				return
			}
			kind = lifecycle.TaskActionStopResource
		default:
			ck.run.Dirty(errors.New("jobmgr kernel: unknown resource completion"))
			return
		}
	}
	if completion.Err != nil {
		ck.run.Dirty(completion.Err)
	}
	action := lifecycle.TaskAction{Ref: completion.Ref, Sequence: completion.Sequence + 1, Kind: kind}
	if kind == lifecycle.TaskActionAcceptStart {
		action.ExpectedGeneration = operation.resourceGeneration
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) completeResourceTransactionTask(
	operation *commandOperation,
	completion lifecycle.TaskCompletion,
) {
	switch completion.Sequence {
	case 1:
		if completion.Err != nil {
			current, err := ck.tasks.TakeDisposedResourceTransaction(
				completion.Ref,
				completion.Sequence,
				operation.transactionScope,
			)
			if err != nil {
				ck.run.Dirty(errors.Join(
					errors.New(
						"jobmgr kernel: failed transaction preparation lost current resource",
					),
					completion.Err,
					err,
				))
				return
			}
			if err := ck.restoreTransactionCurrent(operation, current); err != nil {
				ck.run.Dirty(errors.Join(completion.Err, err))
				return
			}
			operation.terminalErr = errors.Join(
				operation.terminalErr,
				completion.Err,
			)
			status := lifecycle.ControlUnavailable
			if errors.Is(completion.Err, lifecycle.ErrTaskPanic) {
				status = lifecycle.ControlInternal
			}
			if operation.Response == lifecycle.ResponseOpen &&
				!operation.controlQueued {
				ck.enqueueControl(operation, status)
			}
			ck.sendTransactionAction(
				operation,
				completion.Ref,
				completion.Sequence+1,
				lifecycle.TaskActionDispose,
			)
			return
		}
		if completion.Kind != lifecycle.TaskOutcomePreparedResourceTransaction {
			ck.run.Dirty(
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
		ck.sendTransactionAction(
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
			ck.run.Dirty(errors.Join(
				errors.New(
					"jobmgr kernel: resource transaction apply left unprovable state",
				),
				completion.Err,
			))
			return
		}
		if completion.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction {
			ck.run.Dirty(
				errors.New(
					"jobmgr kernel: transaction apply returned the wrong outcome",
				),
			)
			return
		}
		disposition, current, err :=
			ck.tasks.TakeAppliedResourceTransaction(
				completion.Ref,
				completion.Sequence,
				operation.transactionScope,
			)
		if err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.applyTransactionDisposition(
			operation,
			disposition,
			current,
		); err != nil {
			ck.run.Dirty(err)
			return
		}
		operation.transactionApplied = true

		if (operation.cancelled ||
			operation.TimedOut()) &&
			operation.Response ==
				lifecycle.ResponseOpen &&
			!operation.controlQueued {
			ck.enqueueControl(
				operation,
				cancellationControl(operation),
			)
		}
		if operation.Response == lifecycle.ResponseOpen &&
			!operation.controlQueued {
			expiry := lifecycle.ExpiryAt(ck.clock.Now())
			preflight, preflightErr := ck.tasks.PreflightResult(
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
				ck.enqueueControl(operation, status)
			} else {
				total, sizeErr := operationResultAdmissionBytes(
					operation.admissionBase,
					preflight,
				)
				if sizeErr != nil {
					ck.enqueueControl(
						operation,
						lifecycle.ControlPayloadTooLarge,
					)
				} else {
					ready, _, resizeErr := ck.admission.ResizeOrdinary(
						operation.admission,
						total,
					)
					if resizeErr != nil {
						ck.run.Dirty(resizeErr)
						return
					}
					operation.resultExpiry = expiry
					if !ready {
						operation.resultGrowthWaiting = true
						return
					}
					if err := ck.sendEncodeAction(operation); err != nil {
						ck.run.Dirty(err)
					}
					return
				}
			}
		}
		ck.sendTransactionAction(
			operation,
			completion.Ref,
			completion.Sequence+1,
			lifecycle.TaskActionDispose,
		)
	default:
		ck.run.Dirty(
			errors.New("jobmgr kernel: unexpected transaction completion"),
		)
	}
}

func (ck *CommandKernel) sendTransactionAction(
	operation *commandOperation,
	ref lifecycle.TaskRef,
	sequence uint8,
	kind lifecycle.TaskActionKind,
) {
	action := lifecycle.TaskAction{
		Ref: ref, Sequence: sequence, Kind: kind,
	}
	if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) restoreTransactionOutcome(
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
		return ck.restoreTransactionCurrent(operation, current)
	}
	if outcome.Kind() != lifecycle.TaskOutcomeNone {
		return errors.New(
			"jobmgr kernel: graph-only transaction returned a current resource",
		)
	}
	return ck.restoreTransactionCurrent(operation, nil)
}

func (ck *CommandKernel) restoreTransactionCurrent(
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

func (ck *CommandKernel) applyTransactionDisposition(
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
		lane.resourceSource = 0
	case lifecycle.ResourceTransactionInstalled,
		lifecycle.ResourceTransactionReplaced:
		identity = scope.Successor
		if current == nil || !identity.Valid() {
			return errors.New(
				"jobmgr kernel: installed transaction lost its successor",
			)
		}
		lane.resourceSource = operation.Source
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

func (ck *CommandKernel) completeShutdownTask(completion lifecycle.TaskCompletion) {
	if !completion.Ref.Valid() {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task completion"))
		return
	}
	lane := ck.shutdownTasks[completion.Ref]
	if lane == nil || lane.shutdownTask != completion.Ref || lane.shutdownAction != 0 || completion.Sequence != 1 ||
		!lane.currentStopping || lane.current != nil || !lane.currentIdentity.Valid() || lane.retiringIdentity.Valid() {
		ck.run.Dirty(errors.New("jobmgr kernel: completion for unknown or invalid shutdown task"))
		return
	}
	if completion.Err != nil {
		ck.run.Dirty(completion.Err)
		return
	}
	if completion.Kind != lifecycle.TaskOutcomeReadyResource {
		ck.run.Dirty(errors.New("jobmgr kernel: shutdown Stop task returned the wrong outcome"))
		return
	}
	ck.sendShutdownAction(lane, lifecycle.TaskActionStopResource, 2)
}

func (ck *CommandKernel) acknowledgeShutdownTask(ack lifecycle.TaskAcknowledgement) {
	if !ack.Ref.Valid() {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task acknowledgement"))
		return
	}
	lane := ck.shutdownTasks[ack.Ref]
	if lane == nil || lane.shutdownTask != ack.Ref || lane.shutdownAction != ack.Kind {
		ck.run.Dirty(errors.New("jobmgr kernel: acknowledgement for unknown or invalid shutdown task"))
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
		ck.run.Dirty(errors.New("jobmgr kernel: unexpected shutdown task action"))
		return
	}
	if ack.Sequence != wantSequence {
		ck.run.Dirty(errors.New("jobmgr kernel: stale shutdown task acknowledgement"))
		return
	}
	if ack.Err != nil && ack.Kind != lifecycle.TaskActionTerminate {
		ck.run.Dirty(ack.Err)
		return
	}
	lane.shutdownAction = 0
	switch ack.Kind {
	case lifecycle.TaskActionStopResource:
		identity := lane.currentIdentity
		if !lane.currentStopping || lane.current != nil || !identity.Valid() || lane.retiringIdentity.Valid() {
			ck.run.Dirty(errors.New("jobmgr kernel: shutdown stopped resource differs from current slot"))
			return
		}
		lane.currentIdentity = lifecycle.ResourceIdentity{}
		lane.currentStopping = false
		lane.retiringIdentity = identity
		ck.sendShutdownAction(lane, lifecycle.TaskActionFinalizeResource, 3)
	case lifecycle.TaskActionFinalizeResource:
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || !lane.retiringIdentity.Valid() {
			ck.run.Dirty(errors.New("jobmgr kernel: shutdown finalized resource differs from retiring slot"))
			return
		}
		lane.retiringIdentity = lifecycle.ResourceIdentity{}
		lane.resourceSource = 0
		ck.sendShutdownAction(lane, lifecycle.TaskActionTerminate, 4)
	case lifecycle.TaskActionTerminate:
		if ack.Err != nil {
			ck.run.Dirty(ack.Err)
		}
		if err := ck.tasks.Release(ack.Ref); err != nil {
			ck.run.Dirty(err)
			return
		}
		delete(ck.shutdownTasks, ack.Ref)
		lane.shutdownTask = lifecycle.TaskRef{}
		lane.shutdownAction = 0
		ck.releaseUnusedLane(lane)
	default:
		ck.run.Dirty(errors.New("jobmgr kernel: unexpected shutdown task action"))
	}
}

func (ck *CommandKernel) sendShutdownAction(lane *commandLane, kind lifecycle.TaskActionKind, sequence uint8) {
	if lane == nil || !lane.shutdownTask.Valid() || lane.shutdownAction != 0 {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown action transition"))
		return
	}
	lane.shutdownAction = kind
	if err := ck.tasks.SendAction(lifecycle.TaskAction{Ref: lane.shutdownTask, Sequence: sequence, Kind: kind}); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) sendEncodeAction(operation *commandOperation) error {
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
	return ck.tasks.SendAction(action)
}

func (ck *CommandKernel) acknowledgeTask(ack lifecycle.TaskAcknowledgement) {
	if cleanup, ok := ck.functionCleanupTasks[ack.Ref]; ok {
		if ack.Kind != lifecycle.TaskActionTerminate || ack.Sequence != 2 {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid Function cleanup acknowledgement"))
			return
		}
		if err := ck.tasks.Release(ack.Ref); err != nil {
			ck.run.Dirty(err)
			return
		}
		delete(ck.functionCleanupTasks, ack.Ref)
		completeErr := errors.Join(cleanup.err, ack.Err)
		catalogErr := ck.functionCatalog.CompleteCleanup(cleanup.ref, completeErr)
		if completeErr != nil || catalogErr != nil {
			ck.run.Dirty(errors.Join(completeErr, catalogErr))
		}
		return
	}
	if ck.finalizerTask.Valid() && ack.Ref == ck.finalizerTask {
		ck.acknowledgeRunFinalizer(ack)
		return
	}
	if ck.shutdownBarrierTask.Valid() &&
		ack.Ref == ck.shutdownBarrierTask {
		ck.acknowledgeShutdownBarrier(ack)
		return
	}
	operation := ck.tasksByRef[ack.Ref]
	if operation == nil {
		ck.acknowledgeShutdownTask(ack)
		return
	}
	if ack.Kind == lifecycle.TaskActionTerminate {
		if err := operation.ChildExited(ack.Ref, ack.Sequence); err != nil {
			ck.run.Dirty(err)
			return
		}
		if ack.Err != nil {
			operation.PoisonResponse()
			ck.run.Dirty(ack.Err)
		}
		if err := ck.tasks.Release(ack.Ref); err != nil {
			ck.run.Dirty(err)
			return
		}
		delete(ck.tasksByRef, ack.Ref)
		ck.tryDispose(operation)
		return
	}
	ck.markOperationDeadlineIfDue(operation)
	if err := operation.ActionAcknowledged(ack.Ref, ack.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if operation.plan.Transaction != nil {
		ck.acknowledgeResourceTransactionTask(operation, ack)
		return
	}
	if operation.plan.Resource != nil {
		ck.acknowledgeResourceTask(operation, ack)
		return
	}
	if operation.plan.Capability != nil {
		ck.acknowledgeCapabilityTask(operation, ack)
		return
	}
	if ack.Err != nil {
		operation.PoisonResponse()
		ck.run.Dirty(ack.Err)
	} else if ack.Kind == lifecycle.TaskActionEncodeWrite {
		if err := operation.CommitResponse(); err != nil {
			ck.run.Dirty(err)
		}
		if _, _, err := ck.admission.ResizeOrdinary(operation.admission, operation.admissionBase); err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.completeOperationUID(operation, false); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	if ack.Kind == lifecycle.TaskActionCleanup {
		operation.cleanupDone = true
	}
	if operation.plan.Cleanup != nil && !operation.cleanupDone {
		cleanup := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionCleanup}
		if err := operation.ActionPending(cleanup.Ref, cleanup.Sequence); err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.tasks.SendAction(cleanup); err != nil {
			ck.run.Dirty(err)
		}
		return
	}
	termination := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionTerminate}
	if err := operation.TerminationPending(termination.Ref, termination.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(termination); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) acknowledgeResourceTransactionTask(
	operation *commandOperation,
	ack lifecycle.TaskAcknowledgement,
) {
	if ack.Err != nil {
		operation.terminalErr = errors.Join(operation.terminalErr, ack.Err)
		if ack.Kind == lifecycle.TaskActionEncodeWrite {
			operation.PoisonResponse()
		}
		ck.run.Dirty(ack.Err)
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
				current, err := ck.tasks.TakeDisposedResourceTransaction(
					ack.Ref,
					ack.Sequence,
					operation.transactionScope,
				)
				if err != nil {
					ck.run.Dirty(err)
					return
				}
				if err := ck.restoreTransactionCurrent(
					operation,
					current,
				); err != nil {
					ck.run.Dirty(err)
					return
				}
			}
			ck.sendResourceTermination(
				operation,
				ack.Ref,
				ack.Sequence+1,
			)
			return
		}
	case lifecycle.TaskActionEncodeWrite:
		if ack.Err == nil {
			if err := operation.CommitResponse(); err != nil {
				ck.run.Dirty(err)
				return
			}
			if _, _, err := ck.admission.ResizeOrdinary(
				operation.admission,
				operation.admissionBase,
			); err != nil {
				ck.run.Dirty(err)
				return
			}
			if err := ck.completeOperationUID(
				operation,
				false,
			); err != nil {
				ck.run.Dirty(err)
				return
			}
		}
	case lifecycle.TaskActionCleanup:
		operation.cleanupDone = true
	default:
		ck.run.Dirty(
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
			ck.run.Dirty(err)
			return
		}
		if err := ck.tasks.SendAction(cleanup); err != nil {
			ck.run.Dirty(err)
		}
		return
	}
	ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
}

func (ck *CommandKernel) acknowledgeCapabilityTask(operation *commandOperation, ack lifecycle.TaskAcknowledgement) {
	switch ack.Kind {
	case lifecycle.TaskActionCommitCapability:
		switch ack.CapabilityDisposition {
		case lifecycle.CapabilityApplied:
			if ack.Err != nil {
				ck.dirtyCapability(operation, ack.Err)
			}
		case lifecycle.CapabilityDisposed:
			if ack.Err == nil {
				ck.dirtyCapability(operation, errors.New("jobmgr kernel: capability was disposed without a commit error"))
			} else {
				ck.dirtyCapability(operation, ack.Err)
			}
		case lifecycle.CapabilityRetained:
			ck.dirtyCapability(operation, errors.Join(errors.New("jobmgr kernel: capability commit retained ambiguous ownership"), ack.Err))
			return
		default:
			ck.dirtyCapability(operation, errors.New("jobmgr kernel: invalid capability commit disposition"))
			return
		}
	case lifecycle.TaskActionDispose:
		if ack.CapabilityDisposition != 0 {
			ck.dirtyCapability(operation, errors.New("jobmgr kernel: dispose acknowledged a capability disposition"))
			return
		}
		if ack.Err != nil {
			ck.dirtyCapability(operation, ack.Err)
			return
		}
	default:
		ck.dirtyCapability(operation, errors.New("jobmgr kernel: unexpected capability acknowledgement"))
		return
	}
	ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
}

func (ck *CommandKernel) dirtyCapability(operation *commandOperation, err error) {
	if err == nil {
		err = errors.New("jobmgr kernel: unspecified capability failure")
	}
	operation.terminalErr = errors.Join(operation.terminalErr, err)
	ck.run.Dirty(err)
}

func (ck *CommandKernel) acknowledgeResourceTask(operation *commandOperation, ack lifecycle.TaskAcknowledgement) {
	if ack.Err != nil {
		ck.run.Dirty(ack.Err)
		if ack.Kind == lifecycle.TaskActionStopResource || ack.Kind == lifecycle.TaskActionFinalizeResource || ack.Kind == lifecycle.TaskActionDispose {
			return
		}
		if ack.Kind == lifecycle.TaskActionAcceptStart || ack.Kind == lifecycle.TaskActionPublishResource {
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
				ck.run.Dirty(err)
				return
			}
			if err := ck.tasks.SendAction(action); err != nil {
				ck.run.Dirty(err)
			}
			return
		}
		ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
		return
	}
	lane := operation.lane
	switch ack.Kind {
	case lifecycle.TaskActionAcceptStart:
		if operation.cancelled || operation.TimedOut() {
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
				ck.run.Dirty(err)
				return
			}
			if err := ck.tasks.SendAction(action); err != nil {
				ck.run.Dirty(err)
			}
			return
		}
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() {
			ck.run.Dirty(errors.New("jobmgr kernel: resource publication found a nonempty current slot"))
			return
		}
		action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionPublishResource}
		if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.tasks.SendAction(action); err != nil {
			ck.run.Dirty(err)
		}
	case lifecycle.TaskActionPublishResource:
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() {
			ck.run.Dirty(errors.New("jobmgr kernel: resource publication found a nonempty current slot"))
			return
		}
		expected := lifecycle.ResourceIdentity{ID: operation.plan.Resource.ID, Generation: operation.resourceGeneration}
		resource, err := ck.tasks.TakePublishedReadyResource(ack.Ref, ack.Sequence, expected)
		if err != nil {
			ck.run.Dirty(err)
			action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionDispose}
			if actionErr := operation.ActionPending(action.Ref, action.Sequence); actionErr != nil {
				ck.run.Dirty(actionErr)
				return
			}
			if actionErr := ck.tasks.SendAction(action); actionErr != nil {
				ck.run.Dirty(actionErr)
			}
			return
		}
		identity := expected
		lane.current = resource
		lane.currentIdentity = identity
		lane.resourceGeneration = identity.Generation
		lane.resourceSource = operation.Source
		ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	case lifecycle.TaskActionStopResource:
		identity := lane.currentIdentity
		if !lane.currentStopping || lane.current != nil || identity.ID != operation.plan.Resource.ID || identity.Generation != operation.resourceGeneration || lane.retiringIdentity.Valid() {
			ck.run.Dirty(errors.New("jobmgr kernel: stopped resource differs from current slot"))
			return
		}
		lane.currentIdentity = lifecycle.ResourceIdentity{}
		lane.currentStopping = false
		lane.retiringIdentity = identity
		action := lifecycle.TaskAction{Ref: ack.Ref, Sequence: ack.Sequence + 1, Kind: lifecycle.TaskActionFinalizeResource}
		if err := operation.ActionPending(action.Ref, action.Sequence); err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.tasks.SendAction(action); err != nil {
			ck.run.Dirty(err)
		}
	case lifecycle.TaskActionFinalizeResource:
		identity := lane.retiringIdentity
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || identity.ID != operation.plan.Resource.ID || identity.Generation != operation.resourceGeneration {
			ck.run.Dirty(errors.New("jobmgr kernel: finalized resource differs from retiring slot"))
			return
		}
		lane.retiringIdentity = lifecycle.ResourceIdentity{}
		lane.resourceSource = 0
		ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	case lifecycle.TaskActionDispose:
		ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
	default:
		ck.run.Dirty(errors.New("jobmgr kernel: unexpected resource acknowledgement"))
	}
}

func (ck *CommandKernel) sendResourceTermination(operation *commandOperation, ref lifecycle.TaskRef, sequence uint8) {
	termination := lifecycle.TaskAction{Ref: ref, Sequence: sequence, Kind: lifecycle.TaskActionTerminate}
	if err := operation.TerminationPending(termination.Ref, termination.Sequence); err != nil {
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(termination); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) cancelOperation(uid string) {
	operation := ck.operations[uid]
	if operation != nil &&
		operation.activeChild != nil &&
		!operation.activeChild.compositeRollback {
		ck.cancelOperation(operation.activeChild.UID)
	}
	if operation == nil || operation.Response == lifecycle.ResponseCommitted || operation.Response == lifecycle.ResponsePoisoned {
		return
	}
	if operation.TimedOut() {
		return
	}
	operation.cancelled = true
	if operation.Child == lifecycle.ChildExecuting {
		_ = ck.tasks.Cancel(operation.Task)
		if operation.Response != lifecycle.ResponseNotRequired && !operation.plan.CooperativeCancel {
			ck.enqueueControl(operation, lifecycle.ControlCancelled)
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
		ck.unlinkQueued(operation, cause)
		if operation.Response != lifecycle.ResponseNotRequired {
			ck.enqueueControl(operation, lifecycle.ControlCancelled)
		} else {
			ck.tryDispose(operation)
		}
		return
	}
	if operation.Child == lifecycle.ChildResultReady &&
		operation.resultGrowthWaiting {
		if err := ck.admission.CancelWaiting(operation.admission); err != nil {
			ck.run.Dirty(err)
			return
		}
		operation.resultGrowthWaiting = false
		if operation.Response != lifecycle.ResponseNotRequired {
			ck.enqueueControl(
				operation,
				lifecycle.ControlCancelled,
			)
		}
		ck.sendDisposeAction(operation)
		return
	}
	if operation.Child == lifecycle.ChildActionPending && cancellablePendingAction(operation) {
		_ = ck.tasks.Cancel(operation.Task)
	}
}

func (ck *CommandKernel) sendDisposeAction(operation *commandOperation) {
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
		ck.run.Dirty(err)
		return
	}
	if err := ck.tasks.SendAction(action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) serviceDeadlines(now time.Time, quantum int) bool {
	for quantum > 0 && ck.deadlines.Len() > 0 {
		entry := ck.deadlines[0]
		if entry.when.After(now) {
			return false
		}
		heap.Pop(&ck.deadlines)
		quantum--
		operation := entry.operation
		if operation.State >= lifecycle.OperationDisposing ||
			(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
			continue
		}
		ck.markOperationTimedOut(operation)
		if operation.activeChild != nil &&
			!operation.activeChild.compositeRollback {
			ck.cancelOperation(operation.activeChild.UID)
		}
		deferControl := requiresCooperativeDeadlineStart(operation) &&
			(operation.Child == lifecycle.ChildNotStarted || operation.Child == lifecycle.ChildExecuting)
		if operation.Child == lifecycle.ChildExecuting {
			_ = ck.tasks.CancelWithCause(operation.Task, context.DeadlineExceeded)
			if operation.Response == lifecycle.ResponseNotRequired {
				if err := ck.markRetainedTimeout(operation, true); err != nil {
					ck.run.Dirty(err)
				}
			}
		} else if operation.Child == lifecycle.ChildNotStarted {
			if requiresCooperativeDeadlineStart(operation) {
				if err := operation.RequireDeadlineStart(); err != nil {
					ck.run.Dirty(err)
					return false
				}
				if operation.taskRequest.Valid() {
					if err := ck.tasks.SetPendingCancellation(operation.taskRequest, context.DeadlineExceeded); err != nil {
						ck.run.Dirty(err)
						return false
					}
				}
			} else {
				ck.unlinkQueued(operation, context.DeadlineExceeded)
				if operation.Response == lifecycle.ResponseNotRequired {
					ck.tryDispose(operation)
				}
			}
		} else if operation.Child ==
			lifecycle.ChildResultReady &&
			operation.resultGrowthWaiting {
			if err := ck.admission.CancelWaiting(operation.admission); err != nil {
				ck.run.Dirty(err)
				return false
			}
			operation.resultGrowthWaiting = false
			ck.sendDisposeAction(operation)
		} else if operation.Child == lifecycle.ChildActionPending && cancellablePendingAction(operation) {
			_ = ck.tasks.CancelWithCause(operation.Task, context.DeadlineExceeded)
		}
		if operation.Response != lifecycle.ResponseNotRequired && !deferControl {
			ck.enqueueControl(operation, lifecycle.ControlDeadline)
		}
	}
	return ck.deadlines.Len() > 0 && !ck.deadlines[0].when.After(now)
}

func cancellablePendingAction(operation *commandOperation) bool {
	return operation != nil &&
		(operation.plan.Capability != nil ||
			operation.plan.Resource != nil ||
			operation.plan.Transaction != nil)
}

func requiresCooperativeDeadlineStart(operation *commandOperation) bool {
	return operation != nil &&
		(operation.plan.Work != nil || operation.plan.Runner != nil) &&
		operation.plan.CooperativeDeadline
}

func (ck *CommandKernel) markOperationDeadlineIfDue(operation *commandOperation) {
	if operation == nil || operation.request.Deadline.IsZero() ||
		(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
		return
	}
	if operation.TimedOut() {
		if operation.Response == lifecycle.ResponseOpen {
			ck.enqueueControl(operation, lifecycle.ControlDeadline)
		}
		return
	}
	if operation.request.Deadline.After(ck.clock.Now()) {
		return
	}
	ck.markOperationTimedOut(operation)
	if operation.Response == lifecycle.ResponseOpen {
		ck.enqueueControl(operation, lifecycle.ControlDeadline)
	}
}

func (ck *CommandKernel) markOperationTimedOut(
	operation *commandOperation,
) {
	if operation == nil || operation.TimedOut() {
		return
	}
	operation.MarkTimedOut()
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(
			lifecycle.RuntimeCounterOperationTimeouts,
			1,
		)
	}
}

func (ck *CommandKernel) enqueueControl(operation *commandOperation, status lifecycle.ControlStatus) {
	if operation == nil || operation.Response != lifecycle.ResponseOpen || operation.controlQueued {
		return
	}
	operation.control = status
	operation.controlQueued = true
	ck.controls = append(ck.controls, operation)
}

func (ck *CommandKernel) serviceControls(quantum int) bool {
	for quantum > 0 && len(ck.controls) > 0 {
		operation := ck.controls[0]
		if operation.Response == lifecycle.ResponseOpen {
			_ = operation.MarkResponsePending()
		}
		err := ck.frames.TryCommitControl(lifecycle.ControlFramePlan{UID: operation.UID, Status: operation.control, Expiry: lifecycle.ExpiryAt(ck.clock.Now())})
		if errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			return false
		}
		ck.controls = ck.controls[1:]
		operation.controlQueued = false
		quantum--
		if err != nil {
			operation.PoisonResponse()
			ck.run.Dirty(err)
			ck.tryDispose(operation)
			continue
		}
		if err := operation.CommitResponse(); err != nil {
			ck.run.Dirty(err)
			continue
		}
		if err := ck.completeOperationUID(operation, true); err != nil {
			ck.run.Dirty(err)
			return false
		}
		if operation.TimedOut() && operation.Child == lifecycle.ChildExecuting {
			if err := ck.markRetainedTimeout(operation, false); err != nil {
				ck.run.Dirty(err)
			}
		}
		ck.tryDispose(operation)
	}
	return len(ck.controls) > 0
}

func (ck *CommandKernel) markRetainedTimeout(operation *commandOperation, background bool) error {
	if operation == nil || !operation.TimedOut() || operation.Child != lifecycle.ChildExecuting || !operation.Task.Valid() {
		return errors.New("jobmgr kernel: invalid retained-timeout mark")
	}
	saturated, err := ck.tasks.MarkRetainedTimeout(operation.Task)
	if err != nil {
		return err
	}
	if saturated && background {
		return errors.New("jobmgr kernel: fourth background timeout reached the retained-timeout fail-stop threshold")
	}
	if saturated {
		return errors.New("jobmgr kernel: fourth retained timeout reached the fail-stop threshold")
	}
	return nil
}

func (ck *CommandKernel) tryDispose(operation *commandOperation) {
	if operation == nil ||
		operation.activeChild != nil ||
		operation.deferredCompletion != nil {
		return
	}
	if !operation.CanDisposeTerminal() {
		return
	}
	if operation.taskRequest.Valid() {
		ck.run.Dirty(errors.New("jobmgr kernel: terminal operation retained a pending task request"))
		return
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if err := ck.endCompositeFence(operation); err != nil {
		ck.run.Dirty(err)
		return
	}
	if operation.Response == lifecycle.ResponseNotRequired {
		if err := ck.completeOperationUID(operation, false); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	if operation.functionInvocation.Valid() {
		ref := operation.functionInvocation
		operation.functionInvocation = FunctionInvocationRef{}
		if err := ck.releaseFunctionInvocation(ref); err != nil {
			ck.run.Dirty(err)
		}
	}
	if operation.deadline.index >= 0 {
		heap.Remove(&ck.deadlines, operation.deadline.index)
	}
	for _, granted := range ck.releaseClaims(operation) {
		ck.markReady(granted.lane)
	}
	lane := operation.lane
	if lane.active == operation {
		lane.active = nil
	}
	if lane.head == operation {
		ck.removeHead(lane)
	} else if operation.previous != nil || operation.next != nil {
		ck.unlink(operation)
	}
	if operation.admission.Valid() {
		if !operation.admitted {
			ck.run.Dirty(errors.New("jobmgr kernel: terminal operation retained an ungranted admission"))
			return
		}
		if _, err := ck.admission.ReleaseOrdinary(operation.admission); err != nil {
			ck.run.Dirty(err)
			return
		}
		delete(ck.byAdmission, operation.admission)
		operation.admission = lifecycle.AdmissionRef{}
	}
	if resource := operation.plan.Resource; resource != nil {
		switch resource.Action {
		case ResourceInstall:
			if !lane.installPlanned {
				ck.run.Dirty(errors.New("jobmgr kernel: install plan marker cleared twice"))
				return
			}
			lane.installPlanned = false
		case ResourceStop:
			if !lane.stopPlanned {
				ck.run.Dirty(errors.New("jobmgr kernel: stop plan marker cleared twice"))
				return
			}
			lane.stopPlanned = false
		}
	}
	if operation.plan.Transaction != nil {
		if lane.transactionPlanned <= 0 {
			ck.run.Dirty(
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
		ck.run.Dirty(errors.New("jobmgr kernel: negative lane ownership"))
		return
	}
	if ck.shutdownActive && ck.shutdownBarrierDone &&
		lane.shutdownVisited && lane.owners == 0 {
		if err := ck.enqueueShutdownStop(lane); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	_ = operation.Advance(lifecycle.OperationDisposedTerminal)
	delete(ck.operations, operation.UID)
	if operation.Source == lifecycle.SourceFunction {
		ck.functionOperations--
		if ck.functionOperations < 0 {
			ck.run.Dirty(
				errors.New(
					"jobmgr kernel: negative Function operation count",
				),
			)
			return
		}
	}
	ck.unlinkRuntimeOperation(operation)
	ck.observeRuntimeOperations()
	if !ck.shutdownActive || operation.shutdownVisited {
		ck.unlinkOperation(operation)
	}
	ck.completeCompositeChild(operation)
	if operation.terminalResult != nil {
		operation.terminalResult <- operation.terminalErr
		operation.terminalResult = nil
	}
	if lane.active == nil && lane.head != nil {
		ck.markReady(lane)
	}
	ck.releaseUnusedLane(lane)
}

func (ck *CommandKernel) appendOperation(operation *commandOperation) {
	if operation == nil || operation.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid operation-list append"))
		return
	}
	operation.allPrevious = ck.operationTail
	if ck.operationTail != nil {
		ck.operationTail.allNext = operation
	} else {
		ck.operationHead = operation
	}
	ck.operationTail = operation
	operation.allListed = true
}

func (ck *CommandKernel) appendRuntimeOperation(
	operation *commandOperation,
) {
	if operation == nil || operation.runtimeListed {
		ck.run.Dirty(
			errors.New("jobmgr kernel: invalid runtime operation append"),
		)
		return
	}
	operation.runtimePrevious = ck.runtimeTail
	if ck.runtimeTail != nil {
		ck.runtimeTail.runtimeNext = operation
	} else {
		ck.runtimeHead = operation
	}
	ck.runtimeTail = operation
	operation.runtimeListed = true
}

func (ck *CommandKernel) unlinkRuntimeOperation(
	operation *commandOperation,
) {
	if operation == nil || !operation.runtimeListed {
		ck.run.Dirty(
			errors.New("jobmgr kernel: invalid runtime operation removal"),
		)
		return
	}
	if operation.runtimePrevious != nil {
		operation.runtimePrevious.runtimeNext = operation.runtimeNext
	} else {
		ck.runtimeHead = operation.runtimeNext
	}
	if operation.runtimeNext != nil {
		operation.runtimeNext.runtimePrevious =
			operation.runtimePrevious
	} else {
		ck.runtimeTail = operation.runtimePrevious
	}
	operation.runtimePrevious = nil
	operation.runtimeNext = nil
	operation.runtimeListed = false
}

func (ck *CommandKernel) observeRuntimeOperations() {
	if ck == nil || ck.runtimeObserver == nil {
		return
	}
	ck.runtimeObserver.SetRuntimeGauge(
		lifecycle.RuntimeGaugeOperationsActive,
		len(ck.operations),
	)
	ck.runtimeObserver.SetRuntimeGauge(
		lifecycle.RuntimeGaugeFunctionInvocationsActive,
		ck.functionOperations,
	)
	var oldest time.Time
	if ck.runtimeHead != nil {
		oldest = ck.runtimeHead.runtimeStarted
	}
	ck.runtimeObserver.SetRuntimeTimestamp(
		lifecycle.RuntimeTimestampOldestOperation,
		oldest,
	)
}

func (ck *CommandKernel) unlinkOperation(operation *commandOperation) {
	if operation == nil || !operation.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid operation-list removal"))
		return
	}
	if operation.allPrevious != nil {
		operation.allPrevious.allNext = operation.allNext
	} else {
		ck.operationHead = operation.allNext
	}
	if operation.allNext != nil {
		operation.allNext.allPrevious = operation.allPrevious
	} else {
		ck.operationTail = operation.allPrevious
	}
	operation.allPrevious = nil
	operation.allNext = nil
	operation.allListed = false
}

func (ck *CommandKernel) completeOperationUID(operation *commandOperation, tombstone bool) error {
	if operation.uidCompleted {
		return errors.New("jobmgr kernel: operation UID completed twice")
	}
	if err := ck.uids.Complete(operation.UID, tombstone, ck.clock.Now()); err != nil {
		return err
	}
	operation.uidCompleted = true
	return nil
}

func (ck *CommandKernel) unlinkQueued(operation *commandOperation, submissionErr error) {
	if operation.Child == lifecycle.ChildDeadlineStartPending {
		ck.run.Dirty(errors.New("jobmgr kernel: required deadline start was unlinked without abandonment"))
		return
	}
	lane := operation.lane
	if lane.ready {
		ck.ready[sourceIndex(lane.source)].remove(lane)
	}
	if operation.fenceBlocked {
		if err := ck.removeCompositeFenceBlocked(operation); err != nil {
			ck.run.Dirty(err)
		}
	}
	if operation.admission.Valid() && !operation.admitted {
		if err := ck.admission.CancelWaiting(operation.admission); err != nil {
			ck.run.Dirty(err)
		} else {
			delete(ck.byAdmission, operation.admission)
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
			ck.settleSubmission(operation, submissionErr)
		}
	}
	if operation.taskRequest.Valid() {
		var err error
		if operation.plan.Resource != nil && operation.plan.Resource.Action == ResourceStop {
			var outcome lifecycle.TaskOutcome
			outcome, err = ck.tasks.CancelPendingOutcome(operation.taskRequest)
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
			outcome, err = ck.tasks.CancelPendingOutcome(
				operation.taskRequest,
			)
			if err == nil {
				err = ck.restoreTransactionOutcome(
					operation,
					outcome,
				)
			}
		} else {
			err = ck.tasks.CancelPending(operation.taskRequest)
		}
		if err != nil {
			ck.run.Dirty(err)
		} else {
			delete(ck.tasksByRequest, operation.taskRequest)
			operation.taskRequest = lifecycle.TaskRequestRef{}
		}
	}
	if operation.claimsInherited {
		operation.claimsHeld = false
	} else if operation.claimsHeld {
		for _, granted := range ck.releaseClaims(operation) {
			ck.markReady(granted.lane)
		}
	} else if ck.claims.waiting(operation) {
		granted, err := ck.claims.cancel(operation)
		if err != nil {
			ck.run.Dirty(err)
		}
		for _, grantedOperation := range granted {
			ck.markReady(grantedOperation.lane)
		}
	} else if operation.claimRegistered {
		if err := ck.claims.abandon(operation); err != nil {
			ck.run.Dirty(err)
		}
	}
	if lane.head == operation {
		ck.removeHead(lane)
	} else {
		ck.unlink(operation)
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if lane.active == nil && lane.head != nil {
		ck.markReady(lane)
	}
}

func (ck *CommandKernel) releaseClaims(operation *commandOperation) []*commandOperation {
	if operation.claimsInherited {
		operation.claimsHeld = false
		return nil
	}
	if !operation.claimsHeld {
		return nil
	}
	granted, err := ck.claims.release(operation)
	if err != nil {
		ck.run.Dirty(err)
		return nil
	}
	return granted
}

func (ck *CommandKernel) removeHead(lane *commandLane) {
	operation := lane.head
	if operation == nil {
		return
	}
	ck.removingLaneOperation(lane, operation)
	lane.head = operation.next
	if lane.head != nil {
		lane.head.previous = nil
	} else {
		lane.tail = nil
	}
	operation.previous = nil
	operation.next = nil
}

func (ck *CommandKernel) unlink(operation *commandOperation) {
	ck.removingLaneOperation(operation.lane, operation)
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

func (ck *CommandKernel) removingLaneOperation(
	lane *commandLane,
	operation *commandOperation,
) {
	if lane == nil || operation == nil ||
		lane.continuationTail != operation {
		return
	}
	previous := operation.previous
	if previous != nil && previous.parent != nil {
		lane.continuationTail = previous
	} else {
		lane.continuationTail = nil
	}
}

func (ck *CommandKernel) markReady(lane *commandLane) {
	if lane == nil ||
		lane.active != nil ||
		lane.head == nil ||
		!lane.head.admitted ||
		ck.claims.waiting(lane.head) {
		return
	}
	lane.source = lane.head.Source
	index := sourceIndex(lane.source)
	ck.ready[index].push(lane)
}

func (ck *CommandKernel) nextReadyLane() *commandLane {
	first := sourceIndex(ck.nextSource)
	second := 1 - first
	if lane := ck.ready[first].pop(); lane != nil {
		ck.nextSource = otherSource(ck.nextSource)
		return lane
	}
	if lane := ck.ready[second].pop(); lane != nil {
		ck.nextSource = otherSource(lane.source)
		return lane
	}
	return nil
}

func (ck *CommandKernel) nextDeadline() time.Time {
	if ck.deadlines.Len() == 0 {
		return time.Time{}
	}
	return ck.deadlines[0].when
}

func (ck *CommandKernel) beginShutdown(deadline time.Time) error {
	if deadline.IsZero() {
		return errors.New("jobmgr kernel: zero shutdown deadline")
	}
	if ck.functionMutationActive || ck.functionMutationPaused {
		var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
		count, abortErr := ck.functionCatalog.AbortMutation(&cleanups)
		pushErr := ck.pushFunctionCleanupBatch(&cleanups, count)
		terminalErr := error(ck.run.StoppingCause())
		if abortErr != nil || pushErr != nil {
			terminalErr = errors.Join(
				terminalErr,
				abortErr,
				pushErr,
			)
		}
		if ck.functionMutation.result != nil {
			ck.functionMutation.result <- functionMutationResult{
				err: terminalErr,
			}
		}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		ck.functionMutationPaused = false
		if abortErr != nil || pushErr != nil {
			return errors.Join(abortErr, pushErr)
		}
	}
	ck.shutdownActive = true
	ck.shutdownOperationCursor = ck.operationHead
	ck.shutdownOperationsDone = ck.shutdownOperationCursor == nil
	ck.shutdownLaneCursor = ck.laneHead
	ck.shutdownLanesDone = ck.shutdownLaneCursor == nil
	if err := ck.tasks.SealInherited(); err != nil {
		return err
	}
	if err := ck.advanceShutdownAdmission(); err != nil {
		return err
	}
	return nil
}

func (ck *CommandKernel) serviceShutdownOperations(
	quantum int,
) (bool, error) {
	if !ck.shutdownActive || quantum <= 0 {
		return false, errors.New("jobmgr kernel: invalid shutdown operation service")
	}
	for visited := 0; visited < quantum &&
		ck.shutdownOperationCursor != nil; visited++ {
		operation := ck.shutdownOperationCursor
		ck.shutdownOperationCursor = operation.allNext
		operation.shutdownVisited = true
		if operation.State == lifecycle.OperationDisposedTerminal {
			ck.unlinkOperation(operation)
			continue
		}
		if err := ck.cancelOperationForShutdown(operation); err != nil {
			return false, err
		}
	}
	ck.shutdownOperationsDone = ck.shutdownOperationCursor == nil
	return !ck.shutdownOperationsDone, nil
}

func (ck *CommandKernel) cancelOperationForShutdown(
	operation *commandOperation,
) error {
	if operation == nil ||
		operation.State == lifecycle.OperationDisposedTerminal {
		return errors.New("jobmgr kernel: invalid shutdown operation")
	}
	operation.cancelled = true
	switch operation.Child {
	case lifecycle.ChildExecuting:
		if operation.plan.Resource == nil ||
			operation.plan.Resource.Action != ResourceStop {
			if err := ck.tasks.CancelWithCause(
				operation.Task,
				ck.run.StoppingCause(),
			); err != nil {
				return err
			}
		}
		if operation.Response != lifecycle.ResponseNotRequired &&
			!operation.plan.CooperativeCancel &&
			!(operation.TimedOut() &&
				requiresCooperativeDeadlineStart(operation)) {
			ck.enqueueControl(operation, cancellationControl(operation))
		}
	case lifecycle.ChildNotStarted:
		ck.unlinkQueued(
			operation,
			ck.run.StoppingCause(),
		)
		if operation.Response != lifecycle.ResponseNotRequired {
			ck.enqueueControl(operation, cancellationControl(operation))
		} else {
			ck.tryDispose(operation)
		}
	case lifecycle.ChildDeadlineStartPending:
		if !operation.taskRequest.Valid() {
			if err := operation.AbandonDeadlineStart(); err != nil {
				return err
			}
			ck.unlinkQueued(
				operation,
				ck.run.StoppingCause(),
			)
			if operation.Response == lifecycle.ResponseOpen {
				ck.enqueueControl(operation, lifecycle.ControlDeadline)
			} else {
				ck.tryDispose(operation)
			}
		}
	case lifecycle.ChildResultReady:
		if operation.resultGrowthWaiting {
			if err := ck.admission.CancelWaiting(
				operation.admission,
			); err != nil {
				return err
			}
			operation.resultGrowthWaiting = false
			ck.sendDisposeAction(operation)
			if operation.Response != lifecycle.ResponseNotRequired {
				ck.enqueueControl(
					operation,
					cancellationControl(operation),
				)
			}
		}
	case lifecycle.ChildActionPending:
		if shutdownCancellablePendingAction(operation) {
			_ = ck.tasks.Cancel(operation.Task)
		}
	case lifecycle.ChildAbandonedBeforeStart,
		lifecycle.ChildActionAcknowledged,
		lifecycle.ChildTerminationPending,
		lifecycle.ChildExitAcknowledged:
	default:
		return errors.New("jobmgr kernel: invalid shutdown child state")
	}
	return nil
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

func (ck *CommandKernel) advanceShutdownAdmission() error {
	grant, waiting, err := ck.admission.TakeShutdownInputBodyGrant(ck.run.Generation())
	if err != nil {
		return err
	}
	if grant.Kind == lifecycle.ReservationInputBodyGrowth {
		select {
		case ck.inputBodyGrants <- grant:
		default:
			return errors.New("jobmgr kernel: input body grant gate is full during shutdown")
		}
	}
	if waiting {
		return nil
	}
	if !ck.shutdownOperationsDone {
		return nil
	}
	return ck.admission.BeginCleanupOnly(ck.run.Generation())
}

func (ck *CommandKernel) serviceShutdownStops(
	quantum int,
) (bool, error) {
	if !ck.shutdownActive || !ck.shutdownBarrierDone || quantum <= 0 {
		return false, errors.New("jobmgr kernel: invalid shutdown lane service")
	}
	for visited := 0; visited < quantum &&
		ck.shutdownLaneCursor != nil; visited++ {
		lane := ck.shutdownLaneCursor
		ck.shutdownLaneCursor = lane.allNext
		lane.shutdownVisited = true
		if err := ck.enqueueShutdownStop(lane); err != nil {
			return false, err
		}
		ck.releaseUnusedLane(lane)
	}
	ck.shutdownLanesDone = ck.shutdownLaneCursor == nil
	return !ck.shutdownLanesDone, nil
}

func (ck *CommandKernel) enqueueShutdownStop(lane *commandLane) error {
	if lane == nil || !lane.allListed {
		return errors.New("jobmgr kernel: invalid shutdown resource lane")
	}
	if lane.currentStopping {
		if lane.current != nil || !lane.currentIdentity.Valid() ||
			lane.retiringIdentity.Valid() {
			return errors.New(
				"jobmgr kernel: shutdown found an invalid stopping resource",
			)
		}
		return nil
	}
	if lane.retiringIdentity.Valid() {
		if lane.current != nil || lane.currentIdentity.Valid() {
			return errors.New(
				"jobmgr kernel: shutdown found an invalid retiring resource",
			)
		}
		return nil
	}
	if lane.current == nil {
		if lane.currentIdentity.Valid() {
			return errors.New(
				"jobmgr kernel: shutdown found a detached current identity",
			)
		}
		return nil
	}
	if lane.owners != 0 {
		return nil
	}
	if lane.head != nil || lane.tail != nil || lane.active != nil || lane.ready {
		return errors.New(
			"jobmgr kernel: owner-free resource lane retains operation state",
		)
	}
	identity := lane.currentIdentity
	if !identity.Valid() || identity.ID != lane.key {
		return errors.New("jobmgr kernel: shutdown found an invalid current resource")
	}
	budget, err := ck.run.BeginShutdown()
	if err != nil {
		return err
	}
	if !lane.resourceSource.Valid() {
		return errors.New(
			"jobmgr kernel: shutdown resource has no scheduling source",
		)
	}
	plan, err := lifecycle.NewShutdownReadyResourceTaskPlan(
		lane.resourceSource,
		budget,
		lifecycle.TransactionTaskPhases,
		lane.current,
		identity,
	)
	if err != nil {
		return err
	}
	request, err := ck.tasks.Enqueue(
		lifecycle.TaskClassFrameworkControl,
		plan,
	)
	if err != nil {
		return err
	}
	if owner := ck.shutdownRequests[request]; owner != nil {
		outcome, cancelErr := ck.tasks.CancelPendingOutcome(request)
		_, ok := outcome.ReadyResource()
		returnedIdentity, identityOK := outcome.ResourceIdentity()
		if cancelErr != nil || !ok || !identityOK ||
			returnedIdentity != identity {
			return errors.Join(
				errors.New("jobmgr kernel: occupied shutdown request owner"),
				cancelErr,
			)
		}
		return errors.New("jobmgr kernel: occupied shutdown request owner")
	}
	lane.current = nil
	lane.currentStopping = true
	lane.shutdownRequest = request
	ck.shutdownRequests[request] = lane
	return nil
}

func (ck *CommandKernel) advanceShutdownBarrier() error {
	if ck.shutdownBarrier == nil ||
		ck.shutdownBarrierDone ||
		ck.shutdownBarrierFailed ||
		ck.shutdownBarrierRequest.Valid() ||
		ck.shutdownBarrierTask.Valid() {
		return nil
	}
	if !ck.shutdownOperationsDone ||
		ck.tasks.InheritedCancellationPending() {
		return nil
	}
	budget, err := ck.run.BeginShutdown()
	if err != nil {
		return err
	}
	plan, err := lifecycle.NewShutdownWorkTaskPlan(
		lifecycle.SourceFunction,
		budget,
		3,
		func(ctx context.Context) (lifecycle.TaskOutcome, error) {
			return lifecycle.NoValueOutcome(),
				ck.shutdownBarrier.BeforeFunctionCatalogClose(
					ctx,
					ck.run.Generation(),
				)
		},
	)
	if err != nil {
		return err
	}
	request, err := ck.tasks.Enqueue(
		lifecycle.TaskClassFrameworkControl,
		plan,
	)
	if err != nil {
		return err
	}
	ck.shutdownBarrierRequest = request
	return nil
}

func isNoopRunShutdownBarrier(barrier RunShutdownBarrier) bool {
	_, ok := barrier.(noopRunShutdownBarrier)
	return ok
}

func (ck *CommandKernel) completeShutdownBarrier(
	completion lifecycle.TaskCompletion,
) {
	if completion.Sequence != 1 ||
		completion.Kind != lifecycle.TaskOutcomeNone ||
		ck.shutdownBarrierAction != 0 ||
		ck.shutdownBarrierDone ||
		ck.shutdownBarrierFailed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier completion"))
		return
	}
	if completion.Err != nil {
		ck.shutdownBarrierFailed = true
		ck.run.Dirty(completion.Err)
	}
	ck.shutdownBarrierAction = lifecycle.TaskActionTerminate
	if err := ck.tasks.SendAction(lifecycle.TaskAction{
		Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate,
	}); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) acknowledgeShutdownBarrier(
	ack lifecycle.TaskAcknowledgement,
) {
	if ack.Sequence != 2 ||
		ack.Kind != lifecycle.TaskActionTerminate ||
		ck.shutdownBarrierAction != lifecycle.TaskActionTerminate ||
		ck.shutdownBarrierDone {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier acknowledgement"))
		return
	}
	if ack.Err != nil {
		ck.shutdownBarrierFailed = true
		ck.run.Dirty(ack.Err)
	}
	if err := ck.tasks.Release(ack.Ref); err != nil {
		ck.shutdownBarrierFailed = true
		ck.run.Dirty(err)
		return
	}
	ck.shutdownBarrierTask = lifecycle.TaskRef{}
	ck.shutdownBarrierAction = 0
	if !ck.shutdownBarrierFailed {
		ck.shutdownBarrierDone = true
	}
}

func (ck *CommandKernel) runShutdownBarrierFailedTerminal() bool {
	return ck.shutdownBarrierFailed &&
		!ck.shutdownBarrierRequest.Valid() &&
		!ck.shutdownBarrierTask.Valid() &&
		ck.shutdownBarrierAction == 0
}

func (ck *CommandKernel) advanceRunFinalizer() error {
	if ck.finalizer == nil || ck.finalizerDone || ck.finalizerFailed || ck.finalizerRequest.Valid() || ck.finalizerTask.Valid() {
		return nil
	}
	if !ck.shutdownReadyForFinalizer() {
		return nil
	}
	budget, err := ck.run.BeginShutdown()
	if err != nil {
		return err
	}
	plan, err := lifecycle.NewShutdownWorkTaskPlan(
		lifecycle.SourceJobManager, budget, 3,
		func(ctx context.Context) (lifecycle.TaskOutcome, error) {
			return lifecycle.NoValueOutcome(), ck.finalizer.FinalizeRun(ctx, ck.run.Generation())
		},
	)
	if err != nil {
		return err
	}
	request, err := ck.tasks.Enqueue(
		lifecycle.TaskClassFrameworkControl,
		plan,
	)
	if err != nil {
		return err
	}
	ck.finalizerRequest = request
	return nil
}

func (ck *CommandKernel) shutdownReadyForFinalizer() bool {
	if !ck.shutdownBarrierDone || ck.shutdownBarrierFailed ||
		ck.shutdownBarrierRequest.Valid() ||
		ck.shutdownBarrierTask.Valid() ||
		ck.shutdownBarrierAction != 0 {
		return false
	}
	inherited := ck.tasks.InheritedCensus()
	longLived := ck.tasks.LongLivedCensus()
	if !ck.shutdownOperationsDone || !ck.shutdownLanesDone ||
		ck.tasks.InheritedCancellationPending() ||
		len(ck.operations) != 0 || len(ck.tasksByRef) != 0 ||
		len(ck.tasksByRequest) != 0 ||
		ck.runtimeHead != nil || ck.runtimeTail != nil ||
		ck.functionOperations != 0 ||
		ck.operationHead != nil || ck.operationTail != nil ||
		ck.laneHead != nil || ck.laneTail != nil ||
		len(ck.functionCleanupTasks) != 0 || len(ck.functionCleanupRequests) != 0 ||
		ck.functionCleanupBacklog.count != 0 ||
		ck.functionMutationActive || ck.functionMutationPaused ||
		len(ck.byAdmission) != 0 || len(ck.lanes) != 0 ||
		len(ck.compositeFenceClaims) != 0 ||
		ck.compositeFenceHead != nil ||
		ck.compositeFenceTail != nil ||
		ck.compositeFenceCount != 0 ||
		ck.compositeFenceRecheck ||
		ck.tasks.Active() != 0 || ck.tasks.Pending() != 0 || inherited.Active != 0 ||
		len(ck.shutdownRequests) != 0 || len(ck.shutdownTasks) != 0 || len(ck.controls) != 0 || ck.deadlines.Len() != 0 ||
		len(ck.submissions[0]) != 0 || len(ck.submissions[1]) != 0 || ck.blockedSubmission[0] || ck.blockedSubmission[1] ||
		ck.ready[0].len != 0 || ck.ready[1].len != 0 || ck.claims.waitingCount() != 0 || len(ck.claims.keys) != 0 {
		return false
	}
	if !ck.functionCatalogDrained() {
		return false
	}
	if longLived.Active != longLived.FinalizerOwnedActive || longLived.Bytes != longLived.FinalizerOwnedBytes {
		return false
	}
	return ck.admission.RunFinalizerReady(ck.run.Generation(), longLived.FinalizerOwnedRecords, longLived.FinalizerOwnedBytes)
}

func isNoopRunFinalizer(finalizer RunFinalizer) bool {
	_, ok := finalizer.(noopRunFinalizer)
	return ok
}

func (ck *CommandKernel) completeRunFinalizer(completion lifecycle.TaskCompletion) {
	if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone || ck.finalizerAction != 0 || ck.finalizerDone || ck.finalizerFailed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer completion"))
		return
	}
	if completion.Err != nil {
		ck.finalizerFailed = true
		ck.run.Dirty(completion.Err)
	}
	ck.finalizerAction = lifecycle.TaskActionTerminate
	if err := ck.tasks.SendAction(lifecycle.TaskAction{Ref: completion.Ref, Sequence: 2, Kind: lifecycle.TaskActionTerminate}); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) acknowledgeRunFinalizer(ack lifecycle.TaskAcknowledgement) {
	if ack.Sequence != 2 || ack.Kind != lifecycle.TaskActionTerminate || ck.finalizerAction != lifecycle.TaskActionTerminate || ck.finalizerDone {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer acknowledgement"))
		return
	}
	if ack.Err != nil {
		ck.finalizerFailed = true
		ck.run.Dirty(ack.Err)
	}
	if err := ck.tasks.Release(ack.Ref); err != nil {
		ck.finalizerFailed = true
		ck.run.Dirty(err)
		return
	}
	ck.finalizerTask = lifecycle.TaskRef{}
	ck.finalizerAction = 0
	if !ck.finalizerFailed {
		ck.finalizerDone = true
	}
}

func (ck *CommandKernel) runFinalizerFailedTerminal() bool {
	return ck.finalizerFailed && !ck.finalizerRequest.Valid() && !ck.finalizerTask.Valid() && ck.finalizerAction == 0
}

func (ck *CommandKernel) shutdownQuiescent() bool {
	if !ck.shutdownBarrierDone || ck.shutdownBarrierFailed ||
		ck.shutdownBarrierRequest.Valid() ||
		ck.shutdownBarrierTask.Valid() ||
		ck.shutdownBarrierAction != 0 {
		return false
	}
	inherited := ck.tasks.InheritedCensus()
	longLived := ck.tasks.LongLivedCensus()
	frame := ck.frames.Census()
	if !ck.shutdownOperationsDone || !ck.shutdownLanesDone ||
		ck.tasks.InheritedCancellationPending() ||
		len(ck.operations) != 0 || len(ck.tasksByRef) != 0 ||
		len(ck.tasksByRequest) != 0 ||
		ck.operationHead != nil || ck.operationTail != nil ||
		ck.laneHead != nil || ck.laneTail != nil ||
		len(ck.functionCleanupTasks) != 0 || len(ck.functionCleanupRequests) != 0 ||
		ck.functionCleanupBacklog.count != 0 ||
		ck.functionMutationActive || ck.functionMutationPaused ||
		len(ck.byAdmission) != 0 || len(ck.lanes) != 0 ||
		len(ck.compositeFenceClaims) != 0 ||
		ck.compositeFenceHead != nil ||
		ck.compositeFenceTail != nil ||
		ck.compositeFenceCount != 0 ||
		ck.compositeFenceRecheck ||
		ck.tasks.Active() != 0 || ck.tasks.Pending() != 0 || len(ck.shutdownRequests) != 0 || len(ck.shutdownTasks) != 0 || len(ck.controls) != 0 || ck.deadlines.Len() != 0 ||
		len(ck.submissions[0]) != 0 || len(ck.submissions[1]) != 0 || ck.blockedSubmission[0] || ck.blockedSubmission[1] ||
		ck.ready[0].len != 0 || ck.ready[1].len != 0 || ck.claims.waitingCount() != 0 || len(ck.claims.keys) != 0 {
		return false
	}
	return ck.functionCatalogDrained() &&
		ck.admission.RunDrained(ck.run.Generation()) && inherited.Active == 0 &&
		longLived == (lifecycle.LongLivedCensus{}) && !frame.Poisoned &&
		!frame.Busy && !frame.PendingControl && frame.RetainedBytes == 0 &&
		ck.finalizerDone && !ck.finalizerFailed
}

func (ck *CommandKernel) functionCatalogDrained() bool {
	census := ck.functionCatalog.LifecycleCensus()
	return census.Closed && !census.MutationActive && census.Routes == 0 &&
		census.CloseRoutesPending == 0 && census.InvocationLeases == 0 &&
		census.PendingCleanups == 0
}

func (ck *CommandKernel) runCensus() lifecycle.RunCensus {
	return lifecycle.RunCensus{
		AdmissionRunDrained: ck.admission.RunDrained(ck.run.Generation()),
		Admission:           ck.admission.Census(), TransientActive: ck.tasks.Active(), TransientPending: ck.tasks.Pending(),
		Inherited: ck.tasks.InheritedCensus(), LongLived: ck.tasks.LongLivedCensus(),
		Frame:                ck.frames.Census(),
		RunFinalizerComplete: ck.finalizerDone && !ck.finalizerFailed,
	}
}

func (ck *CommandKernel) releaseFunctionInvocation(ref FunctionInvocationRef) error {
	cleanup, err := ck.functionCatalog.ReleaseInvocation(ref)
	if err != nil {
		return err
	}
	return ck.functionCleanupBacklog.push(cleanup)
}

func (ck *CommandKernel) pushFunctionCleanupBatch(cleanups *[MaximumFunctionCleanupBatch]FunctionCleanupPlan, count int) error {
	if cleanups == nil || count < 0 || count > len(cleanups) {
		return errors.New("jobmgr kernel: invalid Function cleanup batch")
	}
	for index := range count {
		if err := ck.functionCleanupBacklog.push(cleanups[index]); err != nil {
			return err
		}
		cleanups[index] = FunctionCleanupPlan{}
	}
	return nil
}

func (ck *CommandKernel) serviceFunctionCleanupBacklog(quantum int) bool {
	if quantum <= 0 {
		return ck.functionCleanupBacklog.count != 0
	}
	for quantum > 0 && ck.functionCleanupBacklog.count != 0 {
		cleanup := ck.functionCleanupBacklog.front()
		request, err := ck.tasks.Enqueue(
			lifecycle.TaskClassFrameworkControl,
			lifecycle.TaskPlan{
				Source: lifecycle.SourceFunction,
				Work:   cleanup.Work,
				Runner: cleanup.Runner,
			},
		)
		if err != nil {
			ck.run.Dirty(err)
			return true
		}
		if _, exists := ck.functionCleanupRequests[request]; exists {
			ck.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup request"))
			return true
		}
		ck.functionCleanupRequests[request] = cleanup.Ref
		ck.functionCleanupBacklog.pop()
		quantum--
	}
	return ck.functionCleanupBacklog.count != 0
}

func (ck *CommandKernel) beginFunctionMutation(submitted functionMutationSubmission) {
	if submitted.mutation == nil || submitted.result == nil ||
		submitted.action < functionMutationQuiesce ||
		submitted.action > functionMutationAbort ||
		ck.functionMutationActive || ck.functionCatalogClosing ||
		!ck.run.Admitting() {
		if submitted.result != nil {
			submitted.result <- functionMutationResult{err: errors.New("jobmgr kernel: Function mutation admission closed")}
		}
		return
	}
	switch submitted.action {
	case functionMutationQuiesce:
		if ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation already quiesced",
				),
			}
			return
		}
		if err := ck.functionCatalog.BeginMutation(
			submitted.mutation,
		); err != nil {
			submitted.result <- functionMutationResult{err: err}
			return
		}
	case functionMutationCommit:
		if !ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation is not quiesced",
				),
			}
			return
		}
		if err := ck.functionCatalog.ResumeMutation(
			submitted.mutation,
		); err != nil {
			submitted.result <- functionMutationResult{err: err}
			return
		}
		ck.functionMutationPaused = false
	case functionMutationAbort:
		if !ck.functionMutationPaused {
			submitted.result <- functionMutationResult{
				err: errors.New(
					"jobmgr kernel: Function mutation is not quiesced",
				),
			}
			return
		}
		if err := ck.functionCatalog.ResumeMutation(
			submitted.mutation,
		); err != nil {
			submitted.result <- functionMutationResult{err: err}
			return
		}
		var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
		count, abortErr := ck.functionCatalog.AbortMutation(&cleanups)
		pushErr := ck.pushFunctionCleanupBatch(&cleanups, count)
		submitted.result <- functionMutationResult{
			err: errors.Join(abortErr, pushErr),
		}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationPaused = false
		if abortErr != nil || pushErr != nil {
			ck.run.Dirty(errors.Join(abortErr, pushErr))
		}
		return
	}
	ck.functionMutation = submitted
	ck.functionMutationActive = true
}

func (ck *CommandKernel) serviceFunctionMutation(quantum int) bool {
	if !ck.functionMutationActive {
		return false
	}
	if ck.functionMutation.action == functionMutationQuiesce {
		progress, err := ck.functionCatalog.AdvanceMutationQuiesce(
			quantum,
		)
		if err != nil {
			var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
			abortCount, abortErr := ck.functionCatalog.AbortMutation(
				&cleanups,
			)
			if pushErr := ck.pushFunctionCleanupBatch(
				&cleanups,
				abortCount,
			); pushErr != nil {
				abortErr = errors.Join(abortErr, pushErr)
			}
			ck.functionMutation.result <- functionMutationResult{
				err: errors.Join(err, abortErr),
			}
			ck.functionMutation = functionMutationSubmission{}
			ck.functionMutationActive = false
			if abortErr != nil {
				ck.run.Dirty(abortErr)
			}
			return false
		}
		if !progress.Quiesced {
			return true
		}
		ck.functionMutation.result <- functionMutationResult{
			version: progress.Version,
		}
		ck.functionMutation.result = nil
		ck.functionMutationActive = false
		ck.functionMutationPaused = true
		return false
	}
	if ck.functionMutation.action != functionMutationCommit {
		invariantErr := errors.New(
			"jobmgr kernel: invalid active Function mutation",
		)
		var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
		abortCount, abortErr := ck.functionCatalog.AbortMutation(&cleanups)
		if pushErr := ck.pushFunctionCleanupBatch(
			&cleanups,
			abortCount,
		); pushErr != nil {
			abortErr = errors.Join(abortErr, pushErr)
		}
		ck.functionMutation.result <- functionMutationResult{
			err: errors.Join(invariantErr, abortErr),
		}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		ck.run.Dirty(errors.Join(invariantErr, abortErr))
		return false
	}
	var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
	progress, count, err := ck.functionCatalog.AdvanceMutation(quantum, &cleanups)
	if err != nil {
		abortCount, abortErr := ck.functionCatalog.AbortMutation(&cleanups)
		if pushErr := ck.pushFunctionCleanupBatch(&cleanups, abortCount); pushErr != nil {
			abortErr = errors.Join(abortErr, pushErr)
		}
		ck.functionMutation.result <- functionMutationResult{err: errors.Join(err, abortErr)}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		if abortErr != nil {
			ck.run.Dirty(abortErr)
		}
		return false
	}
	if err := ck.pushFunctionCleanupBatch(&cleanups, count); err != nil {
		ck.functionMutation.result <- functionMutationResult{err: err}
		ck.functionMutation = functionMutationSubmission{}
		ck.functionMutationActive = false
		ck.run.Dirty(err)
		return false
	}
	if !progress.Done {
		return true
	}
	ck.functionMutation.result <- functionMutationResult{version: progress.Version}
	ck.functionMutation = functionMutationSubmission{}
	ck.functionMutationActive = false
	return false
}

func (ck *CommandKernel) serviceFunctionCatalogClose(quantum int) bool {
	if !ck.shutdownBarrierDone || ck.shutdownBarrierFailed {
		return false
	}
	if !ck.functionCatalogClosing {
		if err := ck.functionCatalog.BeginClose(); err != nil {
			ck.run.Dirty(err)
			return false
		}
		ck.functionCatalogClosing = true
		ck.functionCatalogCloseMore = true
	}
	if !ck.functionCatalogCloseMore {
		return false
	}
	var cleanups [MaximumFunctionCleanupBatch]FunctionCleanupPlan
	count, more, err := ck.functionCatalog.CloseStep(quantum, &cleanups)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	if err := ck.pushFunctionCleanupBatch(&cleanups, count); err != nil {
		ck.run.Dirty(err)
		return false
	}
	ck.functionCatalogCloseMore = more
	return more
}

func (ck *CommandKernel) allocateLane(mapKey commandLaneKey, request Request) (*commandLane, error) {
	slot := ck.freeLane
	if slot == 0 {
		if uint64(len(ck.laneSlots)) > uint64(^uint32(0)) {
			return nil, errors.New("jobmgr kernel: lane reference space exhausted")
		}
		slot = uint32(len(ck.laneSlots))
		ck.laneSlots = append(ck.laneSlots, &commandLane{slot: slot})
	}
	lane := ck.laneSlots[slot]
	if ck.freeLane != 0 {
		ck.freeLane = lane.freeNext
	}
	generation := lane.generation + 1
	if generation == 0 {
		lane.freeNext = ck.freeLane
		ck.freeLane = slot
		return nil, errors.New("jobmgr kernel: lane generation wrapped")
	}
	*lane = commandLane{
		slot: slot, generation: generation, mapKey: mapKey,
		key: request.LaneKey, source: request.Source,
	}
	ck.lanes[mapKey] = lane
	ck.appendLane(lane)
	return lane, nil
}

func resourceCommandLaneKey(id string) commandLaneKey {
	return commandLaneKey{key: id, resource: true}
}

func (ck *CommandKernel) releaseUnusedLane(lane *commandLane) {
	if lane == nil || lane.owners != 0 || lane.head != nil ||
		lane.tail != nil || lane.active != nil ||
		lane.continuationTail != nil || lane.ready ||
		lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping || lane.retiringIdentity.Valid() ||
		lane.installPlanned || lane.stopPlanned ||
		lane.shutdownRequest.Valid() || lane.shutdownTask.Valid() || lane.shutdownAction != 0 {
		return
	}
	if ck.shutdownActive && !lane.shutdownVisited {
		return
	}
	delete(ck.lanes, lane.mapKey)
	ck.unlinkLane(lane)
	slot := lane.slot
	generation := lane.generation
	*lane = commandLane{slot: slot, generation: generation, freeNext: ck.freeLane}
	ck.freeLane = slot
}

func (ck *CommandKernel) appendLane(lane *commandLane) {
	if lane == nil || lane.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid lane-list append"))
		return
	}
	lane.allPrevious = ck.laneTail
	if ck.laneTail != nil {
		ck.laneTail.allNext = lane
	} else {
		ck.laneHead = lane
	}
	ck.laneTail = lane
	lane.allListed = true
}

func (ck *CommandKernel) unlinkLane(lane *commandLane) {
	if lane == nil || !lane.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid lane-list removal"))
		return
	}
	if lane.allPrevious != nil {
		lane.allPrevious.allNext = lane.allNext
	} else {
		ck.laneHead = lane.allNext
	}
	if lane.allNext != nil {
		lane.allNext.allPrevious = lane.allPrevious
	} else {
		ck.laneTail = lane.allPrevious
	}
	lane.allPrevious = nil
	lane.allNext = nil
	lane.allListed = false
}

const (
	// This covers aggregate framework ownership for one ordinary operation.
	operationFrameworkAdmissionBytes = int64(4_608)
)

func operationAdmissionBytes(request Request, plan WorkPlan) (int64, error) {
	bytes := operationFrameworkAdmissionBytes
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
		if !validPersistentAdmission(
			plan.Resource.Permit,
			lifecycle.OrdinaryBudgetBytes-bytes,
		) {
			return 0, errors.New("jobmgr kernel: long-lived resource does not self-fit admission")
		}
		bytes += persistent
	}
	if plan.Transaction != nil && plan.Transaction.AllocateSuccessor {
		persistent := plan.Transaction.Permit.Bytes()
		if !validPersistentAdmission(
			plan.Transaction.Permit,
			lifecycle.OrdinaryBudgetBytes-bytes,
		) {
			return 0, errors.New(
				"jobmgr kernel: transaction successor does not self-fit admission",
			)
		}
		bytes += persistent
	}
	if plan.Capability != nil {
		persistent := plan.Capability.Permit.Bytes()
		if !validPersistentAdmission(
			plan.Capability.Permit,
			lifecycle.OrdinaryBudgetBytes-bytes,
		) {
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

func validPersistentAdmission(
	plan lifecycle.LongLivedPlan,
	available int64,
) bool {
	if available < 0 {
		return false
	}
	switch plan.Class() {
	case lifecycle.LongLivedPipeline:
		return plan.Bytes() == 0
	case lifecycle.LongLivedJob, lifecycle.LongLivedSecretStore:
		return plan.Bytes() > 0 && plan.Bytes() <= available
	default:
		return false
	}
}

func (ck *CommandKernel) abortRequestInputBody(request Request) error {
	if request.InputBodyToken == 0 {
		return nil
	}
	wake, err := ck.admission.AbortInputBody(request.InputBodyToken)
	if wake {
		ck.NotifyControlReady()
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
