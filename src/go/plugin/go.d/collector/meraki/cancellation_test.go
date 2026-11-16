// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRejectsCanceledContext(t *testing.T) {
	c := New()
	c.OrganizationID = "org"
	c.APIKey = "test-key"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Init(ctx), context.Canceled)
}

type cancelingGauge struct {
	metrix.SnapshotGauge
	cancel context.CancelFunc
}

func (g cancelingGauge) Observe(value metrix.SampleValue, labels ...metrix.LabelSet) {
	g.SnapshotGauge.Observe(value, labels...)
	g.cancel()
}

// cancelingMeter delegates to the real meter and cancels once the named gauge
// has staged a value, wherever the meter is bound to a host scope or labels.
type cancelingMeter struct {
	metrix.SnapshotMeter
	gauge  string
	cancel context.CancelFunc
}

func (m cancelingMeter) WithHostScope(s metrix.HostScope) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithHostScope(s)
	return m
}

func (m cancelingMeter) WithLabels(l ...metrix.Label) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithLabels(l...)
	return m
}

func (m cancelingMeter) WithLabelSet(l ...metrix.LabelSet) metrix.SnapshotMeter {
	m.SnapshotMeter = m.SnapshotMeter.WithLabelSet(l...)
	return m
}

func (m cancelingMeter) Gauge(name string, opts ...metrix.InstrumentOption) metrix.SnapshotGauge {
	g := m.SnapshotMeter.Gauge(name, opts...)
	if name != m.gauge {
		return g
	}
	return cancelingGauge{
		SnapshotGauge: g,
		cancel:        m.cancel,
	}
}

func TestCollectAbortsCancellationDuringMetricWrites(t *testing.T) {
	// Delegate to the real store, then cancel while the cycle is still staging
	// observations: at the first write, or at a health write, one of the last.
	// A canceled collection must abort instead of publishing any of them.
	tests := map[string]func(c *Collector, cancel context.CancelFunc){
		"first write": func(c *Collector, cancel context.CancelFunc) {
			c.metrics.organizationAssigned = cancelingGauge{
				SnapshotGauge: c.metrics.organizationAssigned,
				cancel:        cancel,
			}
		},
		"health write": func(c *Collector, cancel context.CancelFunc) {
			c.metrics.meter = cancelingMeter{
				SnapshotMeter: c.metrics.meter,
				gauge:         metricApplianceUplinkLatency,
				cancel:        cancel,
			}
		},
	}
	for name, cancelAt := range tests {
		t.Run(name, func(t *testing.T) {
			_, c, _ := newHealthFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cancelAt(c, cancel)
			require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
			assert.Empty(t, readOrganization(c))
			assert.Empty(t, readDevices(c))
			assert.Empty(t, metricPoints(c, metricSwitchPortTrafficSent))
			assert.Empty(t, metricPoints(c, metricApplianceUplinkTrafficSent))
			assert.Empty(t, metricPoints(c, metricApplianceUplinkStatus))
			assert.Empty(t, metricPoints(c, metricApplianceUplinkLatency))
		})
	}
}
