// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type Stats struct {
	L2          topologyengine.ProjectionStats
	HasL2       bool
	Shape       ShapeStats
	HasShape    bool
	Focus       FocusStats
	HasFocus    bool
	L3          L3EnrichmentStats
	HasL3       bool
	OSPF        OSPFEnrichmentStats
	HasOSPF     bool
	BGP         BGPEnrichmentStats
	HasBGP      bool
	Recomputed  RecomputedStats
	HasComputed bool
}

type ShapeStats struct {
	ActorsCollapsedByIP           int
	ActorsNonIPInferredSuppressed int
	ActorsMapTypeSuppressed       int
	SegmentsSparseSuppressed      int
	MapType                       string
	InferenceStrategy             string
}

type FocusStats struct {
	ManagedSNMPDeviceFocus string
	Depth                  FocusDepth
	ActorsDepthFiltered    int
	LinksDepthFiltered     int
}

type FocusDepth struct {
	All   bool
	Value int
}

type RecomputedStats struct {
	ActorsTotal                    int
	LinksTotal                     int
	LinksProbable                  int
	L3SubnetVisibleLinks           int
	L3SubnetMembershipVisibleLinks int
	OSPFAdjacencyVisibleLinks      int
	BGPAdjacencyVisibleLinks       int
}

type L3EnrichmentStats struct {
	SubnetStats                         L3SubnetBuildStats
	EmittedLinks                        int
	EmittedSegments                     int
	EmittedMembershipLinks              int
	SuppressedUnresolvedActor           int
	SuppressedSelfActor                 int
	SuppressedDuplicateLink             int
	SuppressedNoProducerScope           int
	SuppressedMembershipUnresolvedActor int
	SuppressedMembershipUnmatched       int
	SuppressedDuplicateMembershipLink   int
}

type L3SubnetBuildStats struct {
	CandidateSubnets             int
	CandidateLinks               int
	CandidateSegments            int
	CandidateMemberships         int
	SuppressedInvalid            int
	SuppressedUnsupportedPrefix  int
	SuppressedDuplicateIP        int
	SuppressedSegmentDuplicateIP int
	SuppressedSelfLink           int
	SuppressedUnmatched          int
	SuppressedMultiAccess        int
	SuppressedSegmentUnmatched   int
}

type OSPFEnrichmentStats struct {
	ObservedRows                 int
	EmittedLinks                 int
	AttachedNeighborRows         int
	SuppressedNonFullState       int
	SuppressedUnresolvedLocal    int
	SuppressedUnresolvedNeighbor int
	SuppressedSelfActor          int
	SuppressedDuplicateLink      int
}

type BGPEnrichmentStats struct {
	ObservedRows                 int
	EmittedLinks                 int
	AttachedPeerRows             int
	SuppressedNonEstablished     int
	SuppressedUnresolvedLocal    int
	SuppressedUnresolvedNeighbor int
	SuppressedSelfActor          int
	SuppressedDuplicateLink      int
}

func RecomputeLinkStats(data *Data) {
	if data == nil {
		return
	}
	data.Stats.Recomputed.ActorsTotal = len(data.Actors)
	data.Stats.Recomputed.LinksTotal = len(data.Links)

	probable := 0
	for _, link := range data.Links {
		state := strings.ToLower(strings.TrimSpace(link.State))
		inference := strings.ToLower(LinkInferenceValue(link))
		attachment := strings.ToLower(LinkAttachmentModeValue(link))
		if state == "probable" || inference == "probable" || strings.HasPrefix(attachment, "probable_") {
			probable++
		}
	}
	data.Stats.Recomputed.LinksProbable = probable
	data.Stats.HasComputed = true
	RecomputeL3VisibleLinkStats(data)
	RecomputeOSPFVisibleLinkStats(data)
	RecomputeBGPVisibleLinkStats(data)
}

func RecomputeL3VisibleLinkStats(data *Data) {
	if data == nil || !data.Stats.HasL3 {
		return
	}
	directCount := 0
	membershipCount := 0
	for _, link := range data.Links {
		switch strings.ToLower(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol))) {
		case L3SubnetLinkType:
			directCount++
		case L3SubnetMembershipLinkType:
			membershipCount++
		}
	}
	data.Stats.Recomputed.L3SubnetVisibleLinks = directCount
	data.Stats.Recomputed.L3SubnetMembershipVisibleLinks = membershipCount
}

func RecomputeOSPFVisibleLinkStats(data *Data) {
	if data == nil || !data.Stats.HasOSPF {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), OSPFAdjacencyLinkType) {
			count++
		}
	}
	data.Stats.Recomputed.OSPFAdjacencyVisibleLinks = count
}

func RecomputeBGPVisibleLinkStats(data *Data) {
	if data == nil || !data.Stats.HasBGP {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), BGPAdjacencyLinkType) {
			count++
		}
	}
	data.Stats.Recomputed.BGPAdjacencyVisibleLinks = count
}
