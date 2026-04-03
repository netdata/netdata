// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	ddsnmpcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestCollector_Collect_LicensingAggregation_IgnoresNoiseOnlyRows(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		pm := &ddsnmp.ProfileMetrics{
			Source: "noise-licensing.yaml",
			HiddenMetrics: []ddsnmp.Metric{
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:        "zero-expiry",
						tagLicenseName:      "Zero expiry placeholder",
						tagLicenseExpiryRaw: "0",
						tagLicenseStateRaw:  "active",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:        "perpetual",
						tagLicenseName:      "Perpetual entitlement",
						tagLicenseExpiryRaw: "1",
						tagLicensePerpetual: "true",
						tagLicenseStateRaw:  "active",
					},
				},
				{
					Name:  licenseSourceMetricName,
					Value: 0,
					Tags: map[string]string{
						tagLicenseID:              "unlimited",
						tagLicenseName:            "Unlimited pool",
						tagLicenseUnlimited:       "true",
						tagLicenseUsagePercentRaw: "100",
						tagLicenseStateRaw:        "active",
					},
				},
			},
		}
		for i := range pm.HiddenMetrics {
			pm.HiddenMetrics[i].Profile = pm
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	got := collr.Collect(context.Background())
	require.NotNil(t, got)

	assert.EqualValues(t, 3, got[metricIDLicenseStateHealthy])
	assert.EqualValues(t, 0, got[metricIDLicenseStateDegraded])
	assert.EqualValues(t, 0, got[metricIDLicenseStateBroken])
	assert.EqualValues(t, 0, got[metricIDLicenseStateIgnored])
	assert.NotContains(t, got, metricIDLicenseRemainingTime)
	assert.NotContains(t, got, metricIDLicenseAuthorizationRemainingTime)
	assert.NotContains(t, got, metricIDLicenseCertificateRemainingTime)
	assert.NotContains(t, got, metricIDLicenseGraceRemainingTime)
	assert.NotContains(t, got, metricIDLicenseUsagePercent)
}
