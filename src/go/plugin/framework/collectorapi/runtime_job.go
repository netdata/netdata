// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

// RuntimeJob is the runtime-facing job contract used by function hooks.
// It is implemented by both legacy Job and JobV2.
type RuntimeJob interface {
	FullName() string
	ModuleName() string
	Name() string
	IsRunning() bool
	Collector() any
}
