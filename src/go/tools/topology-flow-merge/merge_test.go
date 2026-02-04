// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMergeTopology(t *testing.T) {
	now := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	input1 := topologyData{
		SchemaVersion: "1.0",
		AgentID:       "a1",
		CollectedAt:   now,
		Devices: []topologyDevice{
			{ChassisID: "aa", ChassisIDType: "mac"},
			{ChassisID: "bb", ChassisIDType: "mac"},
		},
		Links: []topologyLink{
			{
				Protocol: "lldp",
				Src: topologyEndpoint{ChassisID: "aa", ChassisIDType: "mac", PortID: "1"},
				Dst: topologyEndpoint{ChassisID: "bb", ChassisIDType: "mac", PortID: "1"},
			},
		},
	}
	input2 := topologyData{
		SchemaVersion: "1.0",
		AgentID:       "a2",
		CollectedAt:   now,
		Devices: []topologyDevice{
			{ChassisID: "aa", ChassisIDType: "mac"},
			{ChassisID: "bb", ChassisIDType: "mac"},
		},
		Links: []topologyLink{
			{
				Protocol: "lldp",
				Src: topologyEndpoint{ChassisID: "bb", ChassisIDType: "mac", PortID: "1"},
				Dst: topologyEndpoint{ChassisID: "aa", ChassisIDType: "mac", PortID: "1"},
			},
		},
	}

	merged := mergeTopology([]topologyData{input1, input2})
	require.Len(t, merged.Links, 1)
	require.True(t, merged.Links[0].Bidirectional)
	require.Equal(t, 2, merged.Stats["devices"])
}

func TestMergeFlows(t *testing.T) {
	now := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	key := &flowKey{SrcPrefix: "10.0.0.1/32", DstPrefix: "10.0.0.2/32", SrcPort: 123, DstPort: 80, Protocol: 6}

	f1 := flowsData{
		SchemaVersion: "1.0",
		AgentID:       "a1",
		PeriodStart:   now,
		PeriodEnd:     now.Add(10 * time.Second),
		Buckets: []flowBucket{
			{Timestamp: now, DurationSec: 10, Key: key, Bytes: 100, Packets: 10, Flows: 1},
		},
	}
	f2 := flowsData{
		SchemaVersion: "1.0",
		AgentID:       "a2",
		PeriodStart:   now,
		PeriodEnd:     now.Add(10 * time.Second),
		Buckets: []flowBucket{
			{Timestamp: now, DurationSec: 10, Key: key, Bytes: 200, Packets: 20, Flows: 2},
		},
	}

	merged := mergeFlows([]flowsData{f1, f2})
	require.Len(t, merged.Buckets, 1)
	require.Equal(t, uint64(300), merged.Buckets[0].Bytes)
	require.Equal(t, uint64(30), merged.Buckets[0].Packets)
	require.Equal(t, uint64(3), merged.Buckets[0].Flows)
	require.Equal(t, uint64(300), merged.Summaries["total_bytes"])
}
