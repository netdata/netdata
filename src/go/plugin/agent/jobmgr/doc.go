// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobmgr owns runtime orchestration for discovered and dyncfg-managed jobs.
//
// Concurrency contract (must remain true unless this document is updated):
//
//   - Manager.run() is the single dispatcher: it routes every discovery and
//     dyncfg event into the executor's per-key lanes and executes ALL commits
//     on its goroutine. Executor lane state (keyState) is loop-owned and
//     lock-free; nothing outside the run loop may touch it, except through
//     the loop-synchronous tryLockIdleKey/unlockIdleKey seam.
//
//   - Blocking module work (validation, job creation, detection, stop waits)
//     runs on the effect pool under a flat internal deadline. Effects for one
//     key never overlap (key busy stage-to-commit; all its events park in
//     arrival order); effects for different keys run concurrently. State an
//     effect may reach carries its own synchronization: retryingTasks, the
//     vnode store, runningJobs, emissionGates, fileStatus, secretStoreDeps,
//     and funcctl are all internally locked.
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
//     release) is decided then, on the loop.
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
//   - Secretstore and vnode dyncfg commands execute loop-synchronously; the
//     secretstore dependent-restart bridge claims idle collector keys and
//     skips busy ones - it never waits on effect completions.
package jobmgr
