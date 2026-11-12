// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/stretchr/testify/require"
)

func TestCollectorRefreshReclassificationWithinRegistration(t *testing.T) {
	for _, oldID := range []string{"1.3.6.1.4.1.9.1.1", ""} {
		for _, outcome := range []string{"success", "connect failure", "no profiles"} {
			t.Run(outcome+"/old-id="+oldID, func(t *testing.T) {
				coll, store := newTestSNMPTopologyCollectorWithStore()
				now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
				coll.now = func() time.Time { return now }
				dev := ddsnmp.DeviceConnectionInfo{
					Hostname:       "192.0.2.10",
					Port:           161,
					SNMPVersion:    gosnmp.Version2c.String(),
					SysObjectID:    oldID,
					ManualProfiles: []string{"profile-a"},
					SysName:        "old-device",
				}
				store.Register("job-a", dev)
				registrationID := store.Entries()[0].RegistrationID
				calls := 0
				fail := false
				noProfiles := false
				coll.newSnmpClient = func() gosnmp.Handler {
					calls++
					client := snmpmock.NewMockHandler(gomock.NewController(t))
					switch {
					case fail:
						expectTopologyRefreshSNMPClientConnectError(client, dev, errors.New("unreachable"))
					case noProfiles:
						expectTopologyRefreshSNMPClientConnect(client, dev)
						client.EXPECT().Close().Return(nil)
					default:
						expectTopologyRefreshSNMPClient(client, dev)
					}
					return client
				}
				coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
					if noProfiles {
						return nil
					}
					return []*ddsnmp.Profile{{}}
				}
				var selectedIDs []string
				coll.newDdSnmpColl = func(config ddsnmpcollector.Config) ddCollector {
					selectedIDs = append(selectedIDs, config.SysObjectID)
					return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
				}
				require.Zero(t, coll.refreshTopology(context.Background()).errors)
				first := coll.topologyRegistry.acquireGeneration()
				require.Len(t, first.renderableDevices, 1)
				require.Equal(t, "old-device", first.devices[0].trap.deviceHostname)

				// Normal SNMP replacement withdraws readiness and re-registers synchronously,
				// preserving the accepted writer's registration lifetime.
				store.WithdrawDevice("job-a")
				dev.SysObjectID = "1.3.6.1.4.1.2636.1.1.1"
				dev.SysName = "new-device"
				store.Register("job-a", dev)
				require.Equal(t, registrationID, store.Entries()[0].RegistrationID)
				now = now.Add(time.Minute)
				fail = outcome == "connect failure"
				noProfiles = outcome == "no profiles"
				stats := coll.refreshTopology(context.Background())
				require.Equal(t, 2, calls, "classification change must bypass the old generation's refresh deadline")
				require.EqualValues(t, 2, coll.deviceStates[registrationID].attemptOrdinal)
				cut := coll.acquireTopologyDiagnostics().Topology
				require.Len(t, cut.Devices, 1)
				require.Equal(t, dev.SysObjectID, cut.Devices[0].LatestAttempt.Evidence.Device.SysObjectID)
				require.Equal(t, "old-device", first.devices[0].trap.deviceHostname, "publication must not mutate prior readers")
				if outcome == "success" {
					require.Zero(t, stats.errors)
					require.Len(t, coll.topologyRegistry.acquireGeneration().renderableDevices, 1)
					require.Equal(t, "new-device", coll.topologyRegistry.acquireGeneration().devices[0].trap.deviceHostname)
					require.Equal(t, []string{oldID, dev.SysObjectID}, selectedIDs)
					return
				}
				require.Empty(t, coll.topologyRegistry.acquireGeneration().devices, "incompatible success must not survive a failed or unsupported replacement")
				require.False(t, cut.Devices[0].HasRetainedSuccess)
				require.True(t, coll.deviceStates[registrationID].lastSuccess.IsZero())
				retryAt := now.Add(defaultRefreshEvery)
				if fail {
					require.Equal(t, 1, stats.errors)
					retryAt = now.Add(defaultDeviceCheckEvery)
				}
				require.Equal(t, retryAt, coll.deviceStates[registrationID].nextRetry)
				now = now.Add(time.Second)
				coll.refreshTopology(context.Background())
				require.Equal(t, 2, calls, "failed replacement must use its own retry deadline")
				fail, noProfiles = false, false
				now = retryAt
				require.Zero(t, coll.refreshTopology(context.Background()).errors)
				require.Equal(t, 3, calls)
				current := coll.topologyRegistry.acquireGeneration()
				require.Len(t, current.renderableDevices, 1)
				require.Equal(t, "new-device", current.devices[0].trap.deviceHostname)
				require.Equal(t, topologydiag.RefreshOutcomeSuccess, coll.deviceStates[registrationID].outcome)
			})
		}
	}

}

func TestCollectorRefreshChangedDeviceInformationReacquires(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	coll.now = func() time.Time { return now }
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
		SysObjectID: "1.3.6.1.4.1.9.1.1",
		SysName:     "old-device",
		SysContact:  "old-contact",
		SysLocation: "old-location",
		VnodeLabels: map[string]string{"name": "old-device", "location": "old-location", "site": "configured"},
	}
	store.Register("job-a", dev)
	calls := 0
	fail := false
	coll.newSnmpClient = func() gosnmp.Handler {
		calls++
		client := snmpmock.NewMockHandler(gomock.NewController(t))
		if fail {
			expectTopologyRefreshSNMPClientConnectError(client, dev, errors.New("unreachable"))
		} else {
			expectTopologyRefreshSNMPClient(client, dev)
		}
		return client
	}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	first := coll.topologyRegistry.acquireGeneration()
	id := store.Entries()[0].RegistrationID

	// Unchanged device information waits for the refresh deadline.
	now = now.Add(time.Minute)
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 1, calls)

	dev.SysName, dev.SysContact, dev.SysLocation = "new-device", "new-contact", "new-location"
	dev.VnodeLabels["name"], dev.VnodeLabels["location"] = dev.SysName, dev.SysLocation
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 2, calls, "changed device information must bypass the refresh deadline")
	require.EqualValues(t, 2, coll.deviceStates[id].attemptOrdinal)
	require.Equal(t, now.Add(defaultRefreshEvery), coll.deviceStates[id].nextRetry)
	second := coll.topologyRegistry.acquireGeneration()
	require.Equal(t, "new-device", second.devices[0].trap.deviceHostname)
	require.Equal(t, "new-device", second.devices[0].acquisition.Evidence.Device.SysName)
	enrichment := coll.topologyRegistry.trapEnrichmentForSource(dev.Hostname, "")
	require.NotNil(t, enrichment)
	require.Equal(t, "new-device", enrichment.DeviceHostname)
	local := second.observationSnapshots()[0].LocalDevice
	require.Equal(t, "new-device", local.SysName)
	require.Equal(t, "new-contact", local.SysContact)
	require.Equal(t, "new-location", local.SysLocation)
	require.Equal(t, "configured", local.Labels["site"])
	require.Equal(t, "new-location", local.Labels["location"])
	require.Equal(t, "old-device", first.observationSnapshots()[0].LocalDevice.SysName, "prior readers keep their view")

	// A failed acquisition keeps the retained topology and its own retry deadline.
	dev.SysName = "renamed-again"
	dev.VnodeLabels["name"] = dev.SysName
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	fail = true
	require.Equal(t, 1, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 3, calls)
	require.Equal(t, "new-device", coll.topologyRegistry.acquireGeneration().devices[0].trap.deviceHostname)
	require.True(t, coll.acquireTopologyDiagnostics().Topology.Devices[0].HasRetainedSuccess)
	retryAt := now.Add(defaultDeviceCheckEvery)
	require.Equal(t, retryAt, coll.deviceStates[id].nextRetry)
	now = now.Add(time.Second)
	coll.refreshTopology(context.Background())
	require.Equal(t, 3, calls, "the change must not retry before the failure deadline")

	fail = false
	now = retryAt
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 4, calls)
	require.Equal(t, "renamed-again", coll.topologyRegistry.acquireGeneration().devices[0].trap.deviceHostname)

	// A change that arrives during a failure backoff is acquired without waiting for it.
	fail = true
	dev.SysLocation = "moved"
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	require.Equal(t, 1, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 5, calls)
	require.True(t, now.Before(coll.deviceStates[id].nextRetry))
	fail = false
	dev.SysLocation = "moved-again"
	store.Register("job-a", dev)
	now = now.Add(time.Second)
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 6, calls)
	local = coll.topologyRegistry.acquireGeneration().observationSnapshots()[0].LocalDevice
	require.Equal(t, "moved-again", local.SysLocation)
}

func TestCollectorRefreshReclassificationCancellationKeepsCommittedCut(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	coll.now = func() time.Time { return now }
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: gosnmp.Version2c.String(),
		SysObjectID: "1.3.6.1.4.1.9.1.1",
		SysName:     "old-device",
	}
	store.Register("job-a", dev)
	calls := 0
	coll.newSnmpClient = func() gosnmp.Handler {
		calls++
		client := snmpmock.NewMockHandler(gomock.NewController(t))
		if calls == 2 {
			expectTopologyRefreshSNMPClientConnect(client, dev)
			client.EXPECT().Close().Return(nil).AnyTimes()
		} else {
			expectTopologyRefreshSNMPClient(client, dev)
		}
		return client
	}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
			if calls == 2 {
				cancel()
			}
			return nil, nil
		})
	}
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	first := coll.topologyRegistry.acquireGeneration()
	id := store.Entries()[0].RegistrationID
	state := coll.deviceStates[id]
	dev.SysObjectID, dev.SysName = "1.3.6.1.4.1.2636.1.1.1", "new-device"
	store.WithdrawDevice("job-a")
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	coll.refreshTopology(ctx)
	require.Equal(t, 2, calls)
	require.Same(t, first, coll.topologyRegistry.acquireGeneration())
	require.Equal(t, state, coll.deviceStates[id])
	require.Equal(t, topologydiag.DiagnosticAbortCanceled, coll.acquireTopologyDiagnostics().LastAborted.Reason)
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 3, calls)
	require.Equal(t, "new-device", coll.topologyRegistry.acquireGeneration().devices[0].trap.deviceHostname)
}
