// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
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

func TestCollectorRefreshMetadataDoesNotAcquireTopology(t *testing.T) {
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
	coll.newSnmpClient = func() gosnmp.Handler {
		calls++
		client := snmpmock.NewMockHandler(gomock.NewController(t))
		expectTopologyRefreshSNMPClient(client, dev)
		return client
	}
	coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
	coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) { return nil, nil })
	}
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	first := coll.topologyRegistry.acquireGeneration()
	firstDevice := first.devices[0]
	id := store.Entries()[0].RegistrationID
	firstState := coll.deviceStates[id]
	dev.SysName, dev.SysContact, dev.SysLocation = "new-device", "new-contact", "new-location"
	dev.VnodeLabels["name"], dev.VnodeLabels["location"] = dev.SysName, dev.SysLocation
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	require.Zero(t, coll.refreshTopology(context.Background()).errors)
	require.Equal(t, 1, calls)
	current := coll.topologyRegistry.acquireGeneration()
	require.Equal(t, "new-device", current.devices[0].trap.deviceHostname)
	enrichment := coll.topologyRegistry.trapEnrichmentForSource(dev.Hostname, "")
	require.NotNil(t, enrichment)
	require.Equal(t, "new-device", enrichment.DeviceHostname)
	observation := current.observationSnapshots()[0]
	require.Equal(t, "new-device", observation.LocalDevice.SysName)
	require.Equal(t, "new-device", observation.L2Observations[0].Hostname)
	require.Equal(t, "new-contact", observation.LocalDevice.SysContact)
	require.Equal(t, "new-location", observation.LocalDevice.SysLocation)
	require.Equal(t, "configured", observation.LocalDevice.Labels["site"])
	require.Equal(t, "new-location", observation.LocalDevice.Labels["location"])
	require.Equal(t, firstDevice.collectedAt, current.devices[0].collectedAt)
	require.Equal(t, firstDevice.expiresAt, current.devices[0].expiresAt)
	require.Equal(t, firstState.nextRetry, coll.deviceStates[id].nextRetry)
	require.Equal(t, firstState.attemptOrdinal, coll.deviceStates[id].attemptOrdinal)
	require.Same(t, firstDevice.acquisition, current.devices[0].acquisition)
	require.Equal(t, "old-device", current.devices[0].acquisition.Evidence.Device.SysName)
	require.Equal(t, "old-device", first.observationSnapshots()[0].LocalDevice.SysName)
	require.Equal(t, "old-device", firstDevice.trap.deviceHostname)

	options := defaultTopologyQueryOptionsForTest()
	live, ok, err := (funcDepsAdapter{
		registry: coll.topologyRegistry,
	}).Snapshot(options)
	require.NoError(t, err)
	require.True(t, ok)
	cut := captureTopologyCut(coll.topologyRegistry, nil)
	document, err := newTopologyDiagnosticArchiveDocumentV1(cut, "v-test")
	require.NoError(t, err)
	require.NotNil(t, document.Snapshot.Topology.Devices[0].Presentation)
	require.Greater(t, cut.Topology.RecordCount, first.diagnostic.RecordCount)
	require.Greater(t, cut.Topology.LogicalBytes, first.diagnostic.LogicalBytes)
	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, cut, "v-test"))
	archive, err := readTestDiagnosticArchive(bytes.NewReader(encoded.Bytes()), snmpdiag.DefaultReadLimits())
	require.NoError(t, err)
	replayed, err := archive.Replay(testDiagnosticQuery(options))
	require.NoError(t, err)
	require.Equal(t, live, replayed, "current presentation must survive archive replay separately from acquisition input")

	dev.SysContact, dev.SysLocation = "", ""
	delete(dev.VnodeLabels, "location")
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	coll.refreshTopology(context.Background())
	cleared := coll.topologyRegistry.acquireGeneration().observationSnapshots()[0].LocalDevice
	require.Empty(t, cleared.SysContact)
	require.Empty(t, cleared.SysLocation)
	require.NotContains(t, cleared.Labels, "location")
	require.Equal(t, 1, calls)

	// Returning to the captured metadata must not retain an earlier projection.
	dev.SysName, dev.SysContact, dev.SysLocation = "old-device", "old-contact", "old-location"
	dev.VnodeLabels["name"], dev.VnodeLabels["location"] = dev.SysName, dev.SysLocation
	store.Register("job-a", dev)
	now = now.Add(time.Minute)
	coll.refreshTopology(context.Background())
	require.Equal(t, 1, calls)
	require.Equal(t, first.observationSnapshots(), coll.topologyRegistry.acquireGeneration().observationSnapshots())
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
