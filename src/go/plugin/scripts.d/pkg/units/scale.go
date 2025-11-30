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
	if scale, ok := byteScale(trimmed); ok {
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
	base, perSecond := splitPerSecond(unit)
	if base == "" {
		return Scale{}, false
	}
	mult, kind, ok := byteMultiplier(base)
	if !ok {
		return Scale{}, false
	}
	canonical := kind
	if perSecond {
		canonical += "/s"
	}
	return Scale{CanonicalUnit: canonical, Divisor: 1, multiplier: mult}, true
}

func splitPerSecond(unit string) (string, bool) {
	lower := strings.ToLower(unit)
	switch {
	case strings.HasSuffix(lower, "/s"):
		return strings.TrimSpace(unit[:len(unit)-2]), true
	case strings.HasSuffix(lower, "ps"):
		return strings.TrimSpace(unit[:len(unit)-2]), true
	default:
		return strings.TrimSpace(unit), false
	}
}

func byteMultiplier(unit string) (float64, string, bool) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return 0, "", false
	}
	kind, prefix, ok := splitByteUnit(unit)
	if !ok {
		return 0, "", false
	}
	mult, ok := byteMagnitude(prefix)
	if !ok {
		return 0, "", false
	}
	return mult, kind, true
}

func splitByteUnit(unit string) (string, string, bool) {
	lower := strings.ToLower(unit)
	switch {
	case strings.HasSuffix(lower, "bytes"):
		return "bytes", unit[:len(unit)-5], true
	case strings.HasSuffix(lower, "byte"):
		return "bytes", unit[:len(unit)-4], true
	case strings.HasSuffix(lower, "bits"):
		return "bits", unit[:len(unit)-4], true
	case strings.HasSuffix(lower, "bit"):
		return "bits", unit[:len(unit)-3], true
	}
	if len(unit) == 0 {
		return "", "", false
	}
	last := unit[len(unit)-1]
	switch last {
	case 'B':
		return "bytes", unit[:len(unit)-1], true
	case 'b':
		return "bits", unit[:len(unit)-1], true
	}
	return "", "", false
}

func byteMagnitude(prefix string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "":
		return 1, true
	case "k":
		return 1_000, true
	case "m":
		return 1_000_000, true
	case "g":
		return 1_000_000_000, true
	case "t":
		return 1_000_000_000_000, true
	default:
		return 0, false
	}
}
