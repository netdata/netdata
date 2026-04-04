// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataFRRRPKICacheServer                 []byte
	dataFRRRPKICacheConnection             []byte
	dataFRRRPKICacheConnectionDisconnected []byte
)

func TestBuildFRRRPKICaches(t *testing.T) {
	caches, err := buildFRRRPKICaches(dataFRRRPKICacheServer, dataFRRRPKICacheConnection)
	require.NoError(t, err)
	require.Len(t, caches, 1)

	assert.Equal(t, idPart("tcp 127.0.0.1:3323 pref 1"), caches[0].ID)
	assert.Equal(t, backendFRR, caches[0].Backend)
	assert.Equal(t, "tcp 127.0.0.1:3323 pref 1", caches[0].Name)
	assert.Equal(t, "connected", caches[0].StateText)
	assert.True(t, caches[0].Up)
	assert.False(t, caches[0].HasUptime)
	assert.Zero(t, caches[0].UptimeSecs)
}

func TestBuildFRRRPKICachesDisconnected(t *testing.T) {
	caches, err := buildFRRRPKICaches(dataFRRRPKICacheServer, dataFRRRPKICacheConnectionDisconnected)
	require.NoError(t, err)
	require.Len(t, caches, 1)

	assert.Equal(t, "disconnected", caches[0].StateText)
	assert.False(t, caches[0].Up)
	assert.False(t, caches[0].HasUptime)
}

func TestCollector_CollectFRRRPKIOnly(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:     dataFRRRPKICacheServer,
		frrRPKICacheConnections: dataFRRRPKICacheConnection,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	cacheID := idPart("tcp 127.0.0.1:3323 pref 1")
	expected := map[string]int64{
		"rpki_" + cacheID + "_up":   1,
		"rpki_" + cacheID + "_down": 0,
	}
	assert.Equal(t, expected, mx)

	cacheChart := collr.Charts().Get(rpkiCacheStateChartID(cacheID))
	require.NotNil(t, cacheChart)
	assert.Equal(t, backendFRR, chartLabelValue(cacheChart, "backend"))
	assert.Equal(t, "tcp 127.0.0.1:3323 pref 1", chartLabelValue(cacheChart, "cache"))
	assert.Equal(t, "connected", chartLabelValue(cacheChart, "state_text"))
	require.Nil(t, collr.Charts().Get(rpkiCacheUptimeChartID(cacheID)))
}

func TestCollector_CollectFRRRPKIDisconnected(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:     dataFRRRPKICacheServer,
		frrRPKICacheConnections: dataFRRRPKICacheConnectionDisconnected,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	cacheID := idPart("tcp 127.0.0.1:3323 pref 1")
	expected := map[string]int64{
		"rpki_" + cacheID + "_up":   0,
		"rpki_" + cacheID + "_down": 1,
	}
	assert.Equal(t, expected, mx)
}

func TestCollector_CollectFRRRPKIConnectionQueryError(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:        dataFRRRPKICacheServer,
		frrRPKICacheConnectionsErr: assert.AnError,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 1, 0, 0, 0, 0, 0)
	assert.Empty(t, mx)
}

func TestCollector_CollectFRRRPKICacheParseError(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:     dataFRRRPKICacheServer,
		frrRPKICacheConnections: []byte("{"),
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 1, 0, 0, 0, 0)
	assert.Empty(t, mx)
}

func TestCollector_CheckFRRRPKIOnly(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:     dataFRRRPKICacheServer,
		frrRPKICacheConnections: dataFRRRPKICacheConnection,
	})

	require.NoError(t, collr.Check(context.Background()))
}

func newFRRTestCollector(t *testing.T, mock *mockClient) *Collector {
	t.Helper()

	collr := New()
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	require.NoError(t, collr.Init(context.Background()))
	return collr
}
