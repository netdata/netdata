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

func TestCollector_Collect_CiscoTraditionalLicensingProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectCiscoTraditionalLicensingWalk(mockHandler,
		ciscoTraditionalRow{
			entPhysical:   1,
			storeUsed:     1,
			index:         17,
			name:          "SECURITYK9",
			version:       "1.0",
			licenseType:   5,
			remaining:     0,
			capacity:      100,
			available:     15,
			impact:        "Security subscription",
			status:        3,
			endDate:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
			hasEndDatePDU: true,
		},
		ciscoTraditionalRow{
			entPhysical: 1,
			storeUsed:   2,
			index:       23,
			name:        "APPXK9",
			version:     "2.1",
			licenseType: 3,
			remaining:   7200,
			capacity:    10,
			available:   0,
			impact:      "Session count exhausted",
			status:      6,
		},
	)

	profile := mustLoadLicensingProfile(t, "cisco", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.9.9.543.1.2.3.1"
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
	require.Len(t, pm.HiddenMetrics, 2)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	subscription := byID["17"]
	require.NotNil(t, subscription)
	assert.EqualValues(t, 0, subscription.Value)
	assert.Equal(t, "SECURITYK9", subscription.Tags[testTagLicenseName])
	assert.Equal(t, "paid_subscription", subscription.Tags["_license_type"])
	assert.Equal(t, "in_use", subscription.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "100", subscription.Tags["_license_capacity_raw"])
	assert.Equal(t, "15", subscription.Tags["_license_available_raw"])
	assert.Equal(t, "Security subscription", subscription.Tags["_license_impact"])
	expiryRaw, err := strconv.ParseInt(subscription.Tags["_license_expiry_raw"], 10, 64)
	require.NoError(t, err)
	assert.EqualValues(t, time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), expiryRaw)

	grace := byID["23"]
	require.NotNil(t, grace)
	assert.EqualValues(t, 7200, grace.Value)
	assert.Equal(t, "APPXK9", grace.Tags[testTagLicenseName])
	assert.Equal(t, "grace_period", grace.Tags["_license_type"])
	assert.Equal(t, "usage_count_consumed", grace.Tags[testTagLicenseStateRaw])
	assert.Equal(t, "10", grace.Tags["_license_capacity_raw"])
	assert.Equal(t, "0", grace.Tags["_license_available_raw"])
}

func TestCollector_Collect_CiscoTraditionalLicensingProfile_DisablesTableCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectCiscoTraditionalLicensingWalk(mockHandler,
		ciscoTraditionalRow{
			entPhysical:   1,
			storeUsed:     1,
			index:         17,
			name:          "SECURITYK9",
			version:       "1.0",
			licenseType:   5,
			remaining:     0,
			capacity:      100,
			available:     15,
			impact:        "Security subscription",
			status:        3,
			endDate:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
			hasEndDatePDU: true,
		},
		ciscoTraditionalRow{
			entPhysical: 1,
			storeUsed:   2,
			index:       23,
			name:        "APPXK9",
			version:     "2.1",
			licenseType: 3,
			remaining:   7200,
			capacity:    10,
			available:   0,
			impact:      "Session count exhausted",
			status:      6,
		},
	)
	expectCiscoTraditionalLicensingWalk(mockHandler,
		ciscoTraditionalRow{
			entPhysical:   1,
			storeUsed:     1,
			index:         17,
			name:          "SECURITYK9",
			version:       "1.0",
			licenseType:   5,
			remaining:     0,
			capacity:      100,
			available:     5,
			impact:        "Security subscription renewed",
			status:        3,
			endDate:       time.Date(2031, time.January, 15, 0, 0, 0, 0, time.UTC),
			hasEndDatePDU: true,
		},
		ciscoTraditionalRow{
			entPhysical: 1,
			storeUsed:   2,
			index:       23,
			name:        "APPXK9",
			version:     "2.1",
			licenseType: 3,
			remaining:   3600,
			capacity:    10,
			available:   2,
			impact:      "Sessions released",
			status:      3,
		},
	)

	profile := mustLoadLicensingProfile(t, "cisco", func(metric ddprofiledefinition.MetricsConfig) bool {
		return strings.TrimPrefix(metric.Table.OID, ".") == "1.3.6.1.4.1.9.9.543.1.2.3.1"
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
	assert.Empty(t, pm.Metrics)
	require.Len(t, pm.HiddenMetrics, 2)

	byID := licenseMetricsByID(pm.HiddenMetrics)

	subscription := byID["17"]
	require.NotNil(t, subscription)
	assert.Equal(t, "Security subscription renewed", subscription.Tags["_license_impact"])
	assert.Equal(t, "5", subscription.Tags["_license_available_raw"])

	grace := byID["23"]
	require.NotNil(t, grace)
	assert.EqualValues(t, 3600, grace.Value)
	assert.Equal(t, "Sessions released", grace.Tags["_license_impact"])
	assert.Equal(t, "2", grace.Tags["_license_available_raw"])
}

type ciscoTraditionalRow struct {
	entPhysical   int
	storeUsed     int
	index         int
	name          string
	version       string
	licenseType   int
	remaining     uint
	capacity      uint
	available     uint
	impact        string
	status        int
	endDate       time.Time
	hasEndDatePDU bool
}

func expectCiscoTraditionalLicensingWalk(mockHandler *snmpmock.MockHandler, rows ...ciscoTraditionalRow) {
	pdus := make([]gosnmp.SnmpPDU, 0, len(rows)*10)

	for _, row := range rows {
		idx := strconv.Itoa(row.entPhysical) + "." + strconv.Itoa(row.storeUsed) + "." + strconv.Itoa(row.index)

		pdus = append(pdus,
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.3."+idx, row.name),
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.4."+idx, row.version),
			createIntegerPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.5."+idx, row.licenseType),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.8."+idx, row.remaining),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.10."+idx, row.capacity),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.11."+idx, row.available),
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.13."+idx, row.impact),
			createIntegerPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.14."+idx, row.status),
		)
		if row.hasEndDatePDU {
			pdus = append(pdus, createDateAndTimePDU("1.3.6.1.4.1.9.9.543.1.2.3.1.16."+idx, row.endDate))
		}
	}

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.9.9.543.1.2.3.1",
		pdus,
	)
}
