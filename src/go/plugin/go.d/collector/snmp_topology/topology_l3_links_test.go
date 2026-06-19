// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyL3ActorResolverSuppressesAmbiguousIP(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.1"),
		},
	}

	resolver := newTopologyL3ActorResolver(&data, nil)
	_, ok := resolver.resolve(topologyL3Interface{
		DeviceID: "unknown-device",
		IP:       "198.51.100.1",
	})

	require.False(t, ok)
}

func TestApplyTopologyL3SubnetEnrichmentSuppressesUnresolvedActor(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("device-a", nil, "198.51.100.1"),
		},
	}
	aggregate := topologyObservationAggregate{
		l3Interfaces: []topologyL3Interface{
			l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
			l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
		},
	}

	stats := applyTopologyL3SubnetEnrichment(&data, aggregate)

	require.Empty(t, data.Links)
	require.Equal(t, 1, stats.suppressedUnresolvedActor)
	require.Equal(t, 1, data.Stats["l3_subnet_candidate_links"])
	require.Equal(t, 0, data.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 0, data.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_suppressed_unresolved_actor"])
}

func TestApplyTopologyL3SubnetEnrichmentSuppressesSelfActor(t *testing.T) {
	data := topologyData{
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1", "198.51.100.2"),
		},
	}
	aggregate := topologyObservationAggregate{
		l3Interfaces: []topologyL3Interface{
			l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
			l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
		},
	}

	stats := applyTopologyL3SubnetEnrichment(&data, aggregate)

	require.Empty(t, data.Links)
	require.Equal(t, 1, stats.suppressedSelfActor)
	require.Equal(t, 1, data.Stats["l3_subnet_candidate_links"])
	require.Equal(t, 0, data.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 0, data.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_suppressed_self_actor"])
}

func TestApplyTopologyL3SubnetEnrichmentSuppressesDuplicateLink(t *testing.T) {
	existing := topologyLink{
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		SrcActorID: "router-a",
		DstActorID: "router-b",
		Metrics: map[string]any{
			"subnet": "198.51.100.0/30",
			"prefix": uint64(30),
		},
	}
	data := topologyData{
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		},
		Links: []topologyLink{existing},
	}
	aggregate := topologyObservationAggregate{
		l3Interfaces: []topologyL3Interface{
			l3InterfaceForTest("router-a", "198.51.100.1", "255.255.255.252", "2"),
			l3InterfaceForTest("router-b", "198.51.100.2", "255.255.255.252", "3"),
		},
	}

	stats := applyTopologyL3SubnetEnrichment(&data, aggregate)

	require.Len(t, data.Links, 1)
	require.Equal(t, existing, data.Links[0])
	require.Equal(t, 1, stats.suppressedDuplicateLink)
	require.Equal(t, 1, data.Stats["l3_subnet_candidate_links"])
	require.Equal(t, 0, data.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_visible_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_suppressed_duplicate_link"])
}

func TestApplyTopologyDepthFocusFilterKeepsIncidentL3SubnetLink(t *testing.T) {
	data := topologyData{
		Stats: map[string]any{
			"l3_subnet_emitted_links": 1,
		},
		Actors: []topologyActor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		},
		Links: []topologyLink{
			{
				Protocol:   topologyL3SubnetLinkType,
				LinkType:   topologyL3SubnetLinkType,
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Metrics: map[string]any{
					"subnet": "198.51.100.0/30",
					"prefix": 30,
				},
			},
		},
	}

	applyTopologyDepthFocusFilter(&data, topologyQueryOptions{
		ManagedDeviceFocus:     "ip:198.51.100.1",
		Depth:                  1,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		MapType:                topologyMapTypeHighConfidenceInferred,
		EliminateNonIPInferred: true,
	})

	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
	require.Equal(t, topologyL3SubnetLinkType, data.Links[0].LinkType)
	require.Equal(t, 1, data.Stats["l3_subnet_emitted_links"])
	require.Equal(t, 1, data.Stats["l3_subnet_visible_links"])
}

func topologyL3ManagedActorForTest(actorID string, attrs map[string]any, ips ...string) topologyActor {
	return topologyActor{
		ActorID:    actorID,
		ActorType:  "router",
		Layer:      "network",
		Source:     "snmp",
		Match:      topologyMatch{IPAddresses: ips},
		Attributes: attrs,
	}
}
