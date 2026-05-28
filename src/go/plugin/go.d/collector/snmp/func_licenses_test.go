// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFuncLicenses(cache *licenseCache) *funcLicenses {
	return newFuncLicenses(cache)
}

func TestLicensesMethodConfig_Parameterless(t *testing.T) {
	tests := map[string]struct {
		method string
	}{
		"licenses method has no required params": {
			method: licensesMethodID,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := licensesMethodConfig()
			assert.Empty(t, cfg.RequiredParams)

			params, err := newTestFuncLicenses(newLicenseCache()).MethodParams(context.Background(), tc.method)
			require.NoError(t, err)
			assert.Empty(t, params)
		})
	}
}

func TestFuncLicensesHandle(t *testing.T) {
	cache := newLicenseCache()
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)
	cache.store(now, []licenseRow{
		{
			ID:           "broken-license",
			Source:       "vendor",
			Name:         "Shared License",
			StateRaw:     "expired",
			StateBucket:  licenseStateBucketBroken,
			ExpiryTS:     now.Add(-time.Hour).Unix(),
			HasExpiry:    true,
			UsagePercent: 100,
			HasUsagePct:  true,
			Impact:       "Feature disabled",
		},
		{
			ID:          "eval-license",
			Source:      "vendor",
			Name:        "Evaluation License",
			StateRaw:    "evaluation",
			StateBucket: licenseStateBucketInformational,
		},
		{
			ID:           "healthy-license",
			Source:       "vendor",
			Name:         "Shared License",
			StateRaw:     "active",
			StateBucket:  licenseStateBucketHealthy,
			ExpiryTS:     now.Add(24 * time.Hour).Unix(),
			HasExpiry:    true,
			UsagePercent: 45,
			HasUsagePct:  true,
		},
		{
			ID:          "ignored-license",
			Source:      "vendor",
			Name:        "Unused License",
			StateRaw:    "not_subscribed",
			StateBucket: licenseStateBucketIgnored,
			IsPerpetual: true,
		},
	})

	resp := newTestFuncLicenses(cache).Handle(context.Background(), licensesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	require.NotNil(t, resp.Columns)
	require.Len(t, resp.Data.([][]any), 4)
	assert.Equal(t, "License", resp.DefaultSortColumn)

	rows := resp.Data.([][]any)
	assert.Equal(t, "Shared License", rows[0][0])
	assert.Equal(t, string(licenseStateBucketBroken), rows[0][1])
	assert.Equal(t, "Evaluation License", rows[1][0])
	assert.Equal(t, string(licenseStateBucketInformational), rows[1][1])
	assert.Equal(t, "Shared License", rows[2][0])
	assert.Equal(t, "Unused License", rows[3][0])
	assert.Equal(t, string(licenseStateBucketIgnored), rows[3][1])
	assert.NotEqual(t, rows[0][4], rows[2][4], "hidden row ID should stay unique when display labels collide")

	rowOptions, ok := resp.Columns["rowOptions"]
	require.True(t, ok)
	assert.NotNil(t, rowOptions)

	columns := resp.Columns
	idCol, ok := columns["ID"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, idCol["visible"])
	assert.Equal(t, true, idCol["unique_key"])
}

func TestFuncLicensesHandle_DefaultSortUsesDisplayedLicenseValue(t *testing.T) {
	tests := map[string]struct {
		rows      []licenseRow
		wantOrder []string
	}{
		"uses display label before hidden row id": {
			rows: []licenseRow{
				{
					ID:          "Zulu",
					Source:      "vendor",
					StateBucket: licenseStateBucketHealthy,
				},
				{
					ID:          "alpha-id",
					Source:      "vendor",
					Name:        "Alpha",
					StateBucket: licenseStateBucketHealthy,
				},
			},
			wantOrder: []string{"Alpha", "Zulu"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newLicenseCache()
			cache.store(time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC), tc.rows)

			resp := newTestFuncLicenses(cache).Handle(context.Background(), licensesMethodID, nil)
			require.NotNil(t, resp)
			require.Equal(t, 200, resp.Status)

			rows := resp.Data.([][]any)
			require.Len(t, rows, len(tc.wantOrder))
			for i, want := range tc.wantOrder {
				assert.Equal(t, want, rows[i][0])
			}
		})
	}
}

func TestLicenseRowUniqueKey_DistinctRows(t *testing.T) {
	tests := map[string]struct {
		left  licenseRow
		right licenseRow
	}{
		"escapes delimiter content": {
			left: licenseRow{
				Source: "source|a",
				Table:  "table",
				ID:     "id",
			},
			right: licenseRow{
				Source: "source",
				Table:  "table|id",
				ID:     "",
			},
		},
		"different tables stay distinct": {
			left: licenseRow{
				Source: "fortinet",
				Table:  "fgLicContractTable",
				ID:     "FortiCare",
				Name:   "FortiCare",
			},
			right: licenseRow{
				Source: "fortinet",
				Table:  "fgLicVersionTable",
				ID:     "FortiCare",
				Name:   "FortiCare",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotEqual(t, licenseRowUniqueKey(tc.left), licenseRowUniqueKey(tc.right))
		})
	}
}

func TestFuncLicensesHandleUnavailable(t *testing.T) {
	tests := map[string]struct {
		prepare func(cache *licenseCache)
	}{
		"before first collect": {},
		"when no rows were collected": {
			prepare: func(cache *licenseCache) {
				cache.store(time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC), nil)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newLicenseCache()
			if tc.prepare != nil {
				tc.prepare(cache)
			}

			resp := newTestFuncLicenses(cache).Handle(context.Background(), licensesMethodID, nil)
			require.NotNil(t, resp)
			assert.Equal(t, 503, resp.Status)
		})
	}
}

func TestFuncLicensesRemainingUsesCurrentTime(t *testing.T) {
	tests := map[string]struct {
		lastUpdate time.Time
		expiry     time.Time
	}{
		"uses current time instead of cache update time": {
			lastUpdate: time.Now().UTC().Add(-time.Hour),
			expiry:     time.Now().UTC().Add(time.Hour),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newLicenseCache()
			cache.store(tc.lastUpdate, []licenseRow{
				{
					ID:          "license-1",
					Name:        "Time Sensitive",
					StateBucket: licenseStateBucketHealthy,
					ExpiryTS:    tc.expiry.Unix(),
					HasExpiry:   true,
				},
			})

			resp := newTestFuncLicenses(cache).Handle(context.Background(), licensesMethodID, nil)
			require.NotNil(t, resp)
			require.Equal(t, 200, resp.Status)

			rows := resp.Data.([][]any)
			require.Len(t, rows, 1)

			remaining, ok := rows[0][8].(int64)
			require.True(t, ok)
			assert.GreaterOrEqual(t, remaining, int64((59*time.Minute)/time.Millisecond))
			assert.LessOrEqual(t, remaining, int64((61*time.Minute)/time.Millisecond))
			assert.Less(t, remaining, int64((2*time.Hour)/time.Millisecond))
		})
	}
}
