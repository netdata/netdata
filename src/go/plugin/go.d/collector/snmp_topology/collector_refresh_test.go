// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollector_RefreshKeepsPublishedSnapshotWhileCollectionRuns(t *testing.T) {
	previousRegistry := snmpTopologyRegistry
	registry := newTopologyRegistry()
	snmpTopologyRegistry = registry
	t.Cleanup(func() { snmpTopologyRegistry = previousRegistry })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "10.0.0.10",
		Port:        161,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	key := "10.0.0.10:161"
	published := newTopologyCache()
	seedPublishedEndpointSnapshot(published)
	registry.register(published)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	coll := New()
	coll.deviceCaches[key] = published
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return &blockingTopologyCollector{
			started: started,
			release: release,
			result:  replacementEndpointProfileMetrics(),
		}
	}

	go func() {
		defer close(done)
		coll.refreshDeviceTopology(key, dev)
	}()

	<-started

	snapshot, ok := published.snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, snapshot.l2Observations, 1)
	require.Len(t, snapshot.l2Observations[0].FDBEntries, 1)
	require.Len(t, snapshot.l2Observations[0].ARPNDEntries, 1)

	close(release)
	<-done
}

type blockingTopologyCollector struct {
	started chan<- struct{}
	release <-chan struct{}
	result  []*ddsnmp.ProfileMetrics
}

func (c *blockingTopologyCollector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	close(c.started)
	<-c.release
	return c.result, nil
}

func expectTopologyRefreshSNMPClient(mockHandler *snmpmock.MockHandler, dev ddsnmp.DeviceConnectionInfo) {
	mockHandler.EXPECT().SetTarget(dev.Hostname)
	mockHandler.EXPECT().SetPort(uint16(dev.Port))
	mockHandler.EXPECT().SetRetries(dev.Retries)
	mockHandler.EXPECT().SetTimeout(time.Duration(dev.Timeout) * time.Second)
	mockHandler.EXPECT().SetMaxOids(dev.MaxOIDs)
	mockHandler.EXPECT().SetMaxRepetitions(uint32(dev.MaxRepetitions))
	mockHandler.EXPECT().SetCommunity(dev.Community)
	mockHandler.EXPECT().SetVersion(gosnmp.Version2c)
	mockHandler.EXPECT().Connect().Return(nil)
	mockHandler.EXPECT().Get(gomock.InAnyOrder([]string{
		snmputils.OidSnmpEngineTime,
		snmputils.OidHrSystemUptime,
		snmputils.OidSysUpTime,
	})).Return(&gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Name: snmputils.OidSnmpEngineTime, Type: gosnmp.Integer, Value: 1234},
		},
	}, nil)
	mockHandler.EXPECT().Close().Return(nil)
}

func seedPublishedEndpointSnapshot(cache *topologyCache) {
	now := time.Now()
	cache.updateTime = now
	cache.lastUpdate = now
	cache.staleAfter = time.Hour
	cache.agentID = "agent-1"
	cache.localDevice = topologyDevice{
		ManagementIP:  "10.0.0.10",
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "switch-a",
	}
	cache.bridgePortToIf["5"] = "5"
	cache.fdbEntries["00:50:56:ab:cd:ef|5||"] = &fdbEntry{
		mac:        "00:50:56:ab:cd:ef",
		bridgePort: "5",
		status:     "learned",
	}
	cache.arpEntries["5|10.0.0.20|00:50:56:ab:cd:ef"] = &arpEntry{
		ifIndex:  "5",
		ip:       "10.0.0.20",
		mac:      "00:50:56:ab:cd:ef",
		addrType: "ipv4",
	}
}

func replacementEndpointProfileMetrics() []*ddsnmp.ProfileMetrics {
	return []*ddsnmp.ProfileMetrics{{
		TopologyMetrics: []ddsnmp.Metric{
			{
				TopologyKind: ddsnmp.KindBridgePortIfIndex,
				Tags: map[string]string{
					tagBridgeBasePort: "5",
					tagBridgeIfIndex:  "5",
				},
			},
			{
				TopologyKind: ddsnmp.KindQbridgeFdbEntry,
				Tags: map[string]string{
					tagDot1qFdbID:   "7",
					tagDot1qFdbMac:  "00:50:56:ab:cd:ef",
					tagDot1qFdbPort: "5",
				},
			},
			{
				TopologyKind: ddsnmp.KindArpEntry,
				Tags: map[string]string{
					tagArpIfIndex:  "5",
					tagArpIP:       "10.0.0.20",
					tagArpMac:      "005056abcdef",
					tagArpAddrType: "ipv4",
				},
			},
		},
	}}
}
