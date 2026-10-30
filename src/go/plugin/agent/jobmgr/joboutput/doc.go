// SPDX-License-Identifier: GPL-3.0-or-later

// Package joboutput adapts collector construction, acknowledged job
// generations, runtime output, and vnode snapshots to Job Manager ports.
//
// Changed enabled bindings with named dependencies pass contained structural
// validation before adoption, even when dependency readiness blocks construction.
// Validation yields graph claims and does not supersede a checked candidate.
//
// Creator.ActivationDependencies selects pure named availability lookups. The
// existing accepted index registers buffered notifications before lookup and
// wakes through NotifyDependencyChanged; no collector-side job scheduler is
// needed. Validation never checks availability and Test never prepares it.
package joboutput
