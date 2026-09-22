// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestCollectProfileStatsPreparation(t *testing.T) {
	collector := New(ddsnmp.NewDeviceStore())
	handler, cleanup := mockInit(t)
	defer cleanup()
	setMockClientInitExpect(handler)
	setMockClientSysInfoExpect(handler)
	handler.EXPECT().Close().Return(nil)
	collector.Config = prepareV2Config()
	collector.CreateVnode = false
	collector.snmpProfiles = []*ddsnmp.Profile{{}}
	collector.newSnmpClient = func() gosnmp.Handler { return handler }
	profile := &ddsnmp.ProfileMetrics{
		Source: "synthetic.yaml",
	}
	profile.Stats.Timing.Preparation = 3 * time.Millisecond
	profile.Stats.Timing.Scalar = 2 * time.Millisecond
	profile.Stats.Errors.Processing.Preparation = 4
	collector.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{profile}}
	}
	require.NoError(t, collector.Init(context.Background()))
	defer collector.Cleanup(context.Background())
	require.NoError(t, collector.Check(context.Background()))
	mx := collector.Collect(context.Background())
	assert.EqualValues(t, 3, mx["snmp_device_prof_synthetic_stats_timings_preparation"])
	assert.EqualValues(t, 4, mx["snmp_device_prof_synthetic_stats_errors_processing_preparation"])
	assert.Equal(t, 5*time.Millisecond, profile.Stats.Timing.Total())
}
