// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// Licensing pipeline contract — profiles emit typed licensing rows through
// ddsnmp.ProfileMetrics.LicenseRows. Hidden metrics remain a generic ddsnmp
// transport but are not part of the licensing consumer contract.

const (
	metricIDLicenseRemainingTime              = "snmp_device_license_remaining_time"
	metricIDLicenseAuthorizationRemainingTime = "snmp_device_license_authorization_remaining_time"
	metricIDLicenseCertificateRemainingTime   = "snmp_device_license_certificate_remaining_time"
	metricIDLicenseGraceRemainingTime         = "snmp_device_license_grace_remaining_time"
	metricIDLicenseUsagePercent               = "snmp_device_license_usage_percent"
	metricIDLicenseStateHealthy               = "snmp_device_license_state_healthy"
	metricIDLicenseStateInformational         = "snmp_device_license_state_informational"
	metricIDLicenseStateDegraded              = "snmp_device_license_state_degraded"
	metricIDLicenseStateBroken                = "snmp_device_license_state_broken"
	metricIDLicenseStateIgnored               = "snmp_device_license_state_ignored"
)

type licenseRow struct {
	ID           string
	StructuralID string
	Source       string
	Table        string
	Name         string

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

	Usage        int64
	HasUsage     bool
	Capacity     int64
	HasCapacity  bool
	Available    int64
	HasAvailable bool
	UsagePercent float64
	HasUsagePct  bool

	IsUnlimited bool
	IsPerpetual bool

	ExpirySource string
	AuthSource   string
	CertSource   string
	GraceSource  string
}

type licenseCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	rows       []licenseRow
}

func newLicenseCache() *licenseCache {
	return &licenseCache{}
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

// extractLicenseRows converts typed ddsnmp licensing rows into the collector's
// cached row shape. HiddenMetrics are intentionally ignored here: they remain a
// generic ddsnmp transport, not a licensing protocol.
func extractLicenseRows(pms []*ddsnmp.ProfileMetrics, now time.Time) []licenseRow {
	var rows []licenseRow

	for _, pm := range pms {
		if pm == nil {
			continue
		}
		for _, src := range pm.LicenseRows {
			row, ok := licenseRowFromTyped(pm, src, now)
			if !ok {
				continue
			}
			rows = append(rows, row)
		}
	}

	rows = dropLicenseRowsWithoutSignals(rows)

	for i := range rows {
		rows[i].StateBucket = normalizeLicenseStateBucket(rows[i], now)
	}

	return rows
}

func licenseRowFromTyped(pm *ddsnmp.ProfileMetrics, src ddsnmp.LicenseRow, now time.Time) (licenseRow, bool) {
	row := licenseRow{
		Source:        licenseRowSource(pm, src),
		Table:         licenseRowTable(src),
		ID:            licenseRowID(src),
		StructuralID:  firstNonBlank(src.StructuralID, src.RowKey, src.ID),
		Name:          src.Name,
		Feature:       src.Feature,
		Component:     src.Component,
		Type:          src.Type,
		Impact:        src.Impact,
		IsUnlimited:   src.IsUnlimited,
		IsPerpetual:   src.IsPerpetual,
		StateRaw:      src.State.Raw,
		StateSeverity: clampLicenseSeverity(src.State.Severity),
		HasState:      src.State.Has,
	}

	setLicenseTimer(&row.ExpiryTS, &row.HasExpiry, &row.ExpirySource, src.Expiry, now)
	setLicenseTimer(&row.AuthorizationExpiry, &row.HasAuthorizationTime, &row.AuthSource, src.Authorization, now)
	setLicenseTimer(&row.CertificateExpiry, &row.HasCertificateTime, &row.CertSource, src.Certificate, now)
	setLicenseTimer(&row.GraceExpiry, &row.HasGraceTime, &row.GraceSource, src.Grace, now)

	row.Usage = src.Usage.Used
	row.HasUsage = src.Usage.HasUsed
	row.Capacity = src.Usage.Capacity
	row.HasCapacity = src.Usage.HasCapacity
	row.Available = src.Usage.Available
	row.HasAvailable = src.Usage.HasAvailable
	row.UsagePercent = float64(src.Usage.Percent)
	row.HasUsagePct = src.Usage.HasPercent
	deriveLicenseUsage(&row)

	return row, firstNonBlank(row.ID, row.StructuralID) != "" && licenseRowHasAnySignal(row)
}

func licenseRowSource(pm *ddsnmp.ProfileMetrics, src ddsnmp.LicenseRow) string {
	if strings.TrimSpace(src.OriginProfileID) != "" {
		return src.OriginProfileID
	}
	if pm == nil {
		return ""
	}
	return pm.Source
}

func licenseRowTable(src ddsnmp.LicenseRow) string {
	return firstNonBlank(src.TableOID, src.Table)
}

func licenseRowID(src ddsnmp.LicenseRow) string {
	return firstNonBlank(src.ID, src.StructuralID, src.RowKey)
}

func setLicenseTimer(dst *int64, has *bool, source *string, timer ddsnmp.LicenseTimer, now time.Time) {
	if !timer.Has {
		return
	}
	if timer.Timestamp != 0 {
		*dst = timer.Timestamp
	} else {
		*dst = now.Unix() + timer.RemainingSeconds
	}
	*has = true
	*source = timer.SourceOID
}

func clampLicenseSeverity(value int64) int64 {
	switch {
	case value < 0:
		return 0
	case value > 2:
		return 2
	default:
		return value
	}
}

func deriveLicenseUsage(row *licenseRow) {
	if !row.HasUsage && row.HasCapacity && row.HasAvailable && row.Available >= 0 && row.Available <= row.Capacity {
		row.Usage = row.Capacity - row.Available
		row.HasUsage = true
	}
	if !row.HasUsagePct && row.HasUsage && row.HasCapacity && row.Capacity > 0 && !row.IsUnlimited {
		row.UsagePercent = float64(row.Usage) * 100 / float64(row.Capacity)
		row.HasUsagePct = true
	}
}

func dropLicenseRowsWithoutSignals(rows []licenseRow) []licenseRow {
	return slices.DeleteFunc(rows, func(row licenseRow) bool {
		return !licenseRowHasAnySignal(row)
	})
}

// licenseRowHasAnySignal returns true when at least one merged signal
// (state, expiry, auth/cert/grace timer, usage shape) is set on the row.
func licenseRowHasAnySignal(row licenseRow) bool {
	return row.HasState ||
		row.HasExpiry ||
		row.HasAuthorizationTime ||
		row.HasCertificateTime ||
		row.HasGraceTime ||
		row.HasUsage ||
		row.HasCapacity ||
		row.HasAvailable ||
		row.HasUsagePct
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
