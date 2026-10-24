// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// writeSnapshot stages a validated snapshot. Checks become one-active-state
// statesets named by checkMetricName.
func (c *Collector) writeSnapshot(snap snapshot) {
	meter := c.store.Write().SnapshotMeter("")
	for _, family := range snap.Metrics {
		if len(family.Samples) == 0 {
			continue
		}
		var opts []metrix.InstrumentOption
		if family.ChartMeta.Title != "" {
			opts = append(opts, metrix.WithDescription(family.ChartMeta.Title))
		}
		if family.ChartMeta.Family != "" {
			opts = append(opts, metrix.WithChartFamily(family.ChartMeta.Family))
		}
		if family.ChartMeta.Priority != nil {
			opts = append(opts, metrix.WithChartPriority(*family.ChartMeta.Priority))
		}
		switch family.Type {
		case metricGauge:
			instrument := meter.Gauge(
				family.Name,
				append(opts, metrix.WithUnit(family.Unit), metrix.WithFloat(true))...)
			for _, sample := range family.Samples {
				instrument.Observe(*sample.Value, meter.LabelSet(metricLabels(sample.Labels)...))
			}
		case metricCounter:
			instrument := meter.Counter(
				family.Name,
				append(opts, metrix.WithUnit(family.Unit), metrix.WithFloat(true))...)
			for _, sample := range family.Samples {
				instrument.ObserveTotal(*sample.Value, meter.LabelSet(metricLabels(sample.Labels)...))
			}
		case metricStateSet:
			mode := metrix.ModeEnum
			if family.Mode == "bitset" {
				mode = metrix.ModeBitSet
			}
			instrument := meter.StateSet(
				family.Name,
				append(
					opts,
					metrix.WithUnit("state"),
					metrix.WithStateSetStates(family.States...),
					metrix.WithStateSetMode(mode),
				)...)
			for _, sample := range family.Samples {
				states := make(map[string]bool, len(sample.Active))
				for _, active := range sample.Active {
					states[active] = true
				}
				instrument.ObserveStateSet(
					metrix.StateSetPoint{
						States: states,
					},
					meter.LabelSet(metricLabels(sample.Labels)...),
				)
			}
		}
	}
	for _, check := range snap.Checks {
		if len(check.Samples) == 0 {
			continue
		}
		state := meter.StateSet(
			checkMetricName(check.ID),
			metrix.WithStateSetStates(checkStates...),
			metrix.WithStateSetMode(metrix.ModeEnum),
		)
		for _, sample := range check.Samples {
			state.ObserveStateSet(metrix.StateSetPoint{
				States: map[string]bool{sample.State: true},
			}, meter.LabelSet(metricLabels(sample.Labels)...))
		}
	}
}

func metricLabels(values map[string]string) []metrix.Label {
	labels := make([]metrix.Label, 0, len(values))
	for key, value := range values {
		labels = append(labels, metrix.Label{
			Key:   key,
			Value: value,
		})
	}
	return labels
}
