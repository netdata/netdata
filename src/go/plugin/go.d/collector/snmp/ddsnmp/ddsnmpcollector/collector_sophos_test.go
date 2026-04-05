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

func TestCollector_Collect_SophosLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSophosLicensingGet(mockHandler, []sophosLicenseScalar{
		{id: "base_firewall", statusOID: "1.3.6.1.4.1.2604.5.1.5.1.1.0", status: 3, expiryOID: "1.3.6.1.4.1.2604.5.1.5.1.2.0", expiry: "2030-11-11"},
		{id: "network_protection", statusOID: "1.3.6.1.4.1.2604.5.1.5.2.1.0", status: 4, expiryOID: "1.3.6.1.4.1.2604.5.1.5.2.2.0", expiry: "11 Nov 2031"},
		{id: "web_protection", statusOID: "1.3.6.1.4.1.2604.5.1.5.3.1.0", status: 2, expiryOID: "1.3.6.1.4.1.2604.5.1.5.3.2.0", expiry: "N/A"},
		{id: "mail_protection", statusOID: "1.3.6.1.4.1.2604.5.1.5.4.1.0", status: 1, expiryOID: "1.3.6.1.4.1.2604.5.1.5.4.2.0", expiry: "2026-04-05 12:00:00"},
		{id: "web_server_protection", statusOID: "1.3.6.1.4.1.2604.5.1.5.5.1.0", status: 3, expiryOID: "1.3.6.1.4.1.2604.5.1.5.5.2.0", expiry: "Mon Jan 15 2031"},
		{id: "sandstorm", statusOID: "1.3.6.1.4.1.2604.5.1.5.6.1.0", status: 5, expiryOID: "1.3.6.1.4.1.2604.5.1.5.6.2.0", expiry: "2031-01-20"},
		{id: "enhanced_support", statusOID: "1.3.6.1.4.1.2604.5.1.5.7.1.0", status: 3, expiryOID: "1.3.6.1.4.1.2604.5.1.5.7.2.0", expiry: "2027-03-01"},
		{id: "enhanced_plus_support", statusOID: "1.3.6.1.4.1.2604.5.1.5.8.1.0", status: 0, expiryOID: "1.3.6.1.4.1.2604.5.1.5.8.2.0", expiry: "Never"},
		{id: "central_orchestration", statusOID: "1.3.6.1.4.1.2604.5.1.5.9.1.0", status: 3, expiryOID: "1.3.6.1.4.1.2604.5.1.5.9.2.0", expiry: "02 Jan 2032"},
	})

	profile := mustLoadLicensingProfile(t, "sophos-xgs-firewall", func(metric ddprofiledefinition.MetricsConfig) bool {
		return metric.MIB == "SFOS-FIREWALL-MIB" && strings.HasPrefix(strings.TrimPrefix(metric.Symbol.OID, "."), "1.3.6.1.4.1.2604.5.1.5.")
	})
	require.Len(t, profile.Definition.Metrics, 9)

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
	require.Len(t, pm.HiddenMetrics, 9)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	base := byID["base_firewall"]
	require.NotNil(t, base)
	assert.EqualValues(t, 0, base.Value)
	assert.Equal(t, "subscribed", base.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "2030-11-11", base.Tags["_license_expiry_raw"])

	expired := byID["network_protection"]
	require.NotNil(t, expired)
	assert.EqualValues(t, 2, expired.Value)
	assert.Equal(t, "expired", expired.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "11 Nov 2031", expired.Tags["_license_expiry_raw"])

	optional := byID["web_protection"]
	require.NotNil(t, optional)
	assert.EqualValues(t, 0, optional.Value)
	assert.Equal(t, "not_subscribed", optional.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "N/A", optional.Tags["_license_expiry_raw"])

	trial := byID["mail_protection"]
	require.NotNil(t, trial)
	assert.EqualValues(t, 1, trial.Value)
	assert.Equal(t, "trial", trial.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "2026-04-05 12:00:00", trial.Tags["_license_expiry_raw"])

	deactivated := byID["sandstorm"]
	require.NotNil(t, deactivated)
	assert.EqualValues(t, 2, deactivated.Value)
	assert.Equal(t, "deactivated", deactivated.Tags[testTagLicenseStateRaw])

	support := byID["enhanced_support"]
	require.NotNil(t, support)
	assert.Equal(t, "support", support.StaticTags["_license_type"])

	enhancedPlus := byID["enhanced_plus_support"]
	require.NotNil(t, enhancedPlus)
	assert.EqualValues(t, 0, enhancedPlus.Value)
	assert.Equal(t, "none", enhancedPlus.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "Never", enhancedPlus.Tags["_license_expiry_raw"])
}

type sophosLicenseScalar struct {
	id        string
	statusOID string
	status    int
	expiryOID string
	expiry    string
}

func expectSophosLicensingGet(mockHandler *snmpmock.MockHandler, rows []sophosLicenseScalar) {
	pdus := make([]gosnmp.SnmpPDU, 0, len(rows)*2)
	oids := make([]string, 0, len(rows)*2)

	for _, row := range rows {
		oids = append(oids, row.statusOID, row.expiryOID)
		pdus = append(pdus,
			createIntegerPDU(row.statusOID, row.status),
			createStringPDU(row.expiryOID, row.expiry),
		)
	}

	const maxOIDs = 10

	for i := 0; i < len(oids); i += maxOIDs {
		end := i + maxOIDs
		if end > len(oids) {
			end = len(oids)
		}
		expectSNMPGet(mockHandler, oids[i:end], pdus[i:end])
	}
}
