// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
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
	defaultWorkerCount          = 1
	defaultQueueSize            = 64
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
}

func (m *Manager) Run(ctx context.Context, quitCh chan struct{}) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()
	m.run(ctx, quitCh)
}

func (m *Manager) run(ctx context.Context, quitCh chan struct{}) {
	parser := newInputParser()
	m.scheduler = newKeyScheduler(m.queueSize)
	var workersWG sync.WaitGroup

	for range m.workerCount {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			m.runWorker()
		}()
	}
	shutdown := func(signalQuit, cancelInflight bool) {
		m.setStopping(true)
		if signalQuit && quitCh != nil {
			quitCh <- struct{}{}
		}

		if m.scheduler != nil {
			m.scheduler.stopAccepting()
		}

		drainCtx, cancelDrain := context.WithTimeout(context.Background(), m.shutdownDrainTimeout)
		defer cancelDrain()

		done := make(chan struct{})
		go func() {
			defer close(done)
			workersWG.Wait()
		}()

		timedOut := false
		select {
		case <-done:
		case <-drainCtx.Done():
			timedOut = true
		}

		if cancelInflight && timedOut {
			m.cancelAllInvocations()
			m.forceFinalizeAll(499, "request canceled during shutdown")
			if m.scheduler != nil {
				m.scheduler.stop()
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			shutdown(false, true)
			return
		case line, ok := <-m.input.lines():
			if !ok {
				shutdown(false, true)
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
				shutdown(true, true)
				return
			case inputEventCancel:
				m.handleCancelEvent(event)
				continue
			case inputEventCall:
				m.dispatchInvocation(ctx, event.fn)
			}
		}
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
	if !m.trySetInvocationState(fn.UID, stateQueued, cancel, scheduleKey) {
		cancel()
		m.respf(fn, 409, "duplicate active transaction id: %s", fn.UID)
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

	if err := m.scheduler.enqueue(req); err != nil {
		cancel()
		switch {
		case errors.Is(err, errSchedulerQueueFull):
			m.respf(fn, 503, "function queue is full")
		case errors.Is(err, errSchedulerStopping):
			m.respf(fn, 503, "functions manager is stopping")
		case errors.Is(err, errSchedulerInvalid):
			m.respf(fn, 500, "invalid scheduler request")
		default:
			m.respf(fn, 503, "function queue is full")
		}
	}
}

func (m *Manager) trySetInvocationState(uid string, state invocationState, cancel context.CancelFunc, scheduleKey string) bool {
	if uid == "" {
		return false
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	m.pruneExpiredTombstonesLocked(time.Now())

	if _, ok := m.tombstones[uid]; ok {
		return false
	}

	if _, ok := m.invState[uid]; ok {
		return false
	}
	m.invState[uid] = &invocationRecord{
		state:       state,
		cancel:      cancel,
		scheduleKey: scheduleKey,
	}
	return true
}

func (m *Manager) setAwaitingResultState(uid string, fnTimeout time.Duration) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	if rec, ok := m.invState[uid]; ok {
		rec.state = stateAwaitingResult
		rec.awaitingSince = time.Now()

		delay := m.awaitingWarnDelay
		if fnTimeout > 0 && fnTimeout < delay {
			delay = fnTimeout
		}
		if delay <= 0 {
			m.invStateMux.Unlock()
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
	m.invStateMux.Unlock()
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

	state, ok := m.requestCancellation(uid)
	if !ok {
		m.Debugf("ignoring cancel for unknown transaction id: %s", uid)
		return
	}
	if state == stateQueued {
		m.respUID(uid, 499, "request canceled")
	}
}

func (m *Manager) requestCancellation(uid string) (invocationState, bool) {
	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	rec, ok := m.invState[uid]
	if !ok || rec == nil {
		return 0, false
	}

	if rec.cancelRequested {
		return rec.state, true
	}
	rec.cancelRequested = true
	if rec.cancel != nil {
		rec.cancel()
	}

	if rec.state == stateQueued && m.scheduler != nil {
		m.scheduler.cancelQueued(rec.scheduleKey, uid)
	}

	if rec.state == stateRunning || rec.state == stateAwaitingResult {
		if rec.fallbackTimer == nil {
			uidCopy := uid
			rec.fallbackTimer = time.AfterFunc(m.cancelFallbackDelay, func() {
				m.respUID(uidCopy, 499, "request canceled")
			})
		}
	}

	return rec.state, true
}

func (m *Manager) setStopping(v bool) {
	m.stopping.Store(v)
}

func (m *Manager) isStopping() bool {
	return m.stopping.Load()
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
		return false
	}

	var scheduleKey string
	if rec, ok := m.invState[uid]; ok && rec != nil {
		if rec.fallbackTimer != nil {
			rec.fallbackTimer.Stop()
			rec.fallbackTimer = nil
		}
		if rec.awaitingTimer != nil {
			rec.awaitingTimer.Stop()
			rec.awaitingTimer = nil
		}
		scheduleKey = rec.scheduleKey
	}
	delete(m.invState, uid)
	m.tombstones[uid] = now.Add(m.tombstoneTTL)
	m.invStateMux.Unlock()

	if scheduleKey != "" && m.scheduler != nil {
		m.scheduler.complete(scheduleKey, uid)
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
			for prefix, handler := range fs.prefixes {
				snap.prefixes[prefix] = handler
			}
		}
	}
	m.mux.Unlock()

	if !ok || fs == nil {
		return functionSnapshot{}, false
	}
	return snap, true
}

func matchBestPrefix(prefixes map[string]func(Function), id string) (string, func(Function), bool) {
	if len(prefixes) == 0 || id == "" {
		return "", nil, false
	}

	var (
		bestPrefix  string
		bestHandler func(Function)
		matched     bool
	)
	for prefix, handler := range prefixes {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		if !matched ||
			len(prefix) > len(bestPrefix) ||
			(len(prefix) == len(bestPrefix) && prefix < bestPrefix) {
			bestPrefix = prefix
			bestHandler = handler
			matched = true
		}
	}

	if !matched {
		return "", nil, false
	}
	return bestPrefix, bestHandler, true
}

func (m *Manager) lookupFunctionRoute(fn Function) (handler func(Function), scheduleKey string, ok bool) {
	snap, ok := m.snapshotFunction(fn.Name)
	if !ok {
		return nil, "", false
	}

	if len(snap.prefixes) > 0 {
		if len(fn.Args) > 0 {
			id := fn.Args[0]
			if prefix, routeHandler, matched := matchBestPrefix(snap.prefixes, id); matched && routeHandler != nil {
				return routeHandler, routeScheduleKey(fn.Name, prefix), true
			}
		}

		return func(f Function) {
			m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
		}, routeScheduleKey(fn.Name, scheduleKeyUnmatched), true
	}

	if snap.direct != nil {
		return snap.direct, routeScheduleKey(fn.Name, ""), true
	}

	return func(f Function) {
		m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
	}, routeScheduleKey(fn.Name, scheduleKeyDirectMissing), true
}

// lookupFunction returns a snapshot handler used by existing tests to verify
// registry snapshot semantics at lookup time.
func (m *Manager) lookupFunction(name string) (func(Function), bool) {
	snap, ok := m.snapshotFunction(name)
	if !ok {
		return nil, false
	}

	return func(f Function) {
		if len(snap.prefixes) > 0 {
			if len(f.Args) > 0 {
				id := f.Args[0]
				if _, handler, matched := matchBestPrefix(snap.prefixes, id); matched && handler != nil {
					handler(f)
					return
				}
			}
			m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
			return
		}

		if snap.direct != nil {
			snap.direct(f)
			return
		}

		m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
	}, true
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

	var bs []byte
	if code >= 400 && code < 600 {
		bs, _ = json.Marshal(struct {
			Status       int    `json:"status"`
			ErrorMessage string `json:"errorMessage"`
		}{
			Status:       code,
			ErrorMessage: msg,
		})
	} else {
		bs, _ = json.Marshal(struct {
			Status  int    `json:"status"`
			Message string `json:"message"`
		}{
			Status:  code,
			Message: msg,
		})
	}

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
