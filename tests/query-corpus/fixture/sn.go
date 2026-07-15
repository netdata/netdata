// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import "math"

// SNRoundTrip models netdata's tier0 storage quantization: the
// pack/unpack cycle of the 32-bit storage_number
// (src/libnetdata/storage_number/storage_number.c:81-174). Values carry a
// 24-bit mantissa scaled by 10^m (m 0..7, multiplier or divider; factor
// 100 for huge values), rounded with round-half-even (lrint), unpacked as
// mantissa × LUT where LUT holds pow(10,m) or 1/pow(10,m) as doubles.
// The oracle mirrors the arithmetic order so expected values match the
// engine to double precision; the JSON print layer adds only formatting
// rounding on top.
func SNRoundTrip(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return math.NaN() // stored as an empty slot
	}
	if v == 0 || math.Abs(v) < math.SmallestNonzeroFloat64*(1<<52) { // FP_ZERO/FP_SUBNORMAL
		return 0
	}

	n := v
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}

	factor := 10.0
	if n/10000000.0 > 0x00ffffff {
		factor = 100
	}

	m := 0
	var out float64
	if n > 0x00ffffff {
		for m < 7 && n > 0x00ffffff {
			n /= factor
			m++
		}
		if n > 0x00ffffff {
			n = 0x00ffffff // saturation: the C code stores the max mantissa
		}
		// multiply branch: unpack multiplies by factor^m (LUT stores pow)
		out = math.RoundToEven(n) * math.Pow(factor, float64(m))
	} else {
		for m < 7 && n < 0x0019999e {
			n *= 10
			m++
		}
		if n > 0x00ffffff {
			n /= 10
			m--
		}
		// divide branch: unpack multiplies by 1/pow(10,m) (LUT stores the
		// reciprocal as a double — mirror that exact operation)
		out = math.RoundToEven(n) * (1.0 / math.Pow(10, float64(m)))
	}

	if neg {
		out = -out
	}
	return out
}
