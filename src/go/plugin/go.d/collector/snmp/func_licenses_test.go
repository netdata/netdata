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
			ID:            "ignored-license",
			Name:          "Optional Bundle",
			StateRaw:      "not_subscribed",
			StateBucket:   licenseStateBucketIgnored,
			IsPerpetual:   true,
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

	rowOptions, ok := resp.Columns["rowOptions"]
	require.True(t, ok)
	assert.NotNil(t, rowOptions)
}

func TestFuncLicensesHandleUnavailable(t *testing.T) {
	resp := newTestFuncLicenses(newLicenseCache()).Handle(context.Background(), licensesMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
}
