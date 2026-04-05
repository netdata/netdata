// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
)

func TestChartLayoutPriorities(t *testing.T) {
	assert.Less(t, prioFamilyPeerStates, prioFamilyPrefixes)
	assert.Less(t, prioFamilyPrefixes, prioFamilyCorrectness)
	assert.Less(t, prioFamilyCorrectness, prioFamilyMessages)
	assert.Less(t, prioFamilyMessages, prioFamilyRIBRoutes)
	assert.Less(t, prioFamilyRIBRoutes, prioFamilyPeerInventory)

	assert.Less(t, prioFamilyPeerInventory, prioRPKICacheState)
	assert.Less(t, prioRPKICachePrefixes, prioRPKIInventoryPrefixes)
	assert.Less(t, prioRPKIInventoryPrefixes, prioVNIEntries)
	assert.Less(t, prioVNIRemoteVTEPs, prioPeerState)

	assert.Less(t, prioPeerState, prioPeerUptime)
	assert.Less(t, prioPeerUptime, prioPeerPrefixes)
	assert.Less(t, prioPeerPrefixes, prioPeerPolicyPrefixes)
	assert.Less(t, prioPeerPolicyPrefixes, prioPeerAdvertisedPrefixes)
	assert.Less(t, prioPeerAdvertisedPrefixes, prioNeighborTransitions)
	assert.Less(t, prioNeighborTransitions, prioNeighborChurn)
	assert.Less(t, prioNeighborChurn, prioNeighborLastResetState)
	assert.Less(t, prioNeighborLastResetState, prioNeighborLastResetAge)
	assert.Less(t, prioNeighborLastResetAge, prioNeighborLastErrorCodes)
	assert.Less(t, prioNeighborLastErrorCodes, prioPeerMessages)
	assert.Less(t, prioPeerMessages, prioNeighborMessageTypes)

	assert.Less(t, prioNeighborMessageTypes, prioCollectorStatus)
	assert.Less(t, prioCollectorStatus, prioCollectorScrapeDuration)
	assert.Less(t, prioCollectorScrapeDuration, prioCollectorFailures)
	assert.Less(t, prioCollectorFailures, prioCollectorDeepQueries)
}

func TestChartDisplayHelpers(t *testing.T) {
	assert.Equal(t, "vrf default / IPv4 / unicast", familyDisplay(familyStats{
		Backend: backendFRR,
		VRF:     "default",
		AFI:     "ipv4",
		SAFI:    "unicast",
	}))

	assert.Equal(t, "table master4 / IPv4 / unicast", familyDisplay(familyStats{
		Backend: backendBIRD,
		VRF:     "master4",
		Table:   "master4",
		AFI:     "ipv4",
		SAFI:    "unicast",
	}))

	assert.Equal(t, "vrf default / L2VPN / EVPN", familyDisplay(familyStats{
		Backend: backendFRR,
		VRF:     "default",
		AFI:     "l2vpn",
		SAFI:    "evpn",
	}))

	assert.Equal(t, "vrf blue neighbor 2001:db8::1", neighborDisplay(neighborStats{
		Backend: backendGoBGP,
		VRF:     "blue",
		Address: "2001:db8::1",
	}))

	assert.Equal(t, "GoBGP RPKI cache cache-a", rpkiCacheDisplay(rpkiCacheStats{
		Backend: backendGoBGP,
		Name:    "cache-a",
	}))

	assert.Equal(t, "vrf tenant-a EVPN VNI 100", vniDisplay(vniStats{
		TenantVRF: "tenant-a",
		VNI:       100,
	}))
}

func TestChartLabelHelpers(t *testing.T) {
	family := familyStats{
		Backend: backendBIRD,
		VRF:     "master4",
		Table:   "master4",
		AFI:     "ipv4",
		SAFI:    "flowspec",
		LocalAS: 65001,
	}
	labels := familyLabels(family)
	assert.Equal(t, "table", labelValue(labels, "scope_kind"))
	assert.Equal(t, "master4", labelValue(labels, "scope_name"))
	assert.Equal(t, "ipv4/flowspec", labelValue(labels, "address_family"))
	assert.Equal(t, "master4", labelValue(labels, "table"))

	neighbor := neighborStats{
		Backend:  backendBIRD,
		VRF:      "flow4tab",
		Table:    "flow4tab",
		Address:  "198.51.100.2",
		RemoteAS: 65010,
	}
	neighborLbls := neighborLabels(neighbor)
	assert.Equal(t, "table", labelValue(neighborLbls, "scope_kind"))
	assert.Equal(t, "flow4tab", labelValue(neighborLbls, "scope_name"))

	vniLbls := vniLabels(vniStats{
		Backend:   backendFRR,
		TenantVRF: "tenant-a",
		VNI:       100,
		Type:      "l2",
	})
	assert.Equal(t, "vrf", labelValue(vniLbls, "scope_kind"))
	assert.Equal(t, "tenant-a", labelValue(vniLbls, "scope_name"))
}

func labelValue(labels []collectorapi.Label, key string) string {
	for _, label := range labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
}
