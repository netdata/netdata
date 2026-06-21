// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestAppendUniqueTopologyStringsSortsAndDeduplicates(t *testing.T) {
	values := appendUniqueTopologyStrings([]string{" b ", "a"}, "a", "", " c ")
	require.Equal(t, []string{"a", "b", "c"}, values)
}

func TestMergeTopologyMatchUnionsIdentityAndFillsMissingSystemNames(t *testing.T) {
	merged := mergeTopologyMatch(
		topologyMatch{
			ChassisIDs:   []string{" chassis-b ", "chassis-a"},
			MacAddresses: []string{"aa:aa:aa:aa:aa:aa"},
			IPAddresses:  []string{"10.0.0.2"},
			Hostnames:    []string{"host-b"},
			DNSNames:     []string{"dns-b"},
			SysObjectID:  "1.3.6.1.4.1.1",
		},
		topologyMatch{
			ChassisIDs:   []string{"chassis-c", "chassis-a"},
			MacAddresses: []string{"bb:bb:bb:bb:bb:bb", "aa:aa:aa:aa:aa:aa"},
			IPAddresses:  []string{"10.0.0.1", "10.0.0.2"},
			Hostnames:    []string{"host-a"},
			DNSNames:     []string{"dns-a"},
			SysName:      " sw-a ",
			SysObjectID:  "1.3.6.1.4.1.2",
		},
	)

	require.Equal(t, []string{"chassis-a", "chassis-b", "chassis-c"}, merged.ChassisIDs)
	require.Equal(t, []string{"aa:aa:aa:aa:aa:aa", "bb:bb:bb:bb:bb:bb"}, merged.MacAddresses)
	require.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, merged.IPAddresses)
	require.Equal(t, []string{"host-a", "host-b"}, merged.Hostnames)
	require.Equal(t, []string{"dns-a", "dns-b"}, merged.DNSNames)
	require.Equal(t, "sw-a", merged.SysName)
	require.Equal(t, "1.3.6.1.4.1.1", merged.SysObjectID)
}

func TestMergeTopologyStringMapKeepsExistingAndIgnoresBlankEntries(t *testing.T) {
	base := map[string]string{
		"existing": "keep",
	}

	merged := mergeTopologyStringMap(base, map[string]string{
		"existing": "replace",
		" new ":    " value ",
		"blank":    " ",
		"":         "ignored",
	})

	require.Equal(t, map[string]string{
		"existing": "keep",
		"new":      "value",
	}, merged)
}

func TestMergeTopologyAnyMapKeepsExistingAndAddsMissingKeys(t *testing.T) {
	base := map[string]any{
		"existing": "keep",
	}

	merged := mergeTopologyAnyMap(base, map[string]any{
		"existing": "replace",
		" new ":    42,
		"":         "ignored",
	})

	require.Equal(t, "keep", merged["existing"])
	require.Equal(t, 42, merged["new"])
	require.NotContains(t, merged, "")
}

func TestMergeTopologyActorDetailPreservesTypedFieldPresence(t *testing.T) {
	merged := mergeTopologyActorDetail(
		topologyActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					PortsTotal: topologyengine.OptionalValue[int]{Value: 2, Has: true},
				},
			},
		},
		topologyActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					PortsTotal:       topologyengine.OptionalValue[int]{Value: 5, Has: true},
					CDPNeighborCount: topologyengine.OptionalValue[int]{Value: 0, Has: true},
				},
				Segment: topologyengine.ProjectionSegmentActorDetail{
					EndpointsTotal: topologyengine.OptionalValue[int]{Value: 0, Has: true},
				},
			},
		},
	)

	require.True(t, merged.L2.Device.PortsTotal.Has)
	require.Equal(t, 2, merged.L2.Device.PortsTotal.Value)
	require.True(t, merged.L2.Device.CDPNeighborCount.Has)
	require.Zero(t, merged.L2.Device.CDPNeighborCount.Value)
	require.True(t, merged.L2.Segment.EndpointsTotal.Has)
	require.Zero(t, merged.L2.Segment.EndpointsTotal.Value)
}

func TestTopologyLinkDeltaKeyUsesStableEndpointAndBridgeFields(t *testing.T) {
	link := topologyLink{
		Protocol:   " LLDP ",
		Direction:  " Bidirectional ",
		SrcActorID: " device:a ",
		DstActorID: " device:b ",
		Src: topologyLinkEndpoint{
			IfIndex: 7,
			IfName:  "Gi0/1",
			PortID:  "port-a",
		},
		Dst: topologyLinkEndpoint{
			IfIndex: 8,
			IfName:  "Gi0/2",
			PortID:  "port-b",
		},
		L2: &graph.LinkL2{
			BridgeDomain: "vlan-10",
		},
		State: "probable",
	}

	require.Equal(t, "lldp|bidirectional|device:a|device:b|7|Gi0/1|port-a|8|Gi0/2|port-b|vlan-10", topologyLinkDeltaKey(link))
}

func TestTopologyLinkActorKeyIncludesStateAndAttachmentMode(t *testing.T) {
	base := topologyLink{
		Protocol:   "bridge",
		Direction:  "bidirectional",
		SrcActorID: "device:a",
		DstActorID: "endpoint:b",
		Src: topologyLinkEndpoint{
			IfName: "Gi0/1",
		},
		Dst: topologyLinkEndpoint{
			IfName: "Gi0/2",
		},
		L2: &graph.LinkL2{
			BridgeDomain: "vlan-200",
		},
		Inference: &graph.LinkInference{
			AttachmentMode: "probable_bridge_anchor",
			Inference:      "probable",
		},
		State: "probable",
	}

	strict := base
	strict.State = ""
	strict.Inference = nil

	require.NotEqual(t, topologyLinkActorKey(base), topologyLinkActorKey(strict))
	require.NotEqual(t, topologyLinkDeltaKey(base), topologyLinkActorKey(base))
}
