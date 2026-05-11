// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strings"
	"time"
)

type licenseStateBucket string

const (
	licenseStateBucketHealthy       licenseStateBucket = "healthy"
	licenseStateBucketInformational licenseStateBucket = "informational"
	licenseStateBucketDegraded      licenseStateBucket = "degraded"
	licenseStateBucketBroken        licenseStateBucket = "broken"
	licenseStateBucketIgnored       licenseStateBucket = "ignored"
)

func normalizeLicenseStateBucket(row licenseRow, now time.Time) licenseStateBucket {
	rawBucket, hasRawBucket := mapLicenseStateBucket(row.StateRaw)
	if hasRawBucket && rawBucket == licenseStateBucketIgnored {
		return licenseStateBucketIgnored
	}

	// Hard-fail conditions: a broken-timer or fully-consumed pool is broken
	// regardless of state. These come from FRESH metric values (the table
	// cache re-fetches symbol PDUs on every poll), so they cannot be stale.
	if licenseRowHasBrokenTimerOrUsage(row, now) {
		return licenseStateBucketBroken
	}

	if row.HasGraceTime {
		return licenseStateBucketDegraded
	}

	if hasRawBucket && rawBucket == licenseStateBucketInformational {
		return licenseStateBucketInformational
	}

	// Fresh severity wins over the cached raw state string. The raw state
	// string lives in a same-table metric_tag and the SNMP table cache
	// reuses row tags on cache hits, so a renewed-or-expired license could
	// continue to read the previous text for up to the cache TTL. Severity,
	// in contrast, is collected as a symbol whose value is re-fetched on
	// every poll, so it is always current.
	if row.HasState {
		return bucketFromSeverity(row.StateSeverity)
	}

	// No fresh severity → fall back to the raw vendor state classification.
	if hasRawBucket {
		return rawBucket
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
	if strings.TrimSpace(raw) == "" {
		return "", false
	}

	if licenseStateMatchesAny(raw, licenseStateIgnoredHints) {
		return licenseStateBucketIgnored, true
	}
	if licenseStateMatchesAny(raw, licenseStateBrokenHints) {
		return licenseStateBucketBroken, true
	}
	if licenseStateMatchesAny(raw, licenseStateInformationalHints) {
		return licenseStateBucketInformational, true
	}
	if licenseStateMatchesAny(raw, licenseStateDegradedHints) {
		return licenseStateBucketDegraded, true
	}
	if licenseStateMatchesAny(raw, licenseStateHealthyHints) {
		return licenseStateBucketHealthy, true
	}

	return "", false
}
