// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

	"github.com/stretchr/testify/assert"
)

func TestAggregateLicenseRows_FromTypedRows(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	earliest := now.Add(2 * time.Hour).Unix()
	latest := now.Add(48 * time.Hour).Unix()

	tests := map[string]struct {
		rows   []ddsnmp.LicenseRow
		assert func(t *testing.T, agg licenseAggregate)
	}{
		"selects min expiry and max usage": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("a", "First", withExpiry(latest), withUsage(30), withCapacity(100)),
				typedLicenseRow("b", "Second", withExpiry(earliest), withUsage(95), withCapacity(100)),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				assert.True(t, agg.hasRemainingTime)
				assert.Equal(t, earliest-now.Unix(), agg.remainingTime)
				assert.True(t, agg.hasUsagePercent)
				assert.EqualValues(t, 95, agg.usagePercent)
			},
		},
		"perpetual rows skip expiry aggregation": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("perp", "Perpetual", withExpiry(now.Add(time.Hour).Unix()), withPerpetual()),
				typedLicenseRow("real", "Subscription", withExpiry(now.Add(24*time.Hour).Unix())),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				assert.True(t, agg.hasRemainingTime)
				assert.Equal(t, int64((24 * time.Hour).Seconds()), agg.remainingTime)
			},
		},
		"unlimited rows skip usage aggregation": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("limited", "Limited", withUsagePercent(60)),
				typedLicenseRow("infinite", "Infinite", withUsagePercent(100), withUnlimited()),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				assert.True(t, agg.hasUsagePercent)
				assert.EqualValues(t, 60, agg.usagePercent)
			},
		},
		"state bucket counts include informational": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("a", "Healthy A", withState(0, "")),
				typedLicenseRow("b", "Healthy B", withState(0, "")),
				typedLicenseRow("c", "Informational", withState(1, "evaluation")),
				typedLicenseRow("d", "Degraded", withState(1, "")),
				typedLicenseRow("e", "Broken", withState(2, "")),
				typedLicenseRow("f", "Ignored", withState(0, "none")),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				assert.True(t, agg.hasStateCounts)
				assert.EqualValues(t, 2, agg.stateHealthy)
				assert.EqualValues(t, 1, agg.stateInformational)
				assert.EqualValues(t, 1, agg.stateDegraded)
				assert.EqualValues(t, 1, agg.stateBroken)
				assert.EqualValues(t, 1, agg.stateIgnored)
			},
		},
		"ignored rows do not drive signal aggregation": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("ignored", "Ignored",
					withRawState("not applicable"),
					withExpiry(now.Add(time.Hour).Unix()),
					withUsagePercent(99),
				),
				typedLicenseRow("healthy", "Healthy",
					withExpiry(now.Add(24*time.Hour).Unix()),
					withUsagePercent(60),
				),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				assert.True(t, agg.hasRemainingTime)
				assert.Equal(t, int64((24 * time.Hour).Seconds()), agg.remainingTime)
				assert.True(t, agg.hasUsagePercent)
				assert.EqualValues(t, 60, agg.usagePercent)
				assert.True(t, agg.hasStateCounts)
				assert.EqualValues(t, 1, agg.stateHealthy)
				assert.EqualValues(t, 1, agg.stateIgnored)
			},
		},
		"writeTo omits absent timer and usage signals": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("a", "A", withState(0, "")),
			},
			assert: func(t *testing.T, agg licenseAggregate) {
				mx := make(map[string]int64)
				agg.writeTo(mx)
				assert.NotContains(t, mx, metricIDLicenseRemainingTime)
				assert.NotContains(t, mx, metricIDLicenseUsagePercent)
				assert.Contains(t, mx, metricIDLicenseStateHealthy)
				assert.Contains(t, mx, metricIDLicenseStateInformational)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rows := extractLicenseRows(profileWithRows(tc.rows...), now)
			tc.assert(t, aggregateLicenseRows(rows, now))
		})
	}
}
