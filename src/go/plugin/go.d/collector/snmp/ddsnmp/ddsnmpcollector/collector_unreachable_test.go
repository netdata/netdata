// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_UnreachableWithNoopProfile(t *testing.T) {
	ctrl, handler := setupMockSession(t)
	defer ctrl.Finish()
	const oid = "1.3.6.1.2.1.1.3.0"
	c := New(Config{
		Version:    gosnmp.Version2c,
		MaxOIDs:    10,
		SnmpClient: handler,
		Log:        logger.New(),
		Profiles: []*ddsnmp.Profile{
			createTestProfile("device.yaml", []ddprofiledefinition.MetricsConfig{createScalarMetric(oid, "uptime")}),
			// Deduplication can leave a matched vendor profile without recurring requests.
			createTestProfile("vendor.yaml", nil),
		},
	})
	for _, blocked := range []bool{false, true, false} {
		if blocked {
			expectSNMPGetError(handler, []string{oid}, context.DeadlineExceeded)
		} else {
			expectSNMPGet(handler, []string{oid}, []gosnmp.VarBind{createTimeTicksPDU(oid, 100)})
		}
		metrics, err := c.Collect(context.Background())
		if blocked {
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Empty(t, metrics, "a no-op profile must not publish statistics for an unresponsive device")
		} else {
			require.NoError(t, err)
			require.Len(t, metrics, 2)
			require.Len(t, metrics[0].Metrics, 1)
		}
	}
}

func TestCollector_FailedSiblingWithGETResponse(t *testing.T) {
	const oid = "1.3.6.1.2.1.1.3.0"
	const failedOID = "1.3.6.1.2.1.1.4.0"
	for name, tc := range map[string]struct {
		packet    *gosnmp.Message
		err       error
		wantError bool
	}{
		"value": {packet: &gosnmp.Message{
			VarBinds: []gosnmp.VarBind{createTimeTicksPDU(oid, 100)},
		}},
		"empty response": {packet: &gosnmp.Message{}},
		"missing object": {packet: &gosnmp.Message{
			VarBinds: []gosnmp.VarBind{{Name: oid, Value: gosnmp.NoSuchObjectValue()}},
		}},
		"v1 missing name":   {err: &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: 1}},
		"packet error":      {err: &gosnmp.StatusError{Status: gosnmp.GenErr}},
		"v3 error response": {err: answeredError(gosnmp.ErrUnknownUsername)},
		"no response":       {wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockSession(t)
			defer ctrl.Finish()
			expectSNMPGetError(handler, []string{failedOID}, context.DeadlineExceeded)
			handler.EXPECT().Get(gomock.Any(), []string{oid}).Return(tc.packet, tc.err)
			c := New(Config{
				Version:    gosnmp.Version1,
				MaxOIDs:    10,
				SnmpClient: handler,
				Log:        logger.New(),
				Profiles: []*ddsnmp.Profile{
					createTestProfile("failed.yaml", []ddprofiledefinition.MetricsConfig{createScalarMetric(failedOID, "contact")}),
					createTestProfile("response.yaml", []ddprofiledefinition.MetricsConfig{createScalarMetric(oid, "uptime")}),
					createTestProfile("vendor.yaml", nil),
				},
			})
			metrics, err := c.Collect(context.Background())
			if tc.wantError {
				require.Error(t, err)
				require.Empty(t, metrics)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, metrics)
			}
		})
	}
}

func TestCollector_FailedSiblingWithWalkResponse(t *testing.T) {
	const tableOID = "1.3.6.1.2.1.2.2"
	const valueOID = tableOID + ".1.10"
	const failedOID = "1.3.6.1.2.1.1.3.0"
	for name, tc := range map[string]struct {
		values    []gosnmp.VarBind
		err       error
		wantError bool
	}{
		"empty response":   {},
		"partial response": {values: []gosnmp.VarBind{createCounter32PDU(valueOID+".1", 100)}, err: context.DeadlineExceeded},
		"no response":      {err: context.DeadlineExceeded, wantError: true},
	} {
		for _, version := range []gosnmp.Version{gosnmp.Version1, gosnmp.Version2c} {
			t.Run(name+"/"+version.String(), func(t *testing.T) {
				ctrl, handler := setupMockSession(t)
				defer ctrl.Finish()
				expectSNMPGetError(handler, []string{failedOID}, context.DeadlineExceeded)
				if version == gosnmp.Version1 {
					handler.EXPECT().WalkAll(gomock.Any(), tableOID).Return(tc.values, gosnmp.WalkEnd{}, tc.err)
				} else {
					handler.EXPECT().BulkWalkAll(gomock.Any(), tableOID, gomock.Any()).Return(tc.values, gosnmp.WalkEnd{}, tc.err)
				}
				c := New(Config{
					Version:    version,
					MaxOIDs:    10,
					SnmpClient: handler,
					Log:        logger.New(),
					Profiles: []*ddsnmp.Profile{
						createTestProfile("failed.yaml", []ddprofiledefinition.MetricsConfig{createScalarMetric(failedOID, "uptime")}),
						createTestProfile("table.yaml", []ddprofiledefinition.MetricsConfig{{
							Table: ddprofiledefinition.SymbolConfig{
								OID:  tableOID,
								Name: "ifTable",
							},
							Symbols: []ddprofiledefinition.SymbolConfig{{OID: valueOID, Name: "ifInOctets"}},
						}}),
						createTestProfile("vendor.yaml", nil),
					},
				})
				metrics, err := c.Collect(context.Background())
				if tc.wantError {
					require.ErrorIs(t, err, context.DeadlineExceeded)
					require.Empty(t, metrics)
				} else {
					require.NoError(t, err)
					require.NotEmpty(t, metrics)
				}
			})
		}
	}
}

func TestCollector_NoRequests(t *testing.T) {
	ctrl, handler := setupMockSession(t)
	defer ctrl.Finish()
	c := New(Config{
		Version:    gosnmp.Version2c,
		MaxOIDs:    10,
		SnmpClient: handler,
		Log:        logger.New(),
		Profiles:   []*ddsnmp.Profile{createTestProfile("vendor.yaml", nil)},
	})
	for range 2 {
		metrics, err := c.Collect(context.Background())
		require.NoError(t, err)
		require.Len(t, metrics, 1)
	}
}

func TestCollector_UnreachableCachedTableAfterClientReplacement(t *testing.T) {
	ctrl, handler := setupMockSession(t)
	defer ctrl.Finish()
	const tableOID = "1.3.6.1.2.1.2.2"
	const valueOID = tableOID + ".1.10"
	c := New(Config{
		Version:    gosnmp.Version2c,
		MaxOIDs:    10,
		SnmpClient: handler,
		Log:        logger.New(),
		Profiles: []*ddsnmp.Profile{
			createTestProfile("table.yaml", []ddprofiledefinition.MetricsConfig{{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  tableOID,
					Name: "ifTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: valueOID, Name: "ifInOctets"}},
			}}),
			createTestProfile("vendor.yaml", nil),
		},
	})
	expectSNMPWalk(handler, gosnmp.Version2c, tableOID, []gosnmp.VarBind{createCounter32PDU(valueOID+".1", 100)})
	metrics, err := c.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics[0].Metrics, 1)

	replacementCtrl, replacement := setupMockSession(t)
	defer replacementCtrl.Finish()
	c.SetSNMPClient(replacement)
	expectSNMPGetError(replacement, []string{valueOID + ".1"}, context.DeadlineExceeded)
	replacement.EXPECT().BulkWalkAll(gomock.Any(), tableOID, gomock.Any()).Return(nil, gosnmp.WalkEnd{}, context.DeadlineExceeded)
	metrics, err = c.Collect(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, metrics)
}

func TestCollector_UnreachableTypedRows(t *testing.T) {
	const licenseOID = "1.3.6.1.4.1.99999.7.1.0"
	for _, kind := range []string{"licensing", "bgp"} {
		t.Run(kind, func(t *testing.T) {
			ctrl, handler := setupMockSession(t)
			defer ctrl.Finish()
			profile := createTestProfile("typed.yaml", nil)
			if kind == "licensing" {
				profile.Definition.Licensing = []ddprofiledefinition.LicensingConfig{{
					ID: "license",
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{
							Value: "license",
						},
					},
					State: ddprofiledefinition.LicenseStateConfig{
						LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
							Symbol: ddprofiledefinition.SymbolConfig{
								OID:  licenseOID,
								Name: "state",
							},
						},
					},
				}}
				expectSNMPGetError(handler, []string{licenseOID}, context.DeadlineExceeded)
			} else {
				profile.Definition.BGP = []ddprofiledefinition.BGPConfig{scalarBGPTestConfig()}
				expectSNMPGetError(handler, []string{"1.3.6.1.4.1.99999.20.1.0", "1.3.6.1.4.1.99999.20.2.0"}, context.DeadlineExceeded)
			}
			c := New(Config{
				Version:    gosnmp.Version2c,
				MaxOIDs:    10,
				SnmpClient: handler,
				Log:        logger.New(),
				Profiles:   []*ddsnmp.Profile{profile, createTestProfile("vendor.yaml", nil)},
			})
			metrics, err := c.Collect(context.Background())
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Empty(t, metrics)
		})
	}
}
