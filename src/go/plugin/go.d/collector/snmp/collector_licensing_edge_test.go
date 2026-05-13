// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	ddsnmpcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestCollector_Collect_LicensingAggregation_ReadsTypedRowsAndIgnoresPrivateMetrics(t *testing.T) {
	tests := map[string]struct {
		profileMetrics *ddsnmp.ProfileMetrics
		want           map[string]int64
	}{
		"typed rows drive aggregate while private metrics are ignored": {
			profileMetrics: &ddsnmp.ProfileMetrics{
				Source: "noise-licensing.yaml",
				LicenseRows: []ddsnmp.LicenseRow{
					typedLicenseRow("healthy", "Healthy license", withState(0, "active")),
				},
				HiddenMetrics: []ddsnmp.Metric{
					{
						Name:  "_private_metric",
						Value: 0,
						Tags: map[string]string{
							"state": "active",
						},
					},
					{
						Name:  "_private_metric_total",
						Value: 123,
						Tags: map[string]string{
							"component": "noise",
						},
					},
					{
						Name:  "_private_helper",
						Value: 999,
						Tags: map[string]string{
							"component": "helper",
						},
					},
				},
			},
			want: map[string]int64{
				metricIDLicenseStateHealthy:       1,
				metricIDLicenseStateInformational: 0,
				metricIDLicenseStateDegraded:      0,
				metricIDLicenseStateBroken:        0,
				metricIDLicenseStateIgnored:       0,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
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
				pm := tc.profileMetrics
				for i := range pm.HiddenMetrics {
					pm.HiddenMetrics[i].Profile = pm
				}
				return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{pm}}
			}

			require.NoError(t, collr.Init(context.Background()))
			_ = collr.Check(context.Background())

			got := collr.Collect(context.Background())
			require.NotNil(t, got)

			for id, want := range tc.want {
				assert.EqualValues(t, want, got[id])
			}
			assert.NotContains(t, got, metricIDLicenseRemainingTime)
			assert.NotContains(t, got, metricIDLicenseAuthorizationRemainingTime)
			assert.NotContains(t, got, metricIDLicenseCertificateRemainingTime)
			assert.NotContains(t, got, metricIDLicenseGraceRemainingTime)
			assert.NotContains(t, got, metricIDLicenseUsagePercent)
		})
	}
}
