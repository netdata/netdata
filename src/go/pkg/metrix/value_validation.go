// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

func mustFiniteSample(v SampleValue) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		panic(errInvalidSampleValue)
	}
}
