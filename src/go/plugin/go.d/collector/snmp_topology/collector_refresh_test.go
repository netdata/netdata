// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollectorGetRegisteredDevicesUsesInjectedDeviceStore(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	registerTestDeviceState(store, ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           161,
		ManualProfiles: []string{"profile-a"},
		VnodeLabels:    map[string]string{"site": "lab"},
	})

	devices := coll.getRegisteredDevices()
	require.Len(t, devices, 1)
	require.Equal(t, "192.0.2.10", devices[0].Hostname)

	devices[0].ManualProfiles[0] = "changed"
	devices[0].VnodeLabels["site"] = "changed"

	again := coll.getRegisteredDevices()
	require.Len(t, again, 1)
	require.Equal(t, []string{"profile-a"}, again[0].ManualProfiles)
	require.Equal(t, "lab", again[0].VnodeLabels["site"])
}

func TestCollectorValidationLifecycleDoesNotStartPolling(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.UpdateEvery = 3600
	registerTestDeviceState(store, ddsnmp.DeviceConnectionInfo{
		Hostname: "192.0.2.10",
		Port:     161,
	})
	coll.newSnmpClient = func() gosnmp.Handler {
		t.Fatal("validation lifecycle must not start topology polling")
		return nil
	}

	require.NoError(t, coll.Init(context.Background()))
	require.NoError(t, coll.Check(context.Background()))
	require.NoError(t, coll.Check(context.Background()))
	coll.Cleanup(context.Background())
}

func TestCollectorRunRefreshesImmediatelyBeforeUpdateEvery(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.UpdateEvery = 3600
	registerTestDeviceState(store, ddsnmp.DeviceConnectionInfo{
		Hostname: "192.0.2.10",
		Port:     161,
	})

	refreshed := make(chan struct{}, 1)
	coll.newSnmpClient = func() gosnmp.Handler {
		select {
		case refreshed <- struct{}{}:
		default:
		}
		panic("stop")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		require.NoError(t, coll.Run(ctx))
	}()

	seen := false
	require.Eventually(t, func() bool {
		if seen {
			return true
		}
		select {
		case <-refreshed:
			seen = true
			return true
		default:
			return false
		}
	}, 2500*time.Millisecond, 10*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "runner did not stop")
	}
}

func TestCollectorRunStopsOnContextCancel(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.UpdateEvery = 3600

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		require.NoError(t, coll.Run(ctx))
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "runner did not stop")
	}
}

func TestCollectorRunDoesNotPollWhenContextAlreadyCanceled(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	registerTestDeviceState(store, ddsnmp.DeviceConnectionInfo{
		Hostname: "192.0.2.10",
		Port:     161,
	})
	coll.newSnmpClient = func() gosnmp.Handler {
		t.Fatal("Run must not poll with an already canceled context")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, coll.Run(ctx))
}

func TestCollectorPruneStaleDeviceCachesRemovesLastDeviceCache(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	cache := newTopologyCache()
	coll.deviceCaches["gone:161"] = cache
	coll.deviceLastCollected["gone:161"] = time.Now()
	coll.topologyRegistry.register(cache)

	coll.refreshTopology(context.Background())

	require.Empty(t, coll.deviceCaches)
	require.Empty(t, coll.deviceLastCollected)
	require.False(t, topologyRegistryHasCache(coll.topologyRegistry, cache))
}

func TestCollectorRefreshTopologyRecoveringHandlesPanic(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	registerTestDeviceState(store, ddsnmp.DeviceConnectionInfo{
		Hostname: "192.0.2.10",
		Port:     161,
	})
	coll.newSnmpClient = func() gosnmp.Handler {
		panic("boom")
	}

	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })
	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })
}

func TestCollectorRunCancelsInFlightRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	mockHandler.EXPECT().SetTarget(dev.Hostname)
	mockHandler.EXPECT().SetPort(uint16(dev.Port))
	mockHandler.EXPECT().SetRetries(dev.Retries)
	mockHandler.EXPECT().SetTimeout(time.Duration(dev.Timeout) * time.Second)
	mockHandler.EXPECT().SetMaxOids(dev.MaxOIDs)
	mockHandler.EXPECT().SetMaxRepetitions(uint32(dev.MaxRepetitions))
	mockHandler.EXPECT().SetCommunity(dev.Community)
	mockHandler.EXPECT().SetVersion(gosnmp.Version2c)
	mockHandler.EXPECT().Connect().Return(nil)

	getStarted := make(chan struct{})
	closeCalled := make(chan struct{})
	var closeOnce sync.Once
	mockHandler.EXPECT().Get(gomock.InAnyOrder([]string{
		snmputils.OidSnmpEngineTime,
		snmputils.OidHrSystemUptime,
		snmputils.OidSysUpTime,
	})).DoAndReturn(func([]string) (*gosnmp.SnmpPacket, error) {
		close(getStarted)
		<-closeCalled
		return nil, context.Canceled
	})
	mockHandler.EXPECT().Close().DoAndReturn(func() error {
		closeOnce.Do(func() { close(closeCalled) })
		return nil
	}).AnyTimes()

	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.UpdateEvery = 3600
	registerTestDeviceState(store, dev)
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
		return []*ddsnmp.Profile{{}}
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return replacementEndpointProfileMetrics(), nil
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- coll.Run(ctx)
	}()

	select {
	case <-getStarted:
	case <-time.After(5 * time.Second):
		require.Fail(t, "refresh did not start")
	}

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.Fail(t, "runner did not stop after context cancellation")
	}
}

func TestCollectorCancelsInFlightVLANContextRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:  "192.0.2.10",
		Port:      161,
		Community: "public",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	mockHandler.EXPECT().SetTarget(dev.Hostname)
	mockHandler.EXPECT().SetPort(uint16(dev.Port))
	mockHandler.EXPECT().SetRetries(dev.Retries)
	mockHandler.EXPECT().SetTimeout(time.Duration(dev.Timeout) * time.Second)
	mockHandler.EXPECT().SetMaxOids(dev.MaxOIDs)
	mockHandler.EXPECT().SetMaxRepetitions(uint32(dev.MaxRepetitions))
	mockHandler.EXPECT().SetCommunity(dev.Community)
	mockHandler.EXPECT().SetVersion(gosnmp.Version2c)
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c)
	mockHandler.EXPECT().Community().Return(dev.Community)
	mockHandler.EXPECT().SetCommunity(dev.Community + "@100")

	closeCalled := make(chan struct{})
	var closeOnce sync.Once
	mockHandler.EXPECT().Connect().Return(nil)
	mockHandler.EXPECT().Close().DoAndReturn(func() error {
		closeOnce.Do(func() { close(closeCalled) })
		return nil
	}).AnyTimes()

	coll := newTestSNMPTopologyCollector()
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	collectStarted := make(chan struct{})
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			close(collectStarted)
			<-closeCalled
			return nil, context.Canceled
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := collectTopologyVLANContext(ctx, coll, dev, "100", nil)
		errCh <- err
	}()

	select {
	case <-collectStarted:
	case <-time.After(time.Second):
		require.Fail(t, "vlan-context refresh did not start")
	}

	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.Fail(t, "vlan-context refresh did not stop after context cancellation")
	}
}

func TestCollectorNewDeviceCollectionCacheUsesEffectiveDeviceCheckEvery(t *testing.T) {
	coll := newTestSNMPTopologyCollector()

	cache := coll.newDeviceCollectionCache(ddsnmp.DeviceConnectionInfo{Hostname: "switch-a"})

	require.Equal(t, defaultRefreshEvery+2*defaultDeviceCheckEvery, cache.staleAfter)
}

func TestCollector_RefreshKeepsPublishedSnapshotWhileCollectionRuns(t *testing.T) {
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

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	coll := newTestSNMPTopologyCollector()
	coll.deviceCaches[key] = published
	coll.topologyRegistry.register(published)
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
		coll.refreshDeviceTopology(context.Background(), key, dev)
	}()

	<-started

	snapshot, ok := published.snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, snapshot.L2Observations, 1)
	require.Len(t, snapshot.L2Observations[0].FDBEntries, 1)
	require.Len(t, snapshot.L2Observations[0].ARPNDEntries, 1)

	close(release)
	<-done
}

func TestCollector_RefreshFailureKeepsPublishedSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "10.0.0.10",
		Port:        161,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClientConnect(mockHandler, dev)
	mockHandler.EXPECT().Close().Return(nil)

	key := "10.0.0.10:161"
	published := newTopologyCache()
	seedPublishedEndpointSnapshot(published)

	coll := newTestSNMPTopologyCollector()
	coll.deviceCaches[key] = published
	coll.topologyRegistry.register(published)
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return nil, errors.New("collection failed")
		})
	}

	require.False(t, coll.refreshDeviceTopology(context.Background(), key, dev))

	snapshot, ok := published.snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, snapshot.L2Observations, 1)
	require.Len(t, snapshot.L2Observations[0].FDBEntries, 1)
	require.Len(t, snapshot.L2Observations[0].ARPNDEntries, 1)
}

type blockingTopologyCollector struct {
	started chan<- struct{}
	release <-chan struct{}
	result  []*ddsnmp.ProfileMetrics
}

type ddCollectorFunc func() ([]*ddsnmp.ProfileMetrics, error)

func (f ddCollectorFunc) Collect() ([]*ddsnmp.ProfileMetrics, error) { return f() }

func (c *blockingTopologyCollector) Collect() ([]*ddsnmp.ProfileMetrics, error) {
	close(c.started)
	<-c.release
	return c.result, nil
}

func expectTopologyRefreshSNMPClient(mockHandler *snmpmock.MockHandler, dev ddsnmp.DeviceConnectionInfo) {
	expectTopologyRefreshSNMPClientConnect(mockHandler, dev)
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

func expectTopologyRefreshSNMPClientConnect(mockHandler *snmpmock.MockHandler, dev ddsnmp.DeviceConnectionInfo) {
	mockHandler.EXPECT().SetTarget(dev.Hostname)
	mockHandler.EXPECT().SetPort(uint16(dev.Port))
	mockHandler.EXPECT().SetRetries(dev.Retries)
	mockHandler.EXPECT().SetTimeout(time.Duration(dev.Timeout) * time.Second)
	mockHandler.EXPECT().SetMaxOids(dev.MaxOIDs)
	mockHandler.EXPECT().SetMaxRepetitions(uint32(dev.MaxRepetitions))
	mockHandler.EXPECT().SetCommunity(dev.Community)
	mockHandler.EXPECT().SetVersion(gosnmp.Version2c)
	mockHandler.EXPECT().Connect().Return(nil)
}

func seedPublishedEndpointSnapshot(cache *topologyCache) {
	now := time.Now()
	cache.updateTime = now
	cache.lastUpdate = now
	cache.staleAfter = time.Hour
	cache.agentID = "agent-1"
	cache.localDevice = topologymodel.Device{
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

func topologyRegistryHasCache(registry *topologyRegistry, cache *topologyCache) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.caches[cache]
	return ok
}
