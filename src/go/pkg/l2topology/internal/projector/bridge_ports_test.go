// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBridgeAttachmentSortKey_DistinguishesVLANAndMethod(t *testing.T) {
	base := model.Attachment{
		DeviceID:   "switch-a",
		IfIndex:    7,
		EndpointID: "mac:00:11:22:33:44:55",
		Labels: map[string]string{
			"if_name":     "swp07",
			"bridge_port": "7",
			"vlan_id":     "20",
		},
	}

	fdb := base
	fdb.Method = "fdb"
	arp := base
	arp.Method = "arp"
	otherVLAN := base
	otherVLAN.Method = "fdb"
	otherVLAN.Labels = map[string]string{
		"if_name":     "swp07",
		"bridge_port": "7",
		"vlan_id":     "30",
	}

	require.NotEqual(t, bridgeAttachmentSortKey(fdb), bridgeAttachmentSortKey(arp))
	require.NotEqual(t, bridgeAttachmentSortKey(fdb), bridgeAttachmentSortKey(otherVLAN))
}

func TestBridgePortFromAttachment_PreservesForwardingDomainWithoutDisplayingVLAN(t *testing.T) {
	first := bridgePortFromAttachment(model.Attachment{
		DeviceID: "switch-a",
		IfIndex:  7,
		Labels:   map[string]string{"fdb_domain_id": "fdb:100"},
	}, nil)
	second := bridgePortFromAttachment(model.Attachment{
		DeviceID: "switch-a",
		IfIndex:  7,
		Labels:   map[string]string{"fdb_domain_id": "fdb:200"},
	}, nil)

	require.Empty(t, first.vlanID)
	require.Empty(t, second.vlanID)
	require.NotEqual(t, bridgePortObservationVLANKey(first), bridgePortObservationVLANKey(second))
}

func TestBridgePortForwardingDomain_CanonicalizesVLANScope(t *testing.T) {
	require.Equal(t, "vlan:100", bridgePortForwardingDomain(bridgePortRef{fdbDomainID: "VLAN:100", vlanID: "100"}))
	require.Equal(t, "vlan:100", bridgePortForwardingDomain(bridgePortRef{fdbDomainID: "vlan:100"}))
	require.Equal(t, "vlan:100", bridgePortForwardingDomain(bridgePortRef{vlanID: "100"}))
}

func TestBridgePortRefDisplayKey_PreservesInterfacePunctuation(t *testing.T) {
	port := bridgePortRef{
		deviceID:   "switch-a",
		ifIndex:    1,
		ifName:     "uplink-a",
		bridgePort: "1",
	}

	ref := parseTopologySegmentPortRef(bridgePortRefDisplayKey(port))
	require.Equal(t, "uplink-a", topologySegmentPortDisplay(ref))
	require.NotEqual(t, bridgePortRefDisplayKey(port), bridgePortRefSortKey(port))
}

func TestCollectBridgeMacLinkRecords_KeepsSameEndpointAcrossRawForwardingDomains(t *testing.T) {
	records := collectBridgeMacLinkRecords([]model.Attachment{
		{
			DeviceID:   "switch-a",
			IfIndex:    7,
			EndpointID: "mac:00:11:22:33:44:55",
			Method:     "fdb",
			Labels:     map[string]string{"bridge_port": "7", "fdb_domain_id": "fdb:100"},
		},
		{
			DeviceID:   "switch-a",
			IfIndex:    7,
			EndpointID: "mac:00:11:22:33:44:55",
			Method:     "fdb",
			Labels:     map[string]string{"bridge_port": "7", "fdb_domain_id": "fdb:200"},
		},
	}, nil, nil)

	require.Len(t, records, 2)
	require.NotEqual(t, bridgePortForwardingDomain(records[0].port), bridgePortForwardingDomain(records[1].port))
}
