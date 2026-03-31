// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollectorCollectSNMPSkipsTopologyRefresh(t *testing.T) {
	coll := New()
	coll.Config = prepareV2Config()
	coll.sysInfo = &snmputils.SysInfo{
		Name:        "switch-a",
		SysObjectID: "1.2.3.4.5",
	}
	coll.ddSnmpColl = &mockDdSnmpCollector{
		pms: []*ddsnmp.ProfileMetrics{
			{
				Tags: map[string]string{
					tagLldpLocChassisID:        "00:11:22:33:44:55",
					tagLldpLocChassisIDSubtype: "macAddress",
				},
				Metrics: []ddsnmp.Metric{
					{
						Name:    metricLldpLocPortEntry,
						IsTable: true,
						Tags: map[string]string{
							tagLldpLocPortNum:       "1",
							tagLldpLocPortID:        "Gi0/1",
							tagLldpLocPortIDSubtype: "interfaceName",
						},
					},
					{
						Name:    metricLldpRemEntry,
						IsTable: true,
						Tags: map[string]string{
							tagLldpLocPortNum:          "1",
							tagLldpRemIndex:            "1",
							tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
							tagLldpRemChassisIDSubtype: "macAddress",
							tagLldpRemPortID:           "Gi0/2",
							tagLldpRemPortIDSubtype:    "interfaceName",
							tagLldpRemSysName:          "switch-b",
							tagLldpRemMgmtAddr:         "10.0.0.2",
							tagLldpRemMgmtAddrSubtype:  "ipv4",
						},
					},
				},
			},
		},
	}

	mx := make(map[string]int64)
	require.NoError(t, coll.collectSNMP(mx))

	coll.topologyCache.mu.RLock()
	defer coll.topologyCache.mu.RUnlock()

	assert.True(t, coll.topologyCache.lastUpdate.IsZero())
	assert.Empty(t, coll.topologyCache.lldpLocPorts)
	assert.Empty(t, coll.topologyCache.lldpRemotes)
	assert.NotContains(t, mx, "snmp_topology_devices_total")
}

func TestCollectorRefreshTopologySnapshotPublishesFreshSnapshot(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	setMockClientInitExpect(mockSNMP)
	mockSNMP.EXPECT().Close().Return(nil).AnyTimes()

	coll := New()
	coll.Config = prepareV2Config()
	coll.Ping.Enabled = false
	coll.CreateVnode = false
	coll.sysInfo = &snmputils.SysInfo{
		Name:        "switch-a",
		SysObjectID: "1.2.3.4.5",
	}
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	coll.topologyProfiles = []*ddsnmp.Profile{{}}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return &mockDdSnmpCollector{
			pms: []*ddsnmp.ProfileMetrics{
				{
					Tags: map[string]string{
						tagLldpLocChassisID:        "00:11:22:33:44:55",
						tagLldpLocChassisIDSubtype: "macAddress",
						tagLldpLocSysName:          "switch-a",
					},
					Metrics: []ddsnmp.Metric{
						{
							Name:    metricLldpLocPortEntry,
							IsTable: true,
							Tags: map[string]string{
								tagLldpLocPortNum:       "1",
								tagLldpLocPortID:        "Gi0/1",
								tagLldpLocPortIDSubtype: "interfaceName",
							},
						},
						{
							Name:    metricLldpRemEntry,
							IsTable: true,
							Tags: map[string]string{
								tagLldpLocPortNum:          "1",
								tagLldpRemIndex:            "1",
								tagLldpRemChassisID:        "aa:bb:cc:dd:ee:ff",
								tagLldpRemChassisIDSubtype: "macAddress",
								tagLldpRemPortID:           "Gi0/2",
								tagLldpRemPortIDSubtype:    "interfaceName",
								tagLldpRemSysName:          "switch-b",
								tagLldpRemMgmtAddr:         "10.0.0.2",
								tagLldpRemMgmtAddrSubtype:  "ipv4",
							},
						},
					},
				},
			},
		}
	}

	require.NoError(t, coll.refreshTopologySnapshot())

	coll.topologyCache.mu.RLock()
	snapshot, ok := coll.topologyCache.snapshot()
	coll.topologyCache.mu.RUnlock()
	require.True(t, ok)
	require.Len(t, snapshot.Links, 1)

	mx := make(map[string]int64)
	coll.collectTopologyMetrics(mx)

	assert.Equal(t, int64(2), mx["snmp_topology_devices_total"])
	assert.Equal(t, int64(1), mx["snmp_topology_devices_discovered"])
	assert.Equal(t, int64(1), mx["snmp_topology_links_total"])
	assert.Equal(t, int64(1), mx["snmp_topology_links_lldp"])
}

func TestTopologyCacheSnapshotExpiresAfterStaleAfter(t *testing.T) {
	cache := newTopologyCache()
	cache.updateTime = time.Now().Add(-2 * time.Minute)
	cache.lastUpdate = cache.updateTime
	cache.staleAfter = time.Minute
	cache.localDevice = topologyDevice{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		ManagementIP:  "192.0.2.1",
	}
	cache.lldpLocPorts["1"] = &lldpLocPort{
		portNum:       "1",
		portID:        "Gi0/1",
		portIDSubtype: "interfaceName",
	}
	cache.lldpRemotes["1:1"] = &lldpRemote{
		localPortNum:     "1",
		remIndex:         "1",
		chassisID:        "aa:bb:cc:dd:ee:ff",
		chassisIDSubtype: "macAddress",
		portID:           "Gi0/2",
		portIDSubtype:    "interfaceName",
	}

	_, ok := cache.snapshot()
	assert.False(t, ok)
}

func TestEnsureTopologySchedulerStartedDoesNotWaitForInitialRefresh(t *testing.T) {
	mockSNMP, cleanup := mockInit(t)
	defer cleanup()

	setMockClientSetterExpect(mockSNMP)

	connectBlocked := make(chan struct{})
	mockSNMP.EXPECT().Connect().DoAndReturn(func() error {
		<-connectBlocked
		return nil
	}).AnyTimes()
	mockSNMP.EXPECT().Close().Return(nil).AnyTimes()

	coll := New()
	coll.Config = prepareV2Config()
	coll.sysInfo = &snmputils.SysInfo{SysObjectID: "1.2.3.4.5"}
	coll.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector { return &mockDdSnmpCollector{} }
	coll.topologyProfiles = []*ddsnmp.Profile{
		{
			Definition: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{
						Symbol: ddprofiledefinition.SymbolConfig{Name: metricLldpLocPortEntry},
					},
				},
			},
		},
	}

	done := make(chan struct{})
	go func() {
		coll.ensureTopologySchedulerStarted()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for topology scheduler startup to return")
	}

	close(connectBlocked)
	coll.stopTopologyScheduler()
}
