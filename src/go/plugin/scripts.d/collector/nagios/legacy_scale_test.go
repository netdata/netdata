// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"math"
	"strings"
	"testing"
)

const (
	legacyDefaultDivisor = 1000
	legacyTimeDivisor    = 1_000_000_000
)

type legacyScale struct {
	canonicalUnit string
	divisor       int
	multiplier    float64
}

func legacyDisplayValue(unit string, raw float64) float64 {
	scale := legacyScaleFromUnit(unit)
	scaled := int64(math.Round(raw * scale.multiplier))
	return float64(scaled) / float64(scale.divisor)
}

func legacyScaleFromUnit(unit string) legacyScale {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return legacyScale{canonicalUnit: "", divisor: legacyDefaultDivisor, multiplier: legacyDefaultDivisor}
	}
	lower := strings.ToLower(trimmed)
	if scale, ok := legacyTimeScale(lower); ok {
		return scale
	}
	if scale, ok := legacyByteScale(trimmed); ok {
		return scale
	}
	if lower == "%" {
		return legacyScale{canonicalUnit: "%", divisor: legacyDefaultDivisor, multiplier: legacyDefaultDivisor}
	}
	if lower == "c" {
		return legacyScale{canonicalUnit: "c", divisor: 1, multiplier: 1}
	}
	return legacyScale{canonicalUnit: trimmed, divisor: legacyDefaultDivisor, multiplier: legacyDefaultDivisor}
}

func legacyTimeScale(unit string) (legacyScale, bool) {
	switch unit {
	case "s", "sec", "secs", "second", "seconds":
		return legacyScale{canonicalUnit: "seconds", divisor: legacyTimeDivisor, multiplier: legacyTimeDivisor}, true
	case "ms", "millisecond", "milliseconds":
		return legacyScale{canonicalUnit: "seconds", divisor: legacyTimeDivisor, multiplier: 1_000_000}, true
	case "us", "µs", "usec", "microsecond", "microseconds":
		return legacyScale{canonicalUnit: "seconds", divisor: legacyTimeDivisor, multiplier: 1_000}, true
	case "ns", "nanosecond", "nanoseconds":
		return legacyScale{canonicalUnit: "seconds", divisor: legacyTimeDivisor, multiplier: 1}, true
	default:
		return legacyScale{}, false
	}
}

func legacyByteScale(unit string) (legacyScale, bool) {
	base, perSecond := legacySplitPerSecond(unit)
	if base == "" {
		return legacyScale{}, false
	}
	mult, kind, ok := legacyByteMultiplier(base)
	if !ok {
		return legacyScale{}, false
	}
	canonical := kind
	if perSecond {
		canonical += "/s"
	}
	return legacyScale{canonicalUnit: canonical, divisor: 1, multiplier: mult}, true
}

func legacySplitPerSecond(unit string) (string, bool) {
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

func legacyByteMultiplier(unit string) (float64, string, bool) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return 0, "", false
	}
	kind, prefix, ok := legacySplitByteUnit(unit)
	if !ok {
		return 0, "", false
	}
	mult, ok := legacyByteMagnitude(prefix)
	if !ok {
		return 0, "", false
	}
	return mult, kind, true
}

func legacySplitByteUnit(unit string) (string, string, bool) {
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

func legacyByteMagnitude(prefix string) (float64, bool) {
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

func TestLegacyScaleDistinguishesBitsAndBytes(t *testing.T) {
	cases := []struct {
		name          string
		unit          string
		value         float64
		canonicalUnit string
		want          float64
	}{
		{name: "megabytes per second", unit: "MBps", value: 1.5, canonicalUnit: "bytes/s", want: 1_500_000},
		{name: "megabits per second", unit: "Mbps", value: 1.5, canonicalUnit: "bits/s", want: 1_500_000},
		{name: "megabytes", unit: "MB", value: 2.5, canonicalUnit: "bytes", want: 2_500_000},
		{name: "megabits", unit: "Mb", value: 2.5, canonicalUnit: "bits", want: 2_500_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scale := legacyScaleFromUnit(tc.unit)
			if scale.canonicalUnit != tc.canonicalUnit {
				t.Fatalf("expected canonical unit %q, got %q", tc.canonicalUnit, scale.canonicalUnit)
			}
			if got := legacyDisplayValue(tc.unit, tc.value); got != tc.want {
				t.Fatalf("expected %f, got %f", tc.want, got)
			}
		})
	}
}
