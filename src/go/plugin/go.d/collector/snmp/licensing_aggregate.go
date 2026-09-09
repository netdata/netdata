// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"time"
)

type licenseAggregate struct {
	remainingTime      int64
	hasRemainingTime   bool
	authRemainingTime  int64
	hasAuthRemaining   bool
	certRemainingTime  int64
	hasCertRemaining   bool
	graceRemainingTime int64
	hasGraceRemaining  bool
	usagePercent       int64
	hasUsagePercent    bool
	stateHealthy       int64
	stateInformational int64
	stateDegraded      int64
	stateBroken        int64
	stateIgnored       int64
	hasStateCounts     bool
}

func aggregateLicenseRows(rows []licenseRow, now time.Time) licenseAggregate {
	var agg licenseAggregate

	for _, row := range rows {
		switch row.StateBucket {
		case licenseStateBucketHealthy:
			agg.stateHealthy++
			agg.hasStateCounts = true
		case licenseStateBucketInformational:
			agg.stateInformational++
			agg.hasStateCounts = true
		case licenseStateBucketDegraded:
			agg.stateDegraded++
			agg.hasStateCounts = true
		case licenseStateBucketBroken:
			agg.stateBroken++
			agg.hasStateCounts = true
		case licenseStateBucketIgnored:
			agg.stateIgnored++
			agg.hasStateCounts = true
		}

		if row.StateBucket == licenseStateBucketIgnored {
			continue
		}

		if row.HasExpiry && !row.IsPerpetual {
			agg.setMin(row.ExpiryTS-now.Unix(), &agg.remainingTime, &agg.hasRemainingTime)
		}
		if row.HasAuthorizationTime {
			agg.setMin(row.AuthorizationExpiry-now.Unix(), &agg.authRemainingTime, &agg.hasAuthRemaining)
		}
		if row.HasCertificateTime {
			agg.setMin(row.CertificateExpiry-now.Unix(), &agg.certRemainingTime, &agg.hasCertRemaining)
		}
		if row.HasGraceTime {
			agg.setMin(row.GraceExpiry-now.Unix(), &agg.graceRemainingTime, &agg.hasGraceRemaining)
		}
		if row.HasUsagePct && !row.IsUnlimited {
			pct := int64(math.Round(row.UsagePercent))
			if !agg.hasUsagePercent || pct > agg.usagePercent {
				agg.usagePercent = pct
				agg.hasUsagePercent = true
			}
		}
	}

	return agg
}

func (agg *licenseAggregate) setMin(value int64, current *int64, seen *bool) {
	if !*seen || value < *current {
		*current = value
		*seen = true
	}
}

func (agg licenseAggregate) empty() bool {
	return !agg.hasRemainingTime &&
		!agg.hasAuthRemaining &&
		!agg.hasCertRemaining &&
		!agg.hasGraceRemaining &&
		!agg.hasUsagePercent &&
		!agg.hasStateCounts
}

func (agg licenseAggregate) writeTo(mx map[string]int64) {
	if agg.hasRemainingTime {
		mx[metricIDLicenseRemainingTime] = agg.remainingTime
	}
	if agg.hasAuthRemaining {
		mx[metricIDLicenseAuthorizationRemainingTime] = agg.authRemainingTime
	}
	if agg.hasCertRemaining {
		mx[metricIDLicenseCertificateRemainingTime] = agg.certRemainingTime
	}
	if agg.hasGraceRemaining {
		mx[metricIDLicenseGraceRemainingTime] = agg.graceRemainingTime
	}
	if agg.hasUsagePercent {
		mx[metricIDLicenseUsagePercent] = agg.usagePercent
	}
	if agg.hasStateCounts {
		mx[metricIDLicenseStateHealthy] = agg.stateHealthy
		mx[metricIDLicenseStateInformational] = agg.stateInformational
		mx[metricIDLicenseStateDegraded] = agg.stateDegraded
		mx[metricIDLicenseStateBroken] = agg.stateBroken
		mx[metricIDLicenseStateIgnored] = agg.stateIgnored
	}
}
