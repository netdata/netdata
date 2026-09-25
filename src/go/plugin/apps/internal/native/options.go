// SPDX-License-Identifier: GPL-3.0-or-later

// Package native owns the self-contained apps.plugin-derived procfs backend.
package native

// Options selects the procfs mount and optional expensive observations.
type Options struct {
	ProcPath   string
	CollectFDs bool
	CollectPSS bool
}
