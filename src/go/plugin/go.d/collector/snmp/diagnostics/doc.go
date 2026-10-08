// SPDX-License-Identifier: GPL-3.0-or-later

// Package diagnostics owns the process-wide SNMP diagnostic publisher and file
// transport. lifecycle.zst is an independently captured status file; topology
// checkpoints are immutable, self-contained historical samples, each including
// its own lifecycle cut. Readers must not join a historical checkpoint to the
// current lifecycle file by registration ID: those IDs are scoped to a run.
//
// Publication is asynchronous and best effort. Completed files survive process
// restart; live state is never restored from them. Atomic rename protects readers
// from partial files, but this diagnostic store does not promise power-loss
// durability. Rotation follows successful publication and only removes recognized
// checkpoint files. An IO failure can temporarily leave more than the retained
// three files until cleanup succeeds.
package diagnostics
