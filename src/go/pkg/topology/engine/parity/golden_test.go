// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoldenYAMLValidation_RejectsBidirectionalPairMismatch(t *testing.T) {
	doc := GoldenDocument{
		Version:    GoldenVersion,
		ScenarioID: "pair-mismatch",
		Devices: []GoldenDevice{
			{ID: "a", Hostname: "a"},
			{ID: "b", Hostname: "b"},
		},
		Adjacencies: []GoldenAdjacency{
			{Protocol: "lldp", SourceDevice: "a", SourcePort: "Gi0/1", TargetDevice: "b", TargetPort: "Gi0/2"},
			{Protocol: "lldp", SourceDevice: "b", SourcePort: "Gi0/2", TargetDevice: "a", TargetPort: "Gi0/1"},
		},
		Expectations: GoldenCounts{
			DirectionalAdjacencies: 2,
			BidirectionalPairs:     0,
			Devices:                2,
		},
	}

	err := doc.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "expectations.bidirectional_pairs")
}

func TestGoldenYAMLValidation_AcceptsBidirectionalDevicePairsWithPortAliases(t *testing.T) {
	doc := GoldenDocument{
		Version:    GoldenVersion,
		ScenarioID: "pair-port-aliases",
		Devices: []GoldenDevice{
			{ID: "a", Hostname: "a"},
			{ID: "b", Hostname: "b"},
		},
		Adjacencies: []GoldenAdjacency{
			{Protocol: "cdp", SourceDevice: "a", SourcePort: "Gi0/0", TargetDevice: "b", TargetPort: "GigabitEthernet0/1"},
			{Protocol: "cdp", SourceDevice: "b", SourcePort: "Gi0/1", TargetDevice: "a", TargetPort: "GigabitEthernet0/0"},
		},
		Expectations: GoldenCounts{
			DirectionalAdjacencies: 2,
			BidirectionalPairs:     1,
			Devices:                2,
		},
	}

	require.NoError(t, doc.validate())
}

func TestGoldenYAMLValidation_SelfLoopsDoNotCountAsBidirectionalPairs(t *testing.T) {
	doc := GoldenDocument{
		Version:    GoldenVersion,
		ScenarioID: "self-loop",
		Devices: []GoldenDevice{
			{ID: "a", Hostname: "a"},
		},
		Adjacencies: []GoldenAdjacency{
			{Protocol: "lldp", SourceDevice: "a", SourcePort: "Gi0/1", TargetDevice: "a", TargetPort: "Gi0/1"},
		},
		Expectations: GoldenCounts{
			DirectionalAdjacencies: 1,
			BidirectionalPairs:     1,
			Devices:                1,
		},
	}

	err := doc.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "expectations.bidirectional_pairs")
}

func TestGoldenCanonicalJSON_UsesEmptyArrayForEmptyAdjacencies(t *testing.T) {
	doc := GoldenDocument{
		Version:     GoldenVersion,
		ScenarioID:  "empty-adjacencies",
		Devices:     []GoldenDevice{{ID: "a", Hostname: "a"}},
		Adjacencies: nil,
		Expectations: GoldenCounts{
			DirectionalAdjacencies: 0,
			BidirectionalPairs:     0,
			Devices:                1,
		},
	}

	payload, err := doc.CanonicalJSON()
	require.NoError(t, err)
	require.Contains(t, string(payload), "\"adjacencies\": []")
}
