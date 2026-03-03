// SPDX-License-Identifier: GPL-3.0-or-later

package policy

// RunModePolicy defines runtime-mode behavior gates injected from composition root.
type RunModePolicy struct {
	IsTerminal               bool
	AutoEnableDiscovered     bool
	UseFileStatusPersistence bool
}
