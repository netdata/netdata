// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_CiscoTraditionalLicensingProfile(t *testing.T) {
	tests := map[string]struct {
		rows       []ciscoTraditionalRow
		assertRows func(*testing.T, []ddsnmp.LicenseRow)
	}{
		"subscription and grace rows": {
			rows: []ciscoTraditionalRow{
				{
					entPhysical:   1,
					storeUsed:     1,
					index:         17,
					name:          "SECURITYK9",
					version:       "1.0",
					licenseType:   5,
					remaining:     0,
					capacity:      100,
					available:     15,
					impact:        "Security subscription",
					status:        3,
					endDate:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
					hasEndDatePDU: true,
				},
				{
					entPhysical: 1,
					storeUsed:   2,
					index:       23,
					name:        "APPXK9",
					version:     "2.1",
					licenseType: 3,
					remaining:   7200,
					capacity:    10,
					available:   0,
					impact:      "Session count exhausted",
					status:      6,
				},
			},
			assertRows: func(t *testing.T, rows []ddsnmp.LicenseRow) {
				require.Len(t, rows, 2)
				byName := licenseRowsByName(rows)

				subscription := byName["SECURITYK9"]
				require.NotEmpty(t, subscription.ID)
				assert.Equal(t, "1.1.17", subscription.ID)
				assert.Equal(t, "1.0", subscription.Feature)
				assert.Equal(t, "traditional_licensing", subscription.Component)
				assert.Equal(t, "paid_subscription", subscription.Type)
				assert.Equal(t, "Security subscription", subscription.Impact)
				require.True(t, subscription.Expiry.Has)
				assert.EqualValues(t, time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), subscription.Expiry.Timestamp)
				require.True(t, subscription.Usage.HasCapacity)
				assert.EqualValues(t, 100, subscription.Usage.Capacity)
				require.True(t, subscription.Usage.HasAvailable)
				assert.EqualValues(t, 15, subscription.Usage.Available)
				require.True(t, subscription.State.Has)
				assert.EqualValues(t, 0, subscription.State.Severity)
				assert.Equal(t, "3", subscription.State.Raw)

				grace := byName["APPXK9"]
				require.NotEmpty(t, grace.ID)
				assert.Equal(t, "1.2.23", grace.ID)
				assert.Equal(t, "2.1", grace.Feature)
				assert.Equal(t, "grace_period", grace.Type)
				assert.Equal(t, "Session count exhausted", grace.Impact)
				require.True(t, grace.Usage.HasCapacity)
				assert.EqualValues(t, 10, grace.Usage.Capacity)
				require.True(t, grace.Usage.HasAvailable)
				assert.EqualValues(t, 0, grace.Usage.Available)
				require.True(t, grace.State.Has)
				assert.EqualValues(t, 2, grace.State.Severity)
				assert.Equal(t, "6", grace.State.Raw)
				assert.False(t, grace.Expiry.Has)
			},
		},
		"invalid expiry DateAndTime is absent": {
			rows: []ciscoTraditionalRow{
				{
					entPhysical:   1,
					storeUsed:     1,
					index:         2,
					name:          "WLC-BASE",
					version:       "1.0",
					licenseType:   4,
					remaining:     0,
					capacity:      100,
					available:     100,
					impact:        "Permanent base license",
					status:        1,
					endDatePDU:    []byte("0"),
					hasEndDatePDU: true,
				},
			},
			assertRows: func(t *testing.T, rows []ddsnmp.LicenseRow) {
				require.Len(t, rows, 1)
				row := rows[0]
				assert.Equal(t, "1.1.2", row.ID)
				assert.Equal(t, "WLC-BASE", row.Name)
				assert.Equal(t, "permanent", row.Type)
				assert.True(t, row.IsPerpetual)
				assert.False(t, row.Expiry.Has)
				require.True(t, row.Usage.HasCapacity)
				assert.EqualValues(t, 100, row.Usage.Capacity)
				require.True(t, row.Usage.HasAvailable)
				assert.EqualValues(t, 100, row.Usage.Available)
				require.True(t, row.State.Has)
				assert.EqualValues(t, 0, row.State.Severity)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectCiscoTraditionalLicensingWalk(mockHandler, tc.rows...)
			profile := mustLoadTypedLicensingProfile(t, "cisco", func(row ddprofiledefinition.LicensingConfig) bool {
				return strings.TrimPrefix(row.Table.OID, ".") == "1.3.6.1.4.1.9.9.543.1.2.3.1"
			})
			require.Len(t, profile.Definition.Licensing, 1)

			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: "",
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			assert.Empty(t, pm.Metrics)
			assert.Empty(t, pm.HiddenMetrics)
			tc.assertRows(t, pm.LicenseRows)
		})
	}
}

func TestCollector_Collect_CiscoTraditionalLicensingProfile_RejectsMalformedNonPlaceholderExpiry(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectCiscoTraditionalLicensingWalk(mockHandler, ciscoTraditionalRow{
		entPhysical:   1,
		storeUsed:     1,
		index:         2,
		name:          "WLC-BASE",
		version:       "1.0",
		licenseType:   4,
		capacity:      100,
		available:     100,
		impact:        "Permanent base license",
		status:        1,
		endDatePDU:    []byte("bad"),
		hasEndDatePDU: true,
	})
	profile := mustLoadTypedLicensingProfile(t, "cisco", func(row ddprofiledefinition.LicensingConfig) bool {
		return strings.TrimPrefix(row.Table.OID, ".") == "1.3.6.1.4.1.9.9.543.1.2.3.1"
	})
	require.Len(t, profile.Definition.Licensing, 1)

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "",
	})

	rows, err := collector.collectLicenseRows(profile, &ddsnmp.CollectionStats{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expiry.timestamp: invalid SNMP DateAndTime length 3")
	assert.Empty(t, rows)
}

type ciscoTraditionalRow struct {
	entPhysical   int
	storeUsed     int
	index         int
	name          string
	version       string
	licenseType   int
	remaining     uint
	capacity      uint
	available     uint
	impact        string
	status        int
	endDate       time.Time
	endDatePDU    []byte
	hasEndDatePDU bool
}

func expectCiscoTraditionalLicensingWalk(mockHandler *snmpmock.MockHandler, rows ...ciscoTraditionalRow) {
	pdus := make([]gosnmp.SnmpPDU, 0, len(rows)*10)

	for _, row := range rows {
		idx := strconv.Itoa(row.entPhysical) + "." + strconv.Itoa(row.storeUsed) + "." + strconv.Itoa(row.index)

		pdus = append(pdus,
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.3."+idx, row.name),
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.4."+idx, row.version),
			createIntegerPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.5."+idx, row.licenseType),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.8."+idx, row.remaining),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.10."+idx, row.capacity),
			createGauge32PDU("1.3.6.1.4.1.9.9.543.1.2.3.1.11."+idx, row.available),
			createStringPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.13."+idx, row.impact),
			createIntegerPDU("1.3.6.1.4.1.9.9.543.1.2.3.1.14."+idx, row.status),
		)
		if row.hasEndDatePDU {
			oid := "1.3.6.1.4.1.9.9.543.1.2.3.1.16." + idx
			if row.endDatePDU != nil {
				pdus = append(pdus, createPDU(oid, gosnmp.OctetString, row.endDatePDU))
			} else {
				pdus = append(pdus, createDateAndTimePDU(oid, row.endDate))
			}
		}
	}

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.9.9.543.1.2.3.1",
		pdus,
	)
}

func licenseRowsByName(rows []ddsnmp.LicenseRow) map[string]ddsnmp.LicenseRow {
	out := make(map[string]ddsnmp.LicenseRow, len(rows))
	for _, row := range rows {
		out[row.Name] = row
	}
	return out
}
