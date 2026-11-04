// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compositeRefreshTestProfile(device *refreshTestDevice, table bool) (*ddsnmp.Profile, string, string) {
	inOID, outOID := "1.3.6.1.4.1.99999.101.0", "1.3.6.1.4.1.99999.102.0"
	metrics := []ddprofiledefinition.MetricsConfig{
		{Symbol: ddprofiledefinition.SymbolConfig{
			OID:  inOID,
			Name: "_in",
		}},
		{Symbol: ddprofiledefinition.SymbolConfig{
			OID:  outOID,
			Name: "_out",
		}},
	}
	key := "traffic"
	if table {
		inOID, outOID = refreshTestTable+".1.1", refreshTestTable+".2.1"
		metrics = []ddprofiledefinition.MetricsConfig{{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  refreshTestTable,
				Name: "trafficTable",
			},
			Symbols: []ddprofiledefinition.SymbolConfig{
				{OID: refreshTestTable + ".1", Name: "_in"},
				{OID: refreshTestTable + ".2", Name: "_out"},
			},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{Tag: "interface", Index: 1}},
		}}
		key = "traffic_1"
	}
	device.values[inOID] = gosnmp.SnmpPDU{
		Name:  inOID,
		Type:  gosnmp.Counter64,
		Value: uint64(12),
	}
	device.values[outOID] = gosnmp.SnmpPDU{
		Name:  outOID,
		Type:  gosnmp.Counter64,
		Value: uint64(34),
	}
	virtual := ddprofiledefinition.VirtualMetricConfig{
		Name: "traffic",
		Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
			{Metric: "_in", As: "in"},
			{Metric: "_out", As: "out"},
		},
	}
	if table {
		virtual.PerRow = true
		for i := range virtual.Sources {
			virtual.Sources[i].Table = "trafficTable"
		}
	}
	return &ddsnmp.Profile{
		SourceFile: "composite.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics:        metrics,
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{virtual},
		},
	}, key, outOID
}

func TestCollectorRuntimeMetadataRedrawRetainsDimensions(t *testing.T) {
	for name, table := range map[string]bool{"scalar": false, "table": true} {
		t.Run(name, func(t *testing.T) {
			c, device, now := newRefreshTestCollector(t)
			profile, key, outOID := compositeRefreshTestProfile(device, table)
			c.snmpProfiles = []*ddsnmp.Profile{profile}
			require.Contains(t, c.Collect(context.Background()), metricIDFromKey(key, "out"))
			chart := c.Charts().Get(chartIDFromKey(key))
			require.NotNil(t, chart)
			require.Len(t, chart.Dims, 2)
			device.omittedOID = outOID
			device.setText(snmputils.OidSysName, "changed")
			*now = now.Add(time.Minute)
			partial := c.Collect(context.Background())
			require.Contains(t, partial, metricIDFromKey(key, "in"))
			require.NotContains(
				t,
				partial,
				metricIDFromKey(key, "out"),
				"the source must actually omit this dimension during redraw",
			)
			assert.True(
				t,
				chart.HasDim(metricIDFromKey(key, "out")),
				"metadata redraw cannot infer dimension removal from a partial observation",
			)
			device.omittedOID = ""
			require.Contains(t, c.Collect(context.Background()), metricIDFromKey(key, "out"))
			assert.True(t, chart.HasDim(metricIDFromKey(key, "out")))
		})
	}
}

func TestCollectorRuntimeReplacementPartialDimensionsRecover(t *testing.T) {
	for name, incumbent := range map[string]bool{"new dimension": false, "retired incumbent dimension": true} {
		t.Run(name, func(t *testing.T) {
			c, device, now := newRefreshTestCollector(t)
			profile, key, outOID := compositeRefreshTestProfile(device, false)
			if incumbent {
				c.snmpProfiles = []*ddsnmp.Profile{profile}
			}
			require.NotEmpty(t, c.Collect(context.Background()))
			client, _ := newReplacementTestClient(t, device, nil)
			t.Cleanup(func() { c.Cleanup(context.Background()) })
			c.newSnmpClient = func() gosnmp.Handler { return client }
			c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
				cfg.Profiles = []*ddsnmp.Profile{profile}
				return ddsnmpcollector.New(cfg)
			}
			device.omittedOID = outOID
			device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
				Name:  snmputils.OidSysObject,
				Type:  gosnmp.ObjectIdentifier,
				Value: replacementTestObjectID,
			}
			*now = now.Add(time.Minute)
			first := c.Collect(context.Background())
			require.Contains(t, first, metricIDFromKey(key, "in"))
			require.NotContains(t, first, metricIDFromKey(key, "out"))
			device.omittedOID = ""
			require.Contains(t, c.Collect(context.Background()), metricIDFromKey(key, "out"))
			chart := c.Charts().Get(chartIDFromKey(key))
			require.NotNil(t, chart)
			dim := chart.GetDim(metricIDFromKey(key, "out"))
			require.NotNil(
				t,
				dim,
				"a current-profile dimension absent from its first frame must be declared when observed",
			)
			assert.False(t, dim.IsRemoved(), "a current-profile dimension must revive a pending incumbent removal")
		})
	}
}
