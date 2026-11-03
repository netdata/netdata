// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	trafficHistoryPath = "/organizations/org/switch/ports/usage/history/byDevice/byInterval"
	trafficUplinkPath  = "/organizations/org/appliance/uplinks/usage/byNetwork"
	trafficPacketPath  = "/devices/S1/switch/ports/statuses/packets"
)

func newTrafficFixture(t *testing.T) (*fixtureAPI, *Collector, time.Time) {
	t.Helper()
	f, server := newFixtureAPI(t)
	f.set("/organizations/org/devices", fixtureResponse{body: `[{"serial":"S1","networkId":"N1","name":"Switch","model":"MS120","productType":"switch"},{"serial":"A","networkId":"N1","productType":"appliance"},{"serial":"B","networkId":"N1","productType":"appliance"}]`})
	f.set("/organizations/org/devices/availabilities", fixtureResponse{body: `[{"serial":"S1","status":"online","network":{"id":"N1"}},{"serial":"A","status":"online"},{"serial":"B","status":"dormant"}]`})
	c := newTestCollector(t, server)
	now := time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	c.CollectSwitchPortTraffic = true
	c.CollectSwitchPortPackets = true
	c.CollectApplianceUplinkUsage = true
	f.set(trafficHistoryPath, fixtureResponse{body: trafficHistoryBody("S1", "N1", "1/0", trafficInterval(now.Add(-60*time.Second), 300, `{"upstream":10.5,"downstream":20.25}`))})
	f.set(trafficPacketPath, fixtureResponse{body: `[{"portId":"1/0","packets":[{"desc":"Total","sent":99999999,"recv":88888888,"ratePerSec":{"sent":2.5,"recv":3.25}},{"desc":"Broadcast","ratePerSec":{"sent":999,"recv":999}}]}]`})
	f.set(trafficUplinkPath, fixtureResponse{body: `[{"networkId":"N1","byUplink":[{"interface":"wan1","serial":"UNRELATED_HA_SERIAL","sent":300000,"received":600000}]}]`})
	return f, c, now
}

func trafficInterval(end time.Time, seconds int, rates string) string {
	return fmt.Sprintf(`{"startTs":%q,"endTs":%q,"bandwidth":{"usage":%s}}`, end.Add(-time.Duration(seconds)*time.Second).Format(time.RFC3339), end.Format(time.RFC3339), rates)
}

func trafficHistoryBody(serial, network, port, intervals string) string {
	return fmt.Sprintf(`{"items":[{"serial":%q,"network":{"id":%q},"ports":[{"portId":%q,"intervals":[%s]}]}]}`, serial, network, port, intervals)
}

type trafficPoint struct {
	Scope  metrix.HostScope
	Labels map[string]string
	Value  metrix.SampleValue
}

func trafficPoints(c *Collector, name string) []trafficPoint {
	var points []trafficPoint
	read := func(scope metrix.HostScope) {
		opts := []metrix.ReadOption{metrix.ReadFlatten()}
		if !scope.IsDefault() {
			opts = append(opts, metrix.ReadHostScope(scope.ScopeKey))
		}
		c.store.Read(opts...).ForEachSeries(func(metric string, labels metrix.LabelView, value metrix.SampleValue) {
			if metric == name {
				points = append(points, trafficPoint{Scope: scope, Labels: labels.CloneMap(), Value: value})
			}
		})
	}
	read(metrix.HostScope{})
	for _, scope := range c.store.Read().HostScopes() {
		if !scope.IsDefault() {
			read(scope)
		}
	}
	return points
}

// The wrapper changes only notification timing; observations still reach the
// real snapshot store and are committed or aborted by the public cycle.
type trafficWriteMeter struct {
	metrix.SnapshotMeter
	after func(string)
}

func (m trafficWriteMeter) WithHostScope(scope metrix.HostScope) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithHostScope(scope)
	return m
}
func (m trafficWriteMeter) WithLabels(labels ...metrix.Label) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithLabels(labels...)
	return m
}
func (m trafficWriteMeter) WithLabelSet(labels ...metrix.LabelSet) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithLabelSet(labels...)
	return m
}
func (m trafficWriteMeter) Gauge(name string, opts ...metrix.InstrumentOption) metrix.SnapshotGauge {
	return trafficWriteGauge{SnapshotGauge: m.SnapshotMeter.Gauge(name, opts...), name: name, after: m.after}
}

type trafficWriteGauge struct {
	metrix.SnapshotGauge
	name  string
	after func(string)
}

func (g trafficWriteGauge) Observe(v metrix.SampleValue, labels ...metrix.LabelSet) {
	g.SnapshotGauge.Observe(v, labels...)
	g.after(g.name)
}
