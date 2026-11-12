// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestCollectorRefreshPresentationKeepsDerivedMetadataCoherent(t *testing.T) {
	for canonical, tc := range map[string]struct {
		alias string
		value func(topologymodel.Device) string
	}{
		"software_version": {"version", func(d topologymodel.Device) string { return d.SoftwareVersion }},
		"firmware_version": {"firmware_revision", func(d topologymodel.Device) string { return d.FirmwareVersion }},
		"hardware_version": {"hw_rev", func(d topologymodel.Device) string { return d.HardwareVersion }},
		"serial_number":    {"serial_no", func(d topologymodel.Device) string { return d.SerialNumber }},
	} {
		t.Run(canonical, func(t *testing.T) {
			coll, store := newTestSNMPTopologyCollectorWithStore()
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			coll.now = func() time.Time { return now }
			dev := ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.10",
				Port:        161,
				SNMPVersion: gosnmp.Version2c.String(),
				SysObjectID: "1.3.6.1.4.1.9.1.1",
				SysDescr:    "release 1",
				VnodeLabels: map[string]string{tc.alias: "1.0", "sys_uptime": "77", tagOSPFRouterID: "192.0.2.1", "sys_object_id": "1.3.6.1.4.1.9.1.1"},
			}
			store.Register("job", dev)
			calls := 0
			coll.newSnmpClient = func() gosnmp.Handler {
				calls++
				client := snmpmock.NewMockHandler(gomock.NewController(t))
				expectTopologyRefreshSNMPClient(client, dev)
				return client
			}
			coll.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return []*ddsnmp.Profile{{}} }
			coll.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
				return ddCollectorFunc(func() ([]*ddsnmp.ProfileMetrics, error) {
					return []*ddsnmp.ProfileMetrics{{DeviceMetadata: map[string]ddsnmp.MetaTag{
						"vendor": {Value: "Cisco"}, tagLldpLocSysName: {Value: "profile-name"},
					}}}, nil
				})
			}
			require.Zero(t, coll.refreshTopology(context.Background()).errors)
			first := coll.topologyRegistry.acquireGeneration()
			original := first.observationSnapshots()[0].LocalDevice
			require.Equal(t, "1.0", tc.value(original))
			require.Equal(t, "1.0", original.Labels[canonical])
			require.Equal(t, "profile-name", original.SysName)
			state := coll.deviceStates[store.Entries()[0].RegistrationID]
			check := func(want string) {
				t.Helper()
				store.Register("job", dev)
				now = now.Add(time.Minute)
				require.Zero(t, coll.refreshTopology(context.Background()).errors)
				current := coll.topologyRegistry.acquireGeneration()
				local := current.observationSnapshots()[0].LocalDevice
				require.Equal(t, want, tc.value(local), "typed metadata follows current normalized registration")
				require.Equal(t, want, local.Labels[canonical], "canonical metadata follows alias precedence and removal")
				if want == "" {
					require.NotContains(t, local.Labels, canonical)
				}
				require.Equal(t, original.SysName, local.SysName, "unmodified profile fallback remains authoritative")
				require.Equal(t, original.Vendor, local.Vendor)
				require.Equal(t, original.SysObjectID, local.SysObjectID)
				require.Equal(t, original.OSPFRouterID, local.OSPFRouterID)
				require.Equal(t, original.SysUptime, local.SysUptime)
				for _, key := range []string{"sys_uptime", tagOSPFRouterID, "sys_object_id"} {
					require.Equal(t, original.Labels[key], local.Labels[key], "acquired facts remain in their acquisition generation")
				}
				require.Equal(t, 1, calls, "presentation changes must not reacquire topology")
				require.Same(t, first.devices[0].acquisition, current.devices[0].acquisition)
				require.Equal(t, first.devices[0].expiresAt, current.devices[0].expiresAt)
				require.Equal(t, first.devices[0].collectedAt, current.devices[0].collectedAt)
				require.Equal(t, state.nextRetry, coll.deviceStates[store.Entries()[0].RegistrationID].nextRetry)
				require.Equal(t, "1.0", tc.value(original), "old readers remain immutable")
				options := defaultTopologyQueryOptionsForTest()
				live, ok, err := (funcDepsAdapter{
					registry: coll.topologyRegistry,
				}).Snapshot(options)
				require.NoError(t, err)
				require.True(t, ok)
				var encoded bytes.Buffer
				require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, captureTopologyCut(coll.topologyRegistry, nil), "v-test"))
				archive, err := readTestDiagnosticArchive(bytes.NewReader(encoded.Bytes()), snmpdiag.DefaultReadLimits())
				require.NoError(t, err)
				replayed, err := archive.Replay(testDiagnosticQuery(options))
				require.NoError(t, err)
				require.Equal(t, live.Data, replayed)
				if want == "1.0" {
					// Earlier archives carry the six original presentation fields only.
					// Omission must keep captured derived metadata, rather than clear it.
					document, err := newTopologyDiagnosticArchiveDocumentV1(captureTopologyCut(coll.topologyRegistry, nil), "v-test")
					require.NoError(t, err)
					p := document.Snapshot.Topology.Devices[0].Presentation
					p.SerialNumber, p.SoftwareVersion, p.FirmwareVersion, p.HardwareVersion = nil, nil, nil, nil
					oldArchive, err := InspectDiagnosticDocument(document)
					require.NoError(t, err)
					oldReplay, err := oldArchive.Replay(testDiagnosticQuery(options))
					require.NoError(t, err)
					require.Equal(t, live.Data, oldReplay)
				}
			}
			dev.SysDescr = "release 2"
			dev.VnodeLabels[tc.alias] = "2.0"
			dev.VnodeLabels["sys_uptime"] = "999"
			dev.VnodeLabels[tagOSPFRouterID] = "192.0.2.99"
			dev.VnodeLabels["sys_object_id"] = "1.3.6.1.4.1.2636"
			check("2.0")
			dev.VnodeLabels[canonical] = "pinned"
			check("pinned")
			dev.VnodeLabels[tc.alias] = "3.0"
			check("pinned")
			delete(dev.VnodeLabels, canonical)
			check("3.0")
			delete(dev.VnodeLabels, tc.alias)
			check("")
			dev.VnodeLabels[tc.alias] = "1.0"
			check("1.0")

			for _, name := range []string{" updated-switch ", "   "} {
				dev.SysName = name
				store.Register("job", dev)
				now = now.Add(time.Minute)
				require.Zero(t, coll.refreshTopology(context.Background()).errors)
				current := coll.topologyRegistry.acquireGeneration()
				snapshot := current.observationSnapshots()[0]
				require.Equal(t, name, snapshot.LocalDevice.SysName)
				require.NotEmpty(t, snapshot.L2Observations)
				require.Equal(t, strings.TrimSpace(name), snapshot.L2Observations[0].Hostname)
				require.Equal(t, 1, calls, "name changes must not reacquire topology")
				require.Same(t, first.devices[0].acquisition, current.devices[0].acquisition)
				require.Equal(t, original.SysName, first.observationSnapshots()[0].LocalDevice.SysName)
			}
		})
	}
}
