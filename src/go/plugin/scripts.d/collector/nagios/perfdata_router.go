// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"math"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
)

const defaultPerfdataMetricKeyBudget = 64

type perfdataRouter struct {
	maxPerJob int
}

func newPerfdataRouter(maxPerJob int) *perfdataRouter {
	if maxPerJob <= 0 {
		maxPerJob = defaultPerfdataMetricKeyBudget
	}
	return &perfdataRouter{
		maxPerJob: maxPerJob,
	}
}

func (r *perfdataRouter) route(pluginPath string, perf []output.PerfDatum) perfRouteResult {
	if len(perf) == 0 {
		return perfRouteResult{}
	}
	source := perfSourceFromPlugin(pluginPath)

	items := make([]perfPreparedDatum, 0, len(perf))
	for _, datum := range perf {
		item, ok := preparePerfDatum(datum)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return perfRouteResult{}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rawLabel == items[j].rawLabel {
			return items[i].metricKey < items[j].metricKey
		}
		return items[i].rawLabel < items[j].rawLabel
	})

	// Collision policy: keep the first final metric identity after lexical raw-label sort.
	deduped := make([]perfPreparedDatum, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		identity := perfMetricIdentity(source, item)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		deduped = append(deduped, item)
	}
	if len(deduped) == 0 {
		return perfRouteResult{}
	}

	// Budget policy: deterministic cap by metric-key lexical order.
	sort.SliceStable(deduped, func(i, j int) bool {
		return deduped[i].metricKey < deduped[j].metricKey
	})
	if len(deduped) > r.maxPerJob {
		deduped = deduped[:r.maxPerJob]
	}

	result := perfRouteResult{
		values:          make([]perfValueMeasureSet, 0, len(deduped)),
		thresholdStates: make([]perfThresholdStateSet, 0, len(deduped)),
	}
	for _, item := range deduped {
		base := perfMetricIdentity(source, item)
		tail := perfMetricTail(item)
		warnLow, warnHigh := perfThresholdBounds(item.warn)
		critLow, critHigh := perfThresholdBounds(item.crit)
		result.values = append(result.values, perfValueMeasureSet{
			name:       base,
			scriptName: source,
			unit:       unitForClass(item.class),
			counter:    item.class == perfClassCounter,
			fieldMask:  perfThresholdFieldMask(item.warn, item.crit),
			value:      item.value,
			warnLow:    warnLow,
			warnHigh:   warnHigh,
			critLow:    critLow,
			critHigh:   critHigh,
		})

		if item.class == perfClassCounter {
			// TODO: Add threshold-state handling for counter perfdata in a follow-up branch.
			// Nagios counter thresholds are authored against raw totals, while the value family
			// is emitted with counter semantics and flattened as deltas.
			continue
		}

		result.thresholdStates = append(result.thresholdStates, perfThresholdStateSet{
			name:          perfThresholdStateMetricName(base),
			scriptName:    source,
			perfdataValue: tail,
			state:         thresholdStateForPerfDatum(item),
		})
	}

	return result
}

func perfMetricIdentity(source string, item perfPreparedDatum) string {
	return fmt.Sprintf("perfdata.%s.%s", source, perfMetricTail(item))
}

func perfMetricTail(item perfPreparedDatum) string {
	return fmt.Sprintf("%s_%s", item.class, item.metricKey)
}

func perfThresholdStateMetricName(base string) string {
	return base + "_threshold_state"
}

func thresholdStateForPerfDatum(item perfPreparedDatum) string {
	warnDefined := item.warn != nil
	critDefined := item.crit != nil
	switch {
	case !warnDefined && !critDefined:
		return perfThresholdStateNone
	case thresholdAlertable(item.value, item.crit):
		return perfThresholdStateCritical
	case thresholdAlertable(item.value, item.warn):
		return perfThresholdStateWarning
	default:
		return perfThresholdStateOK
	}
}

func thresholdAlertable(value float64, rng *output.ThresholdRange) bool {
	if rng == nil {
		return false
	}

	low := math.Inf(-1)
	if rng.Low != nil {
		low = *rng.Low
	}
	high := math.Inf(1)
	if rng.High != nil {
		high = *rng.High
	}

	inside := value >= low && value <= high
	if rng.Inclusive {
		return inside
	}
	return !inside
}

func perfThresholdBounds(rng *output.ThresholdRange) (low, high metrix.SampleValue) {
	if rng == nil {
		return 0, 0
	}
	return perfThresholdBoundValue(rng.Low), perfThresholdBoundValue(rng.High)
}

func perfThresholdBoundValue(v *float64) metrix.SampleValue {
	if v == nil {
		// MeasureSet fields must be finite and always present once declared.
		// Use zero for absent/open-ended bounds and keep threshold-state charts
		// as the source of truth for whether a threshold exists.
		return 0
	}
	return metrix.SampleValue(*v)
}

func perfThresholdFieldMask(warn, crit *output.ThresholdRange) perfMeasureFieldMask {
	var mask perfMeasureFieldMask
	if warn != nil {
		if warn.LowSet {
			mask |= perfMeasureFieldWarnLow
		}
		if warn.HighSet {
			mask |= perfMeasureFieldWarnHigh
		}
	}
	if crit != nil {
		if crit.LowSet {
			mask |= perfMeasureFieldCritLow
		}
		if crit.HighSet {
			mask |= perfMeasureFieldCritHigh
		}
	}
	return mask
}
