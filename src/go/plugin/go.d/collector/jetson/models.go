// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"slices"
	"time"
)

// sample holds the GPU and EMC readings of one tegrastats record. A nil reading
// was absent or invalid in the record. Frequencies are MHz.
type sample struct {
	GPUUtilization *float64
	GPUFrequency   *float64   // a scalar GPU clock, never relabeled as GPC 0
	GPCFrequencies []*float64 // per-GPC clocks; a nil element keeps its GPC index
	EMCUtilization *float64
	EMCFrequency   *float64
}

func (s sample) hasReadings() bool {
	return s.GPUUtilization != nil || s.GPUFrequency != nil || s.EMCUtilization != nil || s.EMCFrequency != nil ||
		slices.ContainsFunc(s.GPCFrequencies, func(f *float64) bool { return f != nil })
}

// observation is a sample and the time its record was read.
type observation struct {
	sample sample
	at     time.Time
}
