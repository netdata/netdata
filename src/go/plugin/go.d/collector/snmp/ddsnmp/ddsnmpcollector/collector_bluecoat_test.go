// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strconv"
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

func TestCollector_Collect_BlueCoatLicensingProfile(t *testing.T) {
	tests := map[string]struct {
		rows       []blueCoatLicenseRow
		assertRows func(*testing.T, []ddsnmp.LicenseRow)
	}{
		"mixed subscription demo and perpetual rows": {
			rows: []blueCoatLicenseRow{
				{
					index:        1,
					application:  "ProxySG",
					feature:      "WebFilter",
					component:    "policy-engine",
					expireType:   2,
					state:        1,
					expiry:       time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC),
					hasExpiryPDU: true,
				},
				{
					index:        2,
					application:  "ProxySG",
					feature:      "Demo Malware Analysis",
					component:    "sandbox",
					expireType:   3,
					state:        2,
					expiry:       time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
					hasExpiryPDU: true,
				},
				{
					index:       3,
					application: "ProxySG",
					feature:     "Base Platform",
					component:   "proxy-core",
					expireType:  1,
					state:       1,
				},
			},
			assertRows: func(t *testing.T, rows []ddsnmp.LicenseRow) {
				require.Len(t, rows, 3)
				byID := licenseRowsByID(rows)

				subscription := byID["1"]
				require.NotEmpty(t, subscription)
				require.EqualValues(t, time.Date(2030, time.November, 11, 0, 0, 0, 0, time.UTC).Unix(), subscription.Expiry.Timestamp)
				require.EqualValues(t, 0, subscription.State.Severity)
				assert.Equal(t, "ProxySG", subscription.Name)
				assert.Equal(t, "WebFilter", subscription.Feature)
				assert.Equal(t, "policy-engine", subscription.Component)
				assert.Equal(t, "subscription", subscription.Type)
				assert.Equal(t, "1", subscription.State.Raw)

				expiredDemo := byID["2"]
				require.NotEmpty(t, expiredDemo)
				require.EqualValues(t, time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC).Unix(), expiredDemo.Expiry.Timestamp)
				require.EqualValues(t, 2, expiredDemo.State.Severity)
				assert.Equal(t, "demo", expiredDemo.Type)
				assert.Equal(t, "2", expiredDemo.State.Raw)

				perpetual := byID["3"]
				require.NotEmpty(t, perpetual)
				require.EqualValues(t, 0, perpetual.State.Severity)
				assert.Equal(t, "perpetual", perpetual.Type)
				assert.True(t, perpetual.IsPerpetual)
				assert.Equal(t, "1", perpetual.State.Raw)
				assert.False(t, perpetual.Expiry.Has)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectBlueCoatLicensingWalk(mockHandler, tc.rows...)
			profile := mustLoadTypedLicensingProfile(t, "bluecoat-proxysg", func(row ddprofiledefinition.LicensingConfig) bool {
				return row.ID == "app_license_status"
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

type blueCoatLicenseRow struct {
	index        int
	application  string
	feature      string
	component    string
	expireType   int
	state        int
	expiry       time.Time
	hasExpiryPDU bool
}

func expectBlueCoatLicensingWalk(mockHandler *snmpmock.MockHandler, rows ...blueCoatLicenseRow) {
	pdus := make([]gosnmp.SnmpPDU, 0, len(rows)*7)

	for _, row := range rows {
		idx := strconv.Itoa(row.index)

		pdus = append(pdus,
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.1."+idx, row.index),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.2."+idx, row.application),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.3."+idx, row.feature),
			createStringPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.4."+idx, row.component),
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.5."+idx, row.expireType),
			createIntegerPDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.7."+idx, row.state),
		)
		if row.hasExpiryPDU {
			pdus = append(pdus, createDateAndTimePDU("1.3.6.1.4.1.3417.2.16.1.1.1.1.6."+idx, row.expiry))
		}
	}

	expectSNMPWalk(mockHandler,
		gosnmp.Version2c,
		"1.3.6.1.4.1.3417.2.16.1.1.1",
		pdus,
	)
}
