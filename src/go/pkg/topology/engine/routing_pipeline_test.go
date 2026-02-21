// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOSPFLinksEnlinkd_AddressLessAssociation(t *testing.T) {
	neighbors := []OSPFNeighborObservation{
		{
			RemoteRouterID:       "192.168.9.1",
			RemoteIP:             "172.16.7.2",
			RemoteAddressLessIdx: 7,
		},
	}
	interfaces := []OSPFInterfaceObservation{
		{
			IP:             "172.16.7.1",
			AddressLessIdx: 17619,
			AreaID:         "0.0.0.0",
			IfIndex:        17619,
			IfName:         "ge-1/1/6.0",
			Netmask:        "255.255.255.252",
		},
	}

	links := BuildOSPFLinksEnlinkd("delhi", neighbors, interfaces)
	require.Len(t, links, 1)
	require.Equal(t, "delhi", links[0].DeviceID)
	require.Equal(t, "172.16.7.1", links[0].LocalIP)
	require.Equal(t, "172.16.7.2", links[0].RemoteIP)
	require.Equal(t, 17619, links[0].IfIndex)
	require.Equal(t, "ge-1/1/6.0", links[0].IfName)
	require.Equal(t, "0.0.0.0", links[0].AreaID)
}

func TestBuildOSPFLinksEnlinkd_NetworkAssociation(t *testing.T) {
	neighbors := []OSPFNeighborObservation{
		{
			RemoteRouterID:       "192.168.7.1",
			RemoteIP:             "192.168.1.6",
			RemoteAddressLessIdx: 0,
		},
	}
	interfaces := []OSPFInterfaceObservation{
		{
			IP:             "192.168.1.5",
			AddressLessIdx: 0,
			AreaID:         "0.0.0.0",
			IfIndex:        3674,
			IfName:         "ge-1/0/1.0",
			Netmask:        "255.255.255.252",
		},
	}

	links := BuildOSPFLinksEnlinkd("delhi", neighbors, interfaces)
	require.Len(t, links, 1)
	require.Equal(t, "192.168.1.5", links[0].LocalIP)
	require.Equal(t, "192.168.1.6", links[0].RemoteIP)
	require.Equal(t, 3674, links[0].IfIndex)
}

func TestMatchOSPFLinksEnlinkd_ReversedPair(t *testing.T) {
	links := []OSPFLinkObservation{
		{
			ID:       "mumbai-1",
			DeviceID: "mumbai",
			LocalIP:  "192.168.5.9",
			RemoteIP: "192.168.5.10",
			IfIndex:  519,
		},
		{
			ID:       "delhi-1",
			DeviceID: "delhi",
			LocalIP:  "192.168.5.10",
			RemoteIP: "192.168.5.9",
			IfIndex:  28503,
		},
		{
			ID:       "orphan",
			DeviceID: "orphan",
			LocalIP:  "10.0.0.1",
			RemoteIP: "10.0.0.2",
		},
	}

	pairs := MatchOSPFLinksEnlinkd(links)
	require.Len(t, pairs, 1)
	require.Equal(t, "mumbai", pairs[0].Source.DeviceID)
	require.Equal(t, "delhi", pairs[0].Target.DeviceID)
	require.Equal(t, "192.168.5.9", pairs[0].Source.LocalIP)
	require.Equal(t, "192.168.5.10", pairs[0].Target.LocalIP)
}

func TestMatchISISLinksEnlinkd_SymmetricPair(t *testing.T) {
	elements := []ISISElementObservation{
		{DeviceID: "froh", SysID: "000110088500"},
		{DeviceID: "siegfrie", SysID: "000110255054"},
		{DeviceID: "oedipus", SysID: "000110255062"},
	}
	links := []ISISLinkObservation{
		{
			ID:            "froh-600",
			DeviceID:      "froh",
			AdjIndex:      1,
			CircIfIndex:   600,
			NeighborSysID: "000110255054",
		},
		{
			ID:            "siegfrie-533",
			DeviceID:      "siegfrie",
			AdjIndex:      1,
			CircIfIndex:   533,
			NeighborSysID: "000110088500",
		},
		{
			ID:            "unpaired",
			DeviceID:      "oedipus",
			AdjIndex:      1,
			CircIfIndex:   575,
			NeighborSysID: "000110255054",
		},
	}

	pairs := MatchISISLinksEnlinkd(elements, links)
	require.Len(t, pairs, 1)
	require.Equal(t, "froh", pairs[0].Source.DeviceID)
	require.Equal(t, "siegfrie", pairs[0].Target.DeviceID)
	require.Equal(t, 1, pairs[0].Source.AdjIndex)
}
