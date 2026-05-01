// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dataFRRRPKIPrefixCount []byte

func TestBuildFRRRPKIInventory(t *testing.T) {
	inv, err := buildFRRRPKIInventory(dataFRRRPKIPrefixCount)
	require.NoError(t, err)

	assert.Equal(t, "daemon", inv.ID)
	assert.Equal(t, backendFRR, inv.Backend)
	assert.Equal(t, "daemon", inv.Scope)
	assert.Equal(t, int64(3), inv.PrefixIPv4)
	assert.Equal(t, int64(0), inv.PrefixIPv6)
}

func TestCollector_CollectFRRRPKIInventoryOnly(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKIPrefixCount: dataFRRRPKIPrefixCount,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expected := map[string]int64{
		"rpki_inventory_daemon_prefix_ipv4": 3,
		"rpki_inventory_daemon_prefix_ipv6": 0,
	}
	assert.Equal(t, expected, mx)

	chart := collr.Charts().Get(rpkiInventoryPrefixesChartID("daemon"))
	require.NotNil(t, chart)
	assert.Equal(t, backendFRR, chartLabelValue(chart, "backend"))
	assert.Equal(t, "daemon", chartLabelValue(chart, "inventory_scope"))
}

func TestCollector_CollectFRRRPKICachesAndInventory(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKICacheServers:     dataFRRRPKICacheServer,
		frrRPKICacheConnections: dataFRRRPKICacheConnection,
		frrRPKIPrefixCount:      dataFRRRPKIPrefixCount,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	cacheID := idPart("tcp 127.0.0.1:3323 pref 1")
	expected := map[string]int64{
		"rpki_" + cacheID + "_up":           1,
		"rpki_" + cacheID + "_down":         0,
		"rpki_inventory_daemon_prefix_ipv4": 3,
		"rpki_inventory_daemon_prefix_ipv6": 0,
	}
	assert.Equal(t, expected, mx)
}

func TestCollector_CollectFRRRPKIInventoryQueryError(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKIPrefixCountErr: assert.AnError,
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 1, 0, 0, 0, 0, 0)
	assert.Empty(t, mx)
}

func TestCollector_CollectFRRRPKIInventoryParseError(t *testing.T) {
	collr := newFRRTestCollector(t, &mockClient{
		frrRPKIPrefixCount: []byte("{"),
	})

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 1, 0, 0, 0, 0)
	assert.Empty(t, mx)
}
