// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"
	"time"
)

type l2BuildState struct {
	devices                      map[string]Device
	managedObservationByDeviceID map[string]bool
	interfaces                   map[string]Interface
	adjacencies                  map[string]Adjacency
	attachments                  map[string]Attachment
	enrichments                  map[string]*enrichmentAccumulator
	ifNameByDeviceIfIndex        map[string]string

	hostToID       map[string]string
	ipToID         map[string]string
	chassisToID    map[string]string
	macToID        map[string]string
	bridgeAddrToID map[string]string

	linksLLDP        int
	linksCDP         int
	linksSTP         int
	attachmentsFDB   int
	enrichmentsARPND int

	bridgeDomains map[string]struct{}
	endpointIDs   map[string]struct{}
}

func newL2BuildState(observationCount int) *l2BuildState {
	return &l2BuildState{
		devices:                      make(map[string]Device, observationCount),
		managedObservationByDeviceID: make(map[string]bool, observationCount),
		interfaces:                   make(map[string]Interface),
		adjacencies:                  make(map[string]Adjacency),
		attachments:                  make(map[string]Attachment),
		enrichments:                  make(map[string]*enrichmentAccumulator),
		ifNameByDeviceIfIndex:        make(map[string]string),
		hostToID:                     make(map[string]string, observationCount),
		ipToID:                       make(map[string]string, observationCount),
		chassisToID:                  make(map[string]string, observationCount),
		macToID:                      make(map[string]string, observationCount),
		bridgeAddrToID:               make(map[string]string, observationCount),
		bridgeDomains:                make(map[string]struct{}),
		endpointIDs:                  make(map[string]struct{}),
	}
}

func (s *l2BuildState) refreshEndpointIndex() {
	for endpointID := range s.endpointIDs {
		delete(s.endpointIDs, endpointID)
	}
	for _, attachment := range s.attachments {
		endpointID := strings.TrimSpace(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		s.endpointIDs[endpointID] = struct{}{}
	}
	for _, enrichment := range s.enrichments {
		if enrichment == nil {
			continue
		}
		endpointID := strings.TrimSpace(enrichment.EndpointID)
		if endpointID == "" {
			continue
		}
		s.endpointIDs[endpointID] = struct{}{}
	}
}

func (s *l2BuildState) markManagedDevices() {
	for id, dev := range s.devices {
		if dev.Labels == nil {
			dev.Labels = make(map[string]string)
		}
		if s.managedObservationByDeviceID[id] {
			dev.Labels["inferred"] = "false"
		} else {
			dev.Labels["inferred"] = "true"
		}
		s.devices[id] = dev
	}
}

func (s *l2BuildState) buildResult(identityAliasStats identityAliasReconcileStats, collectedAt time.Time) Result {
	if !collectedAt.IsZero() {
		collectedAt = collectedAt.UTC()
	}

	stats := newL2ResultStats()
	stats["devices_total"] = len(s.devices)
	stats["links_total"] = len(s.adjacencies)
	stats["links_lldp"] = s.linksLLDP
	stats["links_cdp"] = s.linksCDP
	stats["links_stp"] = s.linksSTP
	stats["attachments_total"] = len(s.attachments)
	stats["attachments_fdb"] = s.attachmentsFDB
	stats["enrichments_total"] = len(s.enrichments)
	stats["enrichments_arp_nd"] = s.enrichmentsARPND
	stats["bridge_domains_total"] = len(s.bridgeDomains)
	stats["endpoints_total"] = len(s.endpointIDs)
	stats["identity_alias_endpoints_mapped"] = identityAliasStats.endpointsMapped
	stats["identity_alias_endpoints_ambiguous_mac"] = identityAliasStats.endpointsAmbiguousMAC
	stats["identity_alias_ips_merged"] = identityAliasStats.ipsMerged
	stats["identity_alias_ips_conflict_skipped"] = identityAliasStats.ipsConflictSkipped

	return Result{
		CollectedAt: collectedAt,
		Devices:     sortedDevices(s.devices),
		Interfaces:  sortedInterfaces(s.interfaces),
		Adjacencies: sortedAdjacencies(s.adjacencies),
		Attachments: sortedAttachments(s.attachments),
		Enrichments: sortedEnrichments(s.enrichments),
		Stats:       stats,
	}
}
