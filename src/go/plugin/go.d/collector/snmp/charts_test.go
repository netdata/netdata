// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollector_AddProfileScalarMetricChart_LabelsIncludeMetricTags(t *testing.T) {
	collr := New()
	collr.Hostname = "192.0.2.1"
	collr.sysInfo = &snmputils.SysInfo{
		Name:   "test-device",
		Vendor: "test-vendor",
	}

	collr.addProfileScalarMetricChart(ddsnmp.Metric{
		Name:  "license.status",
		Value: 1,
		Tags: map[string]string{
			"component":          "vpn",
			"_component":         "private-vpn",
			"_license_state_raw": "active",
		},
		Profile: &ddsnmp.ProfileMetrics{
			Tags: map[string]string{"profile_tag": "profile_value"},
		},
	})

	chart := collr.Charts().Get("snmp_device_prof_license_status")
	require.NotNil(t, chart)

	assert.Equal(t, map[string]string{
		"address":           "192.0.2.1",
		"component":         "vpn",
		"license_state_raw": "active",
		"profile_tag":       "profile_value",
		"sysName":           "test-device",
		"vendor":            "test-vendor",
	}, chartLabels(chart))
}

func TestAddMetricTagLabels_PrefersUnprefixedTags(t *testing.T) {
	labels := map[string]string{
		"empty_profile_label": "",
		"profile_tag":         "profile-value",
		"vendor":              "device-vendor",
	}

	addMetricTagLabels(labels, map[string]string{
		"component":            "vpn",
		"empty_metric_label":   "",
		"profile_tag":          "metric-profile-value",
		"vendor":               "",
		"_component":           "private-vpn",
		"_empty_metric_label":  "metric-fallback",
		"_empty_profile_label": "profile-fallback",
		"_license_state_raw":   "active",
		"_vendor":              "private-vendor",
		"_":                    "ignored",
	})

	assert.Equal(t, map[string]string{
		"component":           "vpn",
		"empty_metric_label":  "metric-fallback",
		"empty_profile_label": "profile-fallback",
		"license_state_raw":   "active",
		"profile_tag":         "profile-value",
		"vendor":              "device-vendor",
	}, labels)
}

func TestLicenseChartsSkipGaps(t *testing.T) {
	tests := map[string]struct {
		skip bool
	}{
		licenseRemainingTimeChart.ID:              {skip: licenseRemainingTimeChart.SkipGaps},
		licenseAuthorizationRemainingTimeChart.ID: {skip: licenseAuthorizationRemainingTimeChart.SkipGaps},
		licenseCertificateRemainingTimeChart.ID:   {skip: licenseCertificateRemainingTimeChart.SkipGaps},
		licenseGraceRemainingTimeChart.ID:         {skip: licenseGraceRemainingTimeChart.SkipGaps},
		licenseUsagePercentChart.ID:               {skip: licenseUsagePercentChart.SkipGaps},
		licenseStateChart.ID:                      {skip: licenseStateChart.SkipGaps},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.True(t, tc.skip, "chart %s must skip gaps to avoid empty licensing charts", name)
		})
	}
}

func TestCollector_AddLicenseCharts_LazyBySignalClass(t *testing.T) {
	tests := map[string]struct {
		agg     licenseAggregate
		present []string
		absent  []string
	}{
		"state only": {
			agg: licenseAggregate{hasStateCounts: true},
			present: []string{
				licenseStateChart.ID,
			},
			absent: []string{
				licenseRemainingTimeChart.ID,
				licenseAuthorizationRemainingTimeChart.ID,
				licenseCertificateRemainingTimeChart.ID,
				licenseGraceRemainingTimeChart.ID,
				licenseUsagePercentChart.ID,
			},
		},
		"expiry and usage only": {
			agg: licenseAggregate{hasRemainingTime: true, hasUsagePercent: true},
			present: []string{
				licenseRemainingTimeChart.ID,
				licenseUsagePercentChart.ID,
			},
			absent: []string{
				licenseAuthorizationRemainingTimeChart.ID,
				licenseCertificateRemainingTimeChart.ID,
				licenseGraceRemainingTimeChart.ID,
				licenseStateChart.ID,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.sysInfo = &snmputils.SysInfo{}
			collr.addLicenseCharts(tc.agg)
			collr.addLicenseCharts(tc.agg)

			for _, id := range tc.present {
				chart := collr.Charts().Get(id)
				require.NotNil(t, chart, "expected chart %s", id)
				assert.Equal(t, "licensing", chartLabels(chart)["component"])
			}
			for _, id := range tc.absent {
				assert.Nil(t, collr.Charts().Get(id), "unexpected chart %s", id)
			}
		})
	}
}

func chartLabels(chart *collectorapi.Chart) map[string]string {
	labels := make(map[string]string, len(chart.Labels))
	for _, label := range chart.Labels {
		labels[label.Key] = label.Value
	}
	return labels
}
