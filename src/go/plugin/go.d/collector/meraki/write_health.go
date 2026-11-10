// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

// writeHealthMetrics writes appliance uplink health on each appliance's node.
// A report or sample time after the write time (a clock step backwards) is
// skipped.
func (c *Collector) writeHealthMetrics(deviceMeters map[string]metrix.SnapshotMeter, health applianceHealthSnapshot) {
	now := c.now()
	for _, report := range health.reports {
		meter, ok := deviceMeters[report.serial]
		age := now.Sub(report.at)
		if !ok || age < 0 {
			continue
		}
		meter.Gauge(metricApplianceReportAge, metrix.WithFloat(true)).Observe(metrix.SampleValue(age.Seconds()))
	}
	for _, uplink := range health.states {
		meter, ok := deviceMeters[uplink.serial]
		if !ok {
			continue
		}
		meter.WithLabels(metrix.Label{
			Key:   "interface",
			Value: uplink.iface,
		}).StateSet(metricApplianceUplinkStatus, uplinkStateSet...).Enable(string(uplink.status))
	}
	for _, probe := range health.probes {
		meter, ok := deviceMeters[probe.serial]
		age := now.Sub(probe.at)
		if !ok || age < 0 {
			continue
		}
		meter = meter.WithLabels(
			metrix.Label{
				Key:   "interface",
				Value: probe.iface,
			},
			metrix.Label{
				Key:   "destination_ip",
				Value: probe.destination,
			},
		)
		if probe.values.loss.ok {
			meter.Gauge(metricApplianceUplinkPacketLoss, metrix.WithFloat(true)).
				Observe(metrix.SampleValue(probe.values.loss.value))
		}
		if probe.values.latency.ok {
			meter.Gauge(metricApplianceUplinkLatency, metrix.WithFloat(true)).
				Observe(metrix.SampleValue(probe.values.latency.value))
		}
		meter.Gauge(metricApplianceUplinkSampleAge, metrix.WithFloat(true)).Observe(metrix.SampleValue(age.Seconds()))
	}
}
