// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"crypto/sha1"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
)

type perfUnitClass string

const (
	perfClassTime    perfUnitClass = "time"
	perfClassBytes   perfUnitClass = "bytes"
	perfClassBits    perfUnitClass = "bits"
	perfClassPercent perfUnitClass = "percent"
	perfClassCounter perfUnitClass = "counter"
	perfClassGeneric perfUnitClass = "generic"
)

type perfPreparedDatum struct {
	rawLabel  string
	metricKey string
	class     perfUnitClass
	value     float64
	warn      *output.ThresholdRange
	crit      *output.ThresholdRange
}

func preparePerfDatum(datum output.PerfDatum) (perfPreparedDatum, bool) {
	rawLabel := strings.TrimSpace(datum.Label)
	if rawLabel == "" || !isFinite(datum.Value) {
		return perfPreparedDatum{}, false
	}

	metricKey := sanitizeMetricKey(rawLabel)
	if metricKey == "" {
		metricKey = "metric"
	}

	class, normalized := normalizePerfValue(datum.Unit, datum.Value)
	item := perfPreparedDatum{
		rawLabel:  rawLabel,
		metricKey: metricKey,
		class:     class,
		value:     normalized,
		warn:      normalizeThresholdRange(datum.Unit, datum.Warn),
		crit:      normalizeThresholdRange(datum.Unit, datum.Crit),
	}
	return item, true
}

func normalizeOptionalFinite(unit string, v *float64) *float64 {
	if v == nil || !isFinite(*v) {
		return nil
	}
	_, normalized := normalizePerfValue(unit, *v)
	out := normalized
	return &out
}

func normalizeThresholdRange(unit string, rng *output.ThresholdRange) *output.ThresholdRange {
	if rng == nil {
		return nil
	}
	return &output.ThresholdRange{
		Inclusive: rng.Inclusive,
		Low:       normalizeOptionalFinite(unit, rng.Low),
		High:      normalizeOptionalFinite(unit, rng.High),
	}
}

func normalizePerfValue(unit string, value float64) (perfUnitClass, float64) {
	lower := strings.ToLower(strings.TrimSpace(unit))
	switch lower {
	case "s", "sec", "secs", "second", "seconds":
		return perfClassTime, value
	case "ms", "millisecond", "milliseconds":
		return perfClassTime, value / 1_000
	case "us", "µs", "usec", "microsecond", "microseconds":
		return perfClassTime, value / 1_000_000
	case "ns", "nanosecond", "nanoseconds":
		return perfClassTime, value / 1_000_000_000
	case "%":
		return perfClassPercent, value
	case "c":
		return perfClassCounter, value
	}

	if class, multiplier, ok := byteOrBitMultiplier(unit); ok {
		return class, value * multiplier
	}
	return perfClassGeneric, value
}

func byteOrBitMultiplier(unit string) (perfUnitClass, float64, bool) {
	base, ok := trimPerSecondSuffix(unit)
	if !ok || base == "" {
		return "", 0, false
	}
	class, prefix, ok := splitByteOrBitUnit(base)
	if !ok {
		return "", 0, false
	}
	multiplier, ok := byteMagnitude(prefix)
	if !ok {
		return "", 0, false
	}
	return class, multiplier, true
}

func trimPerSecondSuffix(unit string) (string, bool) {
	trimmed := strings.TrimSpace(unit)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/s"):
		return strings.TrimSpace(trimmed[:len(trimmed)-2]), true
	case strings.HasSuffix(lower, "ps"):
		return strings.TrimSpace(trimmed[:len(trimmed)-2]), true
	default:
		return trimmed, true
	}
}

func splitByteOrBitUnit(unit string) (perfUnitClass, string, bool) {
	trimmed := strings.TrimSpace(unit)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "bytes"):
		return perfClassBytes, trimmed[:len(trimmed)-5], true
	case strings.HasSuffix(lower, "byte"):
		return perfClassBytes, trimmed[:len(trimmed)-4], true
	case strings.HasSuffix(lower, "bits"):
		return perfClassBits, trimmed[:len(trimmed)-4], true
	case strings.HasSuffix(lower, "bit"):
		return perfClassBits, trimmed[:len(trimmed)-3], true
	}
	if trimmed == "" {
		return "", "", false
	}
	switch last := trimmed[len(trimmed)-1]; last {
	case 'B':
		return perfClassBytes, trimmed[:len(trimmed)-1], true
	case 'b':
		return perfClassBits, trimmed[:len(trimmed)-1], true
	default:
		return "", "", false
	}
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

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func unitForClass(class perfUnitClass) string {
	switch class {
	case perfClassTime:
		return "seconds"
	case perfClassBytes:
		return "bytes"
	case perfClassBits:
		return "bits"
	case perfClassPercent:
		return "%"
	case perfClassCounter:
		return "c"
	default:
		return "generic"
	}
}

func sanitizeMetricKey(name string) string {
	lower := strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(lower))
	lastUnderscore := false
	hasAlnum := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			hasAlnum = true
			continue
		}
		if r == '_' || r == '-' || isWhitespace(r) {
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	result := b.String()
	if hasAlnum && result != "" {
		return result
	}
	sum := sha1.Sum([]byte(name))
	return fmt.Sprintf("id_%x", sum[:6])
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

func perfSourceFromPlugin(pluginPath string) string {
	base := filepath.Base(strings.TrimSpace(pluginPath))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "script"
	}
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return sanitizeMetricKey(base)
}

func perfSourceFromCheckName(checkName string) string {
	trimmed := strings.TrimSpace(checkName)
	if trimmed == "" {
		return "check"
	}
	return sanitizeMetricKey(trimmed)
}

func normalizedCheckName(checkName, pluginPath string) string {
	if strings.TrimSpace(checkName) == "" {
		return perfSourceFromPlugin(pluginPath)
	}
	return perfSourceFromCheckName(checkName)
}
