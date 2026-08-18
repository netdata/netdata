// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"sort"
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

func TestCollectorResolveDeviceTargetManagementIPs(t *testing.T) {
	t.Run("literal target bypasses resolver", func(t *testing.T) {
		coll := newTestSNMPTopologyCollector()
		coll.resolveTargetIPs = func(context.Context, string) ([]netip.Addr, error) {
			t.Fatal("literal target must not use DNS resolution")
			return nil, nil
		}

		require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.10")}, coll.resolveDeviceTargetManagementIPs(context.Background(), ddsnmp.DeviceConnectionInfo{
			Hostname: "::ffff:192.0.2.10",
		}))
		require.Empty(t, coll.resolveDeviceTargetManagementIPs(context.Background(), ddsnmp.DeviceConnectionInfo{
			Hostname: "127.0.0.1",
		}))
	})

	t.Run("DNS target uses bounded deterministic eligible result", func(t *testing.T) {
		coll := newTestSNMPTopologyCollector()
		calls := 0
		coll.resolveTargetIPs = func(ctx context.Context, host string) ([]netip.Addr, error) {
			calls++
			require.Equal(t, "switch.example", host)
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), topologyTargetLookupMaxTimeout)
			return []netip.Addr{
				netip.MustParseAddr("127.0.0.1"),
				netip.MustParseAddr("198.51.100.20"),
				netip.MustParseAddr("10.0.0.20"),
				netip.MustParseAddr("10.0.0.10"),
			}, nil
		}

		got := coll.resolveDeviceTargetManagementIPs(context.Background(), ddsnmp.DeviceConnectionInfo{
			Hostname: "switch.example",
			Timeout:  5,
		})
		require.Equal(t, []netip.Addr{
			netip.MustParseAddr("10.0.0.10"),
			netip.MustParseAddr("10.0.0.20"),
			netip.MustParseAddr("198.51.100.20"),
		}, got)
		require.Equal(t, 1, calls)
	})

	t.Run("lookup failure falls back without an identity", func(t *testing.T) {
		coll := newTestSNMPTopologyCollector()
		coll.resolveTargetIPs = func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("lookup failed")
		}

		require.Empty(t, coll.resolveDeviceTargetManagementIPs(context.Background(), ddsnmp.DeviceConnectionInfo{
			Hostname: "switch.example",
		}))
	})
}

func TestCollectorRefreshBoundsTargetResolutionAcrossDevices(t *testing.T) {
	const deviceCount = 9

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	coll, store := newTestSNMPTopologyCollectorWithStore()
	mockHandler := snmpmock.NewMockHandler(ctrl)
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
		return []*ddsnmp.Profile{{}}
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}

	for i := range deviceCount {
		dev := ddsnmp.DeviceConnectionInfo{
			Hostname: fmt.Sprintf("switch-%02d.example", i),
			Port:     161,
		}
		registerTestDeviceState(store, dev)
		expectTopologyRefreshSNMPClient(mockHandler, dev)
	}

	var resolverMu sync.Mutex
	active := 0
	maxActive := 0
	calls := 0
	coll.resolveTargetIPs = func(context.Context, string) ([]netip.Addr, error) {
		resolverMu.Lock()
		active++
		calls++
		maxActive = max(maxActive, active)
		resolverMu.Unlock()

		time.Sleep(100 * time.Millisecond)

		resolverMu.Lock()
		active--
		resolverMu.Unlock()
		return []netip.Addr{netip.MustParseAddr("192.0.2.10")}, nil
	}

	started := time.Now()
	stats := coll.refreshTopology(context.Background())
	elapsed := time.Since(started)

	require.Zero(t, stats.errors)
	require.Equal(t, deviceCount, calls)
	require.LessOrEqual(t, maxActive, topologyTargetLookupMaxWorkers)
	require.GreaterOrEqual(t, maxActive, 2)
	require.Less(t, elapsed, 2*time.Second)
}

func TestCollectorRefreshResolvesOnlyDueDNSTargets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fresh := ddsnmp.DeviceConnectionInfo{Hostname: "switch-fresh.example", Port: 161}
	due := ddsnmp.DeviceConnectionInfo{Hostname: "switch-due.example", Port: 161}
	literal := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", Port: 161}

	coll, store := newTestSNMPTopologyCollectorWithStore()
	for _, dev := range []ddsnmp.DeviceConnectionInfo{fresh, due, literal} {
		registerTestDeviceState(store, dev)
	}
	freshKey := fresh.Hostname + ":161"
	freshCache := newTopologyCache()
	coll.deviceCaches[freshKey] = freshCache
	coll.deviceLastCollected[freshKey] = time.Now()
	coll.topologyRegistry.register(freshCache)

	mockHandler := snmpmock.NewMockHandler(ctrl)
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
		return []*ddsnmp.Profile{{}}
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	expectTopologyRefreshSNMPClient(mockHandler, due)
	expectTopologyRefreshSNMPClient(mockHandler, literal)

	var mu sync.Mutex
	var calls []string
	coll.resolveTargetIPs = func(_ context.Context, host string) ([]netip.Addr, error) {
		mu.Lock()
		calls = append(calls, host)
		mu.Unlock()
		return []netip.Addr{netip.MustParseAddr("192.0.2.20")}, nil
	}

	stats := coll.refreshTopology(context.Background())

	require.Zero(t, stats.errors)
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	require.Equal(t, []string{"switch-due.example"}, gotCalls)

	dueSnapshot, ok := coll.deviceCaches[due.Hostname+":161"].snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, dueSnapshot.L2Observations, 1)
	require.Equal(t, "192.0.2.20", dueSnapshot.L2Observations[0].ManagementIP)

	data, ok := snapshotTopologyRegistryForTest(coll.topologyRegistry)
	require.True(t, ok)
	managementIPs := make([]string, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if ip := topologymodel.ActorDetailManagementIP(actor); ip != "" {
			managementIPs = append(managementIPs, ip)
		}
	}
	sort.Strings(managementIPs)
	require.Equal(t, []string{"192.0.2.10", "192.0.2.20"}, managementIPs)
}

func TestCollectorResolveTopologyTargetManagementIPsUsesStableBoundedSharedBudget(t *testing.T) {
	const deviceCount = 17

	coll := newTestSNMPTopologyCollector()
	plans := make([]topologyRefreshDevicePlan, 0, deviceCount)
	for i := deviceCount - 1; i >= 0; i-- {
		host := fmt.Sprintf("switch-%02d.example", i)
		plans = append(plans, topologyRefreshDevicePlan{
			key:    host + ":161",
			device: ddsnmp.DeviceConnectionInfo{Hostname: host, Port: 161},
		})
	}

	var mu sync.Mutex
	active := 0
	maxActive := 0
	attempted := make([]string, 0, topologyTargetLookupMaxWorkers)
	coll.resolveTargetIPs = func(ctx context.Context, host string) ([]netip.Addr, error) {
		mu.Lock()
		active++
		maxActive = max(maxActive, active)
		attempted = append(attempted, host)
		mu.Unlock()

		<-ctx.Done()

		mu.Lock()
		active--
		mu.Unlock()
		return nil, ctx.Err()
	}

	parent := context.Background()
	started := time.Now()
	coll.resolveTopologyTargetManagementIPs(parent, plans, 100*time.Millisecond, topologyTargetLookupMaxWorkers)
	elapsed := time.Since(started)

	mu.Lock()
	sort.Strings(attempted)
	gotAttempted := append([]string(nil), attempted...)
	gotMaxActive := maxActive
	mu.Unlock()

	wantAttempted := make([]string, 0, topologyTargetLookupMaxWorkers)
	for i := range topologyTargetLookupMaxWorkers {
		wantAttempted = append(wantAttempted, fmt.Sprintf("switch-%02d.example", i))
	}
	require.Equal(t, wantAttempted, gotAttempted)
	require.Equal(t, topologyTargetLookupMaxWorkers, gotMaxActive)
	require.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
	require.Less(t, elapsed, time.Second)
	require.NoError(t, parent.Err())
	for _, plan := range plans {
		require.Empty(t, plan.targetManagementIPs)
	}
}

func TestCollectorResolveTopologyTargetManagementIPsAssociatesResultsAndBypassesLiterals(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	plans := []topologyRefreshDevicePlan{
		{key: "switch-b.example:161", device: ddsnmp.DeviceConnectionInfo{Hostname: "switch-b.example"}},
		{key: "literal:161", device: ddsnmp.DeviceConnectionInfo{Hostname: "::ffff:192.0.2.10"}},
		{key: "switch-a.example:161", device: ddsnmp.DeviceConnectionInfo{Hostname: "switch-a.example"}},
	}

	var mu sync.Mutex
	var calls []string
	coll.resolveTargetIPs = func(_ context.Context, host string) ([]netip.Addr, error) {
		mu.Lock()
		calls = append(calls, host)
		mu.Unlock()
		switch host {
		case "switch-a.example":
			return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
		case "switch-b.example":
			return []netip.Addr{netip.MustParseAddr("10.0.0.2")}, nil
		default:
			return nil, fmt.Errorf("unexpected host %q", host)
		}
	}

	coll.resolveTopologyTargetManagementIPs(context.Background(), plans, time.Second, 2)

	mu.Lock()
	sort.Strings(calls)
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	require.Equal(t, []string{"switch-a.example", "switch-b.example"}, gotCalls)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("10.0.0.2")}, plans[0].targetManagementIPs)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.0.2.10")}, plans[1].targetManagementIPs)
	require.Equal(t, []netip.Addr{netip.MustParseAddr("10.0.0.1")}, plans[2].targetManagementIPs)
}

func TestCollectorResolveDeviceTargetManagementIPsHonorsShorterDeviceTimeout(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.resolveTargetIPs = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	parent := context.Background()
	started := time.Now()
	got := coll.resolveDeviceTargetManagementIPs(parent, ddsnmp.DeviceConnectionInfo{
		Hostname: "switch.example",
		Timeout:  1,
	})
	elapsed := time.Since(started)

	require.Empty(t, got)
	require.GreaterOrEqual(t, elapsed, 750*time.Millisecond)
	require.Less(t, elapsed, 2*time.Second)
	require.NoError(t, parent.Err())
}

func TestCollectorResolveTopologyTargetManagementIPsJoinsWorkersOnCancellation(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	plans := make([]topologyRefreshDevicePlan, 9)
	for i := range plans {
		host := fmt.Sprintf("switch-%02d.example", i)
		plans[i] = topologyRefreshDevicePlan{
			key:    host + ":161",
			device: ddsnmp.DeviceConnectionInfo{Hostname: host},
		}
	}

	started := make(chan struct{})
	var mu sync.Mutex
	active := 0
	var startedOnce sync.Once
	coll.resolveTargetIPs = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		mu.Lock()
		active++
		if active == topologyTargetLookupMaxWorkers {
			startedOnce.Do(func() { close(started) })
		}
		mu.Unlock()

		<-ctx.Done()

		mu.Lock()
		active--
		mu.Unlock()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		coll.resolveTopologyTargetManagementIPs(ctx, plans, 5*time.Second, topologyTargetLookupMaxWorkers)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "resolver workers did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "resolver workers were not joined after cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	require.Zero(t, active)
}

func TestCollectorRefreshPrefersResolvedTargetManagementIP(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "switch.example",
		Port:        161,
		Timeout:     5,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll := newTestSNMPTopologyCollector()
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
		return []*ddsnmp.Profile{{}}
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return []*ddsnmp.ProfileMetrics{{TopologyMetrics: []ddsnmp.Metric{{
				TopologyKind: ddsnmp.KindIpIfIndex,
				Tags: map[string]string{
					tagTopoIfIndex: "1",
					tagTopoIPAddr:  "10.0.0.1",
					tagTopoIPMask:  "255.255.255.0",
				},
			}}}}, nil
		})
	}

	key := dev.Hostname + ":161"
	require.True(t, coll.refreshDeviceTopology(context.Background(), key, dev, []netip.Addr{netip.MustParseAddr("192.0.2.50")}))
	snapshot, ok := coll.deviceCaches[key].snapshotEngineObservations()
	require.True(t, ok)
	require.Len(t, snapshot.L2Observations, 1)
	require.Equal(t, "192.0.2.50", snapshot.L2Observations[0].ManagementIP)
}

func TestCollectorRefreshSelectsNextDNSTargetAfterMaskRejection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "switch.example",
		Port:        161,
		Timeout:     5,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	registerTestDeviceState(store, dev)
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
		return []*ddsnmp.Profile{{}}
	}
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return []*ddsnmp.ProfileMetrics{{TopologyMetrics: []ddsnmp.Metric{{
				TopologyKind: ddsnmp.KindIpIfIndex,
				Tags: map[string]string{
					tagTopoIfIndex: "1",
					tagTopoIPAddr:  "192.0.2.0",
					tagTopoIPMask:  "255.255.255.0",
				},
			}}}}, nil
		})
	}

	resolverCalls := 0
	coll.resolveTargetIPs = func(_ context.Context, host string) ([]netip.Addr, error) {
		resolverCalls++
		require.Equal(t, dev.Hostname, host)
		return []netip.Addr{
			netip.MustParseAddr("203.0.113.20"),
			netip.MustParseAddr("198.51.100.10"),
			netip.MustParseAddr("192.0.2.0"),
		}, nil
	}

	stats := coll.refreshTopology(context.Background())

	require.Zero(t, stats.errors)
	require.Equal(t, 1, resolverCalls)
	cache := coll.deviceCaches[dev.Hostname+":161"]
	require.NotNil(t, cache)
	cache.mu.RLock()
	local := cache.localDevice
	trapMethods := maps.Clone(cache.trapMatchMethodByIP)
	storedTargets := append([]netip.Addr(nil), cache.targetManagementIPs...)
	cache.mu.RUnlock()
	require.Equal(t, "198.51.100.10", local.ManagementIP)
	require.Empty(t, storedTargets)
	require.NotContains(t, local.ManagementAddresses, topologymodel.ManagementAddress{
		Address:     "192.0.2.0",
		AddressType: "ipv4",
		Source:      "ip_mib",
	})
	require.NotContains(t, local.ManagementAddresses, topologymodel.ManagementAddress{Address: "198.51.100.10"})
	require.NotContains(t, local.ManagementAddresses, topologymodel.ManagementAddress{Address: "203.0.113.20"})
	require.Equal(t, "management_ip", trapMethods["198.51.100.10"])
	require.Equal(t, "local_interface_ip", trapMethods["192.0.2.0"])
	require.NotContains(t, trapMethods, "203.0.113.20")

	data, ok := snapshotTopologyRegistryForTest(coll.topologyRegistry)
	require.True(t, ok)
	require.Len(t, data.Actors, 1)
	require.Equal(t, "198.51.100.10", topologymodel.ActorDetailManagementIP(data.Actors[0]))
	require.Contains(t, data.Actors[0].Match.IPAddresses, "198.51.100.10")
	require.NotContains(t, data.Actors[0].Match.IPAddresses, "192.0.2.0")
	require.NotContains(t, data.Actors[0].Match.IPAddresses, "203.0.113.20")
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
		coll.refreshDeviceTopology(context.Background(), key, dev, []netip.Addr{netip.MustParseAddr("10.0.0.10")})
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

	require.False(t, coll.refreshDeviceTopology(context.Background(), key, dev, []netip.Addr{netip.MustParseAddr("10.0.0.10")}))

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
