// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
)

var sensorStates = []string{"nominal", "warning", "critical", "unknown"}

type collectorMetrics struct {
	meter  metrix.SnapshotMeter
	values map[string]metrix.SnapshotGauge
	state  metrix.StateSetInstrument
	events metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	metrics := &collectorMetrics{meter: m, values: make(map[string]metrix.SnapshotGauge),
		state:  m.StateSet("sensor_state", metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(sensorStates...)),
		events: m.Gauge("sel_events")}
	for _, name := range []string{"temperature_c", "temperature_f", "voltage", "ampere", "fan_speed", "power", "reading_percent"} {
		metrics.values[name] = m.Gauge(name, metrix.WithFloat(true))
	}
	return metrics
}
func (m *collectorMetrics) observe(s *ipmiapi.Snapshot) {
	for _, sensor := range s.Sensors {
		labels := m.meter.LabelSet(metrix.Label{Key: "sensor_id", Value: sensor.Key},
			metrix.Label{Key: "sensor", Value: sensor.Name}, metrix.Label{Key: "type", Value: sensor.Type},
			metrix.Label{Key: "component", Value: sensor.Component})
		m.state.ObserveStateSet(metrix.StateSetPoint{States: map[string]bool{sensor.State: true}}, labels)
		if gauge, ok := m.values[sensor.Metric]; ok && sensor.Value != nil {
			gauge.Observe(*sensor.Value, labels)
		}
	}
	if s.SEL != nil {
		m.events.Observe(*s.SEL)
	}
}
