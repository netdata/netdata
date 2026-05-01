// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGoBGPRPKICaches(t *testing.T) {
	now := time.Date(2026, 4, 3, 19, 30, 0, 0, time.UTC)

	caches := buildGoBGPRPKICaches(now, testGoBGPRPKIServers(now))
	require.Len(t, caches, 2)

	assert.Equal(t, idPart("127.0.0.1:3323"), caches[0].ID)
	assert.Equal(t, backendGoBGP, caches[0].Backend)
	assert.Equal(t, "127.0.0.1:3323", caches[0].Name)
	assert.Equal(t, "up", caches[0].StateText)
	assert.True(t, caches[0].Up)
	assert.Equal(t, int64(180), caches[0].UptimeSecs)
	assert.True(t, caches[0].HasRecords)
	assert.Equal(t, int64(3), caches[0].RecordIPv4)
	assert.Equal(t, int64(0), caches[0].RecordIPv6)
	assert.True(t, caches[0].HasPrefixes)
	assert.Equal(t, int64(3), caches[0].PrefixIPv4)
	assert.Equal(t, int64(0), caches[0].PrefixIPv6)

	assert.Equal(t, idPart("192.0.2.10:3324"), caches[1].ID)
	assert.Equal(t, "192.0.2.10:3324", caches[1].Name)
	assert.Equal(t, "down", caches[1].StateText)
	assert.False(t, caches[1].Up)
	assert.Zero(t, caches[1].UptimeSecs)
	assert.True(t, caches[1].HasRecords)
	assert.Zero(t, caches[1].RecordIPv4)
	assert.Zero(t, caches[1].RecordIPv6)
	assert.True(t, caches[1].HasPrefixes)
	assert.Zero(t, caches[1].PrefixIPv4)
	assert.Zero(t, caches[1].PrefixIPv6)
}

func TestCollector_CollectGoBGPRPKIOnly(t *testing.T) {
	now := time.Now()

	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpRpki:   testGoBGPRPKIServers(now),
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.Address = "127.0.0.1:50051"
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{
		"rpki_" + idPart("127.0.0.1:3323") + "_up":              1,
		"rpki_" + idPart("127.0.0.1:3323") + "_down":            0,
		"rpki_" + idPart("127.0.0.1:3323") + "_record_ipv4":     3,
		"rpki_" + idPart("127.0.0.1:3323") + "_record_ipv6":     0,
		"rpki_" + idPart("127.0.0.1:3323") + "_prefix_ipv4":     3,
		"rpki_" + idPart("127.0.0.1:3323") + "_prefix_ipv6":     0,
		"rpki_" + idPart("192.0.2.10:3324") + "_up":             0,
		"rpki_" + idPart("192.0.2.10:3324") + "_down":           1,
		"rpki_" + idPart("192.0.2.10:3324") + "_uptime_seconds": 0,
		"rpki_" + idPart("192.0.2.10:3324") + "_record_ipv4":    0,
		"rpki_" + idPart("192.0.2.10:3324") + "_record_ipv6":    0,
		"rpki_" + idPart("192.0.2.10:3324") + "_prefix_ipv4":    0,
		"rpki_" + idPart("192.0.2.10:3324") + "_prefix_ipv6":    0,
	}
	assertMetricRange(t, mx, "rpki_"+idPart("127.0.0.1:3323")+"_uptime_seconds", 175, 185)
	delete(mx, "rpki_"+idPart("127.0.0.1:3323")+"_uptime_seconds")
	assert.Equal(t, expected, mx)

	cacheChart := collr.Charts().Get(rpkiCacheStateChartID(idPart("127.0.0.1:3323")))
	require.NotNil(t, cacheChart)
	assert.Equal(t, backendGoBGP, chartLabelValue(cacheChart, "backend"))
	assert.Equal(t, "127.0.0.1:3323", chartLabelValue(cacheChart, "cache"))
	assert.Equal(t, "up", chartLabelValue(cacheChart, "state_text"))
	require.NotNil(t, collr.Charts().Get(rpkiCacheRecordsChartID(idPart("127.0.0.1:3323"))))
	require.NotNil(t, collr.Charts().Get(rpkiCachePrefixesChartID(idPart("127.0.0.1:3323"))))
}

func TestCollector_CheckGoBGPRPKIOnly(t *testing.T) {
	now := time.Now()

	mock := &mockClient{
		gobgpGlobal: testGoBGPGlobal(),
		gobgpRpki:   testGoBGPRPKIServers(now),
	}

	collr := New()
	collr.Backend = backendGoBGP
	collr.Address = "127.0.0.1:50051"
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	require.NoError(t, collr.Check(context.Background()))
}
