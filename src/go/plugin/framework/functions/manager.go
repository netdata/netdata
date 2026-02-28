// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
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
	defaultWorkerCount          = 1
	defaultQueueSize            = 64
	defaultCancelFallbackDelay  = 5 * time.Second
	defaultShutdownDrainTimeout = 8 * time.Second
	defaultTombstoneTTL         = 60 * time.Second
)

type invocationState uint8

const (
	stateQueued invocationState = iota + 1
	stateRunning
	stateAwaitingResult
)

type invocationRequest struct {
	fn      *Function
	handler func(Function)
	ctx     context.Context
}

type invocationRecord struct {
	state           invocationState
	cancel          context.CancelFunc
	cancelRequested bool
	fallbackTimer   *time.Timer
}

func NewManager() *Manager {
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "functions manager"),
		),
		api:                  netdataapi.New(safewriter.Stdout),
		input:                newStdinInput(),
		mux:                  &sync.Mutex{},
		FunctionRegistry:     make(map[string]*functionSet),
		workerCount:          defaultWorkerCount,
		queueSize:            defaultQueueSize,
		invStateMux:          &sync.Mutex{},
		invState:             make(map[string]*invocationRecord),
		tombstones:           make(map[string]time.Time),
		tombstoneTTL:         defaultTombstoneTTL,
		cancelFallbackDelay:  defaultCancelFallbackDelay,
		shutdownDrainTimeout: defaultShutdownDrainTimeout,
	}
}

type Manager struct {
	*logger.Logger

	api *netdataapi.API

	input input

	mux              *sync.Mutex
	FunctionRegistry map[string]*functionSet

	workerCount int
	queueSize   int

	invStateMux          *sync.Mutex
	invState             map[string]*invocationRecord
	tombstones           map[string]time.Time
	tombstoneTTL         time.Duration
	cancelFallbackDelay  time.Duration
	shutdownDrainTimeout time.Duration
	stopping             atomic.Bool
}

func (m *Manager) Run(ctx context.Context, quitCh chan struct{}) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()
	restoreFinalizeHook := setFinalizeHook(m.TryFinalize)
	defer restoreFinalizeHook()
	m.run(ctx, quitCh)
}

func (m *Manager) run(ctx context.Context, quitCh chan struct{}) {
	parser := newInputParser()
	workCh := make(chan *invocationRequest, m.queueSize)
	var workersWG sync.WaitGroup

	for range m.workerCount {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			m.runWorker(workCh)
		}()
	}
	shutdown := func(signalQuit, cancelInflight bool) {
		m.setStopping(true)
		if signalQuit && quitCh != nil {
			quitCh <- struct{}{}
		}

		if cancelInflight {
			m.cancelAllInvocations()
		}
		close(workCh)

		drainCtx, cancelDrain := context.WithTimeout(context.Background(), m.shutdownDrainTimeout)
		defer cancelDrain()

		done := make(chan struct{})
		go func() {
			defer close(done)
			workersWG.Wait()
		}()

		select {
		case <-done:
		case <-drainCtx.Done():
		}

		if cancelInflight {
			m.forceFinalizeAll(499, "request canceled during shutdown")
		}
	}

	for {
		select {
		case <-ctx.Done():
			shutdown(false, true)
			return
		case line, ok := <-m.input.lines():
			if !ok {
				shutdown(false, false)
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
				m.dispatchInvocation(ctx, event.fn, workCh)
			}
		}
	}
}

func (m *Manager) dispatchInvocation(parentCtx context.Context, fn *Function, workCh chan<- *invocationRequest) {
	if fn == nil {
		return
	}
	if m.isStopping() {
		m.respf(fn, 503, "functions manager is stopping")
		return
	}

	handler, ok := m.lookupFunction(fn.Name)
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
	if !m.trySetInvocationState(fn.UID, stateQueued, cancel) {
		cancel()
		m.respf(fn, 409, "duplicate active transaction id: %s", fn.UID)
		return
	}

	req := &invocationRequest{
		fn:      fn,
		handler: handler,
		ctx:     reqCtx,
	}

	select {
	case workCh <- req:
	default:
		cancel()
		m.respf(fn, 503, "function queue is full")
	}
}

func (m *Manager) runWorker(workCh <-chan *invocationRequest) {
	for req := range workCh {
		if req == nil || req.fn == nil || req.handler == nil {
			continue
		}
		if req.ctx != nil && req.ctx.Err() != nil {
			continue
		}
		if !m.startInvocation(req.fn.UID) {
			continue
		}

		panicked := false
		func() {
			defer func() {
				if recover() != nil {
					panicked = true
				}
			}()
			req.handler(*req.fn)
		}()

		if panicked {
			m.respUID(req.fn.UID, 500, "function handler panic")
			continue
		}

		m.setInvocationState(req.fn.UID, stateAwaitingResult)
	}
}

func (m *Manager) trySetInvocationState(uid string, state invocationState, cancel context.CancelFunc) bool {
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
		state:  state,
		cancel: cancel,
	}
	return true
}

func (m *Manager) setInvocationState(uid string, state invocationState) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	if rec, ok := m.invState[uid]; ok {
		rec.state = state
	}
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

// TryFinalize emits a terminal response once per transaction UID.
// Later terminal attempts for the same UID are dropped while tombstone is active.
func (m *Manager) TryFinalize(uid, source string, emit func()) bool {
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

	if rec, ok := m.invState[uid]; ok && rec != nil {
		if rec.fallbackTimer != nil {
			rec.fallbackTimer.Stop()
			rec.fallbackTimer = nil
		}
	}
	delete(m.invState, uid)
	m.tombstones[uid] = now.Add(m.tombstoneTTL)
	m.invStateMux.Unlock()

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

func (m *Manager) lookupFunction(name string) (func(Function), bool) {
	m.mux.Lock()
	fs, ok := m.FunctionRegistry[name]
	var (
		direct   func(Function)
		prefixes map[string]func(Function)
	)
	if ok && fs != nil {
		direct = fs.direct
		if len(fs.prefixes) > 0 {
			prefixes = make(map[string]func(Function), len(fs.prefixes))
			for prefix, handler := range fs.prefixes {
				prefixes[prefix] = handler
			}
		}
	}
	m.mux.Unlock()

	if !ok || fs == nil {
		return nil, false
	}

	return func(f Function) {
		if len(prefixes) > 0 {
			m.handlePrefixRouting(f, prefixes)
			return
		}

		if direct != nil {
			direct(f)
			return
		}

		m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
	}, true
}

func (m *Manager) handlePrefixRouting(f Function, prefixes map[string]func(Function)) {
	if len(f.Args) == 0 {
		m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
		return
	}

	id := f.Args[0]
	for prefix, handler := range prefixes {
		if strings.HasPrefix(id, prefix) {
			handler(f)
			return
		}
	}

	m.respf(&f, 503, "unknown function '%s' (%v)", f.Name, f.Args)
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

	FinalizeTerminal(fn.UID, "functions.manager.respf", func() {
		m.api.FUNCRESULT(res)
	})
}
