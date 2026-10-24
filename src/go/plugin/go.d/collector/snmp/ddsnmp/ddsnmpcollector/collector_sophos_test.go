// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Collect_SophosLicensingProfile_PreservesRawStateAndExpiryOnScalarRows(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.1.1.0",
		"1.3.6.1.4.1.2604.5.1.5.1.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.1.1.0", 3),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.1.2.0", "2026-12-31"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.2.1.0",
		"1.3.6.1.4.1.2604.5.1.5.2.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.2.1.0", 4),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.2.2.0", "2026-11-30"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.3.1.0",
		"1.3.6.1.4.1.2604.5.1.5.3.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.3.1.0", 2),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.3.2.0", "N/A"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.4.1.0",
		"1.3.6.1.4.1.2604.5.1.5.4.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.4.1.0", 1),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.4.2.0", "2026-10-15"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.5.1.0",
		"1.3.6.1.4.1.2604.5.1.5.5.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.5.1.0", 3),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.5.2.0", "2027-01-20"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.6.1.0",
		"1.3.6.1.4.1.2604.5.1.5.6.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.6.1.0", 5),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.6.2.0", "2026-08-15"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.7.1.0",
		"1.3.6.1.4.1.2604.5.1.5.7.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.7.1.0", 3),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.7.2.0", "2026-09-01"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.8.1.0",
		"1.3.6.1.4.1.2604.5.1.5.8.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.8.1.0", 0),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.8.2.0", "Never"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2604.5.1.5.9.1.0",
		"1.3.6.1.4.1.2604.5.1.5.9.2.0",
	}, []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.2604.5.1.5.9.1.0", 3),
		createStringPDU("1.3.6.1.4.1.2604.5.1.5.9.2.0", "2026-07-01"),
	})

	profile := mustLoadTypedLicensingProfile(t, "sophos-xgs-firewall", func(row ddprofiledefinition.LicensingConfig) bool {
		return row.MIB == "SFOS-FIREWALL-MIB"
	})
	require.Len(t, profile.Definition.Licensing, 9)

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
	require.Len(t, pm.LicenseRows, 9)
	assert.EqualValues(t, 9, pm.Stats.Metrics.Licensing)

	byID := licenseRowsByID(pm.LicenseRows)
	require.Len(t, byID, 9)

	expectations := map[string]struct {
		severity  int64
		state     string
		kind      string
		expiryTS  int64
		hasExpiry bool
	}{
		"base_firewall":         {severity: 0, state: "subscribed", kind: "subscription", expiryTS: 1798675200, hasExpiry: true},
		"network_protection":    {severity: 2, state: "expired", kind: "subscription", expiryTS: 1795996800, hasExpiry: true},
		"web_protection":        {severity: 0, state: "not_subscribed", kind: "subscription"},
		"mail_protection":       {severity: 1, state: "trial", kind: "subscription", expiryTS: 1792022400, hasExpiry: true},
		"web_server_protection": {severity: 0, state: "subscribed", kind: "subscription", expiryTS: 1800403200, hasExpiry: true},
		"sandstorm":             {severity: 2, state: "deactivated", kind: "subscription", expiryTS: 1786752000, hasExpiry: true},
		"enhanced_support":      {severity: 0, state: "subscribed", kind: "support", expiryTS: 1788220800, hasExpiry: true},
		"enhanced_plus_support": {severity: 0, state: "none", kind: "support"},
		"central_orchestration": {severity: 0, state: "subscribed", kind: "subscription", expiryTS: 1782864000, hasExpiry: true},
	}

	for id, want := range expectations {
		row, ok := byID[id]
		require.Truef(t, ok, "missing license row for %s", id)

		assert.Equal(t, id, row.ID)
		assert.Equal(t, want.kind, row.Type, "unexpected license type for %s", id)
		require.Truef(t, row.State.Has, "missing state for %s", id)
		assert.EqualValues(t, want.severity, row.State.Severity, "unexpected severity for %s", id)
		assert.Equal(t, want.state, row.State.Raw, "unexpected raw state for %s", id)
		assert.Equal(t, "1.3.6.1.4.1.2604.5.1.5."+licenseIndexForSophosID(id)+".1.0", row.State.SourceOID)

		assert.Equal(t, want.hasExpiry, row.Expiry.Has, "unexpected expiry presence for %s", id)
		if want.hasExpiry {
			assert.EqualValues(t, want.expiryTS, row.Expiry.Timestamp, "unexpected expiry timestamp for %s", id)
			assert.Equal(t, "1.3.6.1.4.1.2604.5.1.5."+licenseIndexForSophosID(id)+".2.0", row.Expiry.SourceOID)
		}
	}
}

func licenseIndexForSophosID(id string) string {
	return map[string]string{
		"base_firewall":         "1",
		"network_protection":    "2",
		"web_protection":        "3",
		"mail_protection":       "4",
		"web_server_protection": "5",
		"sandstorm":             "6",
		"enhanced_support":      "7",
		"enhanced_plus_support": "8",
		"central_orchestration": "9",
	}[id]
}
