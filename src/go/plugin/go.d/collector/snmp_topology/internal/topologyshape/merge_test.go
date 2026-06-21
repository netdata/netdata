// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestAppendUniqueTopologyStringsSortsAndDeduplicates(t *testing.T) {
	values := appendUniqueTopologyStrings([]string{" b ", "a"}, "a", "", " c ")
	require.Equal(t, []string{"a", "b", "c"}, values)
}

func TestMergeTopologyMatchUnionsIdentityAndFillsMissingSystemNames(t *testing.T) {
	merged := mergeTopologyMatch(
		topologymodel.Match{
			ChassisIDs:   []string{" chassis-b ", "chassis-a"},
			MacAddresses: []string{"aa:aa:aa:aa:aa:aa"},
			IPAddresses:  []string{"10.0.0.2"},
			Hostnames:    []string{"host-b"},
			DNSNames:     []string{"dns-b"},
			SysObjectID:  "1.3.6.1.4.1.1",
		},
		topologymodel.Match{
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
		topologymodel.ActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					PortsTotal: topologyengine.OptionalValue[int]{Value: 2, Has: true},
				},
			},
		},
		topologymodel.ActorDetail{
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

func TestCompareCollapseActorPriorityPrefersNonEmptyActorID(t *testing.T) {
	left := topologymodel.Actor{
		ActorID:   "",
		ActorType: "device",
		Layer:     "2",
		Source:    "snmp",
	}
	right := topologymodel.Actor{
		ActorID:   "device-1",
		ActorType: "device",
		Layer:     "2",
		Source:    "snmp",
	}

	require.Greater(t, compareCollapseActorPriority(left, right), 0)
	require.Less(t, compareCollapseActorPriority(right, left), 0)
}

func TestTopologyLinkDeltaKeyUsesStableEndpointAndBridgeFields(t *testing.T) {
	link := topologymodel.Link{
		Protocol:   " LLDP ",
		Direction:  " Bidirectional ",
		SrcActorID: " device:a ",
		DstActorID: " device:b ",
		Src: topologymodel.LinkEndpoint{
			IfIndex: 7,
			IfName:  "Gi0/1",
			PortID:  "port-a",
		},
		Dst: topologymodel.LinkEndpoint{
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

func TestTopologyLinkSortKeyUsesStableEndpointFields(t *testing.T) {
	link := topologymodel.Link{
		Protocol:  "lldp",
		Direction: "bidirectional",
		Src: topologymodel.LinkEndpoint{
			Match: topologymodel.Match{
				ChassisIDs: []string{"00:11:22:33:44:55"},
			},
			IfIndex: -1,
			IfName:  "Gi0/1",
			PortID:  "port-a",
		},
		Dst: topologymodel.LinkEndpoint{
			Match: topologymodel.Match{
				ChassisIDs: []string{"aa:bb:cc:dd:ee:ff"},
			},
			IfName: "Gi0/2",
			PortID: "port-b",
		},
		State: "up",
	}

	require.Equal(t, "lldp|bidirectional|mac:00:11:22:33:44:55|mac:aa:bb:cc:dd:ee:ff||Gi0/1|port-a||Gi0/2|port-b|up", topologymodel.LinkSortKey(link))
}

func TestTopologyEndpointKeyDropsNonPositiveIfIndex(t *testing.T) {
	tests := map[string]struct {
		ifIndex int
		want    string
	}{
		"negative": {ifIndex: -1},
		"zero":     {ifIndex: 0},
		"positive": {ifIndex: 7, want: "7"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, topologymodel.EndpointKey(topologymodel.LinkEndpoint{IfIndex: tc.ifIndex}, "if_index"))
		})
	}
}

func TestTopologyLinkActorKeyIncludesStateAndAttachmentMode(t *testing.T) {
	base := topologymodel.Link{
		Protocol:   "bridge",
		Direction:  "bidirectional",
		SrcActorID: "device:a",
		DstActorID: "endpoint:b",
		Src: topologymodel.LinkEndpoint{
			IfName: "Gi0/1",
		},
		Dst: topologymodel.LinkEndpoint{
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

func TestMarkProbableDeltaLinksPreservesExistingConfidenceAndAttachmentMode(t *testing.T) {
	strictData := topologymodel.Data{
		Links: []topologymodel.Link{
			{
				Protocol:   "lldp",
				Direction:  "bidirectional",
				SrcActorID: "device:a",
				DstActorID: "device:b",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 1,
					IfName:  "Gi0/1",
					PortID:  "1",
				},
				Dst: topologymodel.LinkEndpoint{
					IfIndex: 2,
					IfName:  "Gi0/2",
					PortID:  "2",
				},
			},
		},
	}
	probableData := topologymodel.Data{
		Links: []topologymodel.Link{
			strictData.Links[0],
			{
				Protocol:   "bridge",
				Direction:  "bidirectional",
				SrcActorID: "device:a",
				DstActorID: "segment:10",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 3,
					IfName:  "Gi0/3",
					PortID:  "3",
				},
				L2: &graph.LinkL2{
					BridgeDomain: "vlan-10",
				},
				Inference: &graph.LinkInference{
					Confidence:     "medium",
					AttachmentMode: "source_specific",
				},
			},
		},
	}

	MarkProbableDeltaLinks(&strictData, &probableData)

	require.Len(t, probableData.Links, 2)
	probableLink := probableData.Links[1]
	require.Equal(t, "probable", probableLink.State)
	require.Equal(t, "probable", topologymodel.LinkInferenceValue(probableLink))
	require.Equal(t, "medium", topologymodel.LinkConfidenceValue(probableLink))
	require.Equal(t, "source_specific", topologymodel.LinkAttachmentModeValue(probableLink))
}
