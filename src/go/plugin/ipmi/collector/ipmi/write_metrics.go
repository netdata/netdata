// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

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
	if s.SEL != nil && s.SEL.NewKnown {
		c.writeNewSELRecords(s.SEL.New)
	}
}

// writeNewSELRecords counts new records by event type and state, zeros
// included.
func (c *Collector) writeNewSELRecords(records []bmc.SELRecord) {
	type key struct{ eventType, state string }
	counts := make(map[key]int)
	for _, rec := range records {
		eventType := selEventDiscrete
		if rec.Threshold {
			eventType = selEventThreshold
		}
		counts[key{eventType, rec.State}]++
	}
	m := c.metrics
	for _, eventType := range selEventTypes {
		for _, state := range sensorStates {
			labels := m.meter.LabelSet(
				metrix.Label{
					Key:   "event_type",
					Value: eventType,
				},
				metrix.Label{
					Key:   "state",
					Value: state,
				},
			)
			m.selNew.Observe(float64(counts[key{eventType, state}]), labels)
		}
	}
}
