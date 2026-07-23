// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const externalSourceQueueDepth = 32

const (
	maximumClaimKeyBytes = maximumRequestArgumentBytes
	// Keep lifecycle-event service capacity at least equal to the maximum
	// phase work introduced by one task-start quantum.
	asyncEventServiceQuantum = lifecycle.TaskStartServiceQuantum * lifecycle.TransactionTaskPhases
)

var ErrStopped = errors.New("jobmgr kernel: stopped")

type submission struct {
	request       Request                 // the command being submitted
	plan          WorkPlan                // prepared work for the command
	context       context.Context         // caller context
	composite     *kernelCompositeScope   // parent composite scope for a child submission
	rollback      bool                    // submission is a composite rollback
	controlStatus lifecycle.ControlStatus // pre-decided control status (rejections)
	result        chan error              // channel released once admitted
	terminal      chan error              // channel delivering the terminal disposition
}

type commandLaneKey struct {
	key                string                // resource/lane key that identifies the lane
	functionInvocation FunctionInvocationRef // unique invocation ref for a resource-less Function call
	source             lifecycle.Source      // source that scheduled the command
	resource           bool                  // true for a resource lane, false for a Function-invocation lane
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

type commandShutdownPhase uint8

const (
	commandShutdownRunning commandShutdownPhase = iota
	commandShutdownActionDrain
	commandShutdownCleanupDrain
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

// commandOperation is one command's full lifecycle record, owned exclusively by
// CommandKernel's run loop (the sole mutator of every field below).
type commandOperation struct {
	*lifecycle.OperationGeneration                                    // embedded neutral lifecycle state machine (state, response, child)
	request                        Request                            // immutable admitted command
	plan                           WorkPlan                           // prepared work for the command
	claims                         []string                           // normalized claim set (sorted, deduped)
	authorityClaimEdges            []authorityClaimEdge               // per-claim edge state in the claim authority
	claimCursor                    int                                // index of the next claim edge to acquire
	claimTicket                    uint64                             // global FIFO ticket for cross-key settlement fairness
	claimPrepared                  bool                               // claim edges have been built
	claimRegistered                bool                               // claims registered with the authority
	claimWaiting                   bool                               // parked waiting on a contested claim
	claimsHeld                     bool                               // all claims acquired and held
	claimWaitStarted               time.Time                          // time the current claim wait began (oldest-wait metric)
	claimWaitPrevious              *commandOperation                  // previous op in the claim wait list
	claimWaitNext                  *commandOperation                  // next op in the claim wait list
	claimWaitListed                bool                               // linked into the claim wait list
	lane                           *commandLane                       // owning lane
	previous                       *commandOperation                  // previous op in the lane FIFO
	next                           *commandOperation                  // next op in the lane FIFO
	allPrevious                    *commandOperation                  // previous op in the all-operations list
	allNext                        *commandOperation                  // next op in the all-operations list
	allListed                      bool                               // linked into the all-operations list
	runtimeStarted                 time.Time                          // time this op entered the runtime-metrics list
	runtimePrevious                *commandOperation                  // previous op in the runtime-metrics list
	runtimeNext                    *commandOperation                  // next op in the runtime-metrics list
	runtimeListed                  bool                               // linked into the runtime-metrics list
	shutdownVisited                bool                               // already swept by the shutdown cancel cursor
	control                        lifecycle.ControlStatus            // pending terminal control status (error/cancel/deadline)
	controlQueued                  bool                               // queued in the pending-control FIFO
	cleanupDone                    bool                               // post-terminal cleanup has run
	uidCompleted                   bool                               // UID released from the dedupe ledger
	cancelled                      bool                               // user/deadline/shutdown cancellation requested
	functionInvocation             FunctionInvocationRef              // Function catalog invocation lease (Function commands only)
	transactionScope               lifecycle.ResourceTransactionScope // current/successor identities a resource transaction may touch
	transactionApplied             bool                               // the transaction's ownership-changing apply has started
	transactionRestored            bool                               // a failed transaction's predecessor was restored
	deadline                       deadlineEntry                      // deadline heap entry
	resultExpiry                   int64                              // expiry stamped into the result frame
	taskRequest                    lifecycle.TaskRequestRef           // off-loop task request ref
	terminalResult                 chan error                         // channel that delivers the terminal disposition
	terminalErr                    error                              // terminal error, if any
	parent                         *commandOperation                  // parent composite operation, if this is a child
	composite                      *kernelCompositeScope              // composite scope this op owns (parent side)
	activeChild                    *commandOperation                  // child composite command currently executing
	fencePrevious                  *commandOperation                  // previous op in the composite fence list
	fenceNext                      *commandOperation                  // next op in the composite fence list
	fenceBlocked                   bool                               // blocked by an active composite fence
	fenceChecked                   uint64                             // fence generation last checked against
	deferredCompletion             *lifecycle.TaskCompletion          // task completion deferred until the op can accept it
	claimsInherited                bool                               // claims inherited from the parent composite (not self-registered)
	compositeRollback              bool                               // op is executing a composite rollback
	ownershipChain                 bool                               // part of a protected ownership chain (survives the first shutdown cut)
	shutdownChild                  bool                               // child spawned during shutdown drain
}

// commandLane serializes every command addressed to one resource/invocation key
// into a FIFO; CommandKernel's run loop is its sole mutator.
type commandLane struct {
	mapKey             commandLaneKey             // key this lane is registered under in the lanes map
	owners             int                        // refcount of operations currently referencing the lane
	freeNext           *commandLane               // next recycled lane while on the freelist
	key                string                     // resource/invocation identity this lane serializes
	source             lifecycle.Source           // ingress source (JobManager vs Function) that owns the lane
	head               *commandOperation          // first queued operation (FIFO head)
	tail               *commandOperation          // last queued operation (FIFO tail)
	active             *commandOperation          // operation currently dispatched as a task; nil when idle
	continuationTail   *commandOperation          // insertion anchor for composite child continuations
	ready              bool                       // lane is enqueued in a ready queue awaiting scheduling
	readyPrev          *commandLane               // previous lane in the ready queue
	readyNext          *commandLane               // next lane in the ready queue
	allPrevious        *commandLane               // previous lane in the all-lanes list
	allNext            *commandLane               // next lane in the all-lanes list
	allListed          bool                       // linked into the all-lanes list
	shutdownVisited    bool                       // lane already swept by the shutdown cursor
	resourceSource     lifecycle.Source           // source that scheduled the current resource (for shutdown stop)
	currentIdentity    lifecycle.ResourceIdentity // identity of the installed resource; Valid once installed
	current            lifecycle.ReadyResource    // live ReadyResource published on this lane; nil while stopping/absent
	currentStopping    bool                       // a stop/transaction detached current; a Stop task is in flight
	retiringIdentity   lifecycle.ResourceIdentity // resource stopped, finalize pending (exclusive with current)
	transactionPlanned int                        // count of in-flight resource transactions on the lane
	shutdownRequest    lifecycle.TaskRequestRef   // task-request ref for the lane's shutdown stop
	shutdownTask       lifecycle.TaskRef          // task ref for the lane's shutdown stop
	shutdownAction     lifecycle.TaskActionKind   // task action kind driving the shutdown stop
}

// CommandKernel owns all mutable orchestration state for one run generation.
// Every field below is read and written ONLY on the single run-loop goroutine
// (see runLoop); external callers interact through channels, never these fields.
type CommandKernel struct {
	run                      *lifecycle.RunSupervisor                        // run supervisor (admission gate, stopping cut, shutdown budget, dirty state)
	uids                     *lifecycle.UIDLedger                            // UID dedupe ledger
	tasks                    *lifecycle.TaskSupervisor                       // off-loop task supervisor
	frames                   *lifecycle.FrameOwner                           // the one stdout frame writer
	clock                    lifecycle.Clock                                 // logical/real clock
	claims                   *claimAuthority                                 // cross-resource claim authority
	submissions              [2]chan submission                              // per-source inbound submission queues
	submissionSpace          [2]chan struct{}                                // per-source backpressure signal
	submissionStopped        chan struct{}                                   // closed when submission ingress is drained
	continuationStopped      chan struct{}                                   // closed when composite-continuation ingress is drained
	submissionMu             sync.Mutex                                      // guards the submission close/blocked bookkeeping
	submissionClosed         bool                                            // external submission ingress is closed
	continuationClosed       bool                                            // composite-continuation ingress is closed
	blockedSubmissions       [2]submission                                   // one held-back submission per source awaiting space
	hasBlockedSubmission     [2]bool                                         // whether a blocked submission is held, per source
	cancel                   chan string                                     // inbound cancellation requests (by UID)
	wake                     chan struct{}                                   // coalescing 'work exists' nudge
	stop                     chan struct{}                                   // requests loop stop
	done                     chan struct{}                                   // closed when the loop exits
	doneErr                  error                                           // loop terminal error
	startOnce                sync.Once                                       // guards Start
	startErr                 error                                           // error captured by Start
	stopOnce                 sync.Once                                       // guards Stop
	operations               map[string]*commandOperation                    // all live operations by UID
	tasksByRef               map[lifecycle.TaskRef]*commandOperation         // operations keyed by their active task ref
	tasksByRequest           map[lifecycle.TaskRequestRef]*commandOperation  // operations keyed by their task-request ref
	functionCleanupTasks     map[lifecycle.TaskRef]functionCleanupTask       // in-flight Function cleanup tasks by task ref
	functionCleanupRequests  map[lifecycle.TaskRequestRef]FunctionCleanupRef // in-flight Function cleanup requests by request ref
	functionCleanupBacklog   functionCleanupQueue                            // queued Function cleanup work awaiting dispatch
	functionMutations        chan functionMutationSubmission                 // inbound Function catalog mutation submissions
	functionMutationStopped  chan struct{}                                   // closed when mutation ingress is drained
	functionMutation         functionMutationSubmission                      // the mutation currently being applied
	functionMutationActive   bool                                            // a catalog mutation is in progress
	functionMutationPaused   bool                                            // mutation processing is paused (draining)
	functionCatalogClosing   bool                                            // the Function catalog is closing
	functionCatalogCloseMore bool                                            // more catalog-close work remains
	shutdownRequests         map[lifecycle.TaskRequestRef]*commandLane       // lanes awaiting a shutdown stop, by request ref
	shutdownTasks            map[lifecycle.TaskRef]*commandLane              // lanes with an in-flight shutdown stop, by task ref
	shutdownBarrier          RunShutdownBarrier                              // run shutdown barrier (withdraw publications, close catalog)
	shutdownBarrierRequest   lifecycle.TaskRequestRef                        // shutdown barrier task-request ref
	shutdownBarrierTask      lifecycle.TaskRef                               // shutdown barrier task ref
	shutdownBarrierAction    lifecycle.TaskActionKind                        // shutdown barrier task action kind
	shutdownBarrierDone      bool                                            // barrier completed (or was a noop)
	shutdownBarrierFailed    bool                                            // barrier failed
	finalizer                RunFinalizer                                    // run finalizer callback
	finalizerRequest         lifecycle.TaskRequestRef                        // finalizer task-request ref
	finalizerTask            lifecycle.TaskRef                               // finalizer task ref
	finalizerAction          lifecycle.TaskActionKind                        // finalizer task action kind
	finalizerDone            bool                                            // finalizer completed (or was a noop)
	finalizerFailed          bool                                            // finalizer failed
	lanes                    map[commandLaneKey]*commandLane                 // active lanes by key
	freeLane                 *commandLane                                    // head of the recycled-lane freelist
	ready                    [2]readyQueue                                   // per-source ready-lane queues
	nextID                   lifecycle.OperationID                           // next operation ID to assign
	nextResourceGeneration   uint64                                          // next resource generation to assign
	nextSource               lifecycle.Source                                // round-robin cursor for ready-lane scheduling
	nextExternalSource       lifecycle.Source                                // round-robin cursor for submission draining
	nextAsyncEvent           uint8                                           // round-robin cursor across async event sources
	deadlines                deadlineHeap                                    // operation deadline min-heap
	controls                 fixedChunkQueue[*commandOperation]              // pending terminal-control FIFO
	operationHead            *commandOperation                               // head of the all-operations list
	operationTail            *commandOperation                               // tail of the all-operations list
	compositeFenceClaims     map[string]int                                  // active composite fences per claim key
	compositeFenceHead       *commandOperation                               // head of the composite fence list
	compositeFenceTail       *commandOperation                               // tail of the composite fence list
	compositeFenceCount      int                                             // number of fenced operations
	compositeFenceGeneration uint64                                          // bumped when the fence set changes
	compositeFenceRecheck    bool                                            // fenced waiters need re-evaluation
	laneHead                 *commandLane                                    // head of the all-lanes list
	laneTail                 *commandLane                                    // tail of the all-lanes list
	shutdownPhase            commandShutdownPhase                            // current shutdown phase
	shutdownCancelCursor     *commandOperation                               // cursor sweeping operations to cancel during shutdown
	ownershipChains          int                                             // count of protected ownership chains still running
	shutdownLaneCursor       *commandLane                                    // cursor sweeping lanes to stop during shutdown
	functionCatalog          FunctionCatalogPort                             // Function catalog port (routing + lifecycle drain)
	runtimeObserver          lifecycle.RuntimeObserver                       // sink for jobmgr.runtime metric atomics
	diagnosticObserver       DiagnosticObserver                              // operational log sink
	runtimeHead              *commandOperation                               // head of the runtime-metrics op list
	runtimeTail              *commandOperation                               // tail of the runtime-metrics op list
	functionOperations       int                                             // count of active Function operations
}

func NewCommandKernel(
	run *lifecycle.RunSupervisor,
	uids *lifecycle.UIDLedger,
	tasks *lifecycle.TaskSupervisor,
	frames *lifecycle.FrameOwner,
	clock lifecycle.Clock,
	shutdownBarrier RunShutdownBarrier,
	finalizer RunFinalizer,
	functionCatalog FunctionCatalogPort,
) (*CommandKernel, error) {
	if run == nil || uids == nil || tasks == nil || frames == nil || clock == nil || shutdownBarrier == nil ||
		finalizer == nil {
		return nil, errors.New("jobmgr kernel: incomplete lifecycle capabilities")
	}
	if functionCatalog == nil {
		return nil, errors.New("jobmgr kernel: incomplete command planning ports")
	}
	kernel := &CommandKernel{
		run:                     run,
		uids:                    uids,
		tasks:                   tasks,
		frames:                  frames,
		clock:                   clock,
		claims:                  newClaimAuthority(),
		cancel:                  make(chan string),
		wake:                    make(chan struct{}, 1),
		stop:                    make(chan struct{}),
		done:                    make(chan struct{}),
		submissionStopped:       make(chan struct{}),
		continuationStopped:     make(chan struct{}),
		operations:              make(map[string]*commandOperation),
		tasksByRef:              make(map[lifecycle.TaskRef]*commandOperation),
		tasksByRequest:          make(map[lifecycle.TaskRequestRef]*commandOperation),
		functionCleanupTasks:    make(map[lifecycle.TaskRef]functionCleanupTask),
		functionCleanupRequests: make(map[lifecycle.TaskRequestRef]FunctionCleanupRef),
		shutdownRequests:        make(map[lifecycle.TaskRequestRef]*commandLane),
		shutdownTasks:           make(map[lifecycle.TaskRef]*commandLane),
		functionMutations:       make(chan functionMutationSubmission),
		functionMutationStopped: make(chan struct{}),
		lanes:                   make(map[commandLaneKey]*commandLane),
		compositeFenceClaims:    make(map[string]int),
		nextSource:              lifecycle.SourceJobManager,
		nextExternalSource:      lifecycle.SourceJobManager,
		shutdownBarrier:         shutdownBarrier,
		finalizer:               finalizer,
		functionCatalog:         functionCatalog,
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

func (ck *CommandKernel) BindRuntimeObserver(observer lifecycle.RuntimeObserver) error {
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
	if err := ck.claims.bindRuntimeObserver(observer, ck.clock.Now); err != nil {
		return err
	}
	ck.runtimeObserver = observer
	ck.observeRuntimeOperations()
	return nil
}

func (ck *CommandKernel) BindDiagnosticObserver(observer DiagnosticObserver) error {
	if ck == nil || observer == nil {
		return errors.New("jobmgr kernel: invalid diagnostic observer")
	}
	if ck.diagnosticObserver != nil {
		return errors.New("jobmgr kernel: diagnostic observer already bound")
	}
	select {
	case <-ck.done:
		return errors.New("jobmgr kernel: diagnostic observer bound after terminal")
	default:
	}
	ck.diagnosticObserver = observer
	return nil
}

func (ck *CommandKernel) Start(ctx context.Context) error {
	if ck == nil || ctx == nil {
		return errors.New("jobmgr kernel: invalid start")
	}
	started := false
	ck.startOnce.Do(func() {
		started = true
		ck.startErr = ck.bindRunNotifications()
		if ck.startErr != nil {
			return
		}
		go ck.runLoop(ctx)
	})
	if !started {
		return errors.New("jobmgr kernel: already started")
	}
	return ck.startErr
}

func (ck *CommandKernel) bindRunNotifications() error {
	return ck.frames.BindRunNotifications(
		ck.run.Generation(),
		ck.NotifyControlReady,
		func(err error) {
			ck.observe(DiagnosticEvent{
				Level: DiagnosticError,
				Name:  "job manager frame owner poisoned",
				Err:   err,
			})
			ck.run.Dirty(err)
			ck.NotifyControlReady()
		},
		ck.runtimeObserver,
	)
}

// beginResultEncode validates an operation's terminal result and dispatches its
// encode. It returns false after enqueuing a control frame on a preflight error,
// so the caller falls through to its disposal action. It dispatches no ownership
// action itself, so the ownership-gate invariant (SendAction stays in the
// sanctioned callers) holds.
func (ck *CommandKernel) beginResultEncode(operation *commandOperation, ref lifecycle.TaskRef) bool {
	expiry := lifecycle.ExpiryAt(ck.clock.Now())
	err := ck.tasks.PreflightResult(ref, operation.UID, expiry)
	if err != nil {
		status := lifecycle.ControlInternal
		if errors.Is(err, lifecycle.ErrFunctionResultTooLarge) {
			status = lifecycle.ControlPayloadTooLarge
		}
		ck.enqueueControl(operation, status)
		return false
	}
	operation.resultExpiry = expiry
	if err := ck.sendEncodeAction(operation); err != nil {
		ck.run.Dirty(err)
	}
	return true
}

func (ck *CommandKernel) completeTask(completion lifecycle.TaskCompletion) {
	if errors.Is(completion.Err, lifecycle.ErrTaskPanic) {
		ck.observe(DiagnosticEvent{
			Level:      DiagnosticError,
			Name:       "job manager task panicked",
			Generation: ck.run.Generation(),
			Task:       completion.Ref,
			Sequence:   completion.Sequence,
			Err:        completion.Err,
		})
	}
	if cleanup, ok := ck.functionCleanupTasks[completion.Ref]; ok {
		if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone {
			completion.Err = errors.Join(
				completion.Err,
				errors.New("jobmgr kernel: invalid Function cleanup completion"),
			)
		}
		cleanup.err = completion.Err
		ck.functionCleanupTasks[completion.Ref] = cleanup
		if err := ck.tasks.SendAction(lifecycle.TaskAction{
			Ref:      completion.Ref,
			Sequence: 2,
			Kind:     lifecycle.TaskActionTerminate,
		}); err != nil {
			ck.run.Dirty(err)
		}
		return
	}
	if ck.finalizerTask.Valid() && completion.Ref == ck.finalizerTask {
		ck.completeRunFinalizer(completion)
		return
	}
	if ck.shutdownBarrierTask.Valid() && completion.Ref == ck.shutdownBarrierTask {
		ck.completeShutdownBarrier(completion)
		return
	}
	operation := ck.tasksByRef[completion.Ref]
	if operation == nil {
		ck.completeShutdownTask(completion)
		return
	}
	if operation.composite != nil && operation.activeChild != nil {
		if operation.deferredCompletion != nil {
			ck.run.Dirty(errors.New("jobmgr composite: parent completed twice with a live child"))
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
		resultErr = operation.PhaseResultReady(completion.Ref, completion.Sequence)
	} else {
		resultErr = operation.ResultReady(completion.Ref, completion.Sequence)
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
	ck.enqueueCancellationControlIfNeeded(operation)
	action := lifecycle.TaskAction{
		Ref:      completion.Ref,
		Sequence: completion.Sequence + 1,
		Kind:     lifecycle.TaskActionDispose,
	}
	if completion.Err == nil && operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
		if ck.beginResultEncode(operation, completion.Ref) {
			return
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
	if err := ck.sendOperationAction(operation, action); err != nil {
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
					errors.New("jobmgr kernel: failed transaction preparation lost current resource"),
					completion.Err,
					err,
				))
				return
			}
			if err := ck.restoreTransactionCurrent(operation, current); err != nil {
				ck.run.Dirty(errors.Join(completion.Err, err))
				return
			}
			if lifecycle.OwnershipRetained(completion.Err) {
				ck.run.Dirty(completion.Err)
			}
			operation.terminalErr = errors.Join(operation.terminalErr, completion.Err)
			status := lifecycle.ControlUnavailable
			if operation.cancelled || operation.TimedOut() {
				status = cancellationControl(operation)
			} else if errors.Is(completion.Err, lifecycle.ErrTaskPanic) {
				status = lifecycle.ControlInternal
			}
			if operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
				ck.enqueueControl(operation, status)
			}
			ck.sendTransactionAction(operation, completion.Ref, completion.Sequence+1, lifecycle.TaskActionDispose)
			return
		}
		if completion.Kind != lifecycle.TaskOutcomePreparedResourceTransaction {
			ck.run.Dirty(errors.New("jobmgr kernel: transaction preparation returned the wrong outcome"))
			return
		}
		ownershipAllowed := ck.ownershipActionAllowed(operation)
		action := lifecycle.TaskActionApplyResourceTransaction
		if operation.cancelled ||
			operation.TimedOut() ||
			!ownershipAllowed ||
			(operation.Response != lifecycle.ResponseOpen &&
				operation.Response != lifecycle.ResponseNotRequired) ||
			operation.controlQueued {
			action = lifecycle.TaskActionDispose
		}
		if action == lifecycle.TaskActionDispose && !ownershipAllowed &&
			operation.Response == lifecycle.ResponseOpen &&
			!operation.controlQueued {
			operation.cancelled = true
		}
		ck.sendTransactionAction(operation, completion.Ref, completion.Sequence+1, action)
	case 2:
		if completion.Err != nil {
			operation.terminalErr = errors.Join(operation.terminalErr, completion.Err)
			ck.run.Dirty(errors.Join(
				errors.New("jobmgr kernel: resource transaction apply left unprovable state"),
				completion.Err,
			))
			return
		}
		if completion.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction {
			ck.run.Dirty(errors.New("jobmgr kernel: transaction apply returned the wrong outcome"))
			return
		}
		disposition, current, err :=
			ck.tasks.TakeAppliedResourceTransaction(completion.Ref, completion.Sequence, operation.transactionScope)
		if err != nil {
			ck.run.Dirty(err)
			return
		}
		if err := ck.applyTransactionDisposition(operation, disposition, current); err != nil {
			ck.run.Dirty(err)
			return
		}
		operation.transactionApplied = true

		ck.enqueueCancellationControlIfNeeded(operation)
		if operation.Response == lifecycle.ResponseOpen && !operation.controlQueued {
			if ck.beginResultEncode(operation, completion.Ref) {
				return
			}
		}
		ck.sendTransactionAction(operation, completion.Ref, completion.Sequence+1, lifecycle.TaskActionDispose)
	default:
		ck.run.Dirty(errors.New("jobmgr kernel: unexpected transaction completion"))
	}
}

func (ck *CommandKernel) sendTransactionAction(
	operation *commandOperation,
	ref lifecycle.TaskRef,
	sequence uint8,
	kind lifecycle.TaskActionKind,
) {
	action := lifecycle.TaskAction{
		Ref:      ref,
		Sequence: sequence,
		Kind:     kind,
	}
	if err := ck.sendOperationAction(operation, action); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) sendOperationAction(operation *commandOperation, action lifecycle.TaskAction) error {
	if operation == nil || !action.Ref.Valid() {
		return errors.New("jobmgr kernel: invalid operation action")
	}
	ownershipEntry := action.Kind == lifecycle.TaskActionApplyResourceTransaction
	if ownershipEntry && !operation.ownershipChain && !ck.ownershipActionAllowed(operation) {
		return errors.New("jobmgr kernel: ownership action admission closed")
	}
	var err error
	if action.Kind == lifecycle.TaskActionTerminate {
		err = operation.TerminationPending(action.Ref, action.Sequence)
	} else {
		err = operation.ActionPending(action.Ref, action.Sequence)
	}
	if err != nil {
		return err
	}
	if ownershipEntry && !operation.ownershipChain {
		operation.ownershipChain = true
		ck.ownershipChains++
	}
	return ck.tasks.SendAction(action)
}

func (ck *CommandKernel) ownershipActionAllowed(operation *commandOperation) bool {
	if operation != nil && operation.ownershipChain {
		return true
	}
	if ck.shutdownPhase == commandShutdownRunning {
		return true
	}
	return ck.shutdownPhase == commandShutdownActionDrain &&
		operation != nil &&
		operation.parent != nil &&
		operation.parent.ownershipChain
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
			return errors.New("jobmgr kernel: transaction outcome differs from exact current resource")
		}
		return ck.restoreTransactionCurrent(operation, current)
	}
	if outcome.Kind() != lifecycle.TaskOutcomeNone {
		return errors.New("jobmgr kernel: graph-only transaction returned a current resource")
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
		if current == nil || lane.current != nil || !lane.currentStopping || lane.currentIdentity != scope.Current {
			return errors.New("jobmgr kernel: transaction restoration differs from detached current")
		}
		lane.current = current
		lane.currentStopping = false
	} else {
		if current != nil || lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping {
			return errors.New("jobmgr kernel: graph-only transaction restoration found resource state")
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
		return errors.New("jobmgr kernel: transaction disposition differs from detached lane")
	}

	var identity lifecycle.ResourceIdentity
	switch disposition {
	case lifecycle.ResourceTransactionUnchanged:
		identity = scope.Current
		if identity.Valid() != (current != nil) {
			return errors.New("jobmgr kernel: unchanged transaction returned the wrong current resource")
		}
	case lifecycle.ResourceTransactionRemoved:
		if current != nil {
			return errors.New("jobmgr kernel: removed transaction returned a resource")
		}
		lane.resourceSource = 0
	case lifecycle.ResourceTransactionInstalled,
		lifecycle.ResourceTransactionReplaced:
		identity = scope.Successor
		if current == nil || !identity.Valid() {
			return errors.New("jobmgr kernel: installed transaction lost its successor")
		}
		lane.resourceSource = operation.Source
	default:
		return errors.New("jobmgr kernel: unknown resource transaction disposition")
	}
	lane.current = current
	lane.currentIdentity = identity
	lane.currentStopping = false
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
		if lane.current != nil || lane.currentIdentity.Valid() || lane.currentStopping ||
			!lane.retiringIdentity.Valid() {
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
	if err := ck.tasks.SendAction(
		lifecycle.TaskAction{
			Ref:      lane.shutdownTask,
			Sequence: sequence,
			Kind:     kind,
		},
	); err != nil {
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
			return errors.New("jobmgr kernel: transaction result is not applied")
		}
		sequence = 3
	}
	action := lifecycle.TaskAction{
		Ref:      operation.Task,
		Sequence: sequence,
		Kind:     lifecycle.TaskActionEncodeWrite,
		UID:      operation.UID,
		Expiry:   operation.resultExpiry,
	}
	return ck.sendOperationAction(operation, action)
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
		catalogErr := ck.functionCatalog.CompleteCleanup(cleanup.ref)
		if completeErr != nil || catalogErr != nil {
			ck.run.Dirty(errors.Join(completeErr, catalogErr))
		}
		return
	}
	if ck.finalizerTask.Valid() && ack.Ref == ck.finalizerTask {
		ck.acknowledgeRunFinalizer(ack)
		return
	}
	if ck.shutdownBarrierTask.Valid() && ack.Ref == ck.shutdownBarrierTask {
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
	if ack.Err != nil {
		operation.PoisonResponse()
		ck.run.Dirty(ack.Err)
	} else if ack.Kind == lifecycle.TaskActionEncodeWrite {
		if err := operation.CommitResponse(); err != nil {
			ck.run.Dirty(err)
		}
		if err := ck.completeOperationUID(operation, false); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	termination := lifecycle.TaskAction{
		Ref:      ack.Ref,
		Sequence: ack.Sequence + 1,
		Kind:     lifecycle.TaskActionTerminate,
	}
	if err := ck.sendOperationAction(operation, termination); err != nil {
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
		if ack.Kind == lifecycle.TaskActionDispose && !operation.transactionApplied {
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
				if err := ck.restoreTransactionCurrent(operation, current); err != nil {
					ck.run.Dirty(err)
					return
				}
			}
			ck.enqueueCancellationControlIfNeeded(operation)
			ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
			return
		}
	case lifecycle.TaskActionEncodeWrite:
		if ack.Err == nil {
			if err := operation.CommitResponse(); err != nil {
				ck.run.Dirty(err)
				return
			}
			if err := ck.completeOperationUID(operation, false); err != nil {
				ck.run.Dirty(err)
				return
			}
		}
	case lifecycle.TaskActionCleanup:
		operation.cleanupDone = true
	default:
		ck.run.Dirty(errors.New("jobmgr kernel: unexpected resource transaction acknowledgement"))
		return
	}

	if operation.transactionApplied && !operation.cleanupDone {
		cleanup := lifecycle.TaskAction{
			Ref:      ack.Ref,
			Sequence: ack.Sequence + 1,
			Kind:     lifecycle.TaskActionCleanup,
		}
		if err := ck.sendOperationAction(operation, cleanup); err != nil {
			ck.run.Dirty(err)
		}
		return
	}
	ck.sendResourceTermination(operation, ack.Ref, ack.Sequence+1)
}

func (ck *CommandKernel) sendResourceTermination(operation *commandOperation, ref lifecycle.TaskRef, sequence uint8) {
	termination := lifecycle.TaskAction{
		Ref:      ref,
		Sequence: sequence,
		Kind:     lifecycle.TaskActionTerminate,
	}
	if err := ck.sendOperationAction(operation, termination); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) beginShutdown(deadline time.Time) error {
	if deadline.IsZero() {
		return errors.New("jobmgr kernel: zero shutdown deadline")
	}
	if ck.shutdownPhase != commandShutdownRunning {
		return errors.New("jobmgr kernel: shutdown began twice")
	}
	ck.shutdownPhase = commandShutdownActionDrain
	ck.shutdownCancelCursor = ck.operationHead
	ck.shutdownLaneCursor = nil
	return nil
}

func (ck *CommandKernel) serviceShutdownCancellation(quantum int) (bool, error) {
	if ck.shutdownPhase != commandShutdownActionDrain || quantum <= 0 {
		return false, errors.New("jobmgr kernel: invalid shutdown cancellation service")
	}
	for visited := 0; visited < quantum && ck.shutdownCancelCursor != nil; visited++ {
		operation := ck.shutdownCancelCursor
		ck.shutdownCancelCursor = operation.allNext
		operation.shutdownVisited = true
		if operation.State == lifecycle.OperationDisposedTerminal {
			ck.unlinkOperation(operation)
			continue
		}
		if operation.ownershipChain ||
			operation.shutdownChild ||
			(operation.parent != nil &&
				operation.parent.ownershipChain) {
			operation.shutdownChild = true
			continue
		}
		if err := ck.cancelOperationForShutdown(operation); err != nil {
			return false, err
		}
	}
	return ck.shutdownCancelCursor != nil, nil
}

func (ck *CommandKernel) cancelOperationForShutdown(operation *commandOperation) error {
	if operation == nil || operation.State == lifecycle.OperationDisposedTerminal {
		return errors.New("jobmgr kernel: invalid shutdown operation")
	}
	if !operation.cancelled && !operation.TimedOut() {
		ck.recordCompositeChildTerminalCause(operation, ck.run.StoppingCause())
	}
	operation.cancelled = true
	switch operation.Child {
	case lifecycle.ChildExecuting:
		if err := ck.tasks.CancelWithCause(operation.Task, ck.run.StoppingCause()); err != nil {
			return err
		}
		if operation.Response != lifecycle.ResponseNotRequired &&
			!operation.plan.CooperativeCancel &&
			!(operation.TimedOut() &&
				requiresCooperativeDeadlineStart(operation)) {
			ck.enqueueControl(operation, cancellationControl(operation))
		}
	case lifecycle.ChildNotStarted:
		ck.unlinkQueued(operation)
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
			ck.unlinkQueued(operation)
			if operation.Response == lifecycle.ResponseOpen {
				ck.enqueueControl(operation, lifecycle.ControlDeadline)
			} else {
				ck.tryDispose(operation)
			}
		}
	case lifecycle.ChildResultReady:
	case lifecycle.ChildActionPending:
	case lifecycle.ChildAbandonedBeforeStart,
		lifecycle.ChildActionAcknowledged,
		lifecycle.ChildTerminationPending,
		lifecycle.ChildExitAcknowledged:
	default:
		return errors.New("jobmgr kernel: invalid shutdown child state")
	}
	return nil
}

func (ck *CommandKernel) enqueueCancellationControlIfNeeded(operation *commandOperation) {
	if operation == nil || !operation.cancelled && !operation.TimedOut() {
		return
	}
	ck.enqueueControl(operation, cancellationControl(operation))
}

func cancellationControl(operation *commandOperation) lifecycle.ControlStatus {
	if operation != nil && operation.TimedOut() {
		return lifecycle.ControlDeadline
	}
	return lifecycle.ControlCancelled
}

func (ck *CommandKernel) advanceShutdownAuthority() error {
	if ck.shutdownPhase != commandShutdownActionDrain || ck.shutdownCancelCursor != nil || ck.ownershipChains != 0 {
		return nil
	}
	// A resumed mutation retires predecessors incrementally and cannot be
	// rolled back. Let the bounded commit finish before entering cleanup drain.
	if ck.functionMutationActive && ck.functionMutation.action == functionMutationCommit {
		return nil
	}
	if err := ck.abortFunctionMutationForShutdown(); err != nil {
		return err
	}
	if ck.functionMutationActive || ck.functionMutationPaused {
		return errors.New("jobmgr kernel: Function mutation survived shutdown abort")
	}
	ck.shutdownPhase = commandShutdownCleanupDrain
	close(ck.functionMutationStopped)
	ck.closeContinuationIngress()
	if err := ck.tasks.SealInherited(); err != nil {
		return err
	}
	ck.shutdownLaneCursor = ck.laneHead
	return nil
}

func (ck *CommandKernel) abortFunctionMutationForShutdown() error {
	if !ck.functionMutationActive && !ck.functionMutationPaused {
		return nil
	}
	abortErr := ck.abortFunctionMutation(ck.functionMutation.mutation)
	terminalErr := error(ck.run.StoppingCause())
	if abortErr != nil {
		terminalErr = errors.Join(terminalErr, abortErr)
	}
	if ck.functionMutation.result != nil {
		ck.functionMutation.result <- functionMutationResult{
			err: terminalErr,
		}
	}
	ck.functionMutation = functionMutationSubmission{}
	ck.functionMutationActive = false
	ck.functionMutationPaused = false
	return abortErr
}

func (ck *CommandKernel) serviceShutdownStops(quantum int) (bool, error) {
	if ck.shutdownPhase != commandShutdownCleanupDrain || !ck.shutdownBarrierDone || quantum <= 0 {
		return false, errors.New("jobmgr kernel: invalid shutdown lane service")
	}
	for visited := 0; visited < quantum && ck.shutdownLaneCursor != nil; visited++ {
		lane := ck.shutdownLaneCursor
		ck.shutdownLaneCursor = lane.allNext
		lane.shutdownVisited = true
		if err := ck.enqueueShutdownStop(lane); err != nil {
			return false, err
		}
		ck.releaseUnusedLane(lane)
	}
	return ck.shutdownLaneCursor != nil, nil
}

func (ck *CommandKernel) enqueueShutdownStop(lane *commandLane) error {
	if lane == nil || !lane.allListed {
		return errors.New("jobmgr kernel: invalid shutdown resource lane")
	}
	if lane.currentStopping {
		if lane.current != nil || !lane.currentIdentity.Valid() || lane.retiringIdentity.Valid() {
			return errors.New("jobmgr kernel: shutdown found an invalid stopping resource")
		}
		return nil
	}
	if lane.retiringIdentity.Valid() {
		if lane.current != nil || lane.currentIdentity.Valid() {
			return errors.New("jobmgr kernel: shutdown found an invalid retiring resource")
		}
		return nil
	}
	if lane.current == nil {
		if lane.currentIdentity.Valid() {
			return errors.New("jobmgr kernel: shutdown found a detached current identity")
		}
		return nil
	}
	if lane.owners != 0 {
		return nil
	}
	if lane.head != nil || lane.tail != nil || lane.active != nil || lane.ready {
		return errors.New("jobmgr kernel: owner-free resource lane retains operation state")
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
		return errors.New("jobmgr kernel: shutdown resource has no scheduling source")
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
	request, err := ck.tasks.Enqueue(lifecycle.TaskClassFrameworkControl, plan)
	if err != nil {
		return err
	}
	if owner := ck.shutdownRequests[request]; owner != nil {
		outcome, cancelErr := ck.tasks.CancelPendingOutcome(request)
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
	ck.shutdownRequests[request] = lane
	return nil
}

func (ck *CommandKernel) advanceShutdownBarrier() error {
	if ck.shutdownPhase != commandShutdownCleanupDrain ||
		ck.shutdownBarrier == nil ||
		ck.shutdownBarrierDone ||
		ck.shutdownBarrierFailed ||
		ck.shutdownBarrierRequest.Valid() ||
		ck.shutdownBarrierTask.Valid() {
		return nil
	}
	if ck.shutdownCancelCursor != nil || ck.tasks.InheritedCancellationPending() {
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
			return lifecycle.NoValueOutcome(), ck.shutdownBarrier.BeforeFunctionCatalogClose(ctx, ck.run.Generation())
		},
	)
	if err != nil {
		return err
	}
	request, err := ck.tasks.Enqueue(lifecycle.TaskClassFrameworkControl, plan)
	if err != nil {
		return err
	}
	ck.shutdownBarrierRequest = request
	return nil
}

func (ck *CommandKernel) completeShutdownBarrier(completion lifecycle.TaskCompletion) {
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
		Ref:      completion.Ref,
		Sequence: 2,
		Kind:     lifecycle.TaskActionTerminate,
	}); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) acknowledgeShutdownBarrier(ack lifecycle.TaskAcknowledgement) {
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
	if ck.finalizer == nil || ck.finalizerDone || ck.finalizerFailed || ck.finalizerRequest.Valid() ||
		ck.finalizerTask.Valid() {
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
	request, err := ck.tasks.Enqueue(lifecycle.TaskClassFrameworkControl, plan)
	if err != nil {
		return err
	}
	ck.finalizerRequest = request
	return nil
}

// shutdownBarrierSettled reports that the shutdown barrier completed cleanly and
// the kernel is in the cleanup-drain phase with no outstanding ownership chains —
// the precondition shared by the finalizer-readiness and quiescence checks.
func (ck *CommandKernel) shutdownBarrierSettled() bool {
	return ck.shutdownPhase == commandShutdownCleanupDrain &&
		ck.ownershipChains == 0 &&
		ck.shutdownBarrierDone && !ck.shutdownBarrierFailed &&
		!ck.shutdownBarrierRequest.Valid() &&
		!ck.shutdownBarrierTask.Valid() &&
		ck.shutdownBarrierAction == 0
}

func (ck *CommandKernel) shutdownReadyForFinalizer() bool {
	census := ck.runCensus()
	if !census.KernelDrained || !census.FunctionCatalogDrained ||
		census.UIDActive != 0 || census.TransientActive != 0 ||
		census.TransientPending != 0 || census.InheritedActive != 0 ||
		census.LongLived.Active != census.LongLived.SecretStores {
		return false
	}
	return true
}

func (ck *CommandKernel) completeRunFinalizer(completion lifecycle.TaskCompletion) {
	if completion.Sequence != 1 || completion.Kind != lifecycle.TaskOutcomeNone || ck.finalizerAction != 0 ||
		ck.finalizerDone ||
		ck.finalizerFailed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer completion"))
		return
	}
	if completion.Err != nil {
		ck.finalizerFailed = true
		ck.run.Dirty(completion.Err)
	}
	ck.finalizerAction = lifecycle.TaskActionTerminate
	if err := ck.tasks.SendAction(
		lifecycle.TaskAction{
			Ref:      completion.Ref,
			Sequence: 2,
			Kind:     lifecycle.TaskActionTerminate,
		},
	); err != nil {
		ck.run.Dirty(err)
	}
}

func (ck *CommandKernel) acknowledgeRunFinalizer(ack lifecycle.TaskAcknowledgement) {
	if ack.Sequence != 2 || ack.Kind != lifecycle.TaskActionTerminate ||
		ck.finalizerAction != lifecycle.TaskActionTerminate ||
		ck.finalizerDone {
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
	return ck.runCensus().Quiescent()
}

func (ck *CommandKernel) kernelOwnershipDrained() bool {
	return ck.shutdownBarrierSettled() && ck.kernelStateDrained() &&
		ck.runtimeHead == nil && ck.runtimeTail == nil &&
		ck.functionOperations == 0
}

func (ck *CommandKernel) kernelStateDrained() bool {
	if ck.shutdownCancelCursor != nil || ck.shutdownLaneCursor != nil ||
		ck.tasks.InheritedCancellationPending() ||
		len(ck.operations) != 0 || len(ck.tasksByRef) != 0 ||
		len(ck.tasksByRequest) != 0 ||
		ck.operationHead != nil || ck.operationTail != nil ||
		ck.laneHead != nil || ck.laneTail != nil ||
		len(ck.functionCleanupTasks) != 0 || len(ck.functionCleanupRequests) != 0 ||
		ck.functionCleanupBacklog.count != 0 ||
		ck.functionMutationActive || ck.functionMutationPaused ||
		len(ck.lanes) != 0 ||
		len(ck.compositeFenceClaims) != 0 ||
		ck.compositeFenceHead != nil ||
		ck.compositeFenceTail != nil ||
		ck.compositeFenceCount != 0 ||
		ck.compositeFenceRecheck ||
		ck.tasks.Active() != 0 ||
		ck.tasks.Pending() != 0 ||
		len(ck.shutdownRequests) != 0 ||
		len(ck.shutdownTasks) != 0 ||
		ck.controls.count != 0 ||
		ck.deadlines.Len() != 0 ||
		len(ck.submissions[0]) != 0 ||
		len(ck.submissions[1]) != 0 ||
		ck.hasBlockedSubmission[0] ||
		ck.hasBlockedSubmission[1] ||
		ck.ready[0].len != 0 ||
		ck.ready[1].len != 0 ||
		ck.claims.waitingCount() != 0 ||
		len(ck.claims.keys) != 0 {
		return false
	}
	return true
}

func (ck *CommandKernel) functionCatalogDrained() bool {
	return ck.functionCatalog.LifecycleDrained()
}

func (ck *CommandKernel) runCensus() lifecycle.RunCensus {
	uidActive, _, _ := ck.uids.Census()
	return lifecycle.RunCensus{
		KernelDrained:          ck.kernelOwnershipDrained(),
		FunctionCatalogDrained: ck.functionCatalogDrained(),
		UIDActive:              uidActive,
		TransientActive:        ck.tasks.Active(),
		TransientPending:       ck.tasks.Pending(),
		InheritedActive:        ck.tasks.InheritedActive(),
		LongLived:              ck.tasks.LongLivedCensus(),
		Frame:                  ck.frames.Census(),
		RunFinalizerComplete:   ck.finalizerDone && !ck.finalizerFailed,
	}
}
