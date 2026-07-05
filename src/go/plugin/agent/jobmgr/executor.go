// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"
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
// All three dyncfg domains run under PER-KEY lanes: each key is idle,
// occupied (a command anywhere between claim acquisition and its last
// commit, including a blocking phase in flight on the effect pool), or
// wait-parked (a discovered config awaiting its enable/disable decision).
// While a key is occupied ALL of its events park in arrival order; while it
// is wait-parked only discovery events park - inbound dyncfg commands
// execute immediately with the state machine's outcomes, and an
// enable/disable decision unparks the key. Events for different keys never
// wait on each other; cross-key dependencies (a collector command reading a
// store or vnode, a store command restarting dependent jobs) are reserved
// through the claim table before blocking work ships. Commits always run on
// the run-loop goroutine; blocking work runs on the pool. Vnode commands
// are stage+commit only: they execute inline on the loop under their
// vnode-name write claim and their keys never go busy.

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
	// underivable marks a dyncfg command whose config identity could not be
	// derived (key is the domain fallback): it can only ever reach the
	// handler's rejection paths, so it executes claimless-inline - claiming
	// payload refs for it could park an invalid command behind a held key
	// instead of answering 400 immediately.
	underivable bool
}

// effectPoolSize bounds concurrently executing blocking phases. Internal
// constant by design.
const effectPoolSize = 4

// keyState is the loop-owned lane state of one key.
type keyState struct {
	busy          bool
	waiting       bool
	generation    uint64
	fifo          []event
	waitFIFO      []event
	pendingCommit func(error)
	afterIdle     []func()
	firstParkedAt time.Time
	waitSince     time.Time
	// acquiring is the lane's current command while its claim set is being
	// acquired (parked in the claim table); grant is its completed
	// acquisition while the command runs. Together with busy they keep the
	// lane occupied stage-to-commit so same-key events never reorder.
	acquiring *claimRequest
	grant     *claimGrant
	// wedge, when non-nil, IS the wedged state: the lane's command was
	// abandoned at its deadline and the leaked call has not returned (busy
	// stays true for the whole window). Constructed only by the enter-wedged
	// transition action; consumed only by the late return.
	wedge *wedge
}

func (ks *keyState) empty() bool {
	return !ks.occupied() && ks.wedge == nil && !ks.waiting && ks.pendingCommit == nil &&
		len(ks.fifo) == 0 && len(ks.waitFIFO) == 0 && len(ks.afterIdle) == 0
}

// occupied reports whether the lane's current command is anywhere between
// stage (claim acquisition) and commit.
func (ks *keyState) occupied() bool {
	return ks.busy || ks.acquiring != nil || ks.grant != nil
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
	// claims is the multi-key reservation table; claimWork maps in-flight
	// acquisitions back to their events so the shutdown drain can answer
	// claim-parked commands.
	claims    *claimTable
	claimWork map[*claimRequest]event
}

func newExecutor(mgr *Manager) *executor {
	e := &executor{
		mgr:       mgr,
		keys:      make(map[string]*keyState),
		claims:    newClaimTable(),
		claimWork: make(map[*claimRequest]event),
	}
	// A key freed of its last foreign claim may have lane events parked
	// behind it.
	e.claims.released = func(key string) {
		if ks := e.keys[key]; ks != nil {
			e.settle(key, ks)
			e.maybeDelete(key, ks)
		}
	}
	return e
}

// stateKey namespaces lane state by domain so key strings from different
// domains can never collide.
func (ev event) stateKey() string {
	return string('0'+byte(ev.domain)) + "|" + ev.key
}

// dispatch routes ev into its key's lane on the run-loop goroutine.
func (e *executor) dispatch(ev event) {
	if ev.kind == eventDyncfgCommand {
		switch ev.domain {
		case domainUnknown:
			e.mgr.dyncfgRespondUnknown(ev.fn)
			return
		case domainCollector:
			if ev.fn.Command() == dyncfg.CommandTest {
				// Collector test is KEYLESS by contract: it runs in its own
				// capped off-loop pool and must never park behind a busy key
				// (a wedged 120s detection would block an interactive test).
				e.mgr.dyncfgCmdTest(ev.fn)
				return
			}
		}
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
	if ks.occupied() || e.claims.heldForWrite(sk) {
		if !ks.occupied() && len(ks.fifo) == 0 &&
			ev.kind == eventDyncfgCommand && e.planEvent(ev).bypassesForeignWriteHold() {
			// A deterministic rejection/no-op arriving under a FOREIGN write
			// hold (a store command's claim on this dependent job key):
			// answer it claimless-inline instead of parking it behind a hold
			// with no bounded release - a wedged store command's dependent
			// write claims release only at its late return. Same-key order
			// is preserved: the bypass requires an unoccupied lane and an
			// empty lane FIFO, so no earlier same-key event is overtaken
			// (wait-parked discovery events may legitimately be preceded).
			e.execute(sk, ks, ev)
			return
		}
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
				if ks.occupied() {
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
	if e.draining {
		// ONE RULE at shutdown - enforced BEFORE claim acquisition so a
		// command popped from a settling FIFO during the drain can never
		// park in the claim table without a terminal.
		e.refuseAtShutdown(ev)
		return
	}
	plan := e.planEvent(ev)
	if !plan.needsClaims() {
		e.executeEvent(sk, ks, ev)
		return
	}
	// Mutating events reserve their full claim set - the lane key plus the
	// referenced stores and vnode - before any work runs. The lane stays
	// occupied through the acquisition so same-key events keep their order.
	req := &claimRequest{
		label:   claimLabel(ev),
		compute: func() []claim { return e.planEvent(ev).computeClaims() },
	}
	req.granted = func(g *claimGrant) {
		delete(e.claimWork, req)
		ks.acquiring = nil
		ks.grant = g
		e.executeEvent(sk, ks, ev)
		e.finishIfSettled(sk, ks)
	}
	ks.acquiring = req
	e.claimWork[req] = ev
	e.claims.acquire(req)
}

// refuseAtShutdown resolves an event under the one rule: dyncfg commands
// answer 503, discovery events are dropped - nothing executes, nothing
// publishes.
func (e *executor) refuseAtShutdown(ev event) {
	if ev.kind != eventDyncfgCommand {
		return
	}
	e.mgr.dyncfgResponder.SendCodef(ev.fn, 503, dyncfgShuttingDownMsg)
	if mx := e.mgr.executorMetrics; mx != nil {
		mx.shutdownRejected.Add(1)
	}
}

// executeEvent runs an event's body; for claimed events this happens once
// the claim grant is held. The draining re-check covers grants that fire
// during the drain (a release waking a parked acquisition).
func (e *executor) executeEvent(sk string, ks *keyState, ev event) {
	if e.draining {
		e.refuseAtShutdown(ev)
		return
	}
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
		switch ev.domain {
		case domainSecretStore:
			e.executeSecretStoreCommand(sk, ev.fn)
		case domainVnode:
			e.executeVnodeCommand(ev.fn)
		default:
			e.executeCollectorCommand(sk, ks, ev.fn)
		}
	}
}

// executeVnodeCommand runs a vnode command inline: the domain is
// stage+commit only (no blocking work, no effects), so the command completes
// on the loop while its write claim conflicts with collector commands'
// vnode read claims. The cross-vnode uniqueness scan needs no extra claims
// because vnode mutations never leave the loop.
func (e *executor) executeVnodeCommand(fn dyncfg.Function) {
	e.mgr.dyncfgVnodeSeqExec(fn)
}

// executeSecretStoreCommand routes a store command through the controller's
// step API. Note on the remove path's deadline exposure: a store remove
// cannot carry dependent restarts into its effect - removal is 409-refused
// while ANY job references the store (dyncfgCmdRemoveStep's affected-jobs
// guard, checked under the remove's granted store write claim, so no new
// reference can appear before the effect) - and the remaining effect body
// (service.Remove) is in-memory, so a remove effect cannot outlive the
// deadline.
func (e *executor) executeSecretStoreCommand(sk string, fn dyncfg.Function) {
	e.mgr.secretsCtl.StepExec(fn, e.runnerFor(sk))
}

// finishIfSettled releases a claimed command's grant once no blocking phase
// is in flight (the command completed inline or its last commit ran). A
// wedged key keeps its grant until the late return.
func (e *executor) finishIfSettled(sk string, ks *keyState) {
	if ks.busy || ks.grant == nil {
		return
	}
	g := ks.grant
	ks.grant = nil
	e.claims.release(g)
}

func claimLabel(ev event) string {
	if ev.kind == eventDyncfgCommand {
		return string(ev.fn.Command()) + " " + ev.key
	}
	return "discovery " + ev.key
}

// eventClaims computes an event's reservation set.
//
// Collector events: a write claim on their own lane key plus read claims on
// every referenced secret store and the referenced vnode. Update-shaped
// events (dyncfg update, add over an existing config, discovery replace)
// take the UNION of the old and new dependencies - old and new stores/vnodes
// can differ, and claiming only the new set would let commands on the old
// dependencies interleave with the teardown half.
//
// Secretstore events: a write claim on the store key plus write claims on
// every restartable dependent job key (wedged dependents are excluded - they
// are skipped and reported, never waited on); the advisory test takes only a
// read claim on the store.
//
// Recomputed at every (re-)stage attempt by contract; a command that became
// rejection-only while parked (its entry removed, its store deleted)
// recomputes to the EMPTY set, which grants immediately and lets it answer
// its rejection inline.
func (e *executor) eventClaims(ev event) []claim {
	m := e.mgr

	if ev.kind == eventDyncfgCommand && ev.domain == domainSecretStore {
		if ev.fn.Command() == dyncfg.CommandTest {
			return []claim{{key: ev.stateKey(), mode: claimRead}}
		}
		set := []claim{{key: ev.stateKey(), mode: claimWrite}}
		for _, dep := range m.secretStoreRestartPlan(ev.key) {
			if dep.wedged {
				continue
			}
			set = append(set, claim{key: collectorStateKey(dep.cfg.ExposedKey()), mode: claimWrite})
		}
		return set
	}

	if ev.kind == eventDyncfgCommand && ev.domain == domainVnode {
		// Vnode mutations claim only the vnode name: the write conflicts
		// with collector commands' read claims on the same vnode, and the
		// commands themselves are loop-synchronous stage+commit.
		return []claim{{key: ev.stateKey(), mode: claimWrite}}
	}

	set := []claim{{key: ev.stateKey(), mode: claimWrite}}

	addCfgRefs := func(cfg confgroup.Config) {
		for _, storeKey := range extractSecretStoreKeys(cfg) {
			set = append(set, claim{key: secretStoreStateKey(storeKey), mode: claimRead})
		}
		if vnode := cfg.Vnode(); vnode != "" {
			set = append(set, claim{key: vnodeStateKey(vnode), mode: claimRead})
		}
	}
	addExposedRefs := func() {
		if entry, ok := m.collectorExposed.LookupByKey(ev.key); ok {
			addCfgRefs(entry.Cfg)
		}
	}
	addPayloadRefs := func() {
		// Cheap parse only (no I/O). An unparsable payload never reaches
		// this compute - the PayloadParser stage gate classifies it as
		// rejection-only - so the error branch is defensive.
		if cfg, err := configFromPayload(ev.fn); err == nil {
			addCfgRefs(cfg)
		}
	}

	switch ev.kind {
	case eventDiscoveryAdd:
		addCfgRefs(ev.cfg)
		addExposedRefs() // replace of an existing config: union old + new
	case eventDiscoveryRemove:
		// Stop-shaped events hold their OLD references too: the stopping
		// job remains a dependent of its stores and vnode until the stop
		// commits, so store/vnode mutations - and their affected-job gates,
		// which read the exposed cache the stop already cleared at stage -
		// must wait the stop out instead of racing it.
		addExposedRefs()
	case eventDyncfgCommand:
		// Only claim-bearing commands reach here (the event plan filtered the
		// rest): every acting collector mutation claims its config's
		// references - old and new where both exist.
		switch ev.fn.Command() {
		case dyncfg.CommandAdd, dyncfg.CommandUpdate:
			addPayloadRefs()
			addExposedRefs() // union old + new (add over an existing config replaces it)
		case dyncfg.CommandEnable, dyncfg.CommandRestart,
			dyncfg.CommandDisable, dyncfg.CommandRemove:
			addExposedRefs()
		}
	}
	return set
}

// WEDGE LIFECYCLE NOTES. A wedged key (deadline-abandoned effect, leaked
// module call still running) is the one state where normal settle rules are
// asymmetric. Loop-side abandon and late-return ordering is owned by
// planLaneEffectTransition and pinned by executor_transition_test.go:
// writes stay held until late return, reads release at abandon, read-claimed
// store identities are snapshotted before release, late replay runs before
// final release, and shutdown late returns only dispose/drop and release.
//
// Cross-file invariants still live with their mechanism owners:
//
// - The lane remains occupied while wedged; route/settle must park same-key
//   events until the late return.
// - Commit-side dependency cleanup must happen before claim release wakes
//   waiters. Do not move discovery removal or dependency-index cleanup into
//   an effect closure after the blocking wait; abandon would defer cleanup
//   until the leaked call returns while publication already happened.
// - Claim wake at the wedge lets commands whose recomputed claim set
//   excludes wedged keys proceed. Store mutations skip-and-report wedged
//   dependents instead of waiting for an unbounded leaked call.
// - The effect worker's delivered gate must deliver abandon before the
//   child's late result, even when the leaked call returns immediately.
// - Store commands buffer completed dependent restarts as they finish; the
//   buffer flushes at normal/abandon commit and is also registered as
//   lateWork for restarts completing after abandon.
// - resumeWarmJob still verifies the exposed entry before a warm start.
//   disposeWarmResume closes the captured emission gate before Cleanup, and
//   startRunningJob reconciles the current vnode baseline at registration.
// - Late failures evaluate retry eligibility at late return; no provisional
//   retry is scheduled at abandon.
// - Late children gate their sends on lateDrop so shutdown drain cannot
//   block after the bounded wait.
// - Deadline classification is race-independent: runStagedRestarts
//   reclassifies an expired restart sequence as a timeout even if the
//   closure returns after the deadline checkpoint.

// wedge is the loop-owned record of one abandoned command, from its abandon
// commit to its late return. keyState.wedge != nil IS the wedged state:
// the enter-wedged transition action is the only constructor and the
// leave-wedged transition action is the only consumer, so the wedged window's
// loop-side ordering lives in the transition table instead of scattered
// sites.
type wedge struct {
	// gen is the key's generation at the abandon commit: a mismatch at the
	// late return means a newer command committed on the key (assert-only -
	// the lane parks everything while wedged).
	gen uint64
	// deps are the read-claimed store dependencies' identities at the
	// abandon commit.
	deps []wedgedDep
}

// wedgedDep is one read-claimed secret-store dependency of a wedged
// command: its committed identity at the abandon commit. Up to that moment
// the command's read claim excluded store mutations, so the identity pins
// the value its leaked detection actually resolved.
type wedgedDep struct {
	stateKey string
	identity string
}

// snapshotWedgedDeps captures the wedging command's store dependencies from
// its grant's read claims, just before those claims release at the abandon
// commit. Vnode reads are not captured: a warm job's vnode is refreshed at
// resume instead (resumeWarmJob), matching the live-update semantics a
// running job gets.
func (e *executor) snapshotWedgedDeps(g *claimGrant) []wedgedDep {
	var deps []wedgedDep
	for _, c := range g.held {
		if c.mode != claimRead {
			continue
		}
		key, ok := strings.CutPrefix(c.key, secretStoreStateKey(""))
		if !ok {
			continue
		}
		deps = append(deps, wedgedDep{stateKey: c.key, identity: e.mgr.secretStoreDepIdentity(key)})
	}
	return deps
}

// staleWarmDep reports why a warm continuation's store dependencies are no
// longer trustworthy: a store the warm job resolved its secrets from whose
// committed identity changed since the abandon (a rotation committed while
// the key was wedged - that rotation reported this dependent as skipped, so
// nothing would ever refresh a job started on the old value), or one
// currently write-held (a granted store mutation whose effect may already
// have activated a new value off-loop; dropping is the safe direction - if
// that mutation ultimately fails, only this warm start is lost, never
// credential freshness). Union-claimed dependencies the warm config no
// longer references are ignored. Empty means the continuation is safe.
func (e *executor) staleWarmDepStatus(deps []wedgedDep, cfg confgroup.Config) (staleWarmDepKind, string) {
	if len(deps) == 0 {
		return staleWarmDepNone, ""
	}
	referenced := make(map[string]bool, len(deps))
	for _, key := range extractSecretStoreKeys(cfg) {
		referenced[secretStoreStateKey(key)] = true
	}
	for _, dep := range deps {
		if !referenced[dep.stateKey] {
			continue
		}
		if e.claims.heldForWrite(dep.stateKey) {
			return staleWarmDepWriteHeld, fmt.Sprintf("a mutation of store dependency '%s' is in flight", dep.stateKey)
		}
		key, _ := strings.CutPrefix(dep.stateKey, secretStoreStateKey(""))
		if e.mgr.secretStoreDepIdentity(key) != dep.identity {
			return staleWarmDepChanged, fmt.Sprintf("store dependency '%s' changed while the key was wedged", dep.stateKey)
		}
	}
	return staleWarmDepNone, ""
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

// afterIdleHook runs h when the key's current command fully commits (all
// chained phases done); immediately when it never went busy. Hooks run
// BEFORE the command's claims release: they finalize loop-owned state (the
// store-dependency index) that commands unparked by the release recompute
// from - releasing first would hand those commands a stale view.
func (e *executor) afterIdleHook(ks *keyState, h func()) {
	if ks.busy {
		ks.afterIdle = append(ks.afterIdle, h)
		return
	}
	h()
}

// runAfterIdleHooks drains and runs the key's pending after-commit hooks.
func (e *executor) runAfterIdleHooks(ks *keyState) {
	for len(ks.afterIdle) > 0 {
		hooks := ks.afterIdle
		ks.afterIdle = nil
		for _, h := range hooks {
			h()
		}
	}
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
		if ks.wedge != nil {
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

// onEffectDoneShutdown handles a completion under the ONE RULE at shutdown:
// a phase completing after cancellation - success, failure, or
// deadline-abandon - still answers 503 and publishes nothing (a stop
// finishing here would otherwise publish state for a plugin that is
// exiting). Late completions are not command outcomes (their commands
// already committed at the abandon) and only release keys.
func (e *executor) onEffectDoneShutdown(res effectResult) {
	// Draining engages HERE, not just in shutdownDrain: the commit's cascade
	// (claim releases waking parked acquisitions, lane settles popping
	// parked events) must already see the shutdown mode, or a woken command
	// would execute - and publish - through the normal path.
	e.draining = true
	e.onEffectDone(res)
}

// onEffectDone resumes a completed blocking phase's commit on the run loop.
func (e *executor) onEffectDone(res effectResult) {
	ks := e.keys[res.key]
	e.executeEffectTransition(res, ks)
}

// settle drains a key's after-idle hooks and parked events until the key
// goes occupied again (or a foreign claim holds it) or nothing is left.
func (e *executor) settle(sk string, ks *keyState) {
	for !ks.occupied() && !e.claims.heldForWrite(sk) {
		if len(ks.afterIdle) > 0 {
			e.runAfterIdleHooks(ks)
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
		if ks.busy || ks.wedge != nil {
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

	// Claim-parked commands hold no effects and will never be granted:
	// answer them retryably now (one rule).
	e.claims.drainParked(func(req *claimRequest) {
		ev, ok := e.claimWork[req]
		if !ok {
			return
		}
		delete(e.claimWork, req)
		if ks := e.keys[ev.stateKey()]; ks != nil && ks.acquiring == req {
			ks.acquiring = nil
		}
		if ev.kind == eventDyncfgCommand {
			e.mgr.dyncfgResponder.SendCodef(ev.fn, 503, dyncfgShuttingDownMsg)
			if mx := e.mgr.executorMetrics; mx != nil {
				mx.shutdownRejected.Add(1)
			}
		}
	})

	deadline := time.NewTimer(e.mgr.drainWait)
	defer deadline.Stop()
	for e.busyCount() > 0 {
		select {
		case res := <-e.mgr.effectDoneCh:
			e.onEffectDoneShutdown(res)
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
		if g := ks.grant; g != nil {
			ks.grant = nil
			e.claims.release(g)
		}
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

// secretStoreStateKey is the claim key for a secret-store key.
func secretStoreStateKey(key string) string {
	return event{domain: domainSecretStore, key: key}.stateKey()
}

// vnodeStateKey is the claim key for a vnode name.
func vnodeStateKey(name string) string {
	return event{domain: domainVnode, key: name}.stateKey()
}

// collectorKeyWedged reports whether the collector key is held by an
// abandoned effect awaiting its late return. A wedged key has no bounded
// release, so store rotations skip-and-report its job instead of claiming
// it. Loop-owned state: call only on the run-loop goroutine.
func (e *executor) collectorKeyWedged(key string) bool {
	ks := e.keys[collectorStateKey(key)]
	return ks != nil && ks.wedge != nil
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
	mx.claimParked.Set(float64(e.claims.parkedCount()))
	mx.poolInflight.Set(float64(e.inflight))
	mx.poolQueued.Set(float64(len(e.pending) + len(e.mgr.effectCh)))
	mx.oldestParkedSince.Store(unixNanosOrZero(oldestParked))
	mx.oldestWaitSince.Store(unixNanosOrZero(oldestWait))
	oldestClaim, _ := e.claims.oldestParkedSince()
	mx.oldestClaimParkedSince.Store(unixNanosOrZero(oldestClaim))
}

func unixNanosOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
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
		ev.underivable = true
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
