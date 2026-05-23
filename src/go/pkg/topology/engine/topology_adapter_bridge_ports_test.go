// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBridgeAttachmentSortKey_DistinguishesVLANAndMethod(t *testing.T) {
	base := Attachment{
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
