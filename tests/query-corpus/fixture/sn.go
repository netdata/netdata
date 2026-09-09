// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import "math"

// SNRoundTrip models Netdata's tier-0 32-bit storage_number pack/unpack.
//
// Source: netdata/netdata @ 043f50ec075441010c1495250871d37a8ac69f8d
//   - pack: src/libnetdata/storage_number/storage_number.c:81-157
//   - unpack LUT initialization: the same file, lines 159-173
//   - unpack bit extraction/application:
//     src/libnetdata/storage_number/storage_number.h:133-169
//
// Values carry a 24-bit mantissa scaled by 10^m (m 0..7, multiplier or
// divider; factor 100 for huge values). The C packer uses lrint(), whose
// rounding follows the active floating-point rounding mode. This oracle uses
// RoundToEven, matching the normal/default round-to-nearest mode used by the
// corpus process, and mirrors the relevant binary64 arithmetic order.
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
