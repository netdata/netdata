// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	licenseSourceMetricName = "_license_row"

	tagLicenseIndex               = "_license_index"
	tagLicenseID                  = "_license_id"
	tagLicenseName                = "_license_name"
	tagLicenseFeature             = "_license_feature"
	tagLicenseComponent           = "_license_component"
	tagLicenseType                = "_license_type"
	tagLicenseImpact              = "_license_impact"
	tagLicenseStateRaw            = "_license_state_raw"
	tagLicenseStateSeverityRaw    = "_license_state_severity_raw"
	tagLicenseActiveRaw           = "_license_active_raw"
	tagLicenseExpiryRaw           = "_license_expiry_raw"
	tagLicenseAuthorizationRaw    = "_license_authorization_raw"
	tagLicenseCertificateRaw      = "_license_certificate_raw"
	tagLicenseGraceRaw            = "_license_grace_raw"
	tagLicenseUsageRaw            = "_license_usage_raw"
	tagLicenseCapacityRaw         = "_license_capacity_raw"
	tagLicenseAvailableRaw        = "_license_available_raw"
	tagLicenseUnlimited           = "_license_unlimited"
	tagLicensePerpetual           = "_license_perpetual"
	tagLicenseUsagePercentRaw     = "_license_usage_percent_raw"
	tagLicenseAuthorizationSource = "_license_authorization_source"
	tagLicenseCertificateSource   = "_license_certificate_source"
	tagLicenseGraceSource         = "_license_grace_source"
	tagLicenseExpirySource        = "_license_expiry_source"
	tagLicenseValueKind           = "_license_value_kind"

	licenseValueKindStateSeverity          = "state_severity"
	licenseValueKindExpiryTimestamp        = "expiry_timestamp"
	licenseValueKindAuthorizationTimestamp = "authorization_timestamp"
	licenseValueKindCertificateTimestamp   = "certificate_timestamp"
	licenseValueKindGraceTimestamp         = "grace_timestamp"
	licenseValueKindExpiryRemaining        = "expiry_remaining"
	licenseValueKindAuthorizationRemaining = "authorization_remaining"
	licenseValueKindCertificateRemaining   = "certificate_remaining"
	licenseValueKindGraceRemaining         = "grace_remaining"
	licenseValueKindUsage                  = "usage"
	licenseValueKindCapacity               = "capacity"
	licenseValueKindUsagePercent           = "usage_percent"

	metricIDLicenseRemainingTime              = "snmp_device_license_remaining_time"
	metricIDLicenseAuthorizationRemainingTime = "snmp_device_license_authorization_remaining_time"
	metricIDLicenseCertificateRemainingTime   = "snmp_device_license_certificate_remaining_time"
	metricIDLicenseGraceRemainingTime         = "snmp_device_license_grace_remaining_time"
	metricIDLicenseUsagePercent               = "snmp_device_license_usage_percent"
	metricIDLicenseStateHealthy               = "snmp_device_license_state_healthy"
	metricIDLicenseStateDegraded              = "snmp_device_license_state_degraded"
	metricIDLicenseStateBroken                = "snmp_device_license_state_broken"
	metricIDLicenseStateIgnored               = "snmp_device_license_state_ignored"
)

type licenseCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	rows       []licenseRow
}

type licenseRow struct {
	ID     string
	Source string
	Name   string

	Feature   string
	Component string
	Type      string
	Impact    string

	StateRaw      string
	StateSeverity int64
	HasState      bool
	StateBucket   licenseStateBucket

	ExpiryTS             int64
	HasExpiry            bool
	AuthorizationExpiry  int64
	HasAuthorizationTime bool
	CertificateExpiry    int64
	HasCertificateTime   bool
	GraceExpiry          int64
	HasGraceTime         bool

	Usage          int64
	HasUsage       bool
	Capacity       int64
	HasCapacity    bool
	Available      int64
	HasAvailable   bool
	UsagePercent   float64
	HasUsagePct    bool
	IsUnlimited    bool
	IsPerpetual    bool
	ActiveRaw      string
	ExpirySource   string
	AuthSource     string
	CertSource     string
	GraceSource    string
	OriginalMetric string
}

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
	stateDegraded      int64
	stateBroken        int64
	stateIgnored       int64
	hasStateCounts     bool
}

func newLicenseCache() *licenseCache {
	return &licenseCache{}
}

func (c *Collector) collectLicensing(mx map[string]int64, pms []*ddsnmp.ProfileMetrics) {
	now := time.Now().UTC()
	rows := extractLicenseRows(pms, now)

	if c.licenseCache != nil {
		c.licenseCache.store(now, rows)
	}

	if len(rows) == 0 {
		return
	}

	agg := aggregateLicenseRows(rows, now)
	if !agg.empty() {
		c.addLicenseCharts()
	}

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
		mx[metricIDLicenseStateDegraded] = agg.stateDegraded
		mx[metricIDLicenseStateBroken] = agg.stateBroken
		mx[metricIDLicenseStateIgnored] = agg.stateIgnored
	}
}

func (c *licenseCache) store(ts time.Time, rows []licenseRow) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastUpdate = ts
	c.rows = slices.Clone(rows)
}

func (c *licenseCache) snapshot() (time.Time, []licenseRow) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastUpdate, slices.Clone(c.rows)
}

func extractLicenseRows(pms []*ddsnmp.ProfileMetrics, now time.Time) []licenseRow {
	var rows []licenseRow

	for _, pm := range pms {
		for _, metric := range pm.HiddenMetrics {
			if !isLicenseSourceMetric(metric.Name) {
				continue
			}

			row := buildLicenseRow(metric, now)
			if row.ID != "" || row.Name != "" {
				rows = append(rows, row)
			}
		}

		// Defensive fallback for tests or older call paths that still pass helper metrics in Metrics.
		kept := pm.Metrics[:0]
		for _, metric := range pm.Metrics {
			if !isLicenseSourceMetric(metric.Name) {
				kept = append(kept, metric)
				continue
			}

			row := buildLicenseRow(metric, now)
			if row.ID != "" || row.Name != "" {
				rows = append(rows, row)
			}
		}
		pm.Metrics = kept
	}

	return rows
}

func isLicenseSourceMetric(name string) bool {
	return strings.HasPrefix(name, licenseSourceMetricName)
}

func buildLicenseRow(metric ddsnmp.Metric, now time.Time) licenseRow {
	tags := mergeLicenseTags(metric)

	source := ""
	if metric.Profile != nil {
		source = stripFileNameExt(metric.Profile.Source)
	}

	row := licenseRow{
		Source:         source,
		ID:             firstNonEmpty(tags[tagLicenseID], tags[tagLicenseIndex], strconv.FormatInt(metric.Value, 10)),
		Name:           firstNonEmpty(tags[tagLicenseName], tags[tagLicenseFeature], tags[tagLicenseComponent]),
		Feature:        tags[tagLicenseFeature],
		Component:      tags[tagLicenseComponent],
		Type:           tags[tagLicenseType],
		Impact:         tags[tagLicenseImpact],
		StateRaw:       tags[tagLicenseStateRaw],
		ActiveRaw:      tags[tagLicenseActiveRaw],
		ExpirySource:   tags[tagLicenseExpirySource],
		AuthSource:     tags[tagLicenseAuthorizationSource],
		CertSource:     tags[tagLicenseCertificateSource],
		GraceSource:    tags[tagLicenseGraceSource],
		OriginalMetric: metric.Name,
		IsUnlimited:    parseBoolLike(tags[tagLicenseUnlimited]),
		IsPerpetual:    parseBoolLike(tags[tagLicensePerpetual]),
	}

	if sev, ok := parseLicenseSeverity(tags); ok {
		row.StateSeverity = sev
		row.HasState = true
	}

	if ts, ok := parseLicenseTimestamp(tags[tagLicenseExpiryRaw], now); ok {
		row.ExpiryTS = ts
		row.HasExpiry = true
	}
	if ts, ok := parseLicenseTimestamp(tags[tagLicenseAuthorizationRaw], now); ok {
		row.AuthorizationExpiry = ts
		row.HasAuthorizationTime = true
	}
	if ts, ok := parseLicenseTimestamp(tags[tagLicenseCertificateRaw], now); ok {
		row.CertificateExpiry = ts
		row.HasCertificateTime = true
	}
	if ts, ok := parseLicenseTimestamp(tags[tagLicenseGraceRaw], now); ok {
		row.GraceExpiry = ts
		row.HasGraceTime = true
	}

	if usage, ok := parseLicenseInt(tags[tagLicenseUsageRaw]); ok {
		row.Usage = usage
		row.HasUsage = true
	}
	if capacity, ok := parseLicenseInt(tags[tagLicenseCapacityRaw]); ok {
		row.Capacity = capacity
		row.HasCapacity = true
	}
	if available, ok := parseLicenseInt(tags[tagLicenseAvailableRaw]); ok {
		row.Available = available
		row.HasAvailable = true
	}
	if !row.HasUsage && row.HasCapacity && row.HasAvailable && row.Available >= 0 && row.Available <= row.Capacity {
		row.Usage = row.Capacity - row.Available
		row.HasUsage = true
	}
	if pct, ok := parseLicensePercent(tags[tagLicenseUsagePercentRaw]); ok {
		row.UsagePercent = pct
		row.HasUsagePct = true
	}
	if !row.HasUsagePct && row.HasUsage && row.HasCapacity && row.Capacity > 0 && !row.IsUnlimited {
		row.UsagePercent = float64(row.Usage) * 100 / float64(row.Capacity)
		row.HasUsagePct = true
	}

	applyLicenseMetricValueKind(&row, metric.Value, tags, now)
	row.StateBucket = normalizeLicenseStateBucket(row, now)

	return row
}

func mergeLicenseTags(metric ddsnmp.Metric) map[string]string {
	switch {
	case len(metric.StaticTags) == 0:
		return metric.Tags
	case len(metric.Tags) == 0:
		return metric.StaticTags
	}

	tags := make(map[string]string, len(metric.StaticTags)+len(metric.Tags))
	for k, v := range metric.StaticTags {
		tags[k] = v
	}
	for k, v := range metric.Tags {
		tags[k] = v
	}
	return tags
}

func applyLicenseMetricValueKind(row *licenseRow, value int64, tags map[string]string, now time.Time) {
	switch tags[tagLicenseValueKind] {
	case licenseValueKindStateSeverity:
		if row.HasState || value < 0 || value > 2 {
			return
		}
		row.StateSeverity = value
		row.HasState = true
	case licenseValueKindExpiryTimestamp:
		if row.HasExpiry || value <= 0 {
			return
		}
		row.ExpiryTS = value
		row.HasExpiry = true
	case licenseValueKindAuthorizationTimestamp:
		if row.HasAuthorizationTime || value <= 0 {
			return
		}
		row.AuthorizationExpiry = value
		row.HasAuthorizationTime = true
	case licenseValueKindCertificateTimestamp:
		if row.HasCertificateTime || value <= 0 {
			return
		}
		row.CertificateExpiry = value
		row.HasCertificateTime = true
	case licenseValueKindGraceTimestamp:
		if row.HasGraceTime || value <= 0 {
			return
		}
		row.GraceExpiry = value
		row.HasGraceTime = true
	case licenseValueKindExpiryRemaining:
		if row.HasExpiry || value == 0 {
			return
		}
		row.ExpiryTS = now.Unix() + value
		row.HasExpiry = true
	case licenseValueKindAuthorizationRemaining:
		if row.HasAuthorizationTime || value == 0 {
			return
		}
		row.AuthorizationExpiry = now.Unix() + value
		row.HasAuthorizationTime = true
	case licenseValueKindCertificateRemaining:
		if row.HasCertificateTime || value == 0 {
			return
		}
		row.CertificateExpiry = now.Unix() + value
		row.HasCertificateTime = true
	case licenseValueKindGraceRemaining:
		if row.HasGraceTime || value == 0 {
			return
		}
		row.GraceExpiry = now.Unix() + value
		row.HasGraceTime = true
	case licenseValueKindUsage:
		if row.HasUsage {
			return
		}
		row.Usage = value
		row.HasUsage = true
	case licenseValueKindCapacity:
		if row.HasCapacity {
			return
		}
		row.Capacity = value
		row.HasCapacity = true
	case licenseValueKindUsagePercent:
		if row.HasUsagePct {
			return
		}
		row.UsagePercent = float64(value)
		row.HasUsagePct = true
	}
}

func aggregateLicenseRows(rows []licenseRow, now time.Time) licenseAggregate {
	var agg licenseAggregate

	for _, row := range rows {
		if row.StateBucket == "" {
			row.StateBucket = normalizeLicenseStateBucket(row, now)
		}
		if row.HasExpiry && !row.IsPerpetual {
			agg.setRemaining(row.ExpiryTS-now.Unix(), &agg.remainingTime, &agg.hasRemainingTime)
		}
		if row.HasAuthorizationTime {
			agg.setRemaining(row.AuthorizationExpiry-now.Unix(), &agg.authRemainingTime, &agg.hasAuthRemaining)
		}
		if row.HasCertificateTime {
			agg.setRemaining(row.CertificateExpiry-now.Unix(), &agg.certRemainingTime, &agg.hasCertRemaining)
		}
		if row.HasGraceTime {
			agg.setRemaining(row.GraceExpiry-now.Unix(), &agg.graceRemainingTime, &agg.hasGraceRemaining)
		}
		if row.HasUsagePct && !row.IsUnlimited {
			pct := int64(math.Round(row.UsagePercent))
			if !agg.hasUsagePercent || pct > agg.usagePercent {
				agg.usagePercent = pct
				agg.hasUsagePercent = true
			}
		}
		switch row.StateBucket {
		case licenseStateBucketHealthy:
			agg.stateHealthy++
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
	}

	return agg
}

func (agg *licenseAggregate) setRemaining(value int64, current *int64, seen *bool) {
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

func parseLicenseSeverity(tags map[string]string) (int64, bool) {
	if sev, ok := parseLicenseInt(tags[tagLicenseStateSeverityRaw]); ok {
		switch sev {
		case 0, 1, 2:
			return sev, true
		}
	}
	return mapLicenseStateSeverity(tags[tagLicenseStateRaw])
}

func parseLicenseTimestamp(raw string, now time.Time) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}

	switch strings.ToLower(raw) {
	case "0", "none", "n/a", "na", "perpetual", "permanent", "never", "unlimited":
		return 0, false
	}

	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		switch {
		case n <= 0:
			return 0, false
		case n == 4_294_967_295:
			return 0, false
		case n > 1_000_000_000_000:
			return n / 1000, true
		default:
			return n, true
		}
	}

	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
		"Mon Jan 2 15:04:05 2006",
		"Mon Jan 2 2006",
		"Mon 2 January 2006",
		"2 January 2006",
		"January 2 2006",
		"Jan 2 2006",
		"Jan 2 2006 15:04:05",
		"2 Jan 2006",
		"2 Jan 2006 15:04:05",
		"02 Jan 2006",
		"02 Jan 2006 15:04:05",
		"02/01/2006",
	} {
		if ts, ok := parseTimeLayout(raw, now, layout); ok {
			return ts, true
		}
	}

	return 0, false
}

func parseTimeLayout(raw string, now time.Time, layout string) (int64, bool) {
	parsed, err := time.Parse(layout, raw)
	if err != nil {
		parsed, err = time.ParseInLocation(layout, raw, now.Location())
		if err != nil {
			parsed, err = time.ParseInLocation(layout, raw, time.UTC)
			if err != nil {
				return 0, false
			}
		}
	}

	return parsed.Unix(), true
}

func parseLicenseInt(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseLicensePercent(raw string) (float64, bool) {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "%"))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseBoolLike(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "enabled", "active", "perpetual", "unlimited":
		return true
	default:
		return false
	}
}

func mapLicenseStateSeverity(raw string) (int64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	if licenseStateMatchesAny(raw, licenseStateBrokenHints) {
		return 2, true
	}
	if licenseStateMatchesAny(raw, licenseStateDegradedHints) {
		return 1, true
	}
	if licenseStateMatchesAny(raw, licenseStateHealthyHints) {
		return 0, true
	}

	return 1, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
