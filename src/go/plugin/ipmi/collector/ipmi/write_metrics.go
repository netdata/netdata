// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
)

// writeMetrics observes a snapshot. Unavailable readings and SEL counts leave gaps.
func (c *Collector) writeMetrics(s *bmc.Snapshot) {
	m := c.metrics
	for _, sensor := range s.Sensors {
		labels := m.meter.LabelSet(
			metrix.Label{
				Key:   "sensor_id",
				Value: sensor.Key,
			},
			metrix.Label{
				Key:   "sensor",
				Value: sensor.Name,
			},
			metrix.Label{
				Key:   "type",
				Value: sensor.Type,
			},
			metrix.Label{
				Key:   "component",
				Value: sensor.Component,
			},
		)
		m.state.ObserveStateSet(metrix.StateSetPoint{
			States: map[string]bool{sensor.State: true},
		}, labels)
		if gauge, ok := m.readings[sensor.Unit]; ok && sensor.Value != nil {
			gauge.Observe(*sensor.Value, labels)
		}
	}
	if s.SELEntries != nil {
		m.selEntries.Observe(float64(*s.SELEntries))
	}
}
