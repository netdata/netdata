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
	r := &funcRouter{licenseCache: cache}
	return newFuncLicenses(r)
}

func TestFuncLicensesHandle(t *testing.T) {
	cache := newLicenseCache()
	now := time.Date(2026, time.April, 3, 10, 0, 0, 0, time.UTC)
	cache.store(now, []licenseRow{
		{
			ID:           "broken-license",
			Name:         "Broken License",
			StateRaw:     "expired",
			StateBucket:  licenseStateBucketBroken,
			ExpiryTS:     now.Add(-time.Hour).Unix(),
			HasExpiry:    true,
			UsagePercent: 100,
			HasUsagePct:  true,
			Impact:       "Feature disabled",
		},
		{
			ID:             "ignored-license",
			Name:           "Optional Bundle",
			StateRaw:       "not_subscribed",
			StateBucket:    licenseStateBucketIgnored,
			IsPerpetual:    true,
			OriginalMetric: "_license_row",
		},
		{
			ID:           "healthy-license",
			Name:         "Healthy License",
			StateRaw:     "active",
			StateBucket:  licenseStateBucketHealthy,
			ExpiryTS:     now.Add(24 * time.Hour).Unix(),
			HasExpiry:    true,
			UsagePercent: 45,
			HasUsagePct:  true,
		},
	})

	resp := newTestFuncLicenses(cache).Handle(context.Background(), licensesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	require.NotNil(t, resp.Columns)
	require.Len(t, resp.Data.([][]any), 3)
	assert.Equal(t, "License", resp.DefaultSortColumn)

	rows := resp.Data.([][]any)
	assert.Equal(t, "Broken License", rows[0][0])
	assert.Equal(t, string(licenseStateBucketBroken), rows[0][1])
	assert.Equal(t, "Optional Bundle", rows[2][0])
	assert.Equal(t, string(licenseStateBucketIgnored), rows[2][1])
	assert.NotEqual(t, rows[0][4], rows[2][4], "hidden row ID should stay unique even if display labels collide")

	rowOptions, ok := resp.Columns["rowOptions"]
	require.True(t, ok)
	assert.NotNil(t, rowOptions)

	columns := resp.Columns
	idCol, ok := columns["ID"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, idCol["visible"])
	assert.Equal(t, true, idCol["unique_key"])
}

func TestFuncLicensesHandleUnavailable(t *testing.T) {
	resp := newTestFuncLicenses(newLicenseCache()).Handle(context.Background(), licensesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
}

func TestFuncLicensesRemainingUsesCurrentTime(t *testing.T) {
	cache := newLicenseCache()
	lastUpdate := time.Now().UTC().Add(-time.Hour)
	expiry := time.Now().UTC().Add(time.Hour)
	cache.store(lastUpdate, []licenseRow{
		{
			ID:          "license-1",
			Name:        "Time Sensitive",
			StateBucket: licenseStateBucketHealthy,
			ExpiryTS:    expiry.Unix(),
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
}
