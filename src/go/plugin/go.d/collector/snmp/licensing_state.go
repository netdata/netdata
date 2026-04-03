// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strings"
	"time"
)

type licenseStateBucket string

const (
	licenseStateBucketHealthy  licenseStateBucket = "healthy"
	licenseStateBucketDegraded licenseStateBucket = "degraded"
	licenseStateBucketBroken   licenseStateBucket = "broken"
	licenseStateBucketIgnored  licenseStateBucket = "ignored"
)

func normalizeLicenseStateBucket(row licenseRow, now time.Time) licenseStateBucket {
	rawBucket, hasRawBucket := mapLicenseStateBucket(row.StateRaw)

	if hasRawBucket && rawBucket == licenseStateBucketBroken {
		return rawBucket
	}
	if row.HasState && row.StateSeverity >= 2 {
		return licenseStateBucketBroken
	}
	if licenseRowHasBrokenTimerOrUsage(row, now) {
		return licenseStateBucketBroken
	}
	if hasRawBucket {
		switch rawBucket {
		case licenseStateBucketIgnored, licenseStateBucketDegraded, licenseStateBucketHealthy:
			return rawBucket
		}
	}
	if row.HasGraceTime {
		return licenseStateBucketDegraded
	}
	if row.HasState {
		return bucketFromSeverity(row.StateSeverity)
	}
	if row.HasExpiry && !row.IsPerpetual {
		return licenseStateBucketHealthy
	}
	if row.HasAuthorizationTime {
		return licenseStateBucketHealthy
	}
	if row.HasCertificateTime {
		return licenseStateBucketHealthy
	}
	if row.HasUsagePct || row.HasUsage || row.HasCapacity || row.HasAvailable {
		return licenseStateBucketHealthy
	}
	if row.IsPerpetual || row.IsUnlimited {
		return licenseStateBucketHealthy
	}
	if strings.TrimSpace(row.StateRaw) != "" {
		return licenseStateBucketDegraded
	}
	return licenseStateBucketIgnored
}

func licenseRowHasBrokenTimerOrUsage(row licenseRow, now time.Time) bool {
	if row.HasGraceTime && row.GraceExpiry <= now.Unix() {
		return true
	}
	if row.HasExpiry && !row.IsPerpetual && row.ExpiryTS <= now.Unix() {
		return true
	}
	if row.HasAuthorizationTime && row.AuthorizationExpiry <= now.Unix() {
		return true
	}
	if row.HasCertificateTime && row.CertificateExpiry <= now.Unix() {
		return true
	}
	return !row.IsUnlimited && row.HasUsagePct && row.UsagePercent >= 100
}

func bucketFromSeverity(sev int64) licenseStateBucket {
	switch sev {
	case 2:
		return licenseStateBucketBroken
	case 1:
		return licenseStateBucketDegraded
	default:
		return licenseStateBucketHealthy
	}
}

func mapLicenseStateBucket(raw string) (licenseStateBucket, bool) {
	state := strings.ToLower(strings.TrimSpace(raw))
	if state == "" {
		return "", false
	}

	ignoredHints := []string{
		"ignored",
		"not_subscribed",
		"not subscribed",
		"not-applicable",
		"not_applicable",
		"not applicable",
		"none",
		"n/a",
	}
	for _, hint := range ignoredHints {
		if strings.Contains(state, hint) {
			return licenseStateBucketIgnored, true
		}
	}

	criticalHints := []string{
		"expired",
		"invalid",
		"unauthorized",
		"out-of-compliance",
		"out_of_compliance",
		"not-associated",
		"disabled",
		"deactivated",
		"failed",
		"error",
		"violation",
		"broken",
	}
	for _, hint := range criticalHints {
		if strings.Contains(state, hint) {
			return licenseStateBucketBroken, true
		}
	}

	warnHints := []string{
		"about-to-expire",
		"about_to_expire",
		"warning",
		"degrade",
		"grace",
		"evaluation",
		"eval",
		"trial",
		"overage",
		"partial",
		"unknown",
	}
	for _, hint := range warnHints {
		if strings.Contains(state, hint) {
			return licenseStateBucketDegraded, true
		}
	}

	healthyHints := []string{
		"valid",
		"active",
		"authorized",
		"compliant",
		"ok",
		"subscribed",
		"registered",
		"in_use",
		"in-use",
		"up-to-date",
		"up_to_date",
	}
	for _, hint := range healthyHints {
		if strings.Contains(state, hint) {
			return licenseStateBucketHealthy, true
		}
	}

	return "", false
}
