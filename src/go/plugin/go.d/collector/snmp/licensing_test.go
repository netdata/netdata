// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestExtractLicenseRows(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/checkpoint.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  licenseSourceMetricName,
			Value: 17,
			Tags: map[string]string{
				tagLicenseName:        "Application Control",
				tagLicenseStateRaw:    "about-to-expire",
				tagLicenseExpiryRaw:   "1775152800",
				tagLicenseUsageRaw:    "85",
				tagLicenseCapacityRaw: "100",
				tagLicenseImpact:      "Threat prevention coverage",
				tagLicenseIndex:       "1",
				tagLicenseID:          "17",
			},
		},
	}
	pm.Metrics = []ddsnmp.Metric{
		{
			Name:  "cpu.usage",
			Value: 42,
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}
	for i := range pm.Metrics {
		pm.Metrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 1)
	require.Len(t, pm.Metrics, 1)

	row := rows[0]
	assert.Equal(t, "checkpoint", row.Source)
	assert.Equal(t, "17", row.ID)
	assert.Equal(t, "Application Control", row.Name)
	assert.Equal(t, "about-to-expire", row.StateRaw)
	assert.True(t, row.HasState)
	assert.EqualValues(t, 1, row.StateSeverity)
	assert.True(t, row.HasExpiry)
	assert.EqualValues(t, 1775152800, row.ExpiryTS)
	assert.True(t, row.HasUsage)
	assert.EqualValues(t, 85, row.Usage)
	assert.True(t, row.HasCapacity)
	assert.EqualValues(t, 100, row.Capacity)
	assert.True(t, row.HasUsagePct)
	assert.InDelta(t, 85, row.UsagePercent, 0.001)
	assert.Equal(t, "cpu.usage", pm.Metrics[0].Name)
}

func TestExtractLicenseRows_StaticTags(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/fortinet-fortigate.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  licenseSourceMetricName,
			Value: 1,
			StaticTags: map[string]string{
				tagLicenseType:         "account_contract",
				tagLicenseComponent:    "account",
				tagLicenseExpirySource: "fgLicAlContractExpiry",
			},
			Tags: map[string]string{
				tagLicenseIndex:     "7",
				tagLicenseID:        "FortiCare Premium",
				tagLicenseName:      "FortiCare Premium",
				tagLicenseExpiryRaw: "Mon 11 November 2030",
			},
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "fortinet-fortigate", row.Source)
	assert.Equal(t, "FortiCare Premium", row.ID)
	assert.Equal(t, "FortiCare Premium", row.Name)
	assert.Equal(t, "account_contract", row.Type)
	assert.Equal(t, "account", row.Component)
	assert.Equal(t, "fgLicAlContractExpiry", row.ExpirySource)
	assert.True(t, row.HasExpiry)
	assert.EqualValues(t, time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), row.ExpiryTS)
}

func TestExtractLicenseRows_AbbreviatedDateFormats(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/sophos-xgs-firewall.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  licenseSourceMetricName,
			Value: 2,
			Tags: map[string]string{
				tagLicenseID:        "network_protection",
				tagLicenseName:      "Network Protection",
				tagLicenseStateRaw:  "expired",
				tagLicenseExpiryRaw: "11 Nov 2031",
			},
		},
		{
			Name:  licenseSourceMetricName,
			Value: 0,
			Tags: map[string]string{
				tagLicenseID:        "central_orchestration",
				tagLicenseName:      "Central Orchestration",
				tagLicenseStateRaw:  "subscribed",
				tagLicenseExpiryRaw: "02 Jan 2032",
			},
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 2)

	assert.True(t, rows[0].HasExpiry)
	assert.EqualValues(t, time.Date(2031, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), rows[0].ExpiryTS)
	assert.True(t, rows[1].HasExpiry)
	assert.EqualValues(t, time.Date(2032, time.January, 2, 0, 0, 0, 0, time.UTC).Unix(), rows[1].ExpiryTS)
}

func TestExtractLicenseRows_ValueKind(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/cisco.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  licenseSourceMetricName,
			Value: 2,
			StaticTags: map[string]string{
				tagLicenseID:        "smart_registration",
				tagLicenseName:      "Smart Licensing registration",
				tagLicenseType:      "registration",
				tagLicenseComponent: "smart_licensing",
				tagLicenseValueKind: licenseValueKindStateSeverity,
			},
		},
		{
			Name:  licenseSourceMetricName,
			Value: 1775152800,
			StaticTags: map[string]string{
				tagLicenseID:                  "smart_authorization",
				tagLicenseName:                "Smart Licensing authorization",
				tagLicenseType:                "authorization",
				tagLicenseComponent:           "smart_licensing",
				tagLicenseValueKind:           licenseValueKindAuthorizationTimestamp,
				tagLicenseAuthorizationSource: "ciscoSlaAuthExpireTime",
			},
		},
		{
			Name:  licenseSourceMetricName,
			Value: 3600,
			StaticTags: map[string]string{
				tagLicenseID:          "smart_eval",
				tagLicenseName:        "Smart Licensing evaluation",
				tagLicenseType:        "evaluation",
				tagLicenseComponent:   "smart_licensing",
				tagLicenseValueKind:   licenseValueKindGraceRemaining,
				tagLicenseGraceSource: "ciscoSlaAuthEvalPeriodLeft",
			},
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 3)

	assert.Equal(t, "cisco", rows[0].Source)
	assert.EqualValues(t, 2, rows[0].StateSeverity)
	assert.True(t, rows[0].HasState)

	assert.EqualValues(t, 1775152800, rows[1].AuthorizationExpiry)
	assert.True(t, rows[1].HasAuthorizationTime)
	assert.Equal(t, "ciscoSlaAuthExpireTime", rows[1].AuthSource)

	assert.EqualValues(t, now.Add(time.Hour).Unix(), rows[2].GraceExpiry)
	assert.True(t, rows[2].HasGraceTime)
	assert.Equal(t, "ciscoSlaAuthEvalPeriodLeft", rows[2].GraceSource)
}

func TestExtractLicenseRows_DerivesUsageFromAvailable(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/cisco.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  licenseSourceMetricName,
			Value: 0,
			Tags: map[string]string{
				tagLicenseID:           "17",
				tagLicenseName:         "SECURITYK9",
				tagLicenseCapacityRaw:  "100",
				tagLicenseAvailableRaw: "15",
			},
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.True(t, row.HasCapacity)
	assert.EqualValues(t, 100, row.Capacity)
	assert.True(t, row.HasAvailable)
	assert.EqualValues(t, 15, row.Available)
	assert.True(t, row.HasUsage)
	assert.EqualValues(t, 85, row.Usage)
	assert.True(t, row.HasUsagePct)
	assert.InDelta(t, 85, row.UsagePercent, 0.001)
}

func TestExtractLicenseRows_PrefixVariants(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	pm := &ddsnmp.ProfileMetrics{Source: "/tmp/cisco.yaml"}
	pm.HiddenMetrics = []ddsnmp.Metric{
		{
			Name:  "_license_row_cisco_traditional",
			Value: 7200,
			Tags: map[string]string{
				tagLicenseID:        "17",
				tagLicenseName:      "SECURITYK9",
				tagLicenseValueKind: licenseValueKindExpiryRemaining,
			},
		},
		{
			Name:  "_license_row_cisco_smart",
			Value: 2,
			Tags: map[string]string{
				tagLicenseID:        "smart_registration",
				tagLicenseName:      "Smart Licensing registration",
				tagLicenseValueKind: licenseValueKindStateSeverity,
			},
		},
	}
	for i := range pm.HiddenMetrics {
		pm.HiddenMetrics[i].Profile = pm
	}

	rows := extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now)
	require.Len(t, rows, 2)

	assert.Equal(t, "17", rows[0].ID)
	assert.EqualValues(t, now.Add(2*time.Hour).Unix(), rows[0].ExpiryTS)
	assert.True(t, rows[0].HasExpiry)

	assert.Equal(t, "smart_registration", rows[1].ID)
	assert.EqualValues(t, 2, rows[1].StateSeverity)
	assert.True(t, rows[1].HasState)
}

func TestAggregateLicenseRows(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	rows := []licenseRow{
		{
			Name:          "A",
			ExpiryTS:      now.Add(48 * time.Hour).Unix(),
			HasExpiry:     true,
			UsagePercent:  80,
			HasUsagePct:   true,
			StateSeverity: 1,
			HasState:      true,
		},
		{
			Name:                 "B",
			ExpiryTS:             now.Add(12 * time.Hour).Unix(),
			HasExpiry:            true,
			AuthorizationExpiry:  now.Add(24 * time.Hour).Unix(),
			HasAuthorizationTime: true,
			GraceExpiry:          now.Add(6 * time.Hour).Unix(),
			HasGraceTime:         true,
			UsagePercent:         96,
			HasUsagePct:          true,
			StateSeverity:        2,
			HasState:             true,
		},
	}

	agg := aggregateLicenseRows(rows, now)
	assert.True(t, agg.hasRemainingTime)
	assert.EqualValues(t, int64(12*time.Hour/time.Second), agg.remainingTime)
	assert.True(t, agg.hasAuthRemaining)
	assert.EqualValues(t, int64(24*time.Hour/time.Second), agg.authRemainingTime)
	assert.True(t, agg.hasGraceRemaining)
	assert.EqualValues(t, int64(6*time.Hour/time.Second), agg.graceRemainingTime)
	assert.True(t, agg.hasUsagePercent)
	assert.EqualValues(t, 96, agg.usagePercent)
	assert.True(t, agg.hasStateCounts)
	assert.EqualValues(t, 0, agg.stateHealthy)
	assert.EqualValues(t, 1, agg.stateDegraded)
	assert.EqualValues(t, 1, agg.stateBroken)
	assert.EqualValues(t, 0, agg.stateIgnored)
}

func TestAggregateLicenseRows_IgnoresPerpetualAndUnlimited(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	rows := []licenseRow{
		{
			Name:        "perpetual-license",
			ExpiryTS:    now.Add(-72 * time.Hour).Unix(),
			HasExpiry:   true,
			IsPerpetual: true,
		},
		{
			Name:      "expired-license",
			ExpiryTS:  now.Add(-2 * time.Hour).Unix(),
			HasExpiry: true,
		},
		{
			Name:         "unlimited-pool",
			UsagePercent: 99,
			HasUsagePct:  true,
			IsUnlimited:  true,
		},
		{
			Name:         "finite-pool",
			UsagePercent: 85,
			HasUsagePct:  true,
		},
	}

	agg := aggregateLicenseRows(rows, now)
	assert.True(t, agg.hasRemainingTime)
	assert.EqualValues(t, int64(-2*time.Hour/time.Second), agg.remainingTime)
	assert.True(t, agg.hasUsagePercent)
	assert.EqualValues(t, 85, agg.usagePercent)
	assert.True(t, agg.hasStateCounts)
	assert.EqualValues(t, 3, agg.stateHealthy)
	assert.EqualValues(t, 0, agg.stateDegraded)
	assert.EqualValues(t, 1, agg.stateBroken)
	assert.EqualValues(t, 0, agg.stateIgnored)
}

func TestNormalizeLicenseStateBucket(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	assert.Equal(t, licenseStateBucketIgnored, normalizeLicenseStateBucket(licenseRow{StateRaw: "not_subscribed"}, now))
	assert.Equal(t, licenseStateBucketDegraded, normalizeLicenseStateBucket(licenseRow{StateRaw: "trial"}, now))
	assert.Equal(t, licenseStateBucketBroken, normalizeLicenseStateBucket(licenseRow{StateRaw: "expired"}, now))
	assert.Equal(t, licenseStateBucketDegraded, normalizeLicenseStateBucket(licenseRow{StateRaw: "inactive"}, now))
	sev, ok := mapLicenseStateSeverity("inactive")
	require.True(t, ok)
	assert.EqualValues(t, 1, sev)
	sev, ok = mapLicenseStateSeverity("evaluation expired because grace ended")
	require.True(t, ok)
	assert.EqualValues(t, 2, sev)
	assert.Equal(t, licenseStateBucketBroken, normalizeLicenseStateBucket(licenseRow{StateRaw: "authorization expired due to policy"}, now))
	assert.Equal(t, licenseStateBucketBroken, normalizeLicenseStateBucket(licenseRow{StateRaw: "not_authorized"}, now))
	assert.Equal(t, licenseStateBucketHealthy, normalizeLicenseStateBucket(licenseRow{StateRaw: "up_to_date"}, now))
	assert.Equal(t, licenseStateBucketBroken, normalizeLicenseStateBucket(licenseRow{
		StateRaw:  "evaluation",
		HasExpiry: true,
		ExpiryTS:  now.Add(-time.Hour).Unix(),
	}, now))
	assert.Equal(t, licenseStateBucketDegraded, normalizeLicenseStateBucket(licenseRow{
		StateRaw:  "evaluation",
		HasExpiry: true,
		ExpiryTS:  now.Add(time.Hour).Unix(),
	}, now))
	assert.Equal(t, licenseStateBucketHealthy, normalizeLicenseStateBucket(licenseRow{HasExpiry: true, ExpiryTS: now.Add(time.Hour).Unix()}, now))
	assert.Equal(t, licenseStateBucketBroken, normalizeLicenseStateBucket(licenseRow{HasExpiry: true, ExpiryTS: now.Add(-time.Hour).Unix()}, now))
	assert.Equal(t, licenseStateBucketHealthy, normalizeLicenseStateBucket(licenseRow{HasUsagePct: true, UsagePercent: 90}, now))
	assert.Equal(t, licenseStateBucketIgnored, normalizeLicenseStateBucket(licenseRow{}, now))
}

func TestAggregateLicenseRows_RemainingTimeDecreasesAcrossPolls(t *testing.T) {
	now1 := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)
	now2 := now1.Add(5 * time.Second)

	rows := []licenseRow{
		{
			Name:                 "subscription",
			ExpiryTS:             now1.Add(time.Hour).Unix(),
			HasExpiry:            true,
			AuthorizationExpiry:  now1.Add(2 * time.Hour).Unix(),
			HasAuthorizationTime: true,
			CertificateExpiry:    now1.Add(3 * time.Hour).Unix(),
			HasCertificateTime:   true,
			GraceExpiry:          now1.Add(30 * time.Minute).Unix(),
			HasGraceTime:         true,
		},
	}

	agg1 := aggregateLicenseRows(rows, now1)
	agg2 := aggregateLicenseRows(rows, now2)

	assert.EqualValues(t, int64(time.Hour/time.Second), agg1.remainingTime)
	assert.EqualValues(t, int64(time.Hour/time.Second)-5, agg2.remainingTime)
	assert.EqualValues(t, int64(2*time.Hour/time.Second), agg1.authRemainingTime)
	assert.EqualValues(t, int64(2*time.Hour/time.Second)-5, agg2.authRemainingTime)
	assert.EqualValues(t, int64(3*time.Hour/time.Second), agg1.certRemainingTime)
	assert.EqualValues(t, int64(3*time.Hour/time.Second)-5, agg2.certRemainingTime)
	assert.EqualValues(t, int64(30*time.Minute/time.Second), agg1.graceRemainingTime)
	assert.EqualValues(t, int64(30*time.Minute/time.Second)-5, agg2.graceRemainingTime)
}

func TestParseLicenseTimestamp(t *testing.T) {
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		raw  string
		want time.Time
	}{
		{
			name: "epoch seconds",
			raw:  "1775152800",
			want: time.Unix(1775152800, 0).UTC(),
		},
		{
			name: "fortinet simple date",
			raw:  "Mon 11 November 2030",
			want: time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "librenms fortinet style",
			raw:  "Sat Jul 26 01:00:00 2025",
			want: time.Date(2025, time.July, 26, 1, 0, 0, 0, time.UTC),
		},
		{
			name: "max uint32 sentinel",
			raw:  "4294967295",
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseLicenseTimestamp(tt.raw, now)
			if tt.want.IsZero() {
				require.False(t, ok)
				assert.Zero(t, got)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.want.Unix(), got)
		})
	}
}
