// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_CheckPointLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.2620.1.6.18.1",
		[]gosnmp.SnmpPDU{
			createGauge32PDU("1.3.6.1.4.1.2620.1.6.18.1.1.1.17", 17),
			createGauge32PDU("1.3.6.1.4.1.2620.1.6.18.1.1.2.17", 17),
			createStringPDU("1.3.6.1.4.1.2620.1.6.18.1.1.3.17", "Application Control"),
			createStringPDU("1.3.6.1.4.1.2620.1.6.18.1.1.4.17", "about-to-expire"),
			createGauge32PDU("1.3.6.1.4.1.2620.1.6.18.1.1.5.17", 1775152800),
			createStringPDU("1.3.6.1.4.1.2620.1.6.18.1.1.6.17", "Threat prevention coverage"),
			createIntegerPDU("1.3.6.1.4.1.2620.1.6.18.1.1.7.17", 1),
			createGauge32PDU("1.3.6.1.4.1.2620.1.6.18.1.1.8.17", 100),
			createGauge32PDU("1.3.6.1.4.1.2620.1.6.18.1.1.9.17", 85),
		},
	)

	profile := mustLoadLicensingProfile(t, "checkpoint", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.2620.1.6.18.1"
	})

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
	require.Len(t, pm.HiddenMetrics, 1)

	row := licenseMetricsByID(pm.HiddenMetrics)["17"]
	require.NotNil(t, row)
	assert.EqualValues(t, 17, row.Value)
	assert.Equal(t, "Application Control", row.Tags[testTagLicenseName])
	assert.Equal(t, "about-to-expire", row.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "1775152800", row.Tags["_license_expiry_raw"])
	assert.Equal(t, "85", row.Tags[testTagLicenseUsageRaw])
	assert.Equal(t, "100", row.Tags["_license_capacity_raw"])
	assert.Equal(t, "Threat prevention coverage", row.Tags["_license_impact"])
}

func TestCollector_Collect_FortiGateLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectFortiGateLicensingWalks(mockHandler)

	profile := mustLoadLicensingProfile(t, "fortinet-fortigate", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.HasPrefix(strings.TrimPrefix(metric.Table.OID, "."), "1.3.6.1.4.1.12356.101.4.6.3.")
	})

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

	contract := byID["FortiCare Support"]
	require.NotNil(t, contract)
	assert.EqualValues(t, 1, contract.Value)
	assert.Equal(t, "FortiCare Support", contract.Tags[testTagLicenseName])
	assert.Equal(t, "Mon 11 November 2030", contract.Tags["_license_expiry_raw"])
	assert.Equal(t, "contract", contract.StaticTags["_license_type"])
	assert.Equal(t, "device", contract.StaticTags["_license_component"])

	service := byID["FortiGuard Antivirus"]
	require.NotNil(t, service)
	assert.EqualValues(t, 1, service.Value)
	assert.Equal(t, "FortiGuard Antivirus", service.Tags[testTagLicenseName])
	assert.Equal(t, "Sat Jul 26 01:00:00 2025", service.Tags["_license_expiry_raw"])
	assert.Equal(t, "1.00000", service.Tags["_license_feature"])
	assert.Equal(t, "service", service.StaticTags["_license_type"])
	assert.Equal(t, "fortiguard", service.StaticTags["_license_component"])

	accountContract := byID["FortiCare Premium"]
	require.NotNil(t, accountContract)
	assert.EqualValues(t, 1, accountContract.Value)
	assert.Equal(t, "FortiCare Premium", accountContract.Tags[testTagLicenseName])
	assert.Equal(t, "Mon 11 November 2030", accountContract.Tags["_license_expiry_raw"])
	assert.Equal(t, "account_contract", accountContract.StaticTags["_license_type"])
	assert.Equal(t, "account", accountContract.StaticTags["_license_component"])
}

func TestCollector_Collect_FortiGateLicensingProfile_DisablesTableCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectFortiGateLicensingWalks(mockHandler)
	expectFortiGateLicensingWalks(mockHandler)

	profile := mustLoadLicensingProfile(t, "fortinet-fortigate", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.HasPrefix(strings.TrimPrefix(metric.Table.OID, "."), "1.3.6.1.4.1.12356.101.4.6.3.")
	})
	for _, metric := range profile.Definition.Metrics {
		assert.True(t, metric.DisableTableCache)
	}

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})
	collector.tableCache.setTTL(30*time.Second, 0)

	_, err := collector.Collect()
	require.NoError(t, err)

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	pm := results[0]
	assert.Empty(t, pm.Metrics)
	require.Len(t, pm.HiddenMetrics, 3)

	byID := licenseMetricsByID(pm.HiddenMetrics)
	require.NotNil(t, byID["FortiCare Support"])
	require.NotNil(t, byID["FortiGuard Antivirus"])
	require.NotNil(t, byID["FortiCare Premium"])
}

func mustLoadLicensingProfile(t *testing.T, profileName string, keep func(metric ddprofiledefinition.MetricsConfig) bool) *ddsnmp.Profile {
	t.Helper()

	profiles := ddsnmp.FindProfiles("", "", []string{profileName})
	require.Len(t, profiles, 1)

	profile := profiles[0]
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.VirtualMetrics = nil
	profile.Definition.Metrics = slices.DeleteFunc(profile.Definition.Metrics, func(metric ddprofiledefinition.MetricsConfig) bool {
		return !keep(metric)
	})

	require.NotEmpty(t, profile.Definition.Metrics)
	assert.Equal(t, profileName+".yaml", filepath.Base(profile.SourceFile))

	return profile
}

func expectFortiGateLicensingWalks(mockHandler *snmpmock.MockHandler) {
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.12356.101.4.6.3.1.2",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.1.2.1.1.1", "FortiCare Support"),
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.1.2.1.2.1", "Mon 11 November 2030"),
		},
	)
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.12356.101.4.6.3.2.2",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.2.2.1.1.2", "FortiGuard Antivirus"),
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.2.2.1.2.2", "Sat Jul 26 01:00:00 2025"),
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.2.2.1.3.2", "1.00000"),
		},
	)
	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.12356.101.4.6.3.3.2",
		[]gosnmp.SnmpPDU{
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.3.2.1.1.7", "FortiCare Premium"),
			createStringPDU("1.3.6.1.4.1.12356.101.4.6.3.3.2.1.2.7", "Mon 11 November 2030"),
		},
	)
}
