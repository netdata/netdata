// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
)

const defaultPerfdataMetricKeyBudget = 64

type perfUnitClass string

const (
	perfClassTime    perfUnitClass = "time"
	perfClassBytes   perfUnitClass = "bytes"
	perfClassBits    perfUnitClass = "bits"
	perfClassPercent perfUnitClass = "percent"
	perfClassCounter perfUnitClass = "counter"
	perfClassGeneric perfUnitClass = "generic"
)

type perfDropCounters struct {
	Invalid   uint64
	Collision uint64
	UnitDrift uint64
	Budget    uint64
}

type perfMetricSample struct {
	name  string
	value float64
	unit  string
	float bool
}

type perfMetricBindingKey struct {
	scheduler string
	job       string
	metricKey string
}

type perfPreparedDatum struct {
	rawLabel  string
	metricKey string
	class     perfUnitClass
	value     float64
	min       *float64
	max       *float64
	warn      *output.ThresholdRange
	crit      *output.ThresholdRange
}

type perfdataRouter struct {
	maxPerJob int

	classByMetric map[perfMetricBindingKey]perfUnitClass
	counters      perfDropCounters
}

func newPerfdataRouter(maxPerJob int) *perfdataRouter {
	if maxPerJob <= 0 {
		maxPerJob = defaultPerfdataMetricKeyBudget
	}
	return &perfdataRouter{
		maxPerJob:     maxPerJob,
		classByMetric: make(map[perfMetricBindingKey]perfUnitClass),
	}
}

func (r *perfdataRouter) route(scheduler, job string, perf []output.PerfDatum) []perfMetricSample {
	if len(perf) == 0 {
		return nil
	}

	items := make([]perfPreparedDatum, 0, len(perf))
	for _, datum := range perf {
		item, ok := preparePerfDatum(datum)
		if !ok {
			r.counters.Invalid++
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].rawLabel == items[j].rawLabel {
			return items[i].metricKey < items[j].metricKey
		}
		return items[i].rawLabel < items[j].rawLabel
	})

	// Collision policy: keep the first metric key after lexical raw-label sort.
	deduped := make([]perfPreparedDatum, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item.metricKey]; ok {
			r.counters.Collision++
			continue
		}
		seen[item.metricKey] = struct{}{}
		deduped = append(deduped, item)
	}
	if len(deduped) == 0 {
		return nil
	}

	// Budget policy: deterministic cap by metric-key lexical order.
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].metricKey < deduped[j].metricKey
	})
	if len(deduped) > r.maxPerJob {
		r.counters.Budget += uint64(len(deduped) - r.maxPerJob)
		deduped = deduped[:r.maxPerJob]
	}

	samples := make([]perfMetricSample, 0, len(deduped)*16)
	for _, item := range deduped {
		binding := perfMetricBindingKey{
			scheduler: scheduler,
			job:       job,
			metricKey: item.metricKey,
		}
		if prevClass, ok := r.classByMetric[binding]; ok && prevClass != item.class {
			r.counters.UnitDrift++
			continue
		}
		if _, ok := r.classByMetric[binding]; !ok {
			r.classByMetric[binding] = item.class
		}

		base := fmt.Sprintf("perf_%s_%s", item.class, item.metricKey)
		classUnit := unitForClass(item.class)
		samples = append(samples, perfMetricSample{name: base + "_value", value: item.value, unit: classUnit, float: true})
		if item.min != nil {
			samples = append(samples, perfMetricSample{name: base + "_min", value: *item.min, unit: classUnit, float: true})
		}
		if item.max != nil {
			samples = append(samples, perfMetricSample{name: base + "_max", value: *item.max, unit: classUnit, float: true})
		}
		samples = append(samples, thresholdSamples(base, "warn", item.warn, classUnit)...)
		samples = append(samples, thresholdSamples(base, "crit", item.crit, classUnit)...)
	}

	return samples
}

func (r *perfdataRouter) dropCounters() perfDropCounters {
	return r.counters
}

func preparePerfDatum(datum output.PerfDatum) (perfPreparedDatum, bool) {
	rawLabel := strings.TrimSpace(datum.Label)
	if rawLabel == "" || !isFinite(datum.Value) {
		return perfPreparedDatum{}, false
	}

	metricKey := ids.Sanitize(rawLabel)
	if metricKey == "" {
		metricKey = "metric"
	}

	class, normalized := normalizePerfValue(datum.Unit, datum.Value)
	item := perfPreparedDatum{
		rawLabel:  rawLabel,
		metricKey: metricKey,
		class:     class,
		value:     normalized,
		min:       normalizeOptionalFinite(datum.Unit, datum.Min),
		max:       normalizeOptionalFinite(datum.Unit, datum.Max),
		warn:      normalizeThresholdRange(datum.Unit, datum.Warn),
		crit:      normalizeThresholdRange(datum.Unit, datum.Crit),
	}
	return item, true
}

func thresholdSamples(base, kind string, rng *output.ThresholdRange, classUnit string) []perfMetricSample {
	prefix := base + "_" + kind
	if rng == nil {
		return []perfMetricSample{
			{name: prefix + "_defined", value: 0, unit: "state", float: false},
			{name: prefix + "_inclusive", value: 0, unit: "state", float: false},
			{name: prefix + "_low", value: 0, unit: classUnit, float: true},
			{name: prefix + "_high", value: 0, unit: classUnit, float: true},
			{name: prefix + "_low_defined", value: 0, unit: "state", float: false},
			{name: prefix + "_high_defined", value: 0, unit: "state", float: false},
		}
	}

	low := 0.0
	lowDefined := 0.0
	if rng.Low != nil {
		low = *rng.Low
		lowDefined = 1
	}
	high := 0.0
	highDefined := 0.0
	if rng.High != nil {
		high = *rng.High
		highDefined = 1
	}

	return []perfMetricSample{
		{name: prefix + "_defined", value: 1, unit: "state", float: false},
		{name: prefix + "_inclusive", value: boolToFloat(rng.Inclusive), unit: "state", float: false},
		{name: prefix + "_low", value: low, unit: classUnit, float: true},
		{name: prefix + "_high", value: high, unit: classUnit, float: true},
		{name: prefix + "_low_defined", value: lowDefined, unit: "state", float: false},
		{name: prefix + "_high_defined", value: highDefined, unit: "state", float: false},
	}
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
		Raw:       rng.Raw,
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
	if !ok {
		return "", 0, false
	}
	if base == "" {
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
	last := trimmed[len(trimmed)-1]
	switch last {
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
