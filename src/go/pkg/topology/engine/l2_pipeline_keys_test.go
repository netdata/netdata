// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdjacencyKey_NormalizesProtocolAndEndpointWhitespace(t *testing.T) {
	raw := Adjacency{
		Protocol:   " LLDP ",
		SourceID:   " sw1 ",
		SourcePort: " Gi0/1 ",
		TargetID:   " sw2 ",
		TargetPort: " Gi0/2 ",
	}

	normalized := Adjacency{
		Protocol:   "lldp",
		SourceID:   "sw1",
		SourcePort: "Gi0/1",
		TargetID:   "sw2",
		TargetPort: "Gi0/2",
	}

	require.Equal(t, adjacencyKey(normalized), adjacencyKey(raw))
}

func TestAttachmentKey_NormalizesIDsAndMethod(t *testing.T) {
	raw := Attachment{
		DeviceID:   " sw1 ",
		IfIndex:    10,
		EndpointID: " endpoint-1 ",
		Method:     " FDB ",
		Labels: map[string]string{
			"vlan_id": " 100 ",
		},
	}

	normalized := Attachment{
		DeviceID:   "sw1",
		IfIndex:    10,
		EndpointID: "endpoint-1",
		Method:     "fdb",
		Labels: map[string]string{
			"vlan_id": "100",
		},
	}

	require.Equal(t, attachmentKey(normalized), attachmentKey(raw))
}

func TestAdjacencyKey_DistinguishesEmbeddedSeparators(t *testing.T) {
	first := Adjacency{
		Protocol:   "lldp",
		SourceID:   "node-a",
		SourcePort: "Gi0/1\x00Gi0/2",
		TargetID:   "node-b",
		TargetPort: "Eth1",
	}
	second := Adjacency{
		Protocol:   "lldp",
		SourceID:   "node-a\x00Gi0/1",
		SourcePort: "Gi0/2",
		TargetID:   "node-b",
		TargetPort: "Eth1",
	}

	require.NotEqual(t, adjacencyKey(first), adjacencyKey(second))
}

func TestIfaceKey_DistinguishesEmbeddedSeparators(t *testing.T) {
	first := Interface{
		DeviceID: "sw-a",
		IfIndex:  1,
		IfName:   "2\x00uplink",
	}
	second := Interface{
		DeviceID: "sw-a\x001",
		IfIndex:  2,
		IfName:   "uplink",
	}

	require.NotEqual(t, ifaceKey(first), ifaceKey(second))
}
