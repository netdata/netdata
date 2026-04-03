// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "testing"

func TestLicenseChartsSkipGaps(t *testing.T) {
	charts := []struct {
		name  string
		chart bool
	}{
		{name: licenseRemainingTimeChart.ID, chart: licenseRemainingTimeChart.SkipGaps},
		{name: licenseAuthorizationRemainingTimeChart.ID, chart: licenseAuthorizationRemainingTimeChart.SkipGaps},
		{name: licenseCertificateRemainingTimeChart.ID, chart: licenseCertificateRemainingTimeChart.SkipGaps},
		{name: licenseGraceRemainingTimeChart.ID, chart: licenseGraceRemainingTimeChart.SkipGaps},
		{name: licenseUsagePercentChart.ID, chart: licenseUsagePercentChart.SkipGaps},
		{name: licenseStateChart.ID, chart: licenseStateChart.SkipGaps},
	}

	for _, tc := range charts {
		if !tc.chart {
			t.Fatalf("chart %s must skip gaps to avoid empty licensing charts", tc.name)
		}
	}
}
