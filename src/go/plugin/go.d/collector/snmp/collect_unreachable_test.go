// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

func TestCollector_UnreachablePollDoesNotEmitProfileStats(t *testing.T) {
	handler, cleanup := mockInit(t)
	defer cleanup()
	setMockClientInitExpect(handler)
	setMockClientSysInfoExpect(handler)
	handler.EXPECT().Close().Return(nil)
	const oid = "1.3.6.1.2.1.1.3.0"
	c := New(ddsnmp.NewDeviceStore())
	c.Config = prepareV2Config()
	c.CreateVnode = false
	c.Ping.Enabled = true
	c.snmpProfiles = []*ddsnmp.Profile{
		{SourceFile: "device.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{
				OID:  oid,
				Name: "uptime",
			}}},
		}},
		{SourceFile: "vendor.yaml", Definition: &ddprofiledefinition.ProfileDefinition{}},
	}
	c.newSnmpClient = func() gosnmp.Handler { return handler }
	c.newPinger = func(pinger.Config, *logger.Logger) (pinger.Client, error) {
		return &mockPingClient{
			sample: pingSuccessSample("192.0.2.1"),
		}, nil
	}
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))

	for _, blocked := range []bool{false, true, false} {
		if blocked {
			handler.EXPECT().Get([]string{oid}).Return(nil, context.DeadlineExceeded)
		} else {
			handler.EXPECT().Get([]string{oid}).Return(&gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{{Name: oid, Type: gosnmp.TimeTicks, Value: uint32(100)}},
			}, nil)
		}
		metrics := c.Collect(context.Background())
		if blocked {
			require.Empty(t, metrics, "neither profile statistics nor ping may hide a failed SNMP poll")
		} else {
			require.Contains(t, metrics, "snmp_device_prof_uptime")
			require.Contains(t, metrics, "snmp_device_prof_vendor_stats_metrics_scalar")
			require.Contains(t, metrics, "ping_rtt_avg")
		}
	}
}
