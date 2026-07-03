// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// The executor is the single dispatch seam between the run loop and the
// command handlers: every discovery and dyncfg event is classified into an
// event (kind, domain, derived key) and executed through dispatch.
//
// Collector-domain events run under PER-KEY lanes: each key is idle, busy
// (a blocking phase in flight on the effect pool), or wait-parked (a
// discovered config awaiting its enable/disable decision). While a key is
// busy ALL of its events park in arrival order; while it is wait-parked only
// discovery events park - inbound dyncfg commands execute immediately with
// the state machine's outcomes, and an enable/disable decision unparks the
// key. Events for different keys never wait on each other. Commits always
// run on the run-loop goroutine; blocking work runs on the pool.
//
// Secretstore and vnode commands stay loop-synchronous: their domains are
// not on the executor lanes and their keys never go busy.

type eventKind int8

const (
	eventDiscoveryAdd eventKind = iota + 1
	eventDiscoveryRemove
	eventDyncfgCommand
)

type eventDomain int8

const (
	domainUnknown eventDomain = iota
	domainCollector
	domainSecretStore
	domainVnode
)

// event is one unit of work for the executor. Discovery events carry cfg;
// dyncfg command events carry fn.
type event struct {
	kind   eventKind
	domain eventDomain
	key    string
	cfg    confgroup.Config
	fn     dyncfg.Function
}

// effectPoolSize bounds concurrently executing blocking phases. Internal
// constant by design.
const effectPoolSize = 4

// keyState is the loop-owned lane state of one key.
type keyState struct {
	busy          bool
	wedged        bool
	waiting       bool
	generation    uint64
	wedgedGen     uint64
	fifo          []event
	waitFIFO      []event
	pendingCommit func(error)
	afterIdle     []func()
	firstParkedAt time.Time
	waitSince     time.Time
}

func (ks *keyState) empty() bool {
	return !ks.busy && !ks.wedged && !ks.waiting && ks.pendingCommit == nil &&
		len(ks.fifo) == 0 && len(ks.waitFIFO) == 0 && len(ks.afterIdle) == 0
}

// effectTask is one blocking phase handed to the effect pool.
type effectTask struct {
	key    string
	effect func(ctx context.Context) error
}

type executor struct {
	mgr      *Manager
	keys     map[string]*keyState
	inflight int
	// pending holds effect tasks awaiting a pool slot: dispatch NEVER blocks
	// the loop (it must keep draining dyncfgCh and completions even with the
	// pool saturated); completions flush this queue.
	pending []effectTask
	// draining flips at shutdown: chained blocking phases then run inline so
	// pending commits complete without pool workers.
	draining bool
}

func newExecutor(mgr *Manager) *executor {
	return &executor{mgr: mgr, keys: make(map[string]*keyState)}
}

// stateKey namespaces lane state by domain so key strings from different
// domains can never collide.
func (ev event) stateKey() string {
	return string('0'+byte(ev.domain)) + "|" + ev.key
}

// dispatch routes ev into its key's lane on the run-loop goroutine.
func (e *executor) dispatch(ev event) {
	if ev.kind == eventDyncfgCommand && ev.domain != domainCollector {
		e.dispatchDyncfg(ev)
		return
	}
	if ev.kind == eventDyncfgCommand && ev.fn.Command() == dyncfg.CommandTest {
		// Collector test is KEYLESS by contract: it runs in its own capped
		// off-loop pool and must never park behind a busy key (a wedged
		// 120s detection would block an interactive test).
		e.mgr.dyncfgCmdTest(ev.fn)
		return
	}
	sk := ev.stateKey()
	ks := e.keys[sk]
	if ks == nil {
		ks = &keyState{}
		e.keys[sk] = ks
	}
	e.route(sk, ks, ev)
	e.maybeDelete(sk, ks)
	e.observeState()
}

func (e *executor) route(sk string, ks *keyState, ev event) {
	if ks.busy {
		if len(ks.fifo) == 0 {
			ks.firstParkedAt = time.Now()
		}
		ks.fifo = append(ks.fifo, ev)
		e.observePark()
		return
	}
	if ks.waiting {
		if ev.kind != eventDyncfgCommand {
			if len(ks.waitFIFO) == 0 && len(ks.fifo) == 0 {
				ks.firstParkedAt = time.Now()
			}
			ks.waitFIFO = append(ks.waitFIFO, ev)
			e.observePark()
			return
		}
		if cmd := ev.fn.Command(); cmd == dyncfg.CommandEnable || cmd == dyncfg.CommandDisable {
			// The decision unparks the key: it executes first, then the
			// wait-parked events replay. Wait-parked events always arrived
			// BEFORE anything still sitting in the fifo (settle moves them
			// out of earlier fifo positions, and nothing arrives mid-execute
			// on the single loop goroutine), so once the key goes busy the
			// remainder must re-enter AT THE FRONT of the fifo - appending
			// would reorder same-kind events (discovery B before A).
			ks.waiting = false
			ks.waitSince = time.Time{}
			e.execute(sk, ks, ev)
			parked := ks.waitFIFO
			ks.waitFIFO = nil
			for i, pev := range parked {
				if ks.busy {
					rest := append([]event(nil), parked[i:]...)
					ks.fifo = append(rest, ks.fifo...)
					if ks.firstParkedAt.IsZero() {
						ks.firstParkedAt = time.Now()
					}
					return
				}
				e.route(sk, ks, pev)
			}
			return
		}
		// Non-decision dyncfg commands never wait-park: they execute
		// immediately with the state machine's outcomes.
		e.execute(sk, ks, ev)
		return
	}
	e.execute(sk, ks, ev)
}

func (e *executor) execute(sk string, ks *keyState, ev event) {
	switch ev.kind {
	case eventDiscoveryAdd:
		e.mgr.stagedAddConfig(ev.cfg, e.runnerFor(sk), func() {
			ks.waiting = true
			ks.waitSince = time.Now()
		}, func(fn dyncfg.Function) {
			e.dispatch(e.mgr.newDyncfgEvent(fn))
		})
	case eventDiscoveryRemove:
		e.mgr.stagedRemoveConfig(ev.cfg, e.runnerFor(sk))
	case eventDyncfgCommand:
		e.executeCollectorCommand(sk, ks, ev.fn)
	}
}

func (e *executor) executeCollectorCommand(sk string, ks *keyState, fn dyncfg.Function) {
	m := e.mgr
	// jobmgr no longer arms the shared handler's wait gate (waiting lives on
	// the key lanes); SyncDecision on an unarmed gate is a no-op and stays
	// for code parity with the service-discovery consumer.
	m.collectorHandler.SyncDecision(fn)

	run := e.runnerFor(sk)
	switch cmd := fn.Command(); cmd {
	case dyncfg.CommandAdd:
		m.collectorHandler.CmdAddStep(fn, run)
		e.afterIdleHook(ks, func() { m.syncSecretStoreDepsByFunction(fn) })
	case dyncfg.CommandUpdate:
		m.collectorHandler.CmdUpdateStep(fn, run)
		e.afterIdleHook(ks, func() { m.syncSecretStoreDepsByFunction(fn) })
	case dyncfg.CommandRemove:
		m.collectorHandler.CmdRemoveStep(fn, run)
		e.afterIdleHook(ks, func() { m.syncSecretStoreDepsByFunction(fn) })
	case dyncfg.CommandEnable:
		m.collectorHandler.CmdEnableStep(fn, run)
	case dyncfg.CommandDisable:
		m.collectorHandler.CmdDisableStep(fn, run)
	case dyncfg.CommandRestart:
		m.collectorHandler.CmdRestartStep(fn, run)
	case dyncfg.CommandSchema:
		m.dyncfgCmdSchema(fn)
	case dyncfg.CommandGet:
		m.dyncfgCmdGet(fn)
	default:
		m.Warningf("dyncfg: function '%s' command '%s' not implemented", fn.Fn().Name, cmd)
		m.dyncfgResponder.SendCodef(fn, 501, "Function '%s' command '%s' is not implemented.", fn.Fn().Name, cmd)
	}
}

// afterIdleHook runs h when the key's current command fully settles (all
// chained phases committed); immediately when it never went busy.
func (e *executor) afterIdleHook(ks *keyState, h func()) {
	if ks.busy {
		ks.afterIdle = append(ks.afterIdle, h)
		return
	}
	h()
}

// runnerFor is the executor's asynchronous StepRunner: it marks the key
// busy, ships the blocking phase to the pool, and resumes the commit on the
// run loop when the completion arrives. During shutdown drain it degrades to
// inline execution.
func (e *executor) runnerFor(sk string) dyncfg.StepRunner {
	return func(effect func(context.Context) error, commit func(error)) {
		if e.draining {
			// Shutdown starts no new blocking phases: a chained phase during
			// the drain is refused with the never-ran outcome (stop-shaped
			// commits answer 503 instead of publishing untruthful state).
			commit(dyncfg.ErrPhaseNeverRan)
			return
		}
		ks := e.keys[sk]
		if ks.wedged {
			// Reachable when a commit chains a phase after its own stop was
			// abandoned: the key is held by the leaked call, so the chained
			// phase never runs (never-ran keeps the commit truthful).
			e.mgr.Warningf("refusing chained phase on wedged key '%s'", sk)
			commit(dyncfg.ErrPhaseNeverRan)
			return
		}
		ks.busy = true
		ks.pendingCommit = commit
		e.inflight++
		if mx := e.mgr.executorMetrics; mx != nil {
			mx.effectsStarted.Add(1)
		}
		e.dispatchEffect(effectTask{key: sk, effect: effect})
	}
}

// dispatchEffect hands a task to the pool WITHOUT EVER BLOCKING: tasks
// enter the executor's pending queue and move to free pool slots here and
// as completions arrive. The loop therefore always keeps draining dyncfgCh,
// discovery, and completions regardless of pool pressure, and a later
// dispatch never overtakes an earlier one parked against a full pool.
func (e *executor) dispatchEffect(t effectTask) {
	e.pending = append(e.pending, t)
	e.flushPendingEffects()
}

// flushPendingEffects moves parked tasks to freed pool slots.
func (e *executor) flushPendingEffects() {
	for len(e.pending) > 0 {
		select {
		case e.mgr.effectCh <- e.pending[0]:
			e.pending = e.pending[1:]
		default:
			return
		}
	}
}

// failPendingPhase commits the never-ran outcome for a phase that could not
// be dispatched or executed (shutdown).
func (e *executor) failPendingPhase(sk string) {
	ks := e.keys[sk]
	if ks == nil || ks.pendingCommit == nil {
		return
	}
	commit := ks.pendingCommit
	ks.pendingCommit = nil
	ks.busy = false
	e.inflight--
	commit(dyncfg.ErrPhaseNeverRan)
}

// onEffectDone resumes a completed blocking phase's commit on the run loop.
func (e *executor) onEffectDone(res effectResult) {
	ks := e.keys[res.key]
	if res.late {
		e.onLateReturn(res, ks)
		return
	}
	if ks == nil || !ks.busy || ks.pendingCommit == nil {
		e.mgr.Errorf("BUG: stray effect completion for key '%s' (err: %v)", res.key, res.err)
		return
	}
	commit := ks.pendingCommit
	ks.pendingCommit = nil
	e.inflight--
	if mx := e.mgr.executorMetrics; mx != nil && res.busyFor > 0 {
		mx.effectBusySeconds.Add(res.busyFor.Seconds())
	}
	if res.abandoned {
		// The deadline outcome commits now, but the key stays busy (wedged)
		// until the leaked module call returns. No retry and no follow-up
		// work is scheduled here; the late outcome decides.
		ks.wedged = true
		commit(res.err)
		ks.generation++
		ks.wedgedGen = ks.generation
		e.flushPendingEffects()
		e.observeState()
		return
	}
	ks.busy = false
	commit(res.err) // may chain the next phase, re-marking the key busy
	ks.generation++
	e.settle(res.key, ks)
	e.maybeDelete(res.key, ks)
	e.flushPendingEffects()
	e.observeState()
}

// onLateReturn unwedges a key whose abandoned effect finally returned. A
// warm detection success starts through the normal start path when the
// continuation is still valid (nothing committed on the key since the
// abandon - structurally guaranteed while wedged, asserted here); anything
// else was already handled by the late protocol inside the closure.
func (e *executor) onLateReturn(res effectResult, ks *keyState) {
	if ks == nil || !ks.wedged {
		e.mgr.Errorf("BUG: late completion for a key that is not wedged ('%s')", res.key)
		if res.resume != nil {
			e.dropWarmResume(res.resume)
		}
		return
	}
	ks.wedged = false
	ks.busy = false
	e.mgr.Infof("abandoned operation for key '%s' returned (err: %v)", res.key, res.err)

	if res.resume != nil {
		switch {
		case e.draining:
			// Shutdown never starts fresh collector work: a warm job that
			// arrives during the drain is dropped, not resumed (no late
			// re-enables and no running publish after shutdown began).
			e.mgr.Infof("dropping late detection success for key '%s': shutting down", res.key)
			e.dropWarmResume(res.resume)
		case ks.generation != ks.wedgedGen:
			if mx := e.mgr.executorMetrics; mx != nil {
				mx.staleCommits.Add(1)
			}
			e.mgr.Warningf("dropping late detection success for key '%s': config changed during operation", res.key)
			e.dropWarmResume(res.resume)
		case keyFIFOHasStopIntent(ks):
			// A disable/remove is already queued behind the wedge: the user's
			// intent wins over the continuation - the warm job is dropped and
			// the queued command executes against the failed entry.
			e.mgr.Infof("dropping late detection success for key '%s': a stop command is queued", res.key)
			e.dropWarmResume(res.resume)
		default:
			e.mgr.resumeWarmJob(res.resume)
			ks.generation++
		}
	}
	e.settle(res.key, ks)
	e.maybeDelete(res.key, ks)
	e.observeState()
}

// dropWarmResume disposes a warm job that will never start: module cleanup
// plus deregistering its own emission gate (matched by handle - a same-name
// replacement's gate must survive).
func (e *executor) dropWarmResume(r *warmResume) {
	r.job.Cleanup()
	e.mgr.emissionGates.removeMatching(r.cfg.FullName(), r.gate)
}

// settle drains a key's after-idle hooks and parked events until the key
// goes busy again or nothing is left.
func (e *executor) settle(sk string, ks *keyState) {
	for !ks.busy {
		if len(ks.afterIdle) > 0 {
			hooks := ks.afterIdle
			ks.afterIdle = nil
			for _, h := range hooks {
				h()
			}
			continue
		}
		if len(ks.fifo) == 0 {
			return
		}
		ev := ks.fifo[0]
		ks.fifo = ks.fifo[1:]
		if len(ks.fifo) == 0 {
			ks.firstParkedAt = time.Time{}
		}
		// Remaining events keep the popped event's timestamp: overstating the
		// oldest parked age is the safe direction for an aging alarm.
		e.route(sk, ks, ev)
	}
}

func keyFIFOHasStopIntent(ks *keyState) bool {
	for _, ev := range ks.fifo {
		switch ev.kind {
		case eventDiscoveryRemove:
			// The config is going away: starting the warm job would be a
			// transient start immediately torn down by the queued removal.
			return true
		case eventDyncfgCommand:
			if cmd := ev.fn.Command(); cmd == dyncfg.CommandDisable || cmd == dyncfg.CommandRemove {
				return true
			}
		}
	}
	return false
}

func (e *executor) maybeDelete(sk string, ks *keyState) {
	if ks.empty() {
		delete(e.keys, sk)
	}
}

func (e *executor) busyCount() int {
	n := 0
	for _, ks := range e.keys {
		if ks.busy || ks.wedged {
			n++
		}
	}
	return n
}

// shutdownDrain waits out in-flight blocking phases (running their commits,
// with chained phases inline) for a bounded window, then answers every still
// parked dyncfg command with 503.
func (e *executor) shutdownDrain() {
	e.draining = true
	defer close(e.mgr.lateDrop)

	// Commands the handoff accepted into dyncfgCh before cancellation would
	// otherwise be stranded unread (the run loop has exited): answer them
	// retryably. A send racing cancellation itself can still land after this
	// drain; the functions layer's shutdown force-finalize is the terminal
	// backstop for that window.
	for {
		select {
		case fn := <-e.mgr.dyncfgCh:
			e.mgr.dyncfgResponder.SendCodef(fn, 503, dyncfgShuttingDownMsg)
			if mx := e.mgr.executorMetrics; mx != nil {
				mx.shutdownRejected.Add(1)
			}
		default:
			goto dyncfgDrained
		}
	}
dyncfgDrained:

	// Workers exit on shutdown, so parked and queued-but-unpicked effects
	// would leave their commands without a terminal outcome: fail their
	// commits with the never-ran outcome (stop-shaped commits answer 503
	// rather than publish state for stops that never happened).
	for _, t := range e.pending {
		e.failPendingPhase(t.key)
	}
	e.pending = nil
	for {
		select {
		case t := <-e.mgr.effectCh:
			e.failPendingPhase(t.key)
		default:
			goto drained
		}
	}
drained:

	deadline := time.NewTimer(e.mgr.drainWait)
	defer deadline.Stop()
	for e.busyCount() > 0 {
		select {
		case res := <-e.mgr.effectDoneCh:
			e.onEffectDone(res)
		case <-deadline.C:
			e.mgr.Warningf("executor: timeout waiting %s for in-flight effects to drain", e.mgr.drainWait)
			goto force
		}
	}
force:
	for sk, ks := range e.keys {
		// A result that never arrived (or was dropped against a full channel
		// after the window expired) must not leave its command silent: fail
		// the still-pending commit with the never-ran outcome.
		e.failPendingPhase(sk)
		parked := append(append([]event(nil), ks.fifo...), ks.waitFIFO...)
		ks.fifo, ks.waitFIFO = nil, nil
		for _, ev := range parked {
			if ev.kind == eventDyncfgCommand {
				e.mgr.dyncfgResponder.SendCodef(ev.fn, 503, dyncfgShuttingDownMsg)
				if mx := e.mgr.executorMetrics; mx != nil {
					mx.shutdownRejected.Add(1)
				}
			}
		}
	}

	// The bounded drain is over: account for abandoned module calls that
	// still have not returned - their goroutines stay leaked by design
	// (deadline-ignoring collector calls cannot be cancelled) and their late
	// completions will be dropped via lateDrop.
	if n := e.mgr.leakedNow.Load(); n > 0 {
		e.mgr.Warningf("executor: %d abandoned collector call(s) still running at shutdown (leaked)", n)
	}
}

// collectorStateKey is the lane-state key for a collector config key.
func collectorStateKey(key string) string {
	return event{domain: domainCollector, key: key}.stateKey()
}

// collectorKeyHasWork reports whether the collector key has a blocking
// phase in flight (busy or wedged). Wait-parking is NOT work: a config
// awaiting its enable/disable decision has no job and no resolved secret,
// so store rotations must keep ignoring it. Loop-owned state: call only on
// the run-loop goroutine.
func (e *executor) collectorKeyHasWork(key string) bool {
	ks := e.keys[collectorStateKey(key)]
	return ks != nil && (ks.busy || ks.wedged)
}

// tryLockIdleKey claims a collector key for a LOOP-SYNCHRONOUS inline
// operation (the secretstore dependent-restart bridge). It succeeds only for
// a fully idle key - any in-flight effect, wedge, wait-park, or parked work
// means the caller must skip, never wait: effect completions arrive via this
// same loop, so waiting here would self-deadlock.
func (e *executor) tryLockIdleKey(sk string) bool {
	ks := e.keys[sk]
	if ks == nil {
		ks = &keyState{}
		e.keys[sk] = ks
	}
	if !ks.empty() {
		return false
	}
	ks.busy = true
	e.observeState()
	return true
}

// unlockIdleKey releases a tryLockIdleKey claim and drains anything that
// parked behind it meanwhile.
func (e *executor) unlockIdleKey(sk string) {
	ks := e.keys[sk]
	if ks == nil || !ks.busy {
		e.mgr.Errorf("BUG: unlock of a key that is not held ('%s')", sk)
		return
	}
	ks.busy = false
	e.settle(sk, ks)
	e.maybeDelete(sk, ks)
	e.observeState()
}

// observePark counts one parked event.
func (e *executor) observePark() {
	if mx := e.mgr.executorMetrics; mx != nil {
		mx.parkedTotal.Add(1)
	}
}

// observeState recomputes the aggregate lane gauges and the oldest-age
// atomics after a state transition. Runs on the loop; O(active keys).
func (e *executor) observeState() {
	mx := e.mgr.executorMetrics
	if mx == nil {
		return
	}
	var busy, waiting, parked int
	var oldestParked, oldestWait time.Time
	for _, ks := range e.keys {
		if ks.busy {
			busy++
		}
		if ks.waiting {
			waiting++
			if oldestWait.IsZero() || ks.waitSince.Before(oldestWait) {
				oldestWait = ks.waitSince
			}
		}
		parked += len(ks.fifo) + len(ks.waitFIFO)
		if len(ks.fifo)+len(ks.waitFIFO) > 0 && !ks.firstParkedAt.IsZero() &&
			(oldestParked.IsZero() || ks.firstParkedAt.Before(oldestParked)) {
			oldestParked = ks.firstParkedAt
		}
	}
	mx.busyKeys.Set(float64(busy))
	mx.waitParkedKeys.Set(float64(waiting))
	mx.parkedEvents.Set(float64(parked))
	mx.poolInflight.Set(float64(e.inflight))
	mx.poolQueued.Set(float64(len(e.pending) + len(e.mgr.effectCh)))
	mx.oldestParkedSince.Store(unixNanosOrZero(oldestParked))
	mx.oldestWaitSince.Store(unixNanosOrZero(oldestWait))
}

func unixNanosOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func (e *executor) dispatchDyncfg(ev event) {
	switch ev.domain {
	case domainSecretStore:
		e.mgr.dyncfgSecretStoreSeqExec(ev.fn)
	case domainVnode:
		e.mgr.dyncfgVnodeSeqExec(ev.fn)
	default:
		e.mgr.dyncfgRespondUnknown(ev.fn)
	}
}

func (m *Manager) newDiscoveryAddEvent(cfg confgroup.Config) event {
	return event{kind: eventDiscoveryAdd, domain: domainCollector, key: cfg.ExposedKey(), cfg: cfg}
}

func (m *Manager) newDiscoveryRemoveEvent(cfg confgroup.Config) event {
	return event{kind: eventDiscoveryRemove, domain: domainCollector, key: cfg.ExposedKey(), cfg: cfg}
}

// newDyncfgEvent classifies a dyncfg command by ID prefix and derives its
// key. Underivable input keeps the domain's fallback key and still executes,
// so it reaches the handler's existing rejection paths unchanged.
func (m *Manager) newDyncfgEvent(fn dyncfg.Function) event {
	ev := event{kind: eventDyncfgCommand, domain: m.dyncfgDomain(fn), fn: fn}

	var key string
	var ok bool
	switch ev.domain {
	case domainSecretStore:
		key, ok = m.secretsCtl.DeriveKey(fn)
	case domainCollector:
		key, _, _, ok = m.collectorCommandKey(fn)
	case domainVnode:
		key, ok = m.vnodesCtl.DeriveKey(fn)
	}
	if !ok {
		key = m.dyncfgFallbackKey(ev.domain)
	}
	ev.key = key
	return ev
}

func (m *Manager) dyncfgDomain(fn dyncfg.Function) eventDomain {
	switch {
	case strings.HasPrefix(fn.ID(), m.dyncfgSecretStorePrefixValue()):
		return domainSecretStore
	case strings.HasPrefix(fn.ID(), m.dyncfgCollectorPrefixValue()):
		return domainCollector
	case strings.HasPrefix(fn.ID(), m.dyncfgVnodePrefixValue()):
		return domainVnode
	default:
		return domainUnknown
	}
}

// dyncfgFallbackKey is the registration-wide key for commands whose config
// identity cannot be derived; unknown-prefix commands are rejected at
// dispatch and get no domain key.
func (m *Manager) dyncfgFallbackKey(domain eventDomain) string {
	switch domain {
	case domainSecretStore:
		return m.dyncfgSecretStorePrefixValue()
	case domainCollector:
		return m.dyncfgCollectorPrefixValue()
	case domainVnode:
		return m.dyncfgVnodePrefixValue()
	default:
		return ""
	}
}

// laneDeriverRegistry is the optional lane-narrowing capability of the
// functions registry (satisfied by the functions manager; registries without
// it keep one lane per prefix registration).
type laneDeriverRegistry interface {
	RegisterPrefixLaneDeriver(name, prefix string, derive functions.LaneKeyDeriver)
}

// registerDyncfgLaneDerivers narrows the functions schedule lanes for the
// dyncfg prefixes to the config identity each command addresses, using the
// same silent key derivation the executor uses for its events. Underivable
// requests keep the registration-wide lane and still reach the handlers'
// rejection paths. Derivers run on the functions manager goroutine: they only
// read state that is immutable after startup (module registry, controller
// prefixes) or internally synchronized.
func (m *Manager) registerDyncfgLaneDerivers() {
	reg, ok := m.fnReg.(laneDeriverRegistry)
	if !ok {
		return
	}
	reg.RegisterPrefixLaneDeriver("config", m.dyncfgCollectorPrefixValue(), func(fn functions.Function) (string, any) {
		dfn := dyncfg.NewFunction(fn)
		if dfn.Command() == dyncfg.CommandTest {
			// Test is keyless END-TO-END and for EVERY ID form (job-level
			// and template-level): a unique lane per request keeps it from
			// serializing behind the config's mutating lane (held until that
			// command's terminal), other tests, or the registration-wide
			// fallback lane; concurrency is capped by the test pool's
			// cap-and-503 instead.
			return "test|" + fn.UID, nil
		}
		key, _, _, ok := m.collectorCommandKey(dfn)
		if !ok {
			return "", nil
		}
		return key, nil
	})
	reg.RegisterPrefixLaneDeriver("config", m.dyncfgSecretStorePrefixValue(), func(fn functions.Function) (string, any) {
		key, ok := m.secretsCtl.DeriveKey(dyncfg.NewFunction(fn))
		if !ok {
			return "", nil
		}
		return key, nil
	})
	reg.RegisterPrefixLaneDeriver("config", m.dyncfgVnodePrefixValue(), func(fn functions.Function) (string, any) {
		key, ok := m.vnodesCtl.DeriveKey(dyncfg.NewFunction(fn))
		if !ok {
			return "", nil
		}
		return key, nil
	})
}
