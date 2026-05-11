// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioLicenseRemainingTime = prioPingStdDev + 1 + iota
	prioLicenseAuthorizationRemainingTime
	prioLicenseCertificateRemainingTime
	prioLicenseGraceRemainingTime
	prioLicenseUsagePercent
	prioLicenseState
)

var (
	licenseRemainingTimeChart = collectorapi.Chart{
		ID:       "snmp_device_license_remaining_time",
		Title:    "License remaining time",
		Units:    "seconds",
		Fam:      "Licensing/Time",
		Ctx:      "snmp.license.remaining_time",
		Priority: prioLicenseRemainingTime,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_license_remaining_time", Name: "remaining_time"},
		},
	}
	licenseAuthorizationRemainingTimeChart = collectorapi.Chart{
		ID:       "snmp_device_license_authorization_remaining_time",
		Title:    "License authorization remaining time",
		Units:    "seconds",
		Fam:      "Licensing/Time",
		Ctx:      "snmp.license.authorization_remaining_time",
		Priority: prioLicenseAuthorizationRemainingTime,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_license_authorization_remaining_time", Name: "remaining_time"},
		},
	}
	licenseCertificateRemainingTimeChart = collectorapi.Chart{
		ID:       "snmp_device_license_certificate_remaining_time",
		Title:    "License certificate remaining time",
		Units:    "seconds",
		Fam:      "Licensing/Time",
		Ctx:      "snmp.license.certificate_remaining_time",
		Priority: prioLicenseCertificateRemainingTime,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_license_certificate_remaining_time", Name: "remaining_time"},
		},
	}
	licenseGraceRemainingTimeChart = collectorapi.Chart{
		ID:       "snmp_device_license_grace_remaining_time",
		Title:    "License grace remaining time",
		Units:    "seconds",
		Fam:      "Licensing/Time",
		Ctx:      "snmp.license.grace_remaining_time",
		Priority: prioLicenseGraceRemainingTime,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_license_grace_remaining_time", Name: "remaining_time"},
		},
	}
	licenseUsagePercentChart = collectorapi.Chart{
		ID:       "snmp_device_license_usage_percent",
		Title:    "License usage pressure",
		Units:    "percentage",
		Fam:      "Licensing/Usage",
		Ctx:      "snmp.license.usage_percent",
		Priority: prioLicenseUsagePercent,
		Type:     collectorapi.Area,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_license_usage_percent", Name: "usage_percent"},
		},
	}
	licenseStateChart = collectorapi.Chart{
		ID:       "snmp_device_license_state",
		Title:    "License state counts",
		Units:    "licenses",
		Fam:      "Licensing/State",
		Ctx:      "snmp.license.state",
		Priority: prioLicenseState,
		Type:     collectorapi.Stacked,
		SkipGaps: true,
		Dims: collectorapi.Dims{
			{ID: metricIDLicenseStateHealthy, Name: string(licenseStateBucketHealthy)},
			{ID: metricIDLicenseStateInformational, Name: string(licenseStateBucketInformational)},
			{ID: metricIDLicenseStateDegraded, Name: string(licenseStateBucketDegraded)},
			{ID: metricIDLicenseStateBroken, Name: string(licenseStateBucketBroken)},
			{ID: metricIDLicenseStateIgnored, Name: string(licenseStateBucketIgnored)},
		},
	}
)

func (c *Collector) addLicenseCharts(agg licenseAggregate) {
	if agg.hasRemainingTime {
		c.addLicenseChart(licenseRemainingTimeChart)
	}
	if agg.hasAuthRemaining {
		c.addLicenseChart(licenseAuthorizationRemainingTimeChart)
	}
	if agg.hasCertRemaining {
		c.addLicenseChart(licenseCertificateRemainingTimeChart)
	}
	if agg.hasGraceRemaining {
		c.addLicenseChart(licenseGraceRemainingTimeChart)
	}
	if agg.hasUsagePercent {
		c.addLicenseChart(licenseUsagePercentChart)
	}
	if agg.hasStateCounts {
		c.addLicenseChart(licenseStateChart)
	}
}

func (c *Collector) addLicenseChart(chart collectorapi.Chart) {
	if c.Charts().Get(chart.ID) != nil {
		return
	}

	ch := chart.Copy()
	labels := c.chartBaseLabels()
	labels["component"] = "licensing"

	for k, v := range labels {
		ch.Labels = append(ch.Labels, collectorapi.Label{Key: k, Value: v})
	}

	if err := c.Charts().Add(ch); err != nil {
		c.Warningf("failed to add license chart %q: %v", ch.ID, err)
	}
}
