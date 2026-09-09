// SPDX-License-Identifier: GPL-3.0-or-later

// Package promvalidation validates contributed Prometheus chart profiles
// against source-complete exposition evidence and a capability-limited job
// policy.
//
// One strict production parse feeds current inventory and assembly. Current
// evidence and raw future probes then run through separate collector/store/plan
// sequences, so future results cannot satisfy current coverage. Production
// packages own runtime semantics, bounded analysis mechanisms, and opt-in
// diagnostics; this package owns contributor policy, orchestration, findings,
// and deterministic reports.
//
// Extend runtime facts at their semantic owner. Keep evidence obligations,
// severity, and user-facing wording here rather than adding validator policy to
// production APIs or reconstructing production behavior locally.
package promvalidation
