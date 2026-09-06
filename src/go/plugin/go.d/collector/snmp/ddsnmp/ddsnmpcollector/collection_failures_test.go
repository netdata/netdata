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

func TestDiagnosticClientDoesNotInferSuccessfulWalkTermination(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	handler.EXPECT().BulkWalkAll("1.3.6").Return([]gosnmp.SnmpPDU{}, nil)
	var failures ddsnmp.CollectionFailures
	client := &diagnosticClient{Handler: handler, failures: &failures}
	values, err := client.BulkWalkAll("1.3.6")
	require.NoError(t, err)
	require.Empty(t, values)
	require.Zero(t, failures.WALK.Count)
	require.Empty(t, failures.WALK.Last.Reason)
}

func TestDiagnosticClientPreservesPacketAndWalkFailures(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	var failures ddsnmp.CollectionFailures
	client := &diagnosticClient{Handler: handler, failures: &failures}
	packet := &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, ErrorIndex: 2}
	handler.EXPECT().Get([]string{"1.3.6"}).Return(packet, nil)
	got, err := client.Get([]string{"1.3.6"})
	require.NoError(t, err)
	require.Same(t, packet, got)
	require.EqualValues(t, gosnmp.NoSuchName, failures.GET.Last.PacketStatus)
	require.EqualValues(t, 2, failures.GET.Last.ErrorIndex)
	values := []gosnmp.SnmpPDU{{Name: "1.3.6.1", Type: gosnmp.Integer, Value: 1}}
	handler.EXPECT().BulkWalkAll("1.3.6").Return(values, context.DeadlineExceeded)
	gotValues, err := client.BulkWalkAll("1.3.6")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, values, gotValues)
	require.Equal(t, "deadline", failures.WALK.Last.Reason)
	require.True(t, failures.Valid())
}

func TestReplacementClientKeepsFailureObservation(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	profile := createTestProfile("scalar.yaml", []ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "uptime"}}})
	c := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
	c.SetSNMPClient(handler)
	expectSNMPGetError(handler, []string{"1.3.6.1.2.1.1.3.0"}, gosnmp.ErrWrongDigest)
	_, _ = c.Collect()
	require.EqualValues(t, 1, c.CollectionFailures().GET.Count)
	require.Equal(t, "wrong_digest", c.CollectionFailures().GET.Last.Reason)
}
