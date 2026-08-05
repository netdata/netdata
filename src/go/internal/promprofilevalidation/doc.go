// SPDX-License-Identifier: GPL-3.0-or-later

// Package promprofilevalidation validates contributed Prometheus chart
// profiles against source-complete exposition evidence and a capability-limited
// job policy. Production packages own collection semantics and diagnostics;
// this package owns contributor policy, orchestration, findings, and reports.
package promprofilevalidation
