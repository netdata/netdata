// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"time"
)

func evictTimes(ts []time.Time, cutoff time.Time) []time.Time {
	n := 0
	for i, t := range ts {
		if !t.Before(cutoff) {
			if n != i {
				ts[n] = t
			}
			n++
		}
	}
	return ts[:n]
}
