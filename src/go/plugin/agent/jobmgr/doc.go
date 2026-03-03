// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobmgr owns runtime orchestration for discovered and dyncfg-managed jobs.
//
// Concurrency contract (must remain true unless this document is updated):
//
//   - Manager.run() is the serialized owner of dyncfg state transitions:
//     exposed configs, vnode map updates, and add/remove command application.
//
//   - Discovery ingestion (runProcessConfGroups) does not mutate manager state directly.
//     It publishes add/remove intents through channels consumed by Manager.run().
//
//   - runningJobs.items is protected by runningJobs.mux.
//     All traversals must use runningJobs.snapshot() and execute callbacks outside the lock.
//     No external API/job callbacks may execute while holding runningJobs.mux.
//
//   - Function handlers run on framework/functions manager goroutine(s) and must avoid
//     cross-goroutine access to manager-owned mutable maps; async work uses manager-rooted
//     contexts and bounded worker limits where required.
//
//   - Wait-for-decision gating for discovered configs is handler-owned. Manager.run()
//     delegates wait-step orchestration to dyncfg handler and keeps dyncfg command
//     processing progress even while waiting.
package jobmgr
