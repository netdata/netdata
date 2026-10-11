// SPDX-License-Identifier: GPL-3.0-or-later

// Package joboutput adapts collector construction, acknowledged job
// generations, runtime output, and vnode snapshots to Job Manager ports.
//
// Committed enabled intent is separate from graph health and transient accepted
// activation. Optional collectorapi.JobConfigIntentOwner reservations are made
// only for adopted changes, committed after graph adoption and before activation
// or reply, and aborted on rollback. Running/Failed/retry retain the same intent.
// Composition retires these resources with FinalizeJobConfigIntents only after
// the run joins; process owners must protect successor replay before retirement.
//
// Creator.ActivationDependencies selects pure named availability lookups. The
// existing accepted index registers buffered notifications before lookup and
// wakes through NotifyDependencyChanged; no collector-side job scheduler is
// needed. Validation never checks availability and Test never prepares it.
package joboutput
