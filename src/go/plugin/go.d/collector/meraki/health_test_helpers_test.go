// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	applianceUplinkStatusesPath = "/organizations/org/appliance/uplink/statuses"
	uplinkLossAndLatencyPath    = "/organizations/org/devices/uplinksLossAndLatency"
)

func newHealthFixture(t *testing.T) (*fixtureAPI, *Collector, time.Time) {
	t.Helper()
	f, c, now := newTrafficFixture(t)
	c.CollectApplianceUplinkHealth = true
	f.set(applianceUplinkStatusesPath, fixtureResponse{body: "[" + healthStatusRow("A", "N1", now.Add(-30*time.Second).Format(time.RFC3339), `{"interface":"wan1","status":"active"}`) + "," + healthStatusRow("B", "N1", now.Add(-time.Minute).Format(time.RFC3339), `{"interface":"wan1","status":"ready"}`) + "]"})
	f.set(uplinkLossAndLatencyPath, fixtureResponse{body: "[" + healthProbeRow("A", "N1", "wan1", "198.51.100.1", healthSample(now.Add(-3*time.Minute), `"lossPercent":5.25,"latencyMs":12.5`)) + "," + healthProbeRow("B", "N1", "wan1", "198.51.100.1", healthSample(now.Add(-3*time.Minute), `"lossPercent":0,"latencyMs":25.75`)) + "]"})
	return f, c, now
}

func healthOnly(c *Collector) {
	c.CollectSwitchPortTraffic = false
	c.CollectSwitchPortPackets = false
	c.CollectApplianceUplinkTraffic = false
}
func healthStatusRow(serial, network, at, uplinks string) string {
	return fmt.Sprintf(`{"serial":%q,"networkId":%q,"lastReportedAt":%q,"uplinks":[%s]}`, serial, network, at, uplinks)
}
func healthProbeRow(serial, network, iface, destination, samples string) string {
	return fmt.Sprintf(`{"serial":%q,"networkId":%q,"uplink":%q,"ip":%q,"timeSeries":[%s]}`, serial, network, iface, destination, samples)
}
func healthSample(at time.Time, fields string) string {
	comma := ""
	if fields != "" {
		comma = ","
	}
	return fmt.Sprintf(`{"ts":%q%s%s}`, at.Format(time.RFC3339Nano), comma, fields)
}
func healthPointsByKey(c *Collector, metric string) map[string]metrix.SampleValue {
	result := map[string]metrix.SampleValue{}
	for _, p := range trafficPoints(c, metric) {
		key := strings.Join([]string{p.Scope.Labels["serial"], p.Labels["interface"], p.Labels["destination_ip"]}, "/")
		if state := p.Labels[metricApplianceUplinkStatus]; state != "" {
			key += "/" + state
		}
		result[key] = p.Value
	}
	return result
}

// Preserve the real meter's bindings and observations; cancel only after a
// health gauge has staged a value, so the collector's final context check must
// prevent the whole cycle from committing.
type healthCancelMeter struct {
	metrix.SnapshotMeter
	cancel context.CancelFunc
}

func (m healthCancelMeter) WithHostScope(s metrix.HostScope) metrix.SnapshotMeter {
	return healthCancelMeter{m.SnapshotMeter.WithHostScope(s), m.cancel}
}
func (m healthCancelMeter) WithLabels(l ...metrix.Label) metrix.SnapshotMeter {
	return healthCancelMeter{m.SnapshotMeter.WithLabels(l...), m.cancel}
}
func (m healthCancelMeter) WithLabelSet(l ...metrix.LabelSet) metrix.SnapshotMeter {
	return healthCancelMeter{m.SnapshotMeter.WithLabelSet(l...), m.cancel}
}
func (m healthCancelMeter) Gauge(name string, opts ...metrix.InstrumentOption) metrix.SnapshotGauge {
	g := m.SnapshotMeter.Gauge(name, opts...)
	if name == metricApplianceUplinkLatency {
		return cancelingGauge{SnapshotGauge: g, cancel: m.cancel}
	}
	return g
}
