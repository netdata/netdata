// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

// SELCounts counts newly observed records. Fixed buckets retain unpublished
// events without retaining their raw records during failed collections.
type SELCounts struct {
	Threshold SELStateCounts
	Discrete  SELStateCounts
}

type SELStateCounts struct {
	Nominal  uint64
	Warning  uint64
	Critical uint64
	Unknown  uint64
}

func (c SELStateCounts) Count(state string) uint64 {
	switch state {
	case StateNominal:
		return c.Nominal
	case StateWarning:
		return c.Warning
	case StateCritical:
		return c.Critical
	case StateUnknown:
		return c.Unknown
	default:
		return 0
	}
}

func (c *SELCounts) observe(raw [selRecordSize]byte) {
	threshold, state := selRecordClass(raw)
	counts := &c.Discrete
	if threshold {
		counts = &c.Threshold
	}
	switch state {
	case StateNominal:
		counts.Nominal++
	case StateWarning:
		counts.Warning++
	case StateCritical:
		counts.Critical++
	default:
		counts.Unknown++
	}
}
