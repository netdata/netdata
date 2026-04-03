// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTagLicenseID        = "_license_id"
	testTagLicenseName      = "_license_name"
	testTagLicenseStateRaw  = "_license_state_raw"
	testTagLicenseValueKind = "_license_value_kind"
	testTagLicenseUsageRaw  = "_license_usage_raw"
)

func TestCollector_Collect_CiscoSmartLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler,
		[]string{
			"1.3.6.1.4.1.9.9.831.0.6.1",
			"1.3.6.1.4.1.9.9.831.0.7.2",
			"1.3.6.1.4.1.9.9.831.0.7.1",
			"1.3.6.1.4.1.9.9.831.0.6.3",
			"1.3.6.1.4.1.9.9.831.0.7.4.2",
		},
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.9.9.831.0.6.1", 2),
			createStringPDU("1.3.6.1.4.1.9.9.831.0.7.2", "Out of Compliance"),
			createGauge32PDU("1.3.6.1.4.1.9.9.831.0.7.1", 1775152800),
			createGauge32PDU("1.3.6.1.4.1.9.9.831.0.6.3", 1777831200),
			createGauge32PDU("1.3.6.1.4.1.9.9.831.0.7.4.2", 1773943200),
		},
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

	profile := mustLoadCiscoSmartProfile(t)
	require.True(t, hasMetricTable(profile, "1.3.6.1.4.1.9.9.831.0.5.1"), profileMetricsSummary(profile))
	for _, metric := range profile.Definition.Metrics {
		if strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.9.9.831.0.5.1" {
			assert.True(t, metric.DisableTableCache)
		}
	}

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
	require.Len(t, pm.HiddenMetrics, 6)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	registration := byID["smart_registration"]
	require.NotNil(t, registration)
	assert.EqualValues(t, 0, registration.Value)
	assert.Equal(t, "state_severity", registration.StaticTags[testTagLicenseValueKind])
	assert.Equal(t, "Smart Licensing registration", registration.StaticTags[testTagLicenseName])

	authState := byID["smart_authorization_state"]
	require.NotNil(t, authState)
	assert.EqualValues(t, 2, authState.Value)
	assert.Equal(t, "state_severity", authState.StaticTags[testTagLicenseValueKind])

	authExpiry := byID["smart_authorization_expiry"]
	require.NotNil(t, authExpiry)
	assert.EqualValues(t, 1775152800, authExpiry.Value)
	assert.Equal(t, "authorization_timestamp", authExpiry.StaticTags[testTagLicenseValueKind])

	certExpiry := byID["smart_id_certificate_expiry"]
	require.NotNil(t, certExpiry)
	assert.EqualValues(t, 1777831200, certExpiry.Value)
	assert.Equal(t, "certificate_timestamp", certExpiry.StaticTags[testTagLicenseValueKind])

	evalExpiry := byID["smart_evaluation_expiry"]
	require.NotNil(t, evalExpiry)
	assert.EqualValues(t, 1773943200, evalExpiry.Value)
	assert.Equal(t, "grace_timestamp", evalExpiry.StaticTags[testTagLicenseValueKind])

	entitlement := byID["dna_advantage"]
	require.NotNil(t, entitlement)
	assert.Equal(t, "network-advantage", entitlement.Tags[testTagLicenseName])
	assert.Equal(t, "authorization_expired", entitlement.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "42", entitlement.Tags[testTagLicenseUsageRaw])
}

func TestCollector_Collect_CiscoSmartLicensingProfile_PartialData(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler,
		[]string{
			"1.3.6.1.4.1.9.9.831.0.6.1",
			"1.3.6.1.4.1.9.9.831.0.7.2",
			"1.3.6.1.4.1.9.9.831.0.7.1",
			"1.3.6.1.4.1.9.9.831.0.6.3",
			"1.3.6.1.4.1.9.9.831.0.7.4.2",
		},
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.9.9.831.0.6.1", 2),
			createStringPDU("1.3.6.1.4.1.9.9.831.0.7.2", "Authorized"),
		},
	)

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

	profile := mustLoadCiscoSmartProfile(t)
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
	require.Len(t, pm.HiddenMetrics, 3)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	registration := byID["smart_registration"]
	require.NotNil(t, registration)
	assert.EqualValues(t, 0, registration.Value)

	authState := byID["smart_authorization_state"]
	require.NotNil(t, authState)
	assert.EqualValues(t, 0, authState.Value)

	entitlement := byID["dna_essentials"]
	require.NotNil(t, entitlement)
	assert.Equal(t, "authorized", entitlement.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "7", entitlement.Tags[testTagLicenseUsageRaw])

	assert.NotContains(t, byID, "smart_authorization_expiry")
	assert.NotContains(t, byID, "smart_id_certificate_expiry")
	assert.NotContains(t, byID, "smart_evaluation_expiry")
}

func mustLoadCiscoSmartProfile(t *testing.T) *ddsnmp.Profile {
	t.Helper()

	return mustLoadLicensingProfile(t, "cisco", func(metric ddprofiledefinition.MetricsConfig) bool {
		const prefix = "1.3.6.1.4.1.9.9.831."

		if metric.MIB == "CISCO-SMART-LIC-MIB" {
			return true
		}
		if oid := strings.TrimPrefix(metric.Symbol.OID, "."); oid != "" && strings.HasPrefix(oid, prefix) {
			return true
		}
		if oid := strings.TrimPrefix(metric.Table.OID, "."); oid != "" && strings.HasPrefix(oid, prefix) {
			return true
		}
		return false
	})
}

func licenseMetricsByID(metrics []ddsnmp.Metric) map[string]*ddsnmp.Metric {
	out := make(map[string]*ddsnmp.Metric, len(metrics))
	for i := range metrics {
		metric := &metrics[i]
		id := metric.StaticTags[testTagLicenseID]
		if id == "" {
			id = metric.Tags[testTagLicenseID]
		}
		if id == "" {
			continue
		}
		out[id] = metric
	}
	return out
}

func hasMetricTable(profile *ddsnmp.Profile, oid string) bool {
	for _, metric := range profile.Definition.Metrics {
		if strings.TrimPrefix(metric.Table.OID, ".") == oid {
			return true
		}
	}
	return false
}

func profileMetricsSummary(profile *ddsnmp.Profile) string {
	var lines []string
	for _, metric := range profile.Definition.Metrics {
		switch {
		case metric.Symbol.OID != "":
			lines = append(lines, "scalar:"+strings.TrimPrefix(metric.Symbol.OID, ".")+" name="+metric.Symbol.Name+" mib="+metric.MIB)
		case metric.Table.OID != "":
			lines = append(lines, "table:"+strings.TrimPrefix(metric.Table.OID, ".")+" name="+metric.Table.Name+" mib="+metric.MIB)
		default:
			lines = append(lines, "other:table_name="+metric.Table.Name+" symbol_name="+metric.Symbol.Name+" mib="+metric.MIB+" symbols="+strconv.Itoa(len(metric.Symbols)))
		}
	}
	return strings.Join(lines, "\n")
}
