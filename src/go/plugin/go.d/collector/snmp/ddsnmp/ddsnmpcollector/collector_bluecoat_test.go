// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strconv"
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

func TestCollector_Collect_BlueCoatLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectBlueCoatLicensingWalk(mockHandler,
		blueCoatLicenseRow{
			index:        1,
			application:  "ProxySG",
			feature:      "WebFilter",
			component:    "policy-engine",
			expireType:   2,
			state:        1,
			expiry:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
			hasExpiryPDU: true,
		},
		blueCoatLicenseRow{
			index:        2,
			application:  "ProxySG",
			feature:      "Demo Malware Analysis",
			component:    "sandbox",
			expireType:   3,
			state:        2,
			expiry:       time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
			hasExpiryPDU: true,
		},
		blueCoatLicenseRow{
			index:       3,
			application: "ProxySG",
			feature:     "Base Platform",
			component:   "proxy-core",
			expireType:  1,
			state:       1,
		},
	)

	profile := mustLoadLicensingProfile(t, "bluecoat-proxysg", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.3417.2.16.1.1.1"
	})
	require.Len(t, profile.Definition.Metrics, 1)
	assert.True(t, profile.Definition.Metrics[0].DisableTableCache)

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

	subscription := byID["1"]
	require.NotNil(t, subscription)
	assert.EqualValues(t, 1, subscription.Value)
	assert.Equal(t, "ProxySG", subscription.Tags[testTagLicenseName])
	assert.Equal(t, "WebFilter", subscription.Tags["_license_feature"])
	assert.Equal(t, "policy-engine", subscription.Tags["_license_component"])
	assert.Equal(t, "subscription", subscription.Tags["_license_type"])
	assert.Equal(t, "active", subscription.Tags[testTagLicenseStateRaw])
	expiryRaw, err := strconv.ParseInt(subscription.Tags["_license_expiry_raw"], 10, 64)
	require.NoError(t, err)
	assert.EqualValues(t, time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), expiryRaw)

	expiredDemo := byID["2"]
	require.NotNil(t, expiredDemo)
	assert.Equal(t, "demo", expiredDemo.Tags["_license_type"])
	assert.Equal(t, "expired", expiredDemo.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "2", expiredDemo.Tags["_license_state_severity_raw"])

	perpetual := byID["3"]
	require.NotNil(t, perpetual)
	assert.Equal(t, "perpetual", perpetual.Tags["_license_type"])
	assert.Equal(t, "true", perpetual.Tags["_license_perpetual"])
	assert.Equal(t, "active", perpetual.Tags[testTagLicenseStateRaw])
	_, ok := perpetual.Tags["_license_expiry_raw"]
	assert.False(t, ok)
}

func TestCollector_Collect_BlueCoatLicensingProfile_DisablesTableCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectBlueCoatLicensingWalk(mockHandler,
		blueCoatLicenseRow{
			index:        1,
			application:  "ProxySG",
			feature:      "WebFilter",
			component:    "policy-engine",
			expireType:   2,
			state:        1,
			expiry:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
			hasExpiryPDU: true,
		},
	)
	expectBlueCoatLicensingWalk(mockHandler,
		blueCoatLicenseRow{
			index:        1,
			application:  "ProxySG",
			feature:      "WebFilter",
			component:    "policy-engine",
			expireType:   2,
			state:        2,
			expiry:       time.Date(2031, time.January, 15, 0, 0, 0, 0, time.UTC),
			hasExpiryPDU: true,
		},
	)

	profile := mustLoadLicensingProfile(t, "bluecoat-proxysg", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.3417.2.16.1.1.1"
	})
	require.Len(t, profile.Definition.Metrics, 1)
	assert.True(t, profile.Definition.Metrics[0].DisableTableCache)

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
	require.Len(t, pm.HiddenMetrics, 1)

	row := licenseMetricsByID(pm.HiddenMetrics)["1"]
	require.NotNil(t, row)
	assert.Equal(t, "expired", row.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "2", row.Tags["_license_state_severity_raw"])
	expiryRaw, err := strconv.ParseInt(row.Tags["_license_expiry_raw"], 10, 64)
	require.NoError(t, err)
	assert.EqualValues(t, time.Date(2031, time.January, 15, 0, 0, 0, 0, time.UTC).Unix(), expiryRaw)
}

type blueCoatLicenseRow struct {
	index        int
	application  string
	feature      string
	component    string
	expireType   int
	state        int
	expiry       time.Time
	hasExpiryPDU bool
}

func expectBlueCoatLicensingWalk(mockHandler *snmpmock.MockHandler, rows ...blueCoatLicenseRow) {
	pdus := make([]gosnmp.SnmpPDU, 0, len(rows)*7)

	for _, row := range rows {
		idx := strconv.Itoa(row.index)

		pdus = append(pdus,
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.1."+idx, row.index),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.2."+idx, row.application),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.3."+idx, row.feature),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.4."+idx, row.component),
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.5."+idx, row.expireType),
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.7."+idx, row.state),
		)
		if row.hasExpiryPDU {
			pdus = append(pdus, createDateAndTimePDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.6."+idx, row.expiry))
		}
	}

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.3417.2.16.1.1.1",
		pdus,
	)
}
