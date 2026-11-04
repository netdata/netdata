// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package ipmi

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
)

var sensorStates = []string{bmc.StateNominal, bmc.StateWarning, bmc.StateCritical, bmc.StateUnknown}

// SEL event types of the new-records chart. Threshold events also change the
// state of their sensor; every other record is discrete.
const (
	selEventThreshold = "threshold"
	selEventDiscrete  = "discrete"
)

var selEventTypes = []string{selEventThreshold, selEventDiscrete}

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
	// selNew counts the SEL records new since the previous sample.
	selNew metrix.SnapshotGauge
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
		selNew:     meter.Gauge("sel_new_records"),
	}
	for unit, name := range readingMetrics {
		m.readings[unit] = meter.Gauge(name, metrix.WithFloat(true))
	}
	return m
}
