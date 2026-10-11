// SPDX-License-Identifier: GPL-3.0-or-later

// Package joboutput adapts collector construction, acknowledged job
// generations, runtime output, and vnode snapshots to Job Manager ports.
//
// Committed enabled intent is separate from graph health and transient accepted
// activation. Optional collectorapi.JobConfigIntentOwner reservations are prepared
// before graph adoption, committed after adoption and before activation or reply,
// and aborted on rollback. Validation and reservation share a contained stage
// outside graph claims; late results abort before physical attempt release.
// Commit and Abort transfer ownership through bounded in-memory operations.
// Running/Failed/retry retain the same intent.
// Composition retires these resources with FinalizeJobConfigIntents only after
// the run joins. Successor runs restore demand as accepted configuration arrives;
// cached dependencies may expire before that, independently of execution safety.
//
// Creator.ActivationDependencies selects pure named availability lookups. The
// existing accepted index registers buffered notifications before lookup and
// wakes through NotifyDependencyChanged; no collector-side job scheduler is
// needed. Validation never checks availability and Test never prepares it.
package joboutput
