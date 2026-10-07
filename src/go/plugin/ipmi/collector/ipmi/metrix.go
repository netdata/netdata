// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
)

var sensorStates = []string{bmc.StateNominal, bmc.StateWarning, bmc.StateCritical, bmc.StateUnknown}

// readingMetrics names the reading metric of each unit with numeric samples.
var readingMetrics = map[string]string{
	bmc.UnitCelsius:    "temperature_c",
	bmc.UnitFahrenheit: "temperature_f",
	bmc.UnitVolts:      "voltage",
	bmc.UnitAmps:       "ampere",
	bmc.UnitRPM:        "fan_speed",
	bmc.UnitWatts:      "power",
	bmc.UnitPercent:    "reading_percent",
}

type collectorMetrics struct {
	meter      metrix.SnapshotMeter
	readings   map[string]metrix.SnapshotGauge // by unit
	state      metrix.StateSetInstrument
	selEntries metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	m := &collectorMetrics{
		meter:    meter,
		readings: make(map[string]metrix.SnapshotGauge, len(readingMetrics)),
		state: meter.StateSet(
			"sensor_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(sensorStates...),
		),
		selEntries: meter.Gauge("sel_events"),
	}
	for unit, name := range readingMetrics {
		m.readings[unit] = meter.Gauge(name, metrix.WithFloat(true))
	}
	return m
}

// write observes a snapshot. Unavailable readings and SEL counts leave gaps.
func (m *collectorMetrics) write(s *bmc.Snapshot) {
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
