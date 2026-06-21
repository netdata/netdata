// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

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

	stats := ResultStats{
		DevicesTotal:                       len(s.devices),
		LinksTotal:                         len(s.adjacencies),
		LinksLLDP:                          s.linksLLDP,
		LinksCDP:                           s.linksCDP,
		LinksSTP:                           s.linksSTP,
		AttachmentsTotal:                   len(s.attachments),
		AttachmentsFDB:                     s.attachmentsFDB,
		EnrichmentsTotal:                   len(s.enrichments),
		EnrichmentsARPND:                   s.enrichmentsARPND,
		BridgeDomainsTotal:                 len(s.bridgeDomains),
		EndpointsTotal:                     len(s.endpointIDs),
		IdentityAliasEndpointsMapped:       identityAliasStats.endpointsMapped,
		IdentityAliasEndpointsAmbiguousMAC: identityAliasStats.endpointsAmbiguousMAC,
		IdentityAliasIPsMerged:             identityAliasStats.ipsMerged,
		IdentityAliasIPsConflictSkipped:    identityAliasStats.ipsConflictSkipped,
	}

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
