// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import "slices"

// sample holds one tegrastats record. Missing or invalid readings are omitted.
// Frequencies are MHz; power is watts.
type sample struct {
	GPUUtilization *float64
	GPUFrequency   *float64   // a scalar GPU clock, never relabeled as GPC 0
	GPCFrequencies []*float64 // per-GPC clocks; a nil element keeps its GPC index
	EMCUtilization *float64
	EMCFrequency   *float64
	PowerRails     map[string]float64
}

func (s sample) hasReadings() bool {
	return len(s.PowerRails) != 0 || s.GPUUtilization != nil || s.GPUFrequency != nil || s.EMCUtilization != nil ||
		s.EMCFrequency != nil ||
		slices.ContainsFunc(s.GPCFrequencies, func(f *float64) bool { return f != nil })
}
