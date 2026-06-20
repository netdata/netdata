// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func topologyStatsToV1(stats topologyStats) map[string]any {
	if !stats.HasL2 && !stats.HasShape && !stats.HasFocus && !stats.HasL3 && !stats.HasOSPF && !stats.HasBGP && !stats.HasComputed {
		return nil
	}

	out := make(map[string]any)
	if stats.HasL2 {
		addTopologyL2Stats(out, stats)
	}
	if stats.HasShape {
		addTopologyShapeStats(out, stats)
	}
	if stats.HasFocus {
		addTopologyFocusStats(out, stats.Focus)
	}
	if stats.HasL3 {
		addTopologyL3Stats(out, stats)
	}
	if stats.HasOSPF {
		addTopologyOSPFStats(out, stats)
	}
	if stats.HasBGP {
		addTopologyBGPStats(out, stats)
	}
	if stats.HasComputed && !stats.HasL2 {
		out["actors_total"] = stats.Recomputed.ActorsTotal
		out["links_total"] = stats.Recomputed.LinksTotal
		out["links_probable"] = stats.Recomputed.LinksProbable
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func addTopologyL2Stats(out map[string]any, stats topologyStats) {
	l2 := stats.L2

	out["devices_total"] = l2.DevicesTotal
	out["devices_discovered"] = l2.DevicesDiscovered
	out["links_total"] = l2.LinksTotal
	out["links_lldp"] = l2.LinksLLDP
	out["links_cdp"] = l2.LinksCDP
	out["links_stp"] = l2.LinksSTP
	out["links_bidirectional"] = l2.LinksBidirectional
	out["links_unidirectional"] = l2.LinksUnidirectional
	out["links_fdb"] = l2.LinksFDB
	out["links_fdb_endpoint_candidates"] = l2.LinksFDBEndpointCandidates
	out["links_fdb_endpoint_emitted"] = l2.LinksFDBEndpointEmitted
	out["links_fdb_endpoint_suppressed"] = l2.LinksFDBEndpointSuppressed
	out["endpoints_ambiguous_segments"] = l2.EndpointsAmbiguousSegments
	out["links_arp"] = l2.LinksARP
	out["links_probable"] = l2.LinksProbable
	out["segments_suppressed"] = l2.SegmentsSuppressed
	out["actors_total"] = l2.ActorsTotal
	out["actors_unlinked_suppressed"] = l2.ActorsUnlinkedSuppressed
	out["endpoints_total"] = l2.EndpointsTotal
	out["inference_strategy"] = l2.InferenceStrategy

	out["attachments_total"] = l2.AttachmentsTotal
	out["attachments_fdb"] = l2.AttachmentsFDB
	out["enrichments_total"] = l2.EnrichmentsTotal
	out["enrichments_arp_nd"] = l2.EnrichmentsARPND
	out["bridge_domains_total"] = l2.BridgeDomainsTotal
	out["identity_alias_endpoints_mapped"] = l2.IdentityAliasEndpointsMapped
	out["identity_alias_endpoints_ambiguous_mac"] = l2.IdentityAliasEndpointsAmbiguousMAC
	out["identity_alias_ips_merged"] = l2.IdentityAliasIPsMerged
	out["identity_alias_ips_conflict_skipped"] = l2.IdentityAliasIPsConflictSkipped

	if stats.HasComputed {
		out["actors_total"] = stats.Recomputed.ActorsTotal
		out["links_total"] = stats.Recomputed.LinksTotal
		out["links_probable"] = stats.Recomputed.LinksProbable
	}
	if stats.HasShape {
		out["segments_suppressed"] = l2.SegmentsSuppressed + stats.Shape.SegmentsSparseSuppressed
		if stats.Shape.InferenceStrategy != "" {
			out["inference_strategy"] = stats.Shape.InferenceStrategy
		}
	}
}

func addTopologyShapeStats(out map[string]any, stats topologyStats) {
	shape := stats.Shape

	out["actors_collapsed_by_ip"] = shape.ActorsCollapsedByIP
	out["actors_non_ip_inferred_suppressed"] = shape.ActorsNonIPInferredSuppressed
	out["actors_map_type_suppressed"] = shape.ActorsMapTypeSuppressed
	out["segments_sparse_suppressed"] = shape.SegmentsSparseSuppressed
	out["map_type"] = shape.MapType
	if shape.InferenceStrategy != "" {
		out["inference_strategy"] = shape.InferenceStrategy
	}
	if _, ok := out["segments_suppressed"]; !ok {
		out["segments_suppressed"] = shape.SegmentsSparseSuppressed
	}
}

func addTopologyFocusStats(out map[string]any, stats topologyFocusStats) {
	out["managed_snmp_device_focus"] = stats.ManagedSNMPDeviceFocus
	if stats.Depth.All {
		out["depth"] = topologyDepthAll
	} else {
		out["depth"] = stats.Depth.Value
	}
	out["actors_focus_depth_filtered"] = stats.ActorsDepthFiltered
	out["links_focus_depth_filtered"] = stats.LinksDepthFiltered
}

func addTopologyL3Stats(out map[string]any, stats topologyStats) {
	l3 := stats.L3

	out["l3_subnet_candidate_subnets"] = l3.subnetStats.candidateSubnets
	out["l3_subnet_candidate_links"] = l3.subnetStats.candidateLinks
	out["l3_subnet_emitted_links"] = l3.emittedLinks
	out["l3_subnet_suppressed_invalid"] = l3.subnetStats.suppressedInvalid
	out["l3_subnet_suppressed_unsupported_prefix"] = l3.subnetStats.suppressedUnsupportedPrefix
	out["l3_subnet_suppressed_duplicate_ip"] = l3.subnetStats.suppressedDuplicateIP
	out["l3_subnet_suppressed_self_link"] = l3.subnetStats.suppressedSelfLink
	out["l3_subnet_suppressed_unmatched"] = l3.subnetStats.suppressedUnmatched
	out["l3_subnet_suppressed_multi_access"] = l3.subnetStats.suppressedMultiAccess
	out["l3_subnet_suppressed_unresolved_actor"] = l3.suppressedUnresolvedActor
	out["l3_subnet_suppressed_self_actor"] = l3.suppressedSelfActor
	out["l3_subnet_suppressed_duplicate_link"] = l3.suppressedDuplicateLink
	out["l3_subnet_visible_links"] = l3.emittedLinks
	if stats.HasComputed {
		out["l3_subnet_visible_links"] = stats.Recomputed.L3SubnetVisibleLinks
	}
}

func addTopologyOSPFStats(out map[string]any, stats topologyStats) {
	ospf := stats.OSPF

	out["ospf_neighbor_rows"] = ospf.observedRows
	out["ospf_neighbor_detail_rows"] = ospf.attachedNeighborRows
	out["ospf_adjacency_emitted_links"] = ospf.emittedLinks
	out["ospf_adjacency_suppressed_non_full_state"] = ospf.suppressedNonFullState
	out["ospf_adjacency_suppressed_unresolved_local"] = ospf.suppressedUnresolvedLocal
	out["ospf_adjacency_suppressed_unresolved_neighbor"] = ospf.suppressedUnresolvedNeighbor
	out["ospf_adjacency_suppressed_self_actor"] = ospf.suppressedSelfActor
	out["ospf_adjacency_suppressed_duplicate_link"] = ospf.suppressedDuplicateLink
	out["ospf_adjacency_visible_links"] = ospf.emittedLinks
	if stats.HasComputed {
		out["ospf_adjacency_visible_links"] = stats.Recomputed.OSPFAdjacencyVisibleLinks
	}
}

func addTopologyBGPStats(out map[string]any, stats topologyStats) {
	bgp := stats.BGP

	out["bgp_peer_rows"] = bgp.observedRows
	out["bgp_peer_detail_rows"] = bgp.attachedPeerRows
	out["bgp_adjacency_emitted_links"] = bgp.emittedLinks
	out["bgp_adjacency_suppressed_non_established_state"] = bgp.suppressedNonEstablished
	out["bgp_adjacency_suppressed_unresolved_local"] = bgp.suppressedUnresolvedLocal
	out["bgp_adjacency_suppressed_unresolved_neighbor"] = bgp.suppressedUnresolvedNeighbor
	out["bgp_adjacency_suppressed_self_actor"] = bgp.suppressedSelfActor
	out["bgp_adjacency_suppressed_duplicate_link"] = bgp.suppressedDuplicateLink
	out["bgp_adjacency_visible_links"] = bgp.emittedLinks
	if stats.HasComputed {
		out["bgp_adjacency_visible_links"] = stats.Recomputed.BGPAdjacencyVisibleLinks
	}
}
