// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func (b *segmentProjectionBuilder) allowEndpoint(segmentID, endpointID string, probable bool, probableMode string) {
	if strings.TrimSpace(segmentID) == "" || strings.TrimSpace(endpointID) == "" {
		return
	}

	allowed := b.allowedEndpointBySegment[segmentID]
	if allowed == nil {
		allowed = make(map[string]struct{})
		b.allowedEndpointBySegment[segmentID] = allowed
	}
	allowed[endpointID] = struct{}{}
	b.assignedEndpoints[endpointID] = struct{}{}

	if !probable {
		strictSet := b.strictEndpointBySegment[segmentID]
		if strictSet == nil {
			strictSet = make(map[string]struct{})
			b.strictEndpointBySegment[segmentID] = strictSet
		}
		strictSet[endpointID] = struct{}{}
		return
	}

	probableSet := b.probableEndpointBySegment[segmentID]
	if probableSet == nil {
		probableSet = make(map[string]struct{})
		b.probableEndpointBySegment[segmentID] = probableSet
	}
	probableSet[endpointID] = struct{}{}
	if strings.TrimSpace(probableMode) == "" {
		probableMode = "probable_segment"
	}
	modes := b.probableAttachmentModes[segmentID]
	if modes == nil {
		modes = make(map[string]string)
		b.probableAttachmentModes[segmentID] = modes
	}
	modes[endpointID] = probableMode
}

func (b *segmentProjectionBuilder) initializeEndpointCandidates() []string {
	endpointIDs := collectTopologyEndpointIDs(
		b.endpointMatchByID,
		b.endpointLabelsByID,
		b.endpointSegmentCandidates,
		b.rawFDBObservations,
		b.fdbObservations,
	)
	b.baseCandidatesByEndpoint = make(map[string][]string, len(endpointIDs))
	b.probableCandidatesByEP = make(map[string][]string, len(endpointIDs))
	b.strictLinkedEndpoints = make(map[string]struct{}, len(endpointIDs))

	for _, endpointID := range endpointIDs {
		candidates := b.endpointSegmentCandidates[endpointID]
		candidateSet := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			candidateSet[candidate] = struct{}{}
		}
		sortedCandidates := sortedTopologySet(candidateSet)
		b.out.endpointLinksCandidates += len(sortedCandidates)
		b.baseCandidatesByEndpoint[endpointID] = sortedCandidates
		strictSegmentID := ""
		probableCandidates := sortedCandidates
		if len(sortedCandidates) == 1 {
			strictSegmentID = sortedCandidates[0]
		} else if owner, ok := b.fdbOwners[endpointID]; ok {
			filtered := make([]string, 0, len(sortedCandidates))
			for _, segmentID := range sortedCandidates {
				portKeys := b.segmentPortKeys[segmentID]
				if len(portKeys) == 0 {
					continue
				}
				if _, matchesOwnerPort := portKeys[owner.portVLANKey]; matchesOwnerPort {
					filtered = append(filtered, segmentID)
					continue
				}
				if _, matchesOwnerPort := portKeys[owner.portKey]; matchesOwnerPort {
					filtered = append(filtered, segmentID)
				}
			}
			if len(filtered) == 1 {
				strictSegmentID = filtered[0]
			}
			if len(filtered) > 0 {
				probableCandidates = filtered
			}
		}
		b.probableCandidatesByEP[endpointID] = probableCandidates

		if strictSegmentID != "" {
			b.allowEndpoint(strictSegmentID, endpointID, false, "")
			b.strictLinkedEndpoints[endpointID] = struct{}{}
		}
	}

	return endpointIDs
}

func (b *segmentProjectionBuilder) selectManagedProbableHint(endpointID string) probableEndpointReporterHint {
	hint := selectProbableEndpointReporterHint(
		b.endpointLabelsByID[endpointID],
		b.rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
		b.fdbOwners[endpointID],
		b.aliasOwnerIDs,
		b.managedDeviceIDs,
	)
	return ensureManagedProbableReporterHint(
		hint,
		b.endpointLabelsByID[endpointID],
		b.rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
		b.aliasOwnerIDs,
		b.managedDeviceIDs,
		b.managedDeviceIDList,
	)
}

func (b *segmentProjectionBuilder) registerProbableSegment(endpointID string, hint probableEndpointReporterHint) string {
	if strings.TrimSpace(hint.deviceID) == "" {
		return ""
	}
	segmentID, created := ensureProbablePortlessSegment(b.segmentByID, hint)
	if strings.TrimSpace(segmentID) == "" {
		return ""
	}
	if created {
		b.segmentIDs = append(b.segmentIDs, segmentID)
		b.segmentIfIndexes[segmentID] = make(map[string]struct{})
		b.segmentIfNames[segmentID] = make(map[string]struct{})
		if hint.ifIndex > 0 {
			b.segmentIfIndexes[segmentID][strconv.Itoa(hint.ifIndex)] = struct{}{}
		}
		if ifName := strings.ToLower(strings.TrimSpace(hint.ifName)); ifName != "" {
			b.segmentIfNames[segmentID][ifName] = struct{}{}
		}
		match, actor := buildBridgeSegmentActor(segmentID, b.segmentByID[segmentID], b.layer, b.source)
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) > 0 && !topologyIdentityIndexOverlaps(b.actorIndex, keys) {
			addTopologyIdentityKeys(b.actorIndex, keys)
		}
		b.out.actors = append(b.out.actors, actor)
		b.segmentMatchByID[segmentID] = match
	}
	if seg := b.segmentByID[segmentID]; seg != nil {
		seg.addEndpoint(endpointID, "probable")
	}
	return segmentID
}

func (b *segmentProjectionBuilder) ensureProbableManagedSegment(endpointID string) string {
	return b.registerProbableSegment(endpointID, b.selectManagedProbableHint(endpointID))
}

func (b *segmentProjectionBuilder) assignProbableEndpoints(endpointIDs []string) {
	if b.probabilisticConnectivity {
		for _, endpointID := range endpointIDs {
			if _, strictLinked := b.strictLinkedEndpoints[endpointID]; strictLinked {
				continue
			}

			baseCandidates := b.baseCandidatesByEndpoint[endpointID]
			probableCandidates := append([]string(nil), b.probableCandidatesByEP[endpointID]...)
			if len(probableCandidates) == 0 {
				probableCandidates = probableCandidateSegmentsFromReporterHints(
					b.endpointLabelsByID[endpointID],
					b.rawFDBObservations.byEndpoint[normalizeFDBEndpointID(endpointID)],
					b.reporterSegmentIndex,
					b.aliasOwnerIDs,
					b.managedDeviceIDs,
				)
			}

			segmentID := pickMostProbableSegment(
				probableCandidates,
				b.endpointLabelsByID[endpointID],
				b.segmentIfIndexes,
				b.segmentIfNames,
			)
			if segmentID == "" && len(probableCandidates) > 0 {
				segmentID = probableCandidates[0]
			}
			if segmentID != "" && !segmentHasManagedPort(b.segmentByID[segmentID], b.managedDeviceIDs) {
				segmentID = b.ensureProbableManagedSegment(endpointID)
			}
			if segmentID == "" {
				segmentID = b.ensureProbableManagedSegment(endpointID)
			}

			if segmentID != "" {
				probableMode := "probable_segment"
				if strings.HasPrefix(segmentID, "bridge-domain:probable:") {
					probableMode = "probable_portless"
				}
				b.allowEndpoint(segmentID, endpointID, true, probableMode)
				if len(baseCandidates) > 1 {
					b.out.endpointLinksSuppressed += len(baseCandidates) - 1
				}
				continue
			}

			if len(baseCandidates) > 1 {
				b.out.endpointsWithAmbiguousSegment++
				b.out.endpointLinksSuppressed += len(baseCandidates)
			}
		}
		return
	}

	for _, endpointID := range endpointIDs {
		if _, strictLinked := b.strictLinkedEndpoints[endpointID]; strictLinked {
			continue
		}
		baseCandidates := b.baseCandidatesByEndpoint[endpointID]
		if len(baseCandidates) > 1 {
			b.out.endpointsWithAmbiguousSegment++
			b.out.endpointLinksSuppressed += len(baseCandidates)
		}
	}
}

func (b *segmentProjectionBuilder) assignRemainingProbableEndpoints(endpointIDs []string) {
	if !b.probabilisticConnectivity || len(b.managedDeviceIDs) == 0 {
		return
	}

	for _, endpointID := range endpointIDs {
		if _, alreadyAssigned := b.assignedEndpoints[endpointID]; alreadyAssigned {
			continue
		}
		segmentID := b.ensureProbableManagedSegment(endpointID)
		if strings.TrimSpace(segmentID) == "" {
			continue
		}
		b.allowEndpoint(segmentID, endpointID, true, "probable_portless")
	}
}

func (b *segmentProjectionBuilder) buildProbableOnlyAnchorPortIDBySegment() map[string]string {
	out := make(map[string]string)
	for _, segmentID := range b.segmentIDs {
		if len(b.probableEndpointBySegment[segmentID]) == 0 {
			continue
		}
		if len(b.strictEndpointBySegment[segmentID]) > 0 {
			continue
		}
		segment := b.segmentByID[segmentID]
		if segment == nil {
			continue
		}
		if portID := pickProbableSegmentAnchorPortID(segment, b.probableEndpointBySegment[segmentID], b.fdbOwners, b.managedDeviceIDs); portID != "" {
			out[segmentID] = portID
		}
	}
	return out
}
