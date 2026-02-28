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
	defaultWorkerCount  = 1
	defaultQueueSize    = 64
	defaultTombstoneTTL = 60 * time.Second
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
}

func NewManager() *Manager {
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "functions manager"),
		),
		api:              netdataapi.New(safewriter.Stdout),
		input:            newStdinInput(),
		mux:              &sync.Mutex{},
		FunctionRegistry: make(map[string]*functionSet),
		workerCount:      defaultWorkerCount,
		queueSize:        defaultQueueSize,
		invStateMux:      &sync.Mutex{},
		invState:         make(map[string]invocationState),
		tombstones:       make(map[string]time.Time),
		tombstoneTTL:     defaultTombstoneTTL,
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

	invStateMux  *sync.Mutex
	invState     map[string]invocationState
	tombstones   map[string]time.Time
	tombstoneTTL time.Duration
}

func (m *Manager) Run(ctx context.Context, quitCh chan struct{}) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()
	restoreFinalizeHook := setFinalizeHook(m.TryFinalize)
	defer restoreFinalizeHook()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); m.run(ctx, quitCh) }()

	wg.Wait()

	<-ctx.Done()
}

func (m *Manager) run(ctx context.Context, quitCh chan struct{}) {
	parser := newInputParser()
	workCh := make(chan *invocationRequest, m.queueSize)
	var workersWG sync.WaitGroup

	for range m.workerCount {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			m.runWorker(ctx, workCh)
		}()
	}
	defer func() {
		close(workCh)
		workersWG.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-m.input.lines():
			if !ok {
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
				if quitCh != nil {
					quitCh <- struct{}{}
					return
				}
			case inputEventCancel:
				// Step 4 handles cancellation semantics.
				continue
			case inputEventCall:
				m.dispatchInvocation(event.fn, workCh)
			}
		}
	}
}

func (m *Manager) dispatchInvocation(fn *Function, workCh chan<- *invocationRequest) {
	if fn == nil {
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

	if !m.trySetInvocationState(fn.UID, stateQueued) {
		m.respf(fn, 409, "duplicate active transaction id: %s", fn.UID)
		return
	}

	req := &invocationRequest{
		fn:      fn,
		handler: handler,
	}

	select {
	case workCh <- req:
	default:
		m.respf(fn, 503, "function queue is full")
	}
}

func (m *Manager) runWorker(ctx context.Context, workCh <-chan *invocationRequest) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-workCh:
			if !ok {
				return
			}
			if req == nil || req.fn == nil || req.handler == nil {
				continue
			}

			m.setInvocationState(req.fn.UID, stateRunning)
			req.handler(*req.fn)
			m.setInvocationState(req.fn.UID, stateAwaitingResult)
		}
	}
}

func (m *Manager) trySetInvocationState(uid string, state invocationState) bool {
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
	m.invState[uid] = state
	return true
}

func (m *Manager) setInvocationState(uid string, state invocationState) {
	if uid == "" {
		return
	}

	m.invStateMux.Lock()
	defer m.invStateMux.Unlock()

	if _, ok := m.invState[uid]; ok {
		m.invState[uid] = state
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

func (m *Manager) respf(fn *Function, code int, msgf string, a ...any) {
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
