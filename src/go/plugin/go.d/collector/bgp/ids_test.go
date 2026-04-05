// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDPartKeepsEscapeBoundaries(t *testing.T) {
	assert.Equal(t, "_x3a_", idPart(":"))
	assert.Equal(t, "x3a", idPart("x3a"))
	assert.NotEqual(t, idPart(":"), idPart("x3a"))
	assert.NotEqual(t, makeCompositeID(":", "ipv4"), makeCompositeID("x3a", "ipv4"))
}

func TestFRRRPKICacheKeyUsesStructuredEncoding(t *testing.T) {
	key1 := frrRPKICacheKey("tcp a", "b", "c", 1)
	key2 := frrRPKICacheKey("tcp", "a b", "c", 1)
	assert.NotEqual(t, key1, key2)
}

func TestGoBGPPeerScopeAvoidsDelimiterCollisions(t *testing.T) {
	peer1 := &gobgpapi.Peer{
		State: &gobgpapi.PeerState{
			PeerGroup: "10",
			PeerAsn:   20,
		},
	}
	peer2 := &gobgpapi.Peer{
		State: &gobgpapi.PeerState{
			PeerGroup: "10/20",
		},
	}
	assert.NotEqual(t, gobgpPeerScope(peer1), gobgpPeerScope(peer2))
}

func TestGoBGPFamilyRefFromAPIUsesStructuredID(t *testing.T) {
	family := &gobgpapi.Family{
		Afi:  gobgpapi.Family_AFI_IP,
		Safi: gobgpapi.Family_SAFI_UNICAST,
	}

	ref1, ok := gobgpFamilyRefFromAPI(":", family)
	require.True(t, ok)
	ref2, ok := gobgpFamilyRefFromAPI("x3a", family)
	require.True(t, ok)
	assert.NotEqual(t, ref1.ID, ref2.ID)
}

func TestOpenBGPDPeerScopeAvoidsDelimiterCollisions(t *testing.T) {
	entry1 := openbgpdNeighbor{Group: "10"}
	entry2 := openbgpdNeighbor{Group: "10/20"}
	assert.NotEqual(t, openbgpdPeerScope(entry1, 20), openbgpdPeerScope(entry2, 0))
}
