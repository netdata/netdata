// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (o *observationStore) emit(plan []plannedQuery) int {
	meter := o.store.Write().SnapshotMeter("")
	written := 0
	for _, query := range plan {
		state, ok := o.queries[query.key]
		if !ok || !state.hasValue {
			continue
		}
		value := state.value
		if query.rate {
			value /= query.policy.period.Seconds()
		}
		writeSample(meter, query.seriesName, query.labels, query.tagLabels, value)
		written++
	}
	return written
}

func writeSample(meter metrix.SnapshotMeter, seriesName string, labels, tagLabels []metrix.Label, value float64) {
	all := labels
	if len(tagLabels) > 0 {
		all = make([]metrix.Label, 0, len(labels)+len(tagLabels))
		all = append(all, labels...)
		all = append(all, tagLabels...)
	}
	meter.WithLabels(all...).Gauge(seriesName, metrix.WithFloat(true)).Observe(value)
}
