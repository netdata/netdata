// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobmgr owns runtime orchestration for discovered and dyncfg-managed jobs.
//
// Concurrency contract (must remain true unless this document is updated):
//
//   - Manager.run() is the single dispatcher: it routes every discovery and
//     dyncfg event into the executor's per-key lanes and executes ALL commits
//     on its goroutine. Executor lane state (keyState) and the claim table
//     are loop-owned and lock-free; nothing outside the run loop may touch
//     them.
//
//   - Cross-key dependencies are reserved through the claim table BEFORE a
//     command's blocking work ships: every mutating event holds a write
//     claim on its own key; collector commands add read claims on their
//     referenced secret stores and vnode (the UNION of old and new for
//     update-shaped commands); secretstore mutations add write claims on
//     their restartable dependent job keys. Acquisition follows ONE global
//     lexicographic key order (prefix held, park at the blocking key, strict
//     per-key waiter FIFO), and claim sets are recomputed at every re-stage
//     attempt. Claims release when the command fully settles; at a
//     deadline abandon the command's READ claims release with its commit,
//     while its WRITE claims - the wedged key and any keys the leaked work
//     may still mutate, such as a store command's dependent job keys - stay
//     held until the leaked call returns.
//
//   - REJECTION-ONLY dyncfg commands claim NOTHING and bypass foreign-hold
//     lane parking: a command whose execution answers a deterministic
//     rejection or no-op BEFORE its first claim-protected access (identity,
//     existence, state, source/type, payload-presence, and command-support
//     gates) executes claimless-inline instead of parking behind held keys
//     with no bounded release. Each DOMAIN owns its CommandPlan, colocated
//     with the command gates and pinned by parity tests; the executor wraps it
//     in a jobmgr-local event plan that attaches claim computation. Execution
//     order is the criterion, not the eventual outcome: gates that need the
//     claim's exclusion - and every gate execution orders BEHIND them - answer
//     inline under the granted claim (the store remove's affected-jobs 409
//     plus its trailing source/type 405s; the vnode remove's
//     referenced-by-configs 409). The STATUS axis is HOLD-AWARE:
//     enable/restart/disable rejections and no-ops derive from entry.Status,
//     the one input a foreign write-claim holder (a store command's
//     dependent-restart plan) mutates - while the command's key is
//     foreign-write-held they park and answer truthfully after the hold
//     resolves. Same-key order is preserved: the route bypass fires only on
//     an unoccupied lane with an empty FIFO.
//
//   - Blocking module work (validation, job creation, detection, stop waits,
//     secretstore backend I/O, dependent-job restarts) runs on the effect
//     pool under a flat internal deadline. Effects for one key never overlap
//     (key occupied stage-to-commit; all its events park in arrival order);
//     effects for different keys run concurrently. State an effect may reach
//     carries its own synchronization: retryingTasks, the vnode store,
//     runningJobs, emissionGates, fileStatus, secretStoreDeps, and funcctl
//     are all internally locked.
//
//   - Function routing to a stopping job is withdrawn when its stop command
//     STAGES (before the effect reaches a pool worker); a started job's
//     functions publish at COMMIT (OnStatusChange on the running
//     transition), never from inside the effect. A staged stop whose wait
//     never ran (shutdown) is undone so the still-running job stays visible
//     to routing and cleanup.
//
//   - At shutdown ONE RULE holds: every non-terminal dyncfg command answers
//     503, publishes nothing, and everything is disposed - regardless of
//     which phase was running or how far it got. Only the deadline path
//     keeps its classification (fenced abandon publishes stop success;
//     restart/update stop timeouts fail).
//
//   - effectDoneCh has exactly one reader: the run loop. Effect completions
//     and late returns of abandoned effects resume there; per-call channels
//     are used anywhere a non-loop caller awaits.
//
//   - A wait-parked key (discovered config awaiting its enable/disable
//     decision) parks only ITS OWN discovery events; its dyncfg commands
//     execute immediately with the state machine's outcomes, and unrelated
//     keys are never affected. There is no global wait freeze.
//
//   - An abandoned effect (deadline exceeded) frees its pool slot and commits
//     its deadline outcome, but the key stays busy until the leaked module
//     call returns; the late outcome (warm start, retry eligibility, plain
//     release) is decided then, on the loop. The wedge re-attempts claim
//     waiters parked at the key, so commands that exclude wedged keys on
//     recompute (store mutations skip-and-report wedged dependents) proceed
//     instead of waiting out the leaked call. A late warm start additionally
//     requires the job's store dependencies unchanged since the abandon
//     (their identities are captured when the read claims release; a store
//     mutation committed or in flight at the late return drops the warm job,
//     matching the mutation's skip-and-report of the wedged dependent).
//     Dropped continuations dispose SILENTLY: the job's emission gate closes
//     before its cleanup, which would otherwise emit HOST/HOSTINFO lines for
//     a job that never started.
//
//   - Every job registration (startRunningJob) reconciles the job's vnode
//     against the store before Start: a job created before a vnode update but
//     registered after it (a dependent restart's stop/start gap, a warm
//     continuation) receives the current config at registration instead of
//     running on its stale creation-time snapshot. The reconcile commits a
//     versioned snapshot into the job (SetVnodeSnapshot) so cleanup can use the
//     committed vnode even when the job never collects;
//     later running jobs refresh from the versioned vnode lookup at their
//     runtime-defined collection boundary.
//
//   - Discovery ingestion (runProcessConfGroups) does not mutate manager
//     state directly. It publishes add/remove intents through channels
//     consumed by Manager.run().
//
//   - Tick-triggered Function publication reconcile is reconciler-goroutine
//     owned. runNotifyRunningJobs may request module reconcile, but must not
//     perform Function publication directly.
//
//   - runningJobs.items is protected by runningJobs.mux.
//     All traversals must use runningJobs.snapshot() and execute callbacks
//     outside the lock. No external API/job callbacks may execute while
//     holding runningJobs.mux.
//
//   - Function handlers run on framework/functions manager goroutine(s) and
//     must avoid cross-goroutine access to manager-owned mutable maps; async
//     work uses manager-rooted contexts and bounded worker limits where
//     required.
//
//   - Secretstore commands run on the executor lanes with claims: dependent
//     restarts execute inside the store command's single multi-key effect
//     (per-job entry state and CONFIG STATUS replay at commit, in restart
//     order; terminal messages ride the command's own effect contexts, so
//     concurrent store commands cannot cross-attribute them). A dependent
//     with work in flight parks the store command until that work commits;
//     only WEDGED dependents are skipped and reported.
//
//   - Vnode commands run on the executor lanes as stage+commit events under
//     a write claim on the vnode name (conflicting with collector commands'
//     vnode read claims); the commands themselves execute inline on the
//     loop, so the cross-vnode uniqueness scan needs no extra claims.
package jobmgr
