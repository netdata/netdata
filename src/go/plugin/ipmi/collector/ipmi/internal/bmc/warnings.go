// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import "fmt"

// collectionWarnings counts partially collected data in fixed categories, so a
// large SDR repository cannot flood the logs.
type collectionWarnings struct {
	unsupported    int // sensors whose record has unsupported ownership, sharing or ID encoding
	reading        int // unavailable or incomplete readings
	conversion     int // unsupported units or conversions, and invalid results
	selUnavailable bool
}

// messages returns one message per non-empty category, or nil.
func (w collectionWarnings) messages() []string {
	var out []string
	if w.unsupported > 0 {
		out = append(
			out,
			fmt.Sprintf("%d sensors have unsupported ownership, sharing, or ID encoding", w.unsupported),
		)
	}
	if w.reading > 0 {
		out = append(out, fmt.Sprintf("%d sensor readings are unavailable or incomplete", w.reading))
	}
	if w.conversion > 0 {
		out = append(
			out,
			fmt.Sprintf("%d sensor values have unsupported units/conversion or invalid results", w.conversion),
		)
	}
	if w.selUnavailable {
		out = append(out, "SEL entry count unavailable")
	}
	return out
}
