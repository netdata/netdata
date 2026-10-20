// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// writeSnapshot stages a validated snapshot. Checks become one-active-state
// statesets named by checkMetricName.
func (c *Collector) writeSnapshot(snap snapshot) {
	meter := c.store.Write().SnapshotMeter("")
	for _, sample := range snap.Metrics {
		definition := c.definition.metricByName[sample.Name]
		labels := meter.LabelSet(metricLabels(sample.Labels)...)
		opts := []metrix.InstrumentOption{metrix.WithUnit(definition.Unit), metrix.WithFloat(true)}
		if definition.Type == metricGauge {
			meter.Gauge(sample.Name, opts...).Observe(*sample.Value, labels)
		} else {
			meter.Counter(sample.Name, opts...).ObserveTotal(*sample.Value, labels)
		}
	}
	for _, check := range snap.Checks {
		state := meter.StateSet(
			checkMetricName(check.ID),
			metrix.WithStateSetStates(checkStates...),
			metrix.WithStateSetMode(metrix.ModeEnum),
		)
		state.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{check.State: true},
		}, meter.LabelSet(metricLabels(check.Labels)...))
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
