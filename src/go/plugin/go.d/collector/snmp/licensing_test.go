// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type typedLicenseRowOption func(*ddsnmp.LicenseRow)

func profileWithRows(rows ...ddsnmp.LicenseRow) []*ddsnmp.ProfileMetrics {
	return []*ddsnmp.ProfileMetrics{{
		Source:      "profiles/test-profile.yaml",
		LicenseRows: rows,
	}}
}

func typedLicenseRow(id, name string, opts ...typedLicenseRowOption) ddsnmp.LicenseRow {
	structuralID := ""
	if id != "" {
		structuralID = "test-profile|scalar|" + id
	}
	row := ddsnmp.LicenseRow{
		OriginProfileID: "test-profile",
		StructuralID:    structuralID,
		ID:              id,
		Name:            name,
	}
	for _, opt := range opts {
		opt(&row)
	}
	return row
}

func withOriginProfileID(id string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.OriginProfileID = id
	}
}

func withStructuralID(id string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.StructuralID = id
	}
}

func withTable(oid, name, rowKey string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.TableOID = oid
		row.Table = name
		row.RowKey = rowKey
		if row.StructuralID == "" && row.OriginProfileID != "" && oid != "" && rowKey != "" {
			row.StructuralID = row.OriginProfileID + "|table|" + oid + "|" + rowKey
		}
	}
}

func withState(severity int64, raw string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.State.Has = true
		row.State.Severity = severity
		row.State.Raw = raw
	}
}

func withRawState(raw string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.State.Raw = raw
	}
}

func withExpiry(timestamp int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Expiry.Has = true
		row.Expiry.Timestamp = timestamp
	}
}

func withExpiryRemaining(seconds int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Expiry.Has = true
		row.Expiry.RemainingSeconds = seconds
	}
}

func withAuthorizationRemaining(seconds int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Authorization.Has = true
		row.Authorization.RemainingSeconds = seconds
	}
}

func withCertificateRemaining(seconds int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Certificate.Has = true
		row.Certificate.RemainingSeconds = seconds
	}
}

func withGraceRemaining(seconds int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Grace.Has = true
		row.Grace.RemainingSeconds = seconds
	}
}

func withExpirySource(source string) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Expiry.SourceOID = source
	}
}

func withUsage(used int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Usage.HasUsed = true
		row.Usage.Used = used
	}
}

func withCapacity(capacity int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Usage.HasCapacity = true
		row.Usage.Capacity = capacity
	}
}

func withAvailable(available int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Usage.HasAvailable = true
		row.Usage.Available = available
	}
}

func withUsagePercent(percent int64) typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.Usage.HasPercent = true
		row.Usage.Percent = percent
	}
}

func withPerpetual() typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.IsPerpetual = true
	}
}

func withUnlimited() typedLicenseRowOption {
	return func(row *ddsnmp.LicenseRow) {
		row.IsUnlimited = true
	}
}

func TestExtractLicenseRows_FromTypedRows(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(48 * time.Hour).Unix()

	tests := map[string]struct {
		rows   []ddsnmp.LicenseRow
		assert func(t *testing.T, rows []licenseRow)
	}{
		"copies identity descriptors state timer and usage": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("base", "Base Firewall",
					withState(0, "active"),
					withExpiry(expiry),
					withUsage(75),
					withCapacity(100),
				),
			},
			assert: func(t *testing.T, rows []licenseRow) {
				require.Len(t, rows, 1)
				row := rows[0]
				assert.Equal(t, "base", row.ID)
				assert.Equal(t, "Base Firewall", row.Name)
				assert.Equal(t, "test-profile", row.Source)
				assert.True(t, row.HasState)
				assert.EqualValues(t, 0, row.StateSeverity)
				assert.True(t, row.HasExpiry)
				assert.EqualValues(t, expiry, row.ExpiryTS)
				assert.True(t, row.HasUsage)
				assert.EqualValues(t, 75, row.Usage)
				assert.True(t, row.HasCapacity)
				assert.EqualValues(t, 100, row.Capacity)
				assert.True(t, row.HasUsagePct)
				assert.InDelta(t, 75.0, row.UsagePercent, 0.001)
				assert.Equal(t, licenseStateBucketHealthy, row.StateBucket)
			},
		},
		"derives usage from capacity minus available": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("pool", "Connection pool",
					withCapacity(100),
					withAvailable(25),
				),
			},
			assert: func(t *testing.T, rows []licenseRow) {
				require.Len(t, rows, 1)
				assert.True(t, rows[0].HasUsage)
				assert.EqualValues(t, 75, rows[0].Usage)
				assert.True(t, rows[0].HasUsagePct)
				assert.InDelta(t, 75.0, rows[0].UsagePercent, 0.001)
			},
		},
		"rebases remaining timers on collect time": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("auth", "Auth", withAuthorizationRemaining(3600)),
				typedLicenseRow("cert", "Cert", withCertificateRemaining(7200)),
				typedLicenseRow("grace", "Grace", withGraceRemaining(1800)),
				typedLicenseRow("sub", "Sub", withExpiryRemaining(600)),
			},
			assert: func(t *testing.T, rows []licenseRow) {
				require.Len(t, rows, 4)
				byID := make(map[string]licenseRow, len(rows))
				for _, row := range rows {
					byID[row.ID] = row
				}
				assert.EqualValues(t, now.Unix()+3600, byID["auth"].AuthorizationExpiry)
				assert.EqualValues(t, now.Unix()+7200, byID["cert"].CertificateExpiry)
				assert.EqualValues(t, now.Unix()+1800, byID["grace"].GraceExpiry)
				assert.EqualValues(t, now.Unix()+600, byID["sub"].ExpiryTS)
			},
		},
		"drops rows without identity or signal": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("present", "Present", withState(0, "")),
				typedLicenseRow("", "No ID", withState(0, "")),
				typedLicenseRow("stub", "No signal"),
			},
			assert: func(t *testing.T, rows []licenseRow) {
				require.Len(t, rows, 1)
				assert.Equal(t, "present", rows[0].ID)
			},
		},
		"uses table OID as the internal table identity": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("FortiCare", "FortiCare",
					withTable("1.3.6.1.4.1.12356.101.4.6.3.1", "fgLicContractTable", "1"),
					withCapacity(100),
				),
				typedLicenseRow("FortiCare", "FortiCare",
					withTable("1.3.6.1.4.1.12356.101.4.6.4.1", "fgLicVersionTable", "1"),
					withCapacity(200),
				),
			},
			assert: func(t *testing.T, rows []licenseRow) {
				require.Len(t, rows, 2)
				assert.NotEqual(t, rows[0].Table, rows[1].Table)
				caps := []int64{rows[0].Capacity, rows[1].Capacity}
				assert.Contains(t, caps, int64(100))
				assert.Contains(t, caps, int64(200))
			},
		},
		"ignores private metrics": {
			rows: nil,
			assert: func(t *testing.T, _ []licenseRow) {
				pm := &ddsnmp.ProfileMetrics{
					Source: "profiles/test-profile.yaml",
					HiddenMetrics: []ddsnmp.Metric{{
						Name:  "_private_metric",
						Value: 0,
						Tags:  map[string]string{"component": "private"},
					}},
				}
				assert.Empty(t, extractLicenseRows([]*ddsnmp.ProfileMetrics{pm}, now))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rows := extractLicenseRows(profileWithRows(tc.rows...), now)
			tc.assert(t, rows)
		})
	}
}

func TestNormalizeLicenseStateBucket(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		row  ddsnmp.LicenseRow
		want licenseStateBucket
	}{
		"fresh severity recovers from stale broken raw state": {
			row:  typedLicenseRow("renewed", "Renewed", withState(0, "expired")),
			want: licenseStateBucketHealthy,
		},
		"ignored raw state suppresses severity zero": {
			row:  typedLicenseRow("inactive", "Inactive", withState(0, "none")),
			want: licenseStateBucketIgnored,
		},
		"eval raw state becomes informational instead of degraded": {
			row:  typedLicenseRow("eval", "Evaluation", withState(1, "Evaluation")),
			want: licenseStateBucketInformational,
		},
		"raw broken state is used when no fresh severity exists": {
			row:  typedLicenseRow("raw-broken", "Raw Broken", withRawState("expired"), withUsage(1)),
			want: licenseStateBucketBroken,
		},
		"raw degraded state is used when no fresh severity exists": {
			row:  typedLicenseRow("raw-degraded", "Raw Degraded", withRawState("degraded"), withUsage(1)),
			want: licenseStateBucketDegraded,
		},
		"expired timer forces broken over valid raw state": {
			row:  typedLicenseRow("expired", "Expired", withRawState("valid"), withExpiry(now.Add(-time.Hour).Unix())),
			want: licenseStateBucketBroken,
		},
		"grace timer without expiry is degraded": {
			row:  typedLicenseRow("grace", "Grace", withGraceRemaining(3600)),
			want: licenseStateBucketDegraded,
		},
		"fully consumed usage is broken": {
			row:  typedLicenseRow("pool", "Pool", withUsagePercent(100)),
			want: licenseStateBucketBroken,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rows := extractLicenseRows(profileWithRows(tc.row), now)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.want, rows[0].StateBucket)
		})
	}
}

func TestExtractLicenseRows_CheckPointPublicFixtureValues(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		rows []ddsnmp.LicenseRow
	}{
		"public Check Point sample values": {
			rows: []ddsnmp.LicenseRow{
				typedLicenseRow("0", "Firewall", withState(2, "Not Entitled")),
				typedLicenseRow("4", "Application Ctrl", withState(1, "Evaluation"), withExpiry(1619246913)),
				typedLicenseRow("2", "IPS", withState(1, "Evaluation"), withExpiry(1619246941)),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rows := extractLicenseRows(profileWithRows(tc.rows...), now)
			require.Len(t, rows, 3)

			for _, row := range rows {
				switch row.ID {
				case "0":
					assert.Equal(t, licenseStateBucketBroken, row.StateBucket, "Not Entitled -> broken")
					assert.False(t, row.HasExpiry)
				case "4":
					assert.Equal(t, licenseStateBucketBroken, row.StateBucket, "expired Evaluation -> broken")
					assert.True(t, row.HasExpiry)
					assert.EqualValues(t, 1619246913, row.ExpiryTS)
				case "2":
					assert.Equal(t, licenseStateBucketBroken, row.StateBucket, "expired Evaluation -> broken")
					assert.True(t, row.HasExpiry)
					assert.EqualValues(t, 1619246941, row.ExpiryTS)
				default:
					t.Fatalf("unexpected row id %q", row.ID)
				}
			}
		})
	}
}

func TestLicenseRowUniqueKeyHandlesEmbeddedNULs(t *testing.T) {
	tests := map[string]struct {
		left  licenseRow
		right licenseRow
	}{
		"embedded NULs stay unambiguous": {
			left:  licenseRow{Source: "vendor", Table: "table", ID: "row\x00suffix"},
			right: licenseRow{Source: "vendor", Table: "table\x00row", ID: "suffix"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, licenseRowUniqueKey(tc.left), licenseRowUniqueKey(tc.right))
		})
	}
}
