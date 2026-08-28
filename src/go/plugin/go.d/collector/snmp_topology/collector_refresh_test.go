// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
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

	entries := coll.getRegisteredDevices()
	require.Len(t, entries, 1)
	require.Equal(t, "192.0.2.10", entries[0].Info.Hostname)

	entries[0].Info.ManualProfiles[0] = "changed"
	entries[0].Info.VnodeLabels["site"] = "changed"

	again := coll.getRegisteredDevices()
	require.Len(t, again, 1)
	require.Equal(t, []string{"profile-a"}, again[0].Info.ManualProfiles)
	require.Equal(t, "lab", again[0].Info.VnodeLabels["site"])
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

func TestCollectorRefreshPrunesUnregisteredDeviceStateFromPublishedGeneration(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	cache := newTopologyBuilder()
	seedPublishedEndpointSnapshot(cache)
	const registrationID ddsnmp.DeviceRegistrationID = 1
	generation := freezeTestTopologyBuilder(registrationID, cache)
	coll.deviceStates[registrationID] = deviceRefreshState{generation: generation}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, time.Now(), coll.deviceStates))

	coll.refreshTopology(context.Background())

	require.Empty(t, coll.deviceStates)
	require.Zero(t, coll.topologyRegistry.acquireGeneration().deviceCount())
	diagnostics := coll.acquireTopologyDiagnostics()
	require.NotNil(t, diagnostics.topology)
	require.Empty(t, diagnostics.topology.devices)
	require.Len(t, diagnostics.topology.removed, 1)
	require.Equal(t, registrationID, diagnostics.topology.removed[0].registrationID)
	require.True(t, diagnostics.topology.removed[0].hasRetainedSuccess)
}

func TestCollectorRefreshKeepsDistinctRegistrationKeysForSharedEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	first := snmpmock.NewMockHandler(ctrl)
	second := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(first, dev)
	expectTopologyRefreshSNMPClient(second, dev)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", dev)
	store.Register("job-b", dev)
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	clients := []gosnmp.Handler{first, second}
	coll.newSnmpClient = func() gosnmp.Handler {
		client := clients[0]
		clients = clients[1:]
		return client
	}

	stats := coll.refreshTopology(context.Background())

	entries := store.Entries()
	require.Len(t, entries, 2)
	firstRegistrationID := entries[0].RegistrationID
	secondRegistrationID := entries[1].RegistrationID
	require.Zero(t, stats.errors)
	require.Equal(t, 2, stats.registeredDevices)
	require.Equal(t, 2, stats.cachedDevices)
	require.Contains(t, coll.deviceStates, firstRegistrationID)
	require.Contains(t, coll.deviceStates, secondRegistrationID)
	require.NotSame(t, coll.deviceStates[firstRegistrationID].generation, coll.deviceStates[secondRegistrationID].generation)
	require.Equal(t, 2, coll.topologyRegistry.acquireGeneration().deviceCount())
}

func TestCollectorRefreshTreatsReregisteredOwnerAsNewDevice(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }

	base := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	coll.now = func() time.Time { return base }
	store.Register("owner-a", dev)
	oldEntry := store.Entries()[0]
	oldBuilder := newTopologyBuilder()
	seedPublishedEndpointSnapshot(oldBuilder)
	oldGeneration := freezeTestTopologyBuilder(oldEntry.RegistrationID, oldBuilder)
	coll.deviceStates[oldEntry.RegistrationID] = deviceRefreshState{
		generation: oldGeneration,
		nextRetry:  base.Add(time.Hour),
		outcome:    deviceRefreshOutcomeSuccess,
	}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, base, coll.deviceStates))

	store.Unregister("owner-a")
	store.Register("owner-a", dev)
	newEntry := store.Entries()[0]

	stats := coll.refreshTopology(context.Background())

	require.Zero(t, stats.errors)
	require.NotEqual(t, oldEntry.RegistrationID, newEntry.RegistrationID)
	require.NotContains(t, coll.deviceStates, oldEntry.RegistrationID)
	require.Equal(t, deviceRefreshOutcomeSuccess, coll.deviceStates[newEntry.RegistrationID].outcome)
	require.NotSame(t, oldGeneration, coll.deviceStates[newEntry.RegistrationID].generation)
}

func TestCollectorRefreshPublishesOnlyAfterCompleteMultiDeviceSweep(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	devA := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
	devB := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.20", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
	clientA := snmpmock.NewMockHandler(ctrl)
	clientB := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(clientA, devA)
	expectTopologyRefreshSNMPClient(clientB, devB)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", devA)
	store.Register("job-b", devB)
	entries := store.Entries()
	require.Len(t, entries, 2)
	registrationA := entries[0].RegistrationID
	registrationB := entries[1].RegistrationID
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	clients := []gosnmp.Handler{clientA, clientB}
	coll.newSnmpClient = func() gosnmp.Handler {
		client := clients[0]
		clients = clients[1:]
		return client
	}

	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	collectorCalls := 0
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		collectorCalls++
		if collectorCalls == 1 {
			return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
		}
		return &blockingTopologyCollector{started: secondStarted, release: releaseSecond}
	}

	previous := testTopologyGenerationVector(7, time.Now(), "previous")
	previous.devices[0].registrationID = registrationA
	previous.devices[1].registrationID = registrationB
	coll.deviceStates = map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
		registrationA: {generation: previous.devices[0]},
		registrationB: {generation: previous.devices[1]},
	}
	coll.generationSequence = previous.sequence
	coll.topologyRegistry.publishGeneration(previous)

	done := make(chan refreshStats, 1)
	go func() { done <- coll.refreshTopology(context.Background()) }()

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second device refresh did not start")
	}
	require.Same(t, previous, coll.topologyRegistry.acquireGeneration())
	require.Same(t, previous.devices[0], coll.deviceStates[registrationA].generation)
	require.Same(t, previous.devices[1], coll.deviceStates[registrationB].generation)

	close(releaseSecond)
	stats := <-done
	require.Zero(t, stats.errors)
	published := coll.topologyRegistry.acquireGeneration()
	require.NotSame(t, previous, published)
	require.EqualValues(t, 8, published.sequence)
	require.Len(t, published.devices, 2)
	require.Same(t, coll.deviceStates[registrationA].generation, published.devices[0])
	require.Same(t, coll.deviceStates[registrationB].generation, published.devices[1])
}

func TestCollectorRefreshActivatesFreshnessAtAtomicPublication(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	devA := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.10", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
	devB := ddsnmp.DeviceConnectionInfo{Hostname: "192.0.2.20", Port: 161, SNMPVersion: gosnmp.Version2c.String()}
	clientA := snmpmock.NewMockHandler(ctrl)
	clientB := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(clientA, devA)
	expectTopologyRefreshSNMPClient(clientB, devB)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", devA)
	store.Register("job-b", devB)
	entries := store.Entries()
	require.Len(t, entries, 2)
	registrationA := entries[0].RegistrationID
	registrationB := entries[1].RegistrationID
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	clients := []gosnmp.Handler{clientA, clientB}
	coll.newSnmpClient = func() gosnmp.Handler {
		client := clients[0]
		clients = clients[1:]
		return client
	}

	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	collectorCalls := 0
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		collectorCalls++
		if collectorCalls == 1 {
			return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
		}
		return &blockingTopologyCollector{started: secondStarted, release: releaseSecond}
	}

	base := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(base.UnixNano())
	coll.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }

	previous := testTopologyGenerationVector(1, base, "previous")
	previous.devices[0].registrationID = registrationA
	previous.devices[1].registrationID = registrationB
	coll.deviceStates = map[ddsnmp.DeviceRegistrationID]deviceRefreshState{
		registrationA: {generation: previous.devices[0]},
		registrationB: {generation: previous.devices[1]},
	}
	coll.generationSequence = previous.sequence
	coll.topologyRegistry.publishGeneration(previous)

	done := make(chan refreshStats, 1)
	go func() { done <- coll.refreshTopology(context.Background()) }()

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second device refresh did not start")
	}
	clock.Store(base.Add(2 * time.Hour).UnixNano())
	visibleDuringSweep := topologyObservationSnapshots(coll.topologyRegistry.acquireGeneration())

	close(releaseSecond)
	stats := <-done
	require.Zero(t, stats.errors)
	require.Len(t, visibleDuringSweep, 2,
		"published membership must remain fixed while the next sweep is in flight")
	published := coll.topologyRegistry.acquireGeneration()
	require.Len(t, topologyObservationSnapshots(published), 2,
		"every successful result must be fresh when its sweep is first published")
	require.Equal(t, base, coll.deviceStates[registrationA].generation.collectedAt,
		"activation must preserve the exact device acquisition time")
	require.Equal(t,
		published.publishedAt.Add(coll.refreshEvery()+2*coll.deviceCheckEvery()),
		coll.deviceStates[registrationA].generation.expiresAt,
		"the freshness window must begin at atomic publication",
	)
}

func TestCollectorDeviceRefreshWarningsAreLimitedPerRegistrationAndFailureClass(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	first := snmpmock.NewMockHandler(ctrl)
	second := snmpmock.NewMockHandler(ctrl)
	collectionFailure := snmpmock.NewMockHandler(ctrl)
	otherRegistration := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClientConnectError(first, dev, errors.New("first failure"))
	expectTopologyRefreshSNMPClientConnectError(second, dev, errors.New("second failure"))
	expectTopologyRefreshSNMPClientConnect(collectionFailure, dev)
	collectionFailure.EXPECT().Close().Return(nil)
	expectTopologyRefreshSNMPClientConnectError(otherRegistration, dev, errors.New("other registration failure"))

	coll := newTestSNMPTopologyCollector()
	var logs bytes.Buffer
	coll.Logger = logger.NewWithWriter(&logs)
	clients := []gosnmp.Handler{first, second, collectionFailure, otherRegistration}
	coll.newSnmpClient = func() gosnmp.Handler {
		client := clients[0]
		clients = clients[1:]
		return client
	}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return nil, errors.New("collection failure")
		})
	}

	for range 2 {
		snapshot, outcome := coll.refreshDeviceTopology(context.Background(), 1, dev, nil, coll.currentTopologySemanticLimits())
		require.Nil(t, snapshot)
		require.Equal(t, deviceRefreshOutcomeFailed, outcome)
	}
	snapshot, outcome := coll.refreshDeviceTopology(context.Background(), 1, dev, nil, coll.currentTopologySemanticLimits())
	require.Nil(t, snapshot)
	require.Equal(t, deviceRefreshOutcomeFailed, outcome)
	snapshot, outcome = coll.refreshDeviceTopology(context.Background(), 2, dev, nil, coll.currentTopologySemanticLimits())
	require.Nil(t, snapshot)
	require.Equal(t, deviceRefreshOutcomeFailed, outcome)

	require.Equal(t, 2, strings.Count(logs.String(), "failed to connect"), logs.String())
	require.Equal(t, 1, strings.Count(logs.String(), "topology collection failed"), logs.String())
	require.Contains(t, logs.String(), "first failure")
	require.NotContains(t, logs.String(), "second failure")
	require.Contains(t, logs.String(), "collection failure")
	require.Contains(t, logs.String(), "other registration failure")
}

func TestCollectorRefreshFailureUsesExponentialRetryAndPreservesLastSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	firstFailure := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClientConnectError(firstFailure, dev, errors.New("unreachable"))
	success := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(success, dev)
	secondFailure := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClientConnectError(secondFailure, dev, errors.New("unreachable again"))

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", dev)
	registrationID := store.Entries()[0].RegistrationID
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	clients := []gosnmp.Handler{firstFailure, success, secondFailure}
	coll.newSnmpClient = func() gosnmp.Handler {
		client := clients[0]
		clients = clients[1:]
		return client
	}

	base := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	now := base
	coll.now = func() time.Time { return now }

	firstStats := coll.refreshTopology(context.Background())
	firstState := coll.deviceStates[registrationID]
	require.Equal(t, 1, firstStats.errors)
	require.Equal(t, deviceRefreshOutcomeFailed, firstState.outcome)
	require.Equal(t, base, firstState.lastAttempt)
	require.True(t, firstState.lastSuccess.IsZero())
	require.Equal(t, base.Add(defaultDeviceCheckEvery), firstState.nextRetry)
	require.EqualValues(t, 1, firstState.consecutiveFailures)
	require.Nil(t, firstState.generation)
	firstCut := coll.acquireTopologyDiagnostics().topology
	require.NotNil(t, firstCut)
	require.Len(t, firstCut.devices, 1)
	require.True(t, firstCut.devices[0].selected)
	require.Equal(t, deviceRefreshOutcomeFailed, firstCut.devices[0].outcome)
	require.False(t, firstCut.devices[0].hasRetainedSuccess)

	now = base.Add(30 * time.Second)
	skippedStats := coll.refreshTopology(context.Background())
	require.Zero(t, skippedStats.errors)
	require.Len(t, clients, 2, "refresh before nextRetry must not create a client")
	skippedCut := coll.acquireTopologyDiagnostics().topology
	require.NotNil(t, skippedCut)
	require.False(t, skippedCut.devices[0].selected)
	require.Equal(t, deviceRefreshOutcomeFailed, skippedCut.devices[0].outcome)

	now = base.Add(defaultDeviceCheckEvery)
	successStats := coll.refreshTopology(context.Background())
	successState := coll.deviceStates[registrationID]
	require.Zero(t, successStats.errors)
	require.Equal(t, deviceRefreshOutcomeSuccess, successState.outcome)
	require.Equal(t, now, successState.lastSuccess)
	require.Equal(t, now.Add(defaultRefreshEvery), successState.nextRetry)
	require.Zero(t, successState.consecutiveFailures)
	require.NotNil(t, successState.generation)
	require.Equal(t, diagnosticCaptureAvailable, successState.generation.semantic.state)
	require.NotNil(t, successState.generation.semantic.evidence)
	require.Equal(t, registrationID, successState.generation.evidenceRef.registrationID)
	require.NotZero(t, successState.generation.evidenceRef.generation)
	replayed, err := replayTopologySemanticEvidence(successState.generation.semantic.evidence)
	require.NoError(t, err)
	require.Equal(t, successState.generation.observation, replayed.observation)
	lastSuccessGeneration := successState.generation

	now = successState.nextRetry
	failedStats := coll.refreshTopology(context.Background())
	failedState := coll.deviceStates[registrationID]
	require.Equal(t, 1, failedStats.errors)
	require.Equal(t, deviceRefreshOutcomeFailed, failedState.outcome)
	require.Equal(t, successState.lastSuccess, failedState.lastSuccess)
	require.Same(t, lastSuccessGeneration, failedState.generation)
	require.Equal(t, now.Add(defaultDeviceCheckEvery), failedState.nextRetry)
	require.EqualValues(t, 1, failedState.consecutiveFailures)
	require.False(t, failedState.generation.freshAt(failedState.generation.expiresAt.Add(time.Nanosecond)),
		"failure retention must not extend the last successful collection's display freshness")
	failedCut := coll.acquireTopologyDiagnostics().topology
	require.NotNil(t, failedCut)
	require.True(t, failedCut.devices[0].selected)
	require.Equal(t, deviceRefreshOutcomeFailed, failedCut.devices[0].outcome)
	require.True(t, failedCut.devices[0].hasRetainedSuccess)
	require.Equal(t, lastSuccessGeneration.evidenceRef, failedCut.devices[0].retainedSuccess)
}

func TestCollectorSuccessfulRefreshSurvivesSemanticCaptureLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClient(mockHandler, dev)

	coll := newTestSNMPTopologyCollector()
	coll.semanticLimits = topologySemanticLimits{maxRecords: 1, maxLogicalBytes: 1 << 20}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return []*ddsnmp.ProfileMetrics{{TopologyMetrics: []ddsnmp.Metric{{
				TopologyKind: ddsnmp.KindIfName,
				Tags:         map[string]string{tagTopoIfIndex: "7", tagTopoIfName: "Gi1/0/7"},
			}}}}, nil
		})
	}

	snapshot, outcome := coll.refreshDeviceTopology(context.Background(), 1, dev, nil, coll.currentTopologySemanticLimits())
	require.Equal(t, deviceRefreshOutcomeSuccess, outcome)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.hasObservation)
	require.Equal(t, diagnosticCaptureLimitExceeded, snapshot.semantic.state)
	require.Equal(t, diagnosticCaptureReasonRecordLimit, snapshot.semantic.reason)
	require.Nil(t, snapshot.semantic.evidence)
}

func TestCollectorRefreshWithoutProfilesRetainsLastSuccessAndUsesNormalInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
	}
	mockHandler := snmpmock.NewMockHandler(ctrl)
	expectTopologyRefreshSNMPClientConnect(mockHandler, dev)
	mockHandler.EXPECT().Close().Return(nil)

	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.Register("job-a", dev)
	registrationID := store.Entries()[0].RegistrationID
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return nil }
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		t.Fatal("a device without topology profiles must not create a collector")
		return nil
	}

	base := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	previousSuccess := base.Add(-time.Hour)
	builder := newTopologyBuilder()
	seedPublishedEndpointSnapshot(builder)
	previous := freezeTestTopologyBuilder(registrationID, builder)
	coll.deviceStates[registrationID] = deviceRefreshState{
		generation:          previous,
		lastSuccess:         previousSuccess,
		consecutiveFailures: 3,
	}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, base, coll.deviceStates))
	coll.now = func() time.Time { return base }

	stats := coll.refreshTopology(context.Background())
	state := coll.deviceStates[registrationID]

	require.Zero(t, stats.errors)
	require.Equal(t, deviceRefreshOutcomeNoProfiles, state.outcome)
	require.Equal(t, base, state.lastAttempt)
	require.Equal(t, previousSuccess, state.lastSuccess)
	require.Equal(t, base.Add(defaultRefreshEvery), state.nextRetry)
	require.Zero(t, state.consecutiveFailures)
	require.Same(t, previous, state.generation)
	require.Same(t, previous, coll.topologyRegistry.acquireGeneration().devices[0])
	cut := coll.acquireTopologyDiagnostics().topology
	require.NotNil(t, cut)
	require.Len(t, cut.devices, 1)
	require.True(t, cut.devices[0].selected)
	require.Equal(t, deviceRefreshOutcomeNoProfiles, cut.devices[0].outcome)
	require.True(t, cut.devices[0].hasRetainedSuccess)
	require.Equal(t, previous.evidenceRef, cut.devices[0].retainedSuccess)
}

func TestFailedRefreshRetryDelayCapsAtRefreshInterval(t *testing.T) {
	require.Equal(t, time.Minute, failedRefreshRetryDelay(time.Minute, 30*time.Minute, 1))
	require.Equal(t, 2*time.Minute, failedRefreshRetryDelay(time.Minute, 30*time.Minute, 2))
	require.Equal(t, 16*time.Minute, failedRefreshRetryDelay(time.Minute, 30*time.Minute, 5))
	require.Equal(t, 30*time.Minute, failedRefreshRetryDelay(time.Minute, 30*time.Minute, 6))
	require.Equal(t, 30*time.Minute, failedRefreshRetryDelay(time.Minute, 30*time.Minute, 100))
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
	builder := newTopologyBuilder()
	seedPublishedEndpointSnapshot(builder)
	registrationID := store.Entries()[0].RegistrationID
	previousDevice := freezeTestTopologyBuilder(registrationID, builder)
	coll.deviceStates[previousDevice.registrationID] = deviceRefreshState{generation: previousDevice}
	previous := newTopologyGeneration(1, time.Now(), coll.deviceStates)
	coll.generationSequence = previous.sequence
	coll.topologyRegistry.publishGeneration(previous)

	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })
	require.NotPanics(t, func() { coll.refreshTopologyRecovering(context.Background()) })
	require.Same(t, previous, coll.topologyRegistry.acquireGeneration())
	require.Same(t, previousDevice, coll.deviceStates[previousDevice.registrationID].generation)
	require.EqualValues(t, 1, coll.generationSequence)
	aborted := coll.acquireTopologyDiagnostics().lastAborted
	require.NotNil(t, aborted)
	require.Equal(t, topologyDiagnosticAbortPanic, aborted.reason)
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
	diagnostics := coll.acquireTopologyDiagnostics()
	require.Nil(t, diagnostics.topology)
	require.NotNil(t, diagnostics.lastAborted)
	require.Equal(t, topologyDiagnosticAbortCanceled, diagnostics.lastAborted.reason)
	require.Equal(t, topologyDiagnosticSweepPhaseDeviceRefresh, diagnostics.lastAborted.phase)
	require.True(t, diagnostics.lastAborted.hasActiveRegistration)
	require.Equal(t, store.Entries()[0].RegistrationID, diagnostics.lastAborted.activeRegistrationID)
	require.Equal(t, 1, diagnostics.lastAborted.registrationCount)
	require.Equal(t, 1, diagnostics.lastAborted.selectedCount)
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
	var freshRegistrationID, dueRegistrationID ddsnmp.DeviceRegistrationID
	for _, entry := range store.Entries() {
		switch entry.Info.Hostname {
		case fresh.Hostname:
			freshRegistrationID = entry.RegistrationID
		case due.Hostname:
			dueRegistrationID = entry.RegistrationID
		}
	}
	require.NotZero(t, freshRegistrationID)
	require.NotZero(t, dueRegistrationID)
	coll.deviceStates[freshRegistrationID] = deviceRefreshState{
		lastAttempt: time.Now(),
		lastSuccess: time.Now(),
		nextRetry:   time.Now().Add(time.Hour),
		outcome:     deviceRefreshOutcomeSuccess,
	}

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

	dueGeneration := coll.deviceStates[dueRegistrationID].generation
	require.NotNil(t, dueGeneration)
	dueSnapshot := dueGeneration.observation
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
			registrationID: ddsnmp.DeviceRegistrationID(i + 1),
			device:         ddsnmp.DeviceConnectionInfo{Hostname: host, Port: 161},
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
		{registrationID: 2, device: ddsnmp.DeviceConnectionInfo{Hostname: "switch-b.example"}},
		{registrationID: 3, device: ddsnmp.DeviceConnectionInfo{Hostname: "::ffff:192.0.2.10"}},
		{registrationID: 1, device: ddsnmp.DeviceConnectionInfo{Hostname: "switch-a.example"}},
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
			registrationID: ddsnmp.DeviceRegistrationID(i + 1),
			device:         ddsnmp.DeviceConnectionInfo{Hostname: host},
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

	generation, outcome := coll.refreshDeviceTopology(context.Background(), 1, dev, []netip.Addr{netip.MustParseAddr("192.0.2.50")}, coll.currentTopologySemanticLimits())
	require.Equal(t, deviceRefreshOutcomeSuccess, outcome)
	require.NotNil(t, generation)
	snapshot := generation.observation
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
	entries := store.Entries()
	require.Len(t, entries, 1)
	generation := coll.deviceStates[entries[0].RegistrationID].generation
	require.NotNil(t, generation)
	local := generation.observation.LocalDevice
	trapMethods := generation.trap.matchMethodByIP
	require.Equal(t, "198.51.100.10", local.ManagementIP)
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

	const registrationID ddsnmp.DeviceRegistrationID = 1
	published := newTopologyBuilder()
	seedPublishedEndpointSnapshot(published)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	coll := newTestSNMPTopologyCollector()
	publishedGeneration := freezeTestTopologyBuilder(registrationID, published)
	coll.deviceStates[registrationID] = deviceRefreshState{generation: publishedGeneration}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, time.Now(), coll.deviceStates))
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
		coll.refreshDeviceTopology(context.Background(), registrationID, dev, []netip.Addr{netip.MustParseAddr("10.0.0.10")}, coll.currentTopologySemanticLimits())
	}()

	<-started

	visible := coll.topologyRegistry.acquireGeneration()
	require.Same(t, publishedGeneration, visible.devices[0])
	snapshot := visible.devices[0].observation
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

	const registrationID ddsnmp.DeviceRegistrationID = 1
	published := newTopologyBuilder()
	seedPublishedEndpointSnapshot(published)

	coll := newTestSNMPTopologyCollector()
	publishedGeneration := freezeTestTopologyBuilder(registrationID, published)
	coll.deviceStates[registrationID] = deviceRefreshState{generation: publishedGeneration}
	coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, time.Now(), coll.deviceStates))
	coll.newSnmpClient = func() gosnmp.Handler { return mockHandler }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			return nil, errors.New("collection failed")
		})
	}

	generation, outcome := coll.refreshDeviceTopology(context.Background(), registrationID, dev, []netip.Addr{netip.MustParseAddr("10.0.0.10")}, coll.currentTopologySemanticLimits())
	require.Nil(t, generation)
	require.Equal(t, deviceRefreshOutcomeFailed, outcome)

	visible := coll.topologyRegistry.acquireGeneration()
	require.Same(t, publishedGeneration, visible.devices[0])
	snapshot := visible.devices[0].observation
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

func expectTopologyRefreshSNMPClientConnectError(mockHandler *snmpmock.MockHandler, dev ddsnmp.DeviceConnectionInfo, err error) {
	mockHandler.EXPECT().SetTarget(dev.Hostname)
	mockHandler.EXPECT().SetPort(uint16(dev.Port))
	mockHandler.EXPECT().SetRetries(dev.Retries)
	mockHandler.EXPECT().SetTimeout(time.Duration(dev.Timeout) * time.Second)
	mockHandler.EXPECT().SetMaxOids(dev.MaxOIDs)
	mockHandler.EXPECT().SetMaxRepetitions(uint32(dev.MaxRepetitions))
	mockHandler.EXPECT().SetCommunity(dev.Community)
	mockHandler.EXPECT().SetVersion(gosnmp.Version2c)
	mockHandler.EXPECT().Connect().Return(err)
}

func seedPublishedEndpointSnapshot(cache *topologyBuilder) {
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

func BenchmarkCollectorRefreshNoDueDevices(b *testing.B) {
	for _, deviceCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("devices=%d", deviceCount), func(b *testing.B) {
			coll, store := newTestSNMPTopologyCollectorWithStore()
			now := time.Now()
			coll.now = func() time.Time { return now }
			for i := range deviceCount {
				ownerKey := fmt.Sprintf("job-%06d", i)
				store.Register(ownerKey, ddsnmp.DeviceConnectionInfo{
					Hostname:       fmt.Sprintf("192.0.%d.%d", i/254, i%254+1),
					Port:           161,
					ManualProfiles: []string{"generic-device"},
					VnodeLabels:    map[string]string{"site": "benchmark"},
				})
			}
			for _, entry := range store.Entries() {
				registrationID := entry.RegistrationID
				coll.deviceStates[registrationID] = deviceRefreshState{
					generation:  &topologyDeviceGeneration{registrationID: registrationID, collectedAt: now, expiresAt: now.Add(time.Hour)},
					lastSuccess: now,
					nextRetry:   now.Add(time.Hour),
					outcome:     deviceRefreshOutcomeSuccess,
				}
			}
			coll.topologyRegistry.publishGeneration(newTopologyGeneration(1, now, coll.deviceStates))

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				stats := coll.refreshTopology(context.Background())
				if stats.registeredDevices != deviceCount || stats.errors != 0 {
					b.Fatalf("refresh stats: registered=%d errors=%d", stats.registeredDevices, stats.errors)
				}
			}
		})
	}
}
