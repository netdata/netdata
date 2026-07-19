// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobmgr owns the single command kernel used by a Go data-collection
// plugin process.
//
// The kernel serializes mutable orchestration state on KernelLoop, orders
// conflicting work through claims, delegates blocking work to lifecycle
// TaskSupervisor, and commits Function results and protocol notifications
// through lifecycle FrameOwner.
//
// Production construction and process/run rotation live in the composition
// subpackage. Collector implementations remain behind the framework job and
// Function contracts; Job Manager does not own collector-internal concurrency
// or cleanup beyond invoking the declared lifecycle boundary.
//
// See ARCHITECTURE.md for the authority graph and lifecycle invariants.
package jobmgr
