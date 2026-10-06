// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

// sample contains only measurements present in one tegrastats record.
// Frequencies are MHz. A nil array element preserves an unavailable GPC's index.
type sample struct {
	GPUUtilization *float64
	GPUFrequency   *float64
	GPCFrequencies []*float64
	EMCUtilization *float64
	EMCFrequency   *float64
}

func (s sample) hasMetrics() bool {
	if s.GPUUtilization != nil || s.GPUFrequency != nil || s.EMCUtilization != nil || s.EMCFrequency != nil {
		return true
	}
	for _, frequency := range s.GPCFrequencies {
		if frequency != nil {
			return true
		}
	}
	return false
}
