// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (c *Collector) writeHealthMetrics(devices map[string]metrix.SnapshotMeter, health applianceHealthSnapshot) {
	now := c.now()
	for _, report := range health.reports {
		if meter, ok := devices[report.serial]; ok && !report.at.After(now) {
			meter.Gauge(metricApplianceReportAge, metrix.WithFloat(true)).Observe(metrix.SampleValue(now.Sub(report.at).Seconds()))
		}
	}
	for _, uplink := range health.states {
		if meter, ok := devices[uplink.serial]; ok {
			meter.WithLabels(metrix.Label{Key: "interface", Value: uplink.iface}).
				StateSet(metricApplianceUplinkStatus, uplinkStateSet...).Enable(uplink.state)
		}
	}
	for _, probe := range health.probes {
		meter, ok := devices[probe.serial]
		if !ok || probe.at.After(now) {
			continue
		}
		meter = meter.WithLabels(
			metrix.Label{Key: "interface", Value: probe.iface},
			metrix.Label{Key: "destination_ip", Value: probe.destination},
		)
		if probe.values.loss.ok {
			meter.Gauge(metricApplianceUplinkPacketLoss, metrix.WithFloat(true)).Observe(metrix.SampleValue(probe.values.loss.value))
		}
		if probe.values.latency.ok {
			meter.Gauge(metricApplianceUplinkLatency, metrix.WithFloat(true)).Observe(metrix.SampleValue(probe.values.latency.value))
		}
		meter.Gauge(metricApplianceUplinkSampleAge, metrix.WithFloat(true)).Observe(metrix.SampleValue(now.Sub(probe.at).Seconds()))
	}
}
