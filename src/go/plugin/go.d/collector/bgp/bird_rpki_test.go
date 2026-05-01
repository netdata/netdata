// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataBIRDProtocolsAllRPKI []byte
)

func TestBuildBIRDRPKICaches(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllRPKI)
	require.NoError(t, err)

	caches := buildBIRDRPKICaches(protocols)
	require.Len(t, caches, 2)

	assert.Equal(t, "rpki1", caches[0].ID)
	assert.Equal(t, backendBIRD, caches[0].Backend)
	assert.Equal(t, "rpki1", caches[0].Name)
	assert.Equal(t, "Primary validator", caches[0].Desc)
	assert.Equal(t, "Established", caches[0].StateText)
	assert.True(t, caches[0].Up)
	assert.Equal(t, int64(300), caches[0].UptimeSecs)

	assert.Equal(t, "rpki2", caches[1].ID)
	assert.Equal(t, "Backup validator", caches[1].Desc)
	assert.Equal(t, "Active", caches[1].StateText)
	assert.False(t, caches[1].Up)
	assert.Zero(t, caches[1].UptimeSecs)
}

func TestCollector_CollectBIRDRPKIOnly(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	collr := newBIRDTestCollector(t, &mockClient{protocolsAll: dataBIRDProtocolsAllRPKI})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{
		"rpki_rpki1_up":             1,
		"rpki_rpki1_down":           0,
		"rpki_rpki1_uptime_seconds": 300,
		"rpki_rpki2_up":             0,
		"rpki_rpki2_down":           1,
		"rpki_rpki2_uptime_seconds": 0,
	}
	assert.Equal(t, expected, mx)

	require.NotNil(t, collr.Charts().Get(rpkiCacheStateChartID("rpki1")))
	require.NotNil(t, collr.Charts().Get("rpki_rpki1_uptime"))
	require.NotNil(t, collr.Charts().Get(rpkiCacheStateChartID("rpki2")))

	cacheChart := collr.Charts().Get(rpkiCacheStateChartID("rpki1"))
	require.NotNil(t, cacheChart)
	assert.Equal(t, backendBIRD, chartLabelValue(cacheChart, "backend"))
	assert.Equal(t, "rpki1", chartLabelValue(cacheChart, "cache"))
	assert.Equal(t, "Primary validator", chartLabelValue(cacheChart, "cache_desc"))
	assert.Equal(t, "Established", chartLabelValue(cacheChart, "state_text"))
}

func TestCollector_CheckBIRDRPKIOnly(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	collr := newBIRDTestCollector(t, &mockClient{protocolsAll: dataBIRDProtocolsAllRPKI})
	require.NoError(t, collr.Check(context.Background()))
}
