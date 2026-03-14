// SPDX-License-Identifier: GPL-3.0-or-later

// Package secretsctl owns the secretstore dyncfg control plane for jobmgr.
//
// It does not own:
//   - dyncfg prefix routing
//   - dyncfg handoff/channel ownership
//   - Manager.run() serialized execution
//   - collector-supervisor coordination such as dependent collector restarts
//   - dependency-index ownership or writes
package secretsctl
