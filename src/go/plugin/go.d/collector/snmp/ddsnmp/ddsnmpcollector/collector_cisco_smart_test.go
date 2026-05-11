// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Collect_CiscoSmartLicensingProfile(t *testing.T) {
	tests := map[string]struct {
		setup      func(*snmpmock.MockHandler)
		assertRows func(*testing.T, []ddsnmp.LicenseRow)
	}{
		"complete data": {
			setup: func(mockHandler *snmpmock.MockHandler) {
				expectSNMPGet(mockHandler,
					[]string{
						"1.3.6.1.4.1.9.9.831.0.6.1.0",
					},
					[]gosnmp.SnmpPDU{
						createIntegerPDU("1.3.6.1.4.1.9.9.831.0.6.1.0", 2),
					},
				)
				expectSNMPGet(mockHandler,
					[]string{
						"1.3.6.1.4.1.9.9.831.0.7.1.0",
						"1.3.6.1.4.1.9.9.831.0.7.2.0",
					},
					[]gosnmp.SnmpPDU{
						createGauge32PDU("1.3.6.1.4.1.9.9.831.0.7.1.0", 1775152800),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.7.2.0", "Out of Compliance"),
					},
				)
				expectSNMPGet(mockHandler,
					[]string{"1.3.6.1.4.1.9.9.831.0.6.3.0"},
					[]gosnmp.SnmpPDU{createGauge32PDU("1.3.6.1.4.1.9.9.831.0.6.3.0", 1777831200)},
				)
				expectSNMPGet(mockHandler,
					[]string{"1.3.6.1.4.1.9.9.831.0.7.4.2.0"},
					[]gosnmp.SnmpPDU{createGauge32PDU("1.3.6.1.4.1.9.9.831.0.7.4.2.0", 1773943200)},
				)
				expectSNMPWalk(mockHandler,
					gosnmp.Version2c,
					"1.3.6.1.4.1.9.9.831.0.5.1",
					[]gosnmp.SnmpPDU{
						createGauge32PDU("1.3.6.1.4.1.9.9.831.0.5.1.1.2.1", 42),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.3.1", "dna_advantage"),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.4.1", "17.12"),
						createIntegerPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.5.1", 8),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.6.1", "Cisco DNA Advantage entitlement"),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.7.1", "network-advantage"),
					},
				)
			},
			assertRows: func(t *testing.T, rows []ddsnmp.LicenseRow) {
				require.Len(t, rows, 5)
				byID := licenseRowsByID(rows)

				registration := byID["smart_registration"]
				require.True(t, registration.State.Has)
				assert.EqualValues(t, 0, registration.State.Severity)
				assert.Equal(t, "Smart Licensing registration", registration.Name)

				authorization := byID["smart_authorization"]
				require.True(t, authorization.State.Has)
				assert.EqualValues(t, 2, authorization.State.Severity)
				assert.Equal(t, "Out of Compliance", authorization.State.Raw)
				require.True(t, authorization.Authorization.Has)
				assert.EqualValues(t, 1775152800, authorization.Authorization.Timestamp)

				certificate := byID["smart_id_certificate"]
				require.True(t, certificate.Certificate.Has)
				assert.EqualValues(t, 1777831200, certificate.Certificate.Timestamp)

				evaluation := byID["smart_evaluation_period"]
				require.True(t, evaluation.Grace.Has)
				assert.EqualValues(t, 1773943200, evaluation.Grace.Timestamp)

				entitlement := byID["dna_advantage"]
				require.True(t, entitlement.Usage.HasUsed)
				assert.EqualValues(t, 42, entitlement.Usage.Used)
				require.True(t, entitlement.State.Has)
				assert.EqualValues(t, 2, entitlement.State.Severity)
				assert.Equal(t, "network-advantage", entitlement.Name)
				assert.Equal(t, "8", entitlement.State.Raw)
			},
		},
		"partial data": {
			setup: func(mockHandler *snmpmock.MockHandler) {
				expectSNMPGet(mockHandler,
					[]string{
						"1.3.6.1.4.1.9.9.831.0.6.1.0",
					},
					[]gosnmp.SnmpPDU{
						createIntegerPDU("1.3.6.1.4.1.9.9.831.0.6.1.0", 2),
					},
				)
				expectSNMPGet(mockHandler,
					[]string{
						"1.3.6.1.4.1.9.9.831.0.7.1.0",
						"1.3.6.1.4.1.9.9.831.0.7.2.0",
					},
					[]gosnmp.SnmpPDU{
						createStringPDU("1.3.6.1.4.1.9.9.831.0.7.2.0", "Authorized"),
					},
				)
				expectSNMPGet(mockHandler, []string{"1.3.6.1.4.1.9.9.831.0.6.3.0"}, nil)
				expectSNMPGet(mockHandler, []string{"1.3.6.1.4.1.9.9.831.0.7.4.2.0"}, nil)
				expectSNMPWalk(mockHandler,
					gosnmp.Version2c,
					"1.3.6.1.4.1.9.9.831.0.5.1",
					[]gosnmp.SnmpPDU{
						createGauge32PDU("1.3.6.1.4.1.9.9.831.0.5.1.1.2.1", 7),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.3.1", "dna_essentials"),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.4.1", "17.9"),
						createIntegerPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.5.1", 3),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.6.1", "Cisco DNA Essentials entitlement"),
						createStringPDU("1.3.6.1.4.1.9.9.831.0.5.1.1.7.1", "network-essentials"),
					},
				)
			},
			assertRows: func(t *testing.T, rows []ddsnmp.LicenseRow) {
				require.Len(t, rows, 3)
				byID := licenseRowsByID(rows)

				require.True(t, byID["smart_registration"].State.Has)
				assert.EqualValues(t, 0, byID["smart_registration"].State.Severity)
				require.True(t, byID["smart_authorization"].State.Has)
				assert.EqualValues(t, 0, byID["smart_authorization"].State.Severity)

				entitlement := byID["dna_essentials"]
				require.True(t, entitlement.Usage.HasUsed)
				assert.EqualValues(t, 7, entitlement.Usage.Used)
				require.True(t, entitlement.State.Has)
				assert.EqualValues(t, 0, entitlement.State.Severity)
				assert.Equal(t, "3", entitlement.State.Raw)

				assert.NotContains(t, byID, "smart_id_certificate")
				assert.NotContains(t, byID, "smart_evaluation_period")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			tc.setup(mockHandler)
			profile := mustLoadCiscoSmartProfile(t)
			require.True(t, hasLicensingTable(profile, "1.3.6.1.4.1.9.9.831.0.5.1"))
			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: "",
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			assert.Empty(t, pm.Metrics)
			assert.Empty(t, pm.HiddenMetrics)
			tc.assertRows(t, pm.LicenseRows)
		})
	}
}

func mustLoadCiscoSmartProfile(t *testing.T) *ddsnmp.Profile {
	t.Helper()

	return mustLoadTypedLicensingProfile(t, "cisco", func(row ddprofiledefinition.LicensingConfig) bool {
		const prefix = "1.3.6.1.4.1.9.9.831."

		if row.MIB == "CISCO-SMART-LIC-MIB" {
			return true
		}
		for _, sig := range ddprofiledefinition.LicenseSignalValueRefs(row) {
			if oid := strings.TrimPrefix(ddprofiledefinition.LicenseValueSourceOID(sig.Value), "."); oid != "" && strings.HasPrefix(oid, prefix) {
				return true
			}
		}
		if oid := strings.TrimPrefix(ddprofiledefinition.LicenseValueSourceOID(row.State.LicenseValueConfig), "."); oid != "" && strings.HasPrefix(oid, prefix) {
			return true
		}
		if oid := strings.TrimPrefix(row.Table.OID, "."); oid != "" && strings.HasPrefix(oid, prefix) {
			return true
		}
		return false
	})
}
