// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Collect_CiscoTraditionalLicensingProfile_Fixture(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	fixture := mustLoadSNMPFixture(t, "testdata/licensing/cisco-traditional.snmpwalk")
	expectSNMPWalkFromFixture(mockHandler, gosnmp.Version2c, fixture, "1.3.6.1.4.1.9.9.543.1.2.3.1")

	profile := mustLoadLicensingProfile(t, "cisco", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.9.9.543.1.2.3.1"
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
	require.Len(t, pm.HiddenMetrics, 2)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	permanent := byID["1"]
	require.NotNil(t, permanent)
	assert.EqualValues(t, 0, permanent.Value)
	assert.Equal(t, "ipbasek9", permanent.Tags[testTagLicenseName])
	assert.Equal(t, "permanent", permanent.Tags["_license_type"])
	assert.Equal(t, "in_use", permanent.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "4294967295", permanent.Tags["_license_capacity_raw"])
	assert.Equal(t, "4294967295", permanent.Tags["_license_available_raw"])
	assert.Equal(t, "true", permanent.Tags["_license_perpetual"])

	feature := byID["8"]
	require.NotNil(t, feature)
	assert.EqualValues(t, 5184000, feature.Value)
	assert.Equal(t, "cme-srst", feature.Tags[testTagLicenseName])
	assert.Equal(t, "eval_right_to_use", feature.Tags["_license_type"])
	assert.Equal(t, "not_in_use", feature.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "5000", feature.Tags["_license_capacity_raw"])
	assert.Equal(t, "5000", feature.Tags["_license_available_raw"])
}

func TestCollector_Collect_CheckPointLicensingProfile_Fixture(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	fixture := mustLoadSNMPFixture(t, "testdata/licensing/checkpoint.snmprec")
	expectSNMPWalkFromFixture(mockHandler, gosnmp.Version2c, fixture, "1.3.6.1.4.1.2620.1.6.18.1")

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
	require.Len(t, pm.HiddenMetrics, 2)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	first := byID["35886"]
	require.NotNil(t, first)
	assert.EqualValues(t, 35886, first.Value)
	assert.Equal(t, "35886", first.Tags[testTagLicenseID])
	assert.Equal(t, "42214", first.Tags["_license_index"])
	assert.Equal(t, "21", first.Tags["_license_capacity_raw"])
	assert.Equal(t, "4", first.Tags[testTagLicenseUsageRaw])
	assert.NotEmpty(t, first.Tags[testTagLicenseName])
	assert.NotEmpty(t, first.Tags[testTagLicenseStateRaw])
	assert.NotEmpty(t, first.Tags["_license_expiry_raw"])
	assert.NotEmpty(t, first.Tags["_license_impact"])

	second := byID["20832"]
	require.NotNil(t, second)
	assert.EqualValues(t, 20832, second.Value)
	assert.Equal(t, "20832", second.Tags[testTagLicenseID])
	assert.Equal(t, "50465", second.Tags["_license_index"])
	assert.Equal(t, "8", second.Tags["_license_capacity_raw"])
	assert.Equal(t, "30", second.Tags[testTagLicenseUsageRaw])
	assert.NotEmpty(t, second.Tags[testTagLicenseName])
	assert.NotEmpty(t, second.Tags[testTagLicenseStateRaw])
	assert.NotEmpty(t, second.Tags["_license_expiry_raw"])
	assert.NotEmpty(t, second.Tags["_license_impact"])
}

func TestCollector_Collect_CheckPointLicensingProfile_CommunitySample(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	fixture := mustLoadSNMPFixture(t, "testdata/licensing/checkpoint-community.snmpwalk")
	expectSNMPWalkFromFixture(mockHandler, gosnmp.Version2c, fixture, "1.3.6.1.4.1.2620.1.6.18.1")

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
	require.Len(t, pm.HiddenMetrics, 5)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	firewall := byID["0"]
	require.NotNil(t, firewall)
	assert.EqualValues(t, 0, firewall.Value)
	assert.Equal(t, "Firewall", firewall.Tags[testTagLicenseName])
	assert.Equal(t, "Not Entitled", firewall.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "4294967295", firewall.Tags["_license_expiry_raw"])

	appCtrl := byID["4"]
	require.NotNil(t, appCtrl)
	assert.EqualValues(t, 4, appCtrl.Value)
	assert.Equal(t, "Application Ctrl", appCtrl.Tags[testTagLicenseName])
	assert.Equal(t, "Evaluation", appCtrl.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "1619246913", appCtrl.Tags["_license_expiry_raw"])

	urlFiltering := byID["5"]
	require.NotNil(t, urlFiltering)
	assert.Equal(t, "URL Filtering", urlFiltering.Tags[testTagLicenseName])
	assert.Equal(t, "1619246913", urlFiltering.Tags["_license_expiry_raw"])

	ips := byID["2"]
	require.NotNil(t, ips)
	assert.Equal(t, "IPS", ips.Tags[testTagLicenseName])
	assert.Equal(t, "1619246941", ips.Tags["_license_expiry_raw"])

	smartEvent := byID["1003"]
	require.NotNil(t, smartEvent)
	assert.Equal(t, "SmartEvent", smartEvent.Tags[testTagLicenseName])
	assert.Equal(t, "1620542985", smartEvent.Tags["_license_expiry_raw"])
}

func TestCollector_Collect_CiscoSmartLicensingProfile_Fixture(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	fixture := mustLoadSNMPFixture(t, "testdata/licensing/cisco-smart-iosxe-c9800.snmprec")
	expectSNMPGetFromFixture(mockHandler, fixture, []string{
		"1.3.6.1.4.1.9.9.831.0.6.1",
		"1.3.6.1.4.1.9.9.831.0.7.2",
		"1.3.6.1.4.1.9.9.831.0.7.1",
		"1.3.6.1.4.1.9.9.831.0.6.3",
		"1.3.6.1.4.1.9.9.831.0.7.4.2",
	})
	expectSNMPWalkFromFixture(mockHandler, gosnmp.Version2c, fixture, "1.3.6.1.4.1.9.9.831.0.5.1")

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
	require.Len(t, pm.HiddenMetrics, 2)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	dna := byID["DNA_NWSTACK_E"]
	require.NotNil(t, dna)
	assert.EqualValues(t, 55, dna.Value)
	assert.Equal(t, "air-network-essentials", dna.Tags[testTagLicenseName])
	assert.Equal(t, "in_use", dna.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "55", dna.Tags[testTagLicenseUsageRaw])

	airDNA := byID["AIR-DNA-E"]
	require.NotNil(t, airDNA)
	assert.EqualValues(t, 55, airDNA.Value)
	assert.Equal(t, "air-dna-essentials", airDNA.Tags[testTagLicenseName])
	assert.Equal(t, "in_use", airDNA.Tags[testTagLicenseStateRaw])

	assert.NotContains(t, byID, "smart_registration")
	assert.NotContains(t, byID, "smart_authorization_state")
	assert.NotContains(t, byID, "smart_authorization_expiry")
}

func TestCollector_Collect_SophosLicensingProfile_Fixture(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	fixture := mustLoadSNMPFixture(t, "testdata/licensing/sophos-xgs-firewall.snmprec")

	sophosOIDs := []string{
		"1.3.6.1.4.1.2604.5.1.5.1.1.0", "1.3.6.1.4.1.2604.5.1.5.1.2.0",
		"1.3.6.1.4.1.2604.5.1.5.2.1.0", "1.3.6.1.4.1.2604.5.1.5.2.2.0",
		"1.3.6.1.4.1.2604.5.1.5.3.1.0", "1.3.6.1.4.1.2604.5.1.5.3.2.0",
		"1.3.6.1.4.1.2604.5.1.5.4.1.0", "1.3.6.1.4.1.2604.5.1.5.4.2.0",
		"1.3.6.1.4.1.2604.5.1.5.5.1.0", "1.3.6.1.4.1.2604.5.1.5.5.2.0",
		"1.3.6.1.4.1.2604.5.1.5.6.1.0", "1.3.6.1.4.1.2604.5.1.5.6.2.0",
		"1.3.6.1.4.1.2604.5.1.5.7.1.0", "1.3.6.1.4.1.2604.5.1.5.7.2.0",
		"1.3.6.1.4.1.2604.5.1.5.8.1.0", "1.3.6.1.4.1.2604.5.1.5.8.2.0",
		"1.3.6.1.4.1.2604.5.1.5.9.1.0", "1.3.6.1.4.1.2604.5.1.5.9.2.0",
	}
	expectSNMPGetFromFixture(mockHandler, fixture, sophosOIDs[:10])
	expectSNMPGetFromFixture(mockHandler, fixture, sophosOIDs[10:])

	profile := mustLoadLicensingProfile(t, "sophos-xgs-firewall", func(metric ddprofiledefinition.MetricsConfig) bool {
		return metric.MIB == "SFOS-FIREWALL-MIB" && strings.HasPrefix(strings.TrimPrefix(metric.Symbol.OID, "."), "1.3.6.1.4.1.2604.5.1.5.")
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
	require.Len(t, pm.HiddenMetrics, 9)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	base := byID["base_firewall"]
	require.NotNil(t, base)
	assert.EqualValues(t, 2, base.Value)
	assert.Equal(t, "deactivated", base.Tags[testTagLicenseStateRaw])
	assert.NotEmpty(t, base.Tags["_license_expiry_raw"])

	network := byID["network_protection"]
	require.NotNil(t, network)
	assert.EqualValues(t, 0, network.Value)
	assert.Equal(t, "not_subscribed", network.Tags[testTagLicenseStateRaw])

	web := byID["web_protection"]
	require.NotNil(t, web)
	assert.EqualValues(t, 1, web.Value)
	assert.Equal(t, "trial", web.Tags[testTagLicenseStateRaw])

	mail := byID["mail_protection"]
	require.NotNil(t, mail)
	assert.EqualValues(t, 0, mail.Value)
	assert.Equal(t, "subscribed", mail.Tags[testTagLicenseStateRaw])

	central := byID["central_orchestration"]
	require.NotNil(t, central)
	assert.EqualValues(t, 0, central.Value)
	assert.Equal(t, "none", central.Tags[testTagLicenseStateRaw])
	assert.NotEmpty(t, central.Tags["_license_expiry_raw"])
}
