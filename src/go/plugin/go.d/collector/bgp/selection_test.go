// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_SelectFamiliesFallbackWhenOverLimit(t *testing.T) {
	collr := New()
	collr.MaxFamilies = 1
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	selected := collr.selectFamilies([]familyStats{
		{ID: "default_ipv6_unicast", VRF: "default", AFI: "ipv6", SAFI: "unicast"},
		{ID: "default_ipv4_unicast", VRF: "default", AFI: "ipv4", SAFI: "unicast"},
	})

	assert.Equal(t, map[string]bool{
		"default_ipv4_unicast": true,
	}, selected)
}

func TestCollector_SelectPeersFallbackWhenOverLimit(t *testing.T) {
	collr := New()
	collr.MaxPeers = 2
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	families := []familyStats{
		{
			ID: "default_ipv4_unicast",
			Peers: []peerStats{
				{ID: "peer-z"},
				{ID: "peer-a"},
			},
		},
		{
			ID: "default_ipv6_unicast",
			Peers: []peerStats{
				{ID: "peer-m"},
			},
		},
	}

	selected := collr.selectPeers(families, map[string]bool{
		"default_ipv4_unicast": true,
		"default_ipv6_unicast": true,
	})

	assert.Equal(t, map[string]bool{
		"peer-a": true,
		"peer-m": true,
	}, selected)
}

func TestCollector_SelectVNIsFallbackWhenOverLimit(t *testing.T) {
	collr := New()
	collr.MaxVNIs = 1
	require.NoError(t, collr.Init(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	selected := collr.selectVNIs([]vniStats{
		{ID: makeVNIID("default", 200, "l2"), TenantVRF: "default", VNI: 200},
		{ID: makeVNIID("default", 100, "l2"), TenantVRF: "default", VNI: 100},
	})

	assert.Equal(t, map[string]bool{
		makeVNIID("default", 100, "l2"): true,
	}, selected)
}
