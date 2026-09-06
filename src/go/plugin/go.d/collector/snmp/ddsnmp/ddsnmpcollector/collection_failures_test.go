// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

func TestCollectionFailuresObserveWarmPollAndRecovery(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	const scalarOID = "1.3.6.1.2.1.1.3.0"
	const licenseOID = "1.3.6.1.4.1.99999.7.1.0"
	bgpOIDs := []string{"1.3.6.1.4.1.99999.20.1.0", "1.3.6.1.4.1.99999.20.2.0"}
	collector := New(
		Config{
			SnmpClient: handler,
			Log:        logger.New(),
			Profiles: []*ddsnmp.Profile{{SourceFile: "device.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{OID: scalarOID, Name: "sysUpTime"}}},
				BGP:     []ddprofiledefinition.BGPConfig{scalarBGPTestConfig()},
				Licensing: []ddprofiledefinition.LicensingConfig{
					{
						ID:       "license",
						Identity: ddprofiledefinition.LicenseIdentityConfig{ID: ddprofiledefinition.LicenseValueConfig{Value: "license"}},
						State: ddprofiledefinition.LicenseStateConfig{
							LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
								Symbol: ddprofiledefinition.SymbolConfig{OID: licenseOID, Name: "state"},
							},
						},
					},
				},
			}}},
		},
	)
	for cycle := 0; cycle < 3; cycle++ {
		expectSNMPGet(handler, []string{scalarOID}, []gosnmp.SnmpPDU{createTimeTicksPDU(scalarOID, 123456)})
		if cycle == 1 {
			expectSNMPGet(handler, []string{licenseOID}, []gosnmp.SnmpPDU{createStringPDU(licenseOID, "SECRET malformed state")})
			expectSNMPGetError(handler, bgpOIDs, gosnmp.ErrWrongDigest)
		} else {
			expectSNMPGet(handler, []string{licenseOID}, []gosnmp.SnmpPDU{createIntegerPDU(licenseOID, 0)})
			expectSNMPGet(handler, bgpOIDs, []gosnmp.SnmpPDU{createIntegerPDU(bgpOIDs[0], 6), createGauge32PDU(bgpOIDs[1], 3600)})
		}
		metrics, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, metrics, 1)
		require.Len(t, metrics[0].Metrics, 1, "partial typed-row failures must not change ordinary metric output")
		failures := collector.CollectionFailures()
		require.True(t, failures.Valid())
		if cycle == 1 {
			require.EqualValues(t, 1, failures.GET.Count)
			require.Equal(t, "wrong_digest", failures.GET.Last.Reason)
			require.EqualValues(t, 1, failures.BGP.Count)
			require.Equal(t, "wrong_digest", failures.BGP.Last.Reason)
			require.EqualValues(t, 1, failures.Licensing.Count)
			require.Equal(t, "processing", failures.Licensing.Last.Reason)
			require.EqualValues(t, 1, failures.Processing.Licensing)
			require.Zero(t, failures.Profiles.Count)
		} else {
			require.Equal(t, ddsnmp.CollectionFailures{}, failures)
			require.Len(t, metrics[0].BGPRows, 1)
			require.Len(t, metrics[0].LicenseRows, 1)
		}
		encoded, err := json.Marshal(failures)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "SECRET")
	}
}

func TestDiagnosticClientPreservesObservedOutcomes(t *testing.T) {
	tests := map[string]struct {
		get    bool
		packet *gosnmp.SnmpPacket
		values []gosnmp.SnmpPDU
		err    error
		want   ddsnmp.CollectionFailures
	}{
		"packet error preserves response and index": {
			get:    true,
			packet: &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, ErrorIndex: 2},
			want: ddsnmp.CollectionFailures{
				GET: ddsnmp.FailureCount{
					Count: 1,
					Last: snmputils.Failure{
						Operation:    "get",
						Reason:       "packet_error",
						PacketStatus: uint8(gosnmp.NoSuchName),
						ErrorIndex:   2,
					},
				},
			},
		},
		"empty successful walk has no inferred termination reason": {values: []gosnmp.SnmpPDU{}},
		"failed walk preserves partial values": {
			values: []gosnmp.SnmpPDU{{Name: "1.3.6.1", Type: gosnmp.Integer, Value: 1}},
			err:    context.DeadlineExceeded,
			want: ddsnmp.CollectionFailures{
				WALK: ddsnmp.FailureCount{Count: 1, Last: snmputils.Failure{Operation: "walk", Reason: "deadline"}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			var failures ddsnmp.CollectionFailures
			client := &diagnosticClient{Handler: handler, failures: &failures}
			var err error
			if tc.get {
				handler.EXPECT().Get([]string{"1.3.6"}).Return(tc.packet, tc.err)
				var packet *gosnmp.SnmpPacket
				packet, err = client.Get([]string{"1.3.6"})
				require.Same(t, tc.packet, packet)
			} else {
				handler.EXPECT().BulkWalkAll("1.3.6").Return(tc.values, tc.err)
				var values []gosnmp.SnmpPDU
				values, err = client.BulkWalkAll("1.3.6")
				require.Equal(t, tc.values, values)
			}
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, failures)
			require.True(t, failures.Valid())
		})
	}
}

func TestReplacementClientKeepsFailureObservation(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	profile := createTestProfile(
		"scalar.yaml",
		[]ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "uptime"}}},
	)
	c := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
	c.SetSNMPClient(handler)
	expectSNMPGetError(handler, []string{"1.3.6.1.2.1.1.3.0"}, gosnmp.ErrWrongDigest)
	_, _ = c.Collect()
	require.EqualValues(t, 1, c.CollectionFailures().GET.Count)
	require.Equal(t, "wrong_digest", c.CollectionFailures().GET.Last.Reason)
}

func TestMetadataProcessingDoesNotRelabelLaterTransportFailure(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	const oid = "1.3.6.1.4.1.99999"
	const first = oid + ".1.0"
	const second = oid + ".2.0"
	profile := &ddsnmp.Profile{SourceFile: "mixed.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
		Metadata: ddprofiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]ddprofiledefinition.MetadataField{
					"serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: second, Name: "serial"}},
				},
			},
		},
		SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
			{
				SysobjectID: oid,
				Metadata: map[string]ddprofiledefinition.MetadataField{
					"model": {Symbol: ddprofiledefinition.SymbolConfig{OID: first, Name: "model", Format: "uint32"}},
				},
			},
		},
	}}
	c := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, SysObjectID: oid, Log: logger.New()})
	expectSNMPGet(handler, []string{first}, []gosnmp.SnmpPDU{createStringPDU(first, "invalid number")})
	expectSNMPGetError(handler, []string{second}, gosnmp.ErrWrongDigest)
	_, err := c.CollectDeviceMetadata()
	require.ErrorIs(t, err, gosnmp.ErrWrongDigest)
	require.Equal(t, "wrong_digest", snmputils.ClassifyFailure(err).Reason)
	failures := c.CollectionFailures()
	require.Equal(t, "wrong_digest", failures.Profiles.Last.Reason)
	require.Equal(t, "wrong_digest", failures.GET.Last.Reason)
	require.EqualValues(t, 1, failures.Processing.Preparation)
}
