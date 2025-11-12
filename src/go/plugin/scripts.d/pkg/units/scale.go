// SPDX-License-Identifier: GPL-3.0-or-later

package units

import (
	"math"
	"strings"
)

const (
	defaultDivisor = 1000
	timeDivisor    = 1_000_000_000
)

// Scale describes how to convert a floating-point measurement into the integer
// representation Netdata expects.
type Scale struct {
	CanonicalUnit string
	Divisor       int
	multiplier    float64
}

// NewScale determines the appropriate scaling strategy for a Nagios perfdata unit.
func NewScale(unit string) Scale {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return Scale{CanonicalUnit: "", Divisor: defaultDivisor, multiplier: defaultDivisor}
	}
	lower := strings.ToLower(trimmed)
	if scale, ok := timeScale(lower); ok {
		return scale
	}
	if scale, ok := byteScale(lower); ok {
		return scale
	}
	if lower == "%" {
		return Scale{CanonicalUnit: "%", Divisor: defaultDivisor, multiplier: defaultDivisor}
	}
	if lower == "c" {
		return Scale{CanonicalUnit: "c", Divisor: 1, multiplier: 1}
	}
	return Scale{CanonicalUnit: trimmed, Divisor: defaultDivisor, multiplier: defaultDivisor}
}

// Apply converts the provided value into its integer representation using the
// scale's multiplier.
func (s Scale) Apply(value float64) int64 {
	return int64(math.Round(value * s.multiplier))
}

func timeScale(unit string) (Scale, bool) {
	switch unit {
	case "s", "sec", "secs", "second", "seconds":
		return Scale{CanonicalUnit: "seconds", Divisor: timeDivisor, multiplier: timeDivisor}, true
	case "ms", "millisecond", "milliseconds":
		return Scale{CanonicalUnit: "seconds", Divisor: timeDivisor, multiplier: 1_000_000}, true
	case "us", "µs", "usec", "microsecond", "microseconds":
		return Scale{CanonicalUnit: "seconds", Divisor: timeDivisor, multiplier: 1_000}, true
	case "ns", "nanosecond", "nanoseconds":
		return Scale{CanonicalUnit: "seconds", Divisor: timeDivisor, multiplier: 1}, true
	default:
		return Scale{}, false
	}
}

func byteScale(unit string) (Scale, bool) {
	if strings.HasSuffix(unit, "/s") {
		base := strings.TrimSuffix(unit, "/s")
		if mult, ok := byteMultiplier(base); ok {
			return Scale{CanonicalUnit: "bytes/s", Divisor: 1, multiplier: mult}, true
		}
	}
	if strings.HasSuffix(unit, "ps") {
		base := strings.TrimSuffix(unit, "ps")
		if mult, ok := byteMultiplier(base); ok {
			return Scale{CanonicalUnit: "bytes/s", Divisor: 1, multiplier: mult}, true
		}
	}
	if mult, ok := byteMultiplier(unit); ok {
		return Scale{CanonicalUnit: "bytes", Divisor: 1, multiplier: mult}, true
	}
	return Scale{}, false
}

func byteMultiplier(unit string) (float64, bool) {
	switch unit {
	case "b", "byte", "bytes":
		return 1, true
	case "kb":
		return 1_000, true
	case "mb":
		return 1_000_000, true
	case "gb":
		return 1_000_000_000, true
	case "tb":
		return 1_000_000_000_000, true
	default:
		return 0, false
	}
}
