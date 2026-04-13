// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

type functionSet struct {
	direct   func(Function)            // for globally-unique names
	prefixes map[string]func(Function) // for prefix-multiplexed names
}

const (
	// defaultWorkerCount stays at 1 for now.
	// Working theory: remaining concurrency risk is in collector MethodHandler
	// implementations (shared mutable state and lifecycle races), not in manager
	// queue/worker logic.
	// TODO: establish and document a MethodHandler goroutine-safety contract
	// before increasing this default.
	defaultWorkerCount = 1
	// defaultQueueSize is intentionally 1 (not removed: the keyed scheduler is
	// still the dispatch primitive, we just don't want it to absorb bursts).
	// Rationale: every downstream stage is single-threaded today (1 worker, 1
	// jobmgr loop, serial Check()), so admitting more than 1 extra request
	// only buys wedge surface (a queued request that is later cancelled by
	// netdata cannot reach jobmgr, so the dyncfg wait gate stays in
	// 'accepted' forever). With queue=1 the stdin reader back-pressures
	// earlier through the OS pipe instead of
	// piling up admitted-but-unprocessed work in stateQueued. If/when we add
	// real downstream concurrency, raise this again.
	defaultQueueSize            = 1
	defaultCancelFallbackDelay  = 5 * time.Second
	defaultShutdownDrainTimeout = 8 * time.Second
	defaultTombstoneTTL         = 60 * time.Second
	defaultAwaitingWarnDelay    = 30 * time.Second
)

type invocationState uint8

const (
	stateQueued invocationState = iota + 1
	stateRunning
	stateAwaitingResult
)

type invocationAdmission uint8

const (
	invocationAdmissionAccepted invocationAdmission = iota + 1
	invocationAdmissionDuplicateActive
	invocationAdmissionDuplicateTombstone
	invocationAdmissionInvalid
)

type invocationRequest struct {
	fn          *Function
	handler     func(Function)
	ctx         context.Context
	scheduleKey string
}

type invocationRecord struct {
	state           invocationState
	cancel          context.CancelFunc
	cancelRequested bool
	fallbackTimer   *time.Timer
	awaitingTimer   *time.Timer
	awaitingSince   time.Time
	scheduleKey     string
}

func NewManager() *Manager {
	runtimeStore := metrix.NewRuntimeStore()
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "functions manager"),
		),
		api:                  netdataapi.New(safewriter.Stdout),
		input:                newStdinInput(),
		mux:                  &sync.Mutex{},
		functionRegistry:     make(map[string]*functionSet),
		workerCount:          defaultWorkerCount,
		queueSize:            defaultQueueSize,
		invStateMux:          &sync.Mutex{},
		invState:             make(map[string]*invocationRecord),
		tombstones:           make(map[string]time.Time),
		tombstoneTTL:         defaultTombstoneTTL,
		cancelFallbackDelay:  defaultCancelFallbackDelay,
		shutdownDrainTimeout: defaultShutdownDrainTimeout,
		awaitingWarnDelay:    defaultAwaitingWarnDelay,
		runtimeStore:         runtimeStore,
		runtimeMetrics:       newManagerRuntimeMetrics(runtimeStore),
	}
}

type Manager struct {
	*logger.Logger

	api *netdataapi.API

	input input

	mux              *sync.Mutex
	functionRegistry map[string]*functionSet

	workerCount int
	queueSize   int

	scheduler *keyScheduler

	invStateMux          *sync.Mutex
	invState             map[string]*invocationRecord
	tombstones           map[string]time.Time
	tombstoneTTL         time.Duration
	cancelFallbackDelay  time.Duration
	shutdownDrainTimeout time.Duration
	awaitingWarnDelay    time.Duration
	stopping             atomic.Bool

	runtimeService             runtimecomp.Service
	runtimeStore               metrix.RuntimeStore
	runtimeMetrics             *managerRuntimeMetrics
	runtimeComponentName       string
	runtimeComponentRegistered bool
}

func (m *Manager) Run(ctx context.Context, quitCh chan struct{}) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	if err := m.registerRuntimeComponent(); err != nil {
		m.Warningf("runtime metrics registration failed: %v", err)
	} else {
		defer m.unregisterRuntimeComponent()
	}

	m.run(ctx, quitCh)
}

func (m *Manager) run(ctx context.Context, quitCh chan struct{}) {
	parser := newInputParser()
	m.scheduler = newKeyScheduler(m.queueSize)
	m.observeSchedulerPending()
	var workersWG sync.WaitGroup

	m.startWorkers(&workersWG)

	// Wake any enqueue() blocked on a full scheduler when the parent context
	// is canceled. Necessary because the reader loop below may itself be
	// blocked inside dispatchInvocation -> scheduler.enqueue waiting for
	// space, and would otherwise miss the ctx.Done signal.
	stopWatcher := make(chan struct{})
	defer close(stopWatcher)
	go func() {
		select {
		case <-ctx.Done():
			if m.scheduler != nil {
				m.scheduler.stop()
			}
		case <-stopWatcher:
		}
	}()

	for {
		select {
		case <-ctx.Done():
			m.shutdown(false, true, quitCh, &workersWG)
			return
		case line, ok := <-m.input.lines():
			if !ok {
				m.shutdown(false, true, quitCh, &workersWG)
				return
			}
			event, err := parser.parseEvent(line)
			if err != nil {
				m.Warningf("parse function: %v ('%s')", err, line)
				continue
			}

			switch event.kind {
			case inputEventNone, inputEventProgress:
				continue
			case inputEventQuit:
				m.shutdown(true, true, quitCh, &workersWG)
				return
			case inputEventCancel:
				m.handleCancelEvent(event)
				continue
			case inputEventCall:
				m.observeFunctionCall()
				m.dispatchInvocation(ctx, event.fn)
			}
		}
	}
}

func (m *Manager) startWorkers(workersWG *sync.WaitGroup) {
	if workersWG == nil {
		return
	}

	for range m.workerCount {
		workersWG.Go(m.runWorker)
	}
}

func (m *Manager) shutdown(signalQuit, cancelInflight bool, quitCh chan struct{}, workersWG *sync.WaitGroup) {
	m.setStopping(true)
	m.signalQuitIfRequested(signalQuit, quitCh)
	m.stopSchedulerAdmission()
	timedOut := m.waitWorkers(workersWG)
	m.finalizeUnresolvedOnShutdown(cancelInflight, timedOut)
}

func (m *Manager) signalQuitIfRequested(signalQuit bool, quitCh chan struct{}) {
	if signalQuit && quitCh != nil {
		quitCh <- struct{}{}
	}
}

func (m *Manager) stopSchedulerAdmission() {
	if m.scheduler != nil {
		m.scheduler.stopAccepting()
	}
}

func (m *Manager) waitWorkers(workersWG *sync.WaitGroup) bool {
	if workersWG == nil {
		return false
	}

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), m.shutdownDrainTimeout)
	defer cancelDrain()

	done := make(chan struct{})
	go func() {
		defer close(done)
		workersWG.Wait()
	}()

	select {
	case <-done:
		return false
	case <-drainCtx.Done():
		return true
	}
}

func (m *Manager) finalizeUnresolvedOnShutdown(cancelInflight, timedOut bool) {
	if !cancelInflight {
		return
	}
	if !timedOut && !m.hasActiveInvocations() {
		return
	}

	m.cancelAllInvocations()
	m.forceFinalizeAll(499, "request canceled during shutdown")
	if m.scheduler != nil {
		m.scheduler.stop()
	}
}

func (m *Manager) dispatchInvocation(parentCtx context.Context, fn *Function) {
	if fn == nil {
		return
	}
	if m.isStopping() {
		m.respf(fn, 503, "functions manager is stopping")
		return
	}

	handler, scheduleKey, ok := m.lookupFunctionRoute(*fn)
	if !ok {
		m.Infof("skipping execution of '%s': unregistered function", fn.Name)
		m.respf(fn, 501, "unregistered function: %s", fn.Name)
		return
	}
	if handler == nil {
		m.Warningf("skipping execution of '%s': nil function registered", fn.Name)
		m.respf(fn, 501, "nil function: %s", fn.Name)
		return
	}

	reqCtx, cancel := context.WithCancel(parentCtx)
	switch m.trySetInvocationState(fn.UID, stateQueued, cancel, scheduleKey) {
	case invocationAdmissionAccepted:
		// admitted
	case invocationAdmissionDuplicateActive:
		cancel()
		// Do not emit terminal output for duplicates of an active UID. Emitting
		// via tryFinalize would mutate active tracking for the original invocation.
		m.Warningf("ignoring duplicate active transaction id: %s", fn.UID)
		m.observeDuplicateUIDIgnored()
		return
	case invocationAdmissionDuplicateTombstone:
		cancel()
		m.Warningf("ignoring duplicate recently finalized transaction id: %s", fn.UID)
		m.observeDuplicateUIDIgnored()
		return
	case invocationAdmissionInvalid:
		cancel()
		m.Warningf("ignoring invalid transaction id: %q", fn.UID)
		return
	default:
		cancel()
		m.Warningf("ignoring transaction id '%s': unsupported admission state", fn.UID)
		return
	}

	req := &invocationRequest{
		fn:          fn,
		handler:     handler,
		ctx:         reqCtx,
		scheduleKey: scheduleKey,
	}

	if m.scheduler == nil {
		cancel()
		m.respf(fn, 503, "functions manager is stopping")
		return
	}

	// scheduler.enqueue blocks when the queue is full; it only returns an
	// error for invalid inputs or when the scheduler is stopping (shutdown).
	// The blocking is intentional: by stalling here, the stdin reader slows
	// down as well, which propagates back-pressure to netdata through the OS
	// pipe instead of allowing unbounded buffering or dropping requests.
	if err := m.scheduler.enqueue(req); err != nil {
		cancel()
		switch {
		case errors.Is(err, errSchedulerStopping):
			m.respf(fn, 503, "functions manager is stopping")
		case errors.Is(err, errSchedulerInvalid):
			m.respf(fn, 500, "invalid scheduler request")
		default:
			// Should be unreachable: enqueue's other return points block.
			// If a new error variant is added in the future, surface it
			// loudly instead of swallowing it as a generic 5xx.
			m.Warningf("unexpected scheduler enqueue error for '%s': %v", fn.Name, err)
			m.respf(fn, 500, "unexpected scheduler error: %v", err)
		}
		return
	}
	m.observeSchedulerPending()
}

func (m *Manager) trySetInvocationState(uid string, state invocationState, cancel context.CancelFunc, scheduleKey string) invocationAdmission {
	if uid == "" {
		return invocationAdmissionInvalid
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	m.pruneExpiredTombstonesLocked(time.Now())

	if _, ok := m.tombstones[uid]; ok {
		return invocationAdmissionDuplicateTombstone
	}

	if _, ok := m.invState[uid]; ok {
		return invocationAdmissionDuplicateActive
	}
	m.invState[uid] = &invocationRecord{
		state:       state,
		cancel:      cancel,
		scheduleKey: scheduleKey,
	}
	m.observeInvocationsLocked()
	return invocationAdmissionAccepted
}

func (m *Manager) setAwaitingResultState(uid string, fnTimeout time.Duration) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	rec, ok := m.invState[uid]
	if !ok || rec == nil {
		return
	}

	rec.state = stateAwaitingResult
	rec.awaitingSince = time.Now()
	m.startAwaitingTimerLocked(uid, rec, fnTimeout)
	m.observeInvocationsLocked()
}

func (m *Manager) logAwaitingResult(uid string) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	rec, ok := m.invState[uid]
	if !ok || rec == nil || rec.state != stateAwaitingResult {
		m.invStateMux.Unlock()
		return
	}
	age := time.Since(rec.awaitingSince)
	m.invStateMux.Unlock()

	m.Warningf("transaction uid '%s' is still awaiting terminal response after %s", uid, age)
}

func (m *Manager) startInvocation(uid string) bool {
	if uid == "" {
		return false
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	rec, ok := m.invState[uid]
	if !ok || rec == nil || rec.cancelRequested {
		return false
	}
	rec.state = stateRunning
	m.observeInvocationsLocked()
	return true
}

func (m *Manager) handleCancelEvent(event inputEvent) {
	uid := event.uid
	if uid == "" {
		return
	}

	if event.preAdmission {
		m.respUID(uid, 499, "request canceled")
		return
	}

	if _, ok := m.requestCancellation(uid); !ok {
		m.Debugf("ignoring cancel for unknown transaction id: %s", uid)
		return
	}
	// No immediate terminal response. requestCancellation handled state
	// transitions: for queued functions the cancel is intentionally ignored
	// (function will run to completion); for running/awaiting functions a
	// fallback timer was armed and will emit 499 + tombstone after
	// cancelFallbackDelay.
}

func (m *Manager) requestCancellation(uid string) (invocationState, bool) {
	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	rec, ok := m.invState[uid]
	if !ok || rec == nil {
		return 0, false
	}

	// For stateQueued we deliberately ignore the cancel entirely: no
	// cancelRequested flag, no ctx cancellation, no fallback timer, no
	// tombstone. Reason: dyncfg commands (enable/disable/update/restart)
	// carry side-effects that must reach jobmgr, otherwise the wait gate
	// stays in 'accepted' forever (since the wait-decision timeout was
	// removed). Setting cancelRequested would make startInvocation skip the
	// handler when the worker pulls; the fallback timer's tryFinalize would
	// also tombstone+remove from invState before the worker gets there. So
	// either path wedges the function. We let it run to completion as if
	// nothing happened; netdata already considers the transaction done (it
	// 504'd before sending CANCEL), so the eventual terminal response just
	// produces a benign "transaction not found" log on the netdata side.
	if rec.state == stateQueued {
		return rec.state, true
	}

	if rec.cancelRequested {
		return rec.state, true
	}
	rec.cancelRequested = true
	if rec.cancel != nil {
		rec.cancel()
	}

	m.startCancelFallbackTimerLocked(uid, rec)

	return rec.state, true
}

func (m *Manager) setStopping(v bool) {
	m.stopping.Store(v)
}

func (m *Manager) isStopping() bool {
	return m.stopping.Load()
}

func (m *Manager) hasActiveInvocations() bool {
	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()
	return len(m.invState) > 0
}

// TerminalFinalizer returns the manager-bound terminal finalizer for responder wiring.
func (m *Manager) TerminalFinalizer() TerminalFinalizer {
	return m.finalizeTerminal
}

func (m *Manager) finalizeTerminal(uid, source string, emit func()) bool {
	return m.tryFinalize(uid, source, emit)
}

func (m *Manager) cancelAllInvocations() {
	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	for _, rec := range m.invState {
		if rec == nil {
			continue
		}
		rec.cancelRequested = true
		if rec.cancel != nil {
			rec.cancel()
		}
	}
}

func (m *Manager) forceFinalizeAll(code int, message string) {
	m.invStateMux.Lock()
	uids := make([]string, 0, len(m.invState))
	for uid := range m.invState {
		uids = append(uids, uid)
	}
	m.invStateMux.Unlock()

	for _, uid := range uids {
		m.respUID(uid, code, "%s", message)
	}
}

// markCancelled performs the same bookkeeping as tryFinalize (tombstone +
// scheduler.complete + remove from invState) but does NOT emit anything to
// netdata. Used by the cancel fallback path: by the time CANCEL is sent,
// netdata has already timed out the transaction and removed its inflight
// entry, so any response we emit just produces a "transaction not found"
// log on the netdata side. We still need the bookkeeping so the lane
// advances and any later terminal response from the handler is dropped.
func (m *Manager) markCancelled(uid string) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	now := time.Now()
	m.pruneExpiredTombstonesLocked(now)
	if _, ok := m.tombstones[uid]; ok {
		m.invStateMux.Unlock()
		return
	}

	var scheduleKey string
	if rec, ok := m.invState[uid]; ok && rec != nil {
		m.stopTimersLocked(rec)
		scheduleKey = rec.scheduleKey
	}
	delete(m.invState, uid)
	m.tombstones[uid] = now.Add(m.tombstoneTTL)
	m.observeInvocationsLocked()
	m.invStateMux.Unlock()

	if scheduleKey != "" && m.scheduler != nil {
		m.scheduler.complete(scheduleKey, uid)
		m.observeSchedulerPending()
	}
}

// tryFinalize emits a terminal response once per transaction UID.
// Later terminal attempts for the same UID are dropped while tombstone is active.
func (m *Manager) tryFinalize(uid, source string, emit func()) bool {
	if uid == "" || emit == nil {
		return false
	}

	m.invStateMux.Lock()
	now := time.Now()
	m.pruneExpiredTombstonesLocked(now)
	if _, ok := m.tombstones[uid]; ok {
		m.invStateMux.Unlock()
		m.Debugf("dropping late terminal response for uid '%s' (source=%s)", uid, source)
		m.observeLateTerminalDropped()
		return false
	}

	var scheduleKey string
	if rec, ok := m.invState[uid]; ok && rec != nil {
		m.stopTimersLocked(rec)
		scheduleKey = rec.scheduleKey
	}
	delete(m.invState, uid)
	m.tombstones[uid] = now.Add(m.tombstoneTTL)
	m.observeInvocationsLocked()
	m.invStateMux.Unlock()

	if scheduleKey != "" && m.scheduler != nil {
		m.scheduler.complete(scheduleKey, uid)
		m.observeSchedulerPending()
	}

	emit()
	return true
}

func (m *Manager) pruneExpiredTombstonesLocked(now time.Time) {
	for uid, expiresAt := range m.tombstones {
		if !expiresAt.After(now) {
			delete(m.tombstones, uid)
		}
	}
}

// startAwaitingTimerLocked starts/refreshes awaiting-result warning timer.
// Caller must hold m.invStateMux.
func (m *Manager) startAwaitingTimerLocked(uid string, rec *invocationRecord, fnTimeout time.Duration) {
	if uid == "" || rec == nil {
		return
	}

	delay := m.awaitingWarnDelay
	if fnTimeout > 0 && fnTimeout < delay {
		delay = fnTimeout
	}
	if delay <= 0 {
		return
	}

	if rec.awaitingTimer != nil {
		rec.awaitingTimer.Stop()
	}
	uidCopy := uid
	rec.awaitingTimer = time.AfterFunc(delay, func() {
		m.logAwaitingResult(uidCopy)
	})
}

// startCancelFallbackTimerLocked starts cancel fallback timer once.
// Caller must hold m.invStateMux.
func (m *Manager) startCancelFallbackTimerLocked(uid string, rec *invocationRecord) {
	if uid == "" || rec == nil || rec.fallbackTimer != nil {
		return
	}

	uidCopy := uid
	rec.fallbackTimer = time.AfterFunc(m.cancelFallbackDelay, func() {
		m.observeCancelFallback()
		// Don't emit a 499: by the time CANCEL was sent, netdata has already
		// 504'd the transaction and removed its inflight entry, so any
		// response we send just produces a "transaction not found" log.
		// markCancelled does the bookkeeping (tombstone + lane advance) so
		// the late real response from the handler is dropped.
		m.markCancelled(uidCopy)
	})
}

// stopTimersLocked stops invocation timers and clears timer references.
// Caller must hold m.invStateMux.
func (m *Manager) stopTimersLocked(rec *invocationRecord) {
	if rec == nil {
		return
	}

	if rec.fallbackTimer != nil {
		rec.fallbackTimer.Stop()
		rec.fallbackTimer = nil
	}
	if rec.awaitingTimer != nil {
		rec.awaitingTimer.Stop()
		rec.awaitingTimer = nil
	}
}

type functionSnapshot struct {
	direct   func(Function)
	prefixes map[string]func(Function)
}

func (m *Manager) snapshotFunction(name string) (functionSnapshot, bool) {
	m.mux.Lock()
	fs, ok := m.functionRegistry[name]
	snap := functionSnapshot{}
	if ok && fs != nil {
		snap.direct = fs.direct
		if len(fs.prefixes) > 0 {
			snap.prefixes = make(map[string]func(Function), len(fs.prefixes))
			maps.Copy(snap.prefixes, fs.prefixes)
		}
	}
	m.mux.Unlock()

	if !ok || fs == nil {
		return functionSnapshot{}, false
	}
	return snap, true
}

func matchPrefix(prefixes map[string]func(Function), id string) (string, func(Function), bool) {
	if len(prefixes) == 0 || id == "" {
		return "", nil, false
	}

	for prefix, handler := range prefixes {
		if strings.HasPrefix(id, prefix) {
			return prefix, handler, true
		}
	}

	return "", nil, false
}

func (m *Manager) lookupFunctionRoute(fn Function) (handler func(Function), scheduleKey string, ok bool) {
	snap, ok := m.snapshotFunction(fn.Name)
	if !ok {
		return nil, "", false
	}
	unknownHandler := m.unknownFunctionHandler()

	if len(snap.prefixes) > 0 {
		if len(fn.Args) > 0 {
			id := fn.Args[0]
			if prefix, routeHandler, matched := matchPrefix(snap.prefixes, id); matched && routeHandler != nil {
				return routeHandler, routeScheduleKey(fn.Name, prefix), true
			}
		}

		return unknownHandler, routeScheduleKey(fn.Name, scheduleKeyUnmatched), true
	}

	if snap.direct != nil {
		return snap.direct, routeScheduleKey(fn.Name, ""), true
	}

	return unknownHandler, routeScheduleKey(fn.Name, scheduleKeyDirectMissing), true
}

// lookupFunction returns a snapshot handler used by existing tests to verify
// registry snapshot semantics at lookup time.
func (m *Manager) lookupFunction(name string) (func(Function), bool) {
	snap, ok := m.snapshotFunction(name)
	if !ok {
		return nil, false
	}
	unknownHandler := m.unknownFunctionHandler()

	return func(f Function) {
		if len(snap.prefixes) > 0 {
			if len(f.Args) > 0 {
				id := f.Args[0]
				if _, handler, matched := matchPrefix(snap.prefixes, id); matched && handler != nil {
					handler(f)
					return
				}
			}
			unknownHandler(f)
			return
		}

		if snap.direct != nil {
			snap.direct(f)
			return
		}

		unknownHandler(f)
	}, true
}

func (m *Manager) unknownFunctionHandler() func(Function) {
	return func(f Function) {
		m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
	}
}

const (
	scheduleKeyUnmatched     = "__unmatched__"
	scheduleKeyDirectMissing = "__direct_missing__"
)

func routeScheduleKey(name, discriminator string) string {
	if discriminator == "" {
		return name
	}
	return name + "|" + discriminator
}

func (m *Manager) respUID(uid string, code int, msgf string, a ...any) {
	if uid == "" {
		return
	}
	m.respf(&Function{UID: uid}, code, msgf, a...)
}

func (m *Manager) respf(fn *Function, code int, msgf string, a ...any) {
	if fn == nil || fn.UID == "" {
		return
	}

	msg := fmt.Sprintf(msgf, a...)
	bs := BuildJSONPayload(code, msg)

	res := netdataapi.FunctionResult{
		UID:             fn.UID,
		ContentType:     "application/json",
		Payload:         string(bs),
		Code:            strconv.Itoa(code),
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
	}

	m.finalizeTerminal(fn.UID, "functions.manager.respf", func() {
		m.api.FUNCRESULT(res)
	})
}
