// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func (b *segmentProjectionBuilder) emitLinks() {
	sort.Strings(b.segmentIDs)
	probableOnlyAnchorPortIDBySegment := b.buildProbableOnlyAnchorPortIDBySegment()
	segmentsWithAnyLinks := make(map[string]struct{})

	for _, segmentID := range b.segmentIDs {
		segment := b.segmentByID[segmentID]
		if segment == nil {
			continue
		}
		segmentEndpoint := topology.LinkEndpoint{
			Match: b.segmentMatchByID[segmentID],
			Attributes: map[string]any{
				"segment_id": segmentID,
			},
		}

		portIDs := make([]string, 0, len(segment.ports))
		for portID := range segment.ports {
			portIDs = append(portIDs, portID)
		}
		sort.Strings(portIDs)
		probableOnlyAnchorPortID := probableOnlyAnchorPortIDBySegment[segmentID]
		for _, portID := range portIDs {
			if probableOnlyAnchorPortID != "" && portID != probableOnlyAnchorPortID {
				continue
			}
			port := segment.ports[portID]
			device, ok := b.deviceByID[port.deviceID]
			if !ok {
				continue
			}
			localPort := bridgePortDisplay(port)
			if localPort == "" {
				continue
			}
			edgeKey := segmentID + keySep + portID
			if _, seen := b.deviceSegmentEdgeSeen[edgeKey]; seen {
				continue
			}
			b.deviceSegmentEdgeSeen[edgeKey] = struct{}{}

			metrics := map[string]any{
				"bridge_domain": segmentID,
			}
			if segment.portIdentityKey(port) == segment.portIdentityKey(segment.designatedPort) {
				metrics["designated"] = true
			}
			b.out.links = append(b.out.links, topology.Link{
				Layer:        b.layer,
				Protocol:     "bridge",
				LinkType:     "bridge",
				Direction:    "bidirectional",
				Src:          adjacencySideToEndpoint(device, localPort, b.ifIndexByDeviceName, b.ifaceByDeviceIndex),
				Dst:          segmentEndpoint,
				DiscoveredAt: topologyTimePtr(b.collectedAt),
				LastSeen:     topologyTimePtr(b.collectedAt),
				Metrics:      metrics,
			})
			b.out.linksFdb++
			b.out.bidirectionalCount++
			segmentsWithAnyLinks[segmentID] = struct{}{}
		}

		allowedEndpoints := b.allowedEndpointBySegment[segmentID]
		if len(allowedEndpoints) == 0 {
			continue
		}
		endpointSet := make(map[string]struct{}, len(segment.endpointIDs)+len(allowedEndpoints))
		for endpointID := range segment.endpointIDs {
			endpointSet[endpointID] = struct{}{}
		}
		for endpointID := range allowedEndpoints {
			endpointSet[endpointID] = struct{}{}
		}
		endpointIDs := sortedTopologySet(endpointSet)
		for _, endpointID := range endpointIDs {
			if _, ok := allowedEndpoints[endpointID]; !ok {
				continue
			}

			endpointMatch, ok := b.endpointMatchByID[endpointID]
			if !ok {
				endpointMatch = endpointMatchFromID(endpointID)
				if len(topologyMatchIdentityKeys(endpointMatch)) == 0 {
					continue
				}
			}
			overlappingDeviceIDs := endpointMatchOverlappingKnownDeviceIDs(endpointMatch, b.deviceIdentityByID)
			if len(overlappingDeviceIDs) > 0 {
				matchedManagedDeviceIDs := make([]string, 0, len(overlappingDeviceIDs))
				for _, overlapID := range overlappingDeviceIDs {
					if _, ok := b.deviceByID[overlapID]; ok {
						matchedManagedDeviceIDs = append(matchedManagedDeviceIDs, overlapID)
					}
				}
				if len(matchedManagedDeviceIDs) > 0 {
					if len(matchedManagedDeviceIDs) == 1 {
						matchedDeviceID := matchedManagedDeviceIDs[0]
						if segmentContainsDevice(segment, matchedDeviceID) {
							if b.out.suppressedManagedOverlapIDs == nil {
								b.out.suppressedManagedOverlapIDs = make(map[string]struct{})
							}
							b.out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
							b.out.endpointLinksSuppressed++
							continue
						}
						if matchedDevice, ok := b.deviceByID[matchedDeviceID]; ok {
							edgeKey := segmentID + "|managed-device|" + matchedDeviceID
							if _, seen := b.endpointSegmentEdgeSeen[edgeKey]; !seen {
								b.endpointSegmentEdgeSeen[edgeKey] = struct{}{}
								b.out.links = append(b.out.links, topology.Link{
									Layer:        b.layer,
									Protocol:     "fdb",
									LinkType:     "fdb",
									Direction:    "bidirectional",
									Src:          segmentEndpoint,
									Dst:          adjacencySideToEndpoint(matchedDevice, "", b.ifIndexByDeviceName, b.ifaceByDeviceIndex),
									DiscoveredAt: topologyTimePtr(b.collectedAt),
									LastSeen:     topologyTimePtr(b.collectedAt),
									Metrics: map[string]any{
										"bridge_domain":   segmentID,
										"attachment_mode": "managed_device_overlap",
									},
								})
								b.out.linksFdb++
								b.out.bidirectionalCount++
								b.out.endpointLinksEmitted++
								segmentsWithAnyLinks[segmentID] = struct{}{}
							}
							if b.out.suppressedManagedOverlapIDs == nil {
								b.out.suppressedManagedOverlapIDs = make(map[string]struct{})
							}
							b.out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
							continue
						}
					}
					if b.out.suppressedManagedOverlapIDs == nil {
						b.out.suppressedManagedOverlapIDs = make(map[string]struct{})
					}
					b.out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
					b.out.endpointLinksSuppressed++
					continue
				}
				if !b.probabilisticConnectivity {
					b.out.endpointLinksSuppressed++
					continue
				}
				b.allowEndpoint(segmentID, endpointID, true, "probable_segment")
			}

			if owner, hasOwner := b.out.endpointDirectOwners[endpointID]; hasOwner &&
				strings.EqualFold(strings.TrimSpace(owner.source), "single_port_mac") {
				device, ok := b.deviceByID[owner.port.deviceID]
				if ok {
					localPort := bridgePortDisplay(owner.port)
					if localPort != "" {
						edgeKey := "direct" + keySep + bridgePortObservationVLANKey(owner.port) + keySep + endpointID
						if _, seen := b.endpointSegmentEdgeSeen[edgeKey]; !seen {
							b.endpointSegmentEdgeSeen[edgeKey] = struct{}{}
							metrics := map[string]any{
								"attachment_mode": "direct",
							}
							linkState := ""
							if probableSet := b.probableEndpointBySegment[segmentID]; len(probableSet) > 0 {
								if _, isProbable := probableSet[endpointID]; isProbable {
									metrics["attachment_mode"] = "probable_direct"
									metrics["inference"] = "probable"
									metrics["confidence"] = "low"
									linkState = "probable"
								}
							}
							b.out.links = append(b.out.links, topology.Link{
								Layer:        b.layer,
								Protocol:     "fdb",
								LinkType:     "fdb",
								Direction:    "bidirectional",
								Src:          adjacencySideToEndpoint(device, localPort, b.ifIndexByDeviceName, b.ifaceByDeviceIndex),
								Dst:          topology.LinkEndpoint{Match: endpointMatch},
								DiscoveredAt: topologyTimePtr(b.collectedAt),
								LastSeen:     topologyTimePtr(b.collectedAt),
								State:        linkState,
								Metrics:      metrics,
							})
							b.out.linksFdb++
							b.out.bidirectionalCount++
							b.out.endpointLinksEmitted++
							continue
						}
					}
				}
			}

			edgeKey := segmentID + keySep + endpointID
			if _, seen := b.endpointSegmentEdgeSeen[edgeKey]; seen {
				continue
			}
			b.endpointSegmentEdgeSeen[edgeKey] = struct{}{}

			metrics := map[string]any{
				"bridge_domain": segmentID,
			}
			linkState := ""
			if probableSet := b.probableEndpointBySegment[segmentID]; len(probableSet) > 0 {
				if _, isProbable := probableSet[endpointID]; isProbable {
					probableMode := ""
					if modes := b.probableAttachmentModes[segmentID]; len(modes) > 0 {
						probableMode = strings.TrimSpace(modes[endpointID])
					}
					if probableMode == "" {
						probableMode = "probable_segment"
					}
					metrics["attachment_mode"] = probableMode
					metrics["inference"] = "probable"
					metrics["confidence"] = "low"
					linkState = "probable"
				}
			}

			b.out.links = append(b.out.links, topology.Link{
				Layer:        b.layer,
				Protocol:     "fdb",
				LinkType:     "fdb",
				Direction:    "bidirectional",
				Src:          segmentEndpoint,
				Dst:          topology.LinkEndpoint{Match: endpointMatch},
				DiscoveredAt: topologyTimePtr(b.collectedAt),
				LastSeen:     topologyTimePtr(b.collectedAt),
				State:        linkState,
				Metrics:      metrics,
			})
			b.out.linksFdb++
			b.out.bidirectionalCount++
			b.out.endpointLinksEmitted++
			segmentsWithAnyLinks[segmentID] = struct{}{}
		}
	}

	b.pruneSegmentsWithoutLinks(segmentsWithAnyLinks)
}

func (b *segmentProjectionBuilder) pruneSegmentsWithoutLinks(segmentsWithAnyLinks map[string]struct{}) {
	if len(segmentsWithAnyLinks) >= len(b.segmentIDs) {
		return
	}

	filteredActors := make([]topology.Actor, 0, len(b.out.actors))
	for _, actor := range b.out.actors {
		segmentID := topologyAttrString(actor.Attributes, "segment_id")
		if segmentID == "" {
			continue
		}
		if _, ok := segmentsWithAnyLinks[segmentID]; ok {
			filteredActors = append(filteredActors, actor)
		}
	}
	b.out.actors = filteredActors

	filteredLinks := make([]topology.Link, 0, len(b.out.links))
	b.out.linksFdb = 0
	b.out.bidirectionalCount = 0
	b.out.endpointLinksEmitted = 0
	for _, link := range b.out.links {
		segmentID := topologyMetricString(link.Metrics, "bridge_domain")
		if segmentID != "" {
			if _, ok := segmentsWithAnyLinks[segmentID]; !ok {
				continue
			}
		}
		filteredLinks = append(filteredLinks, link)
		b.out.linksFdb++
		if strings.EqualFold(strings.TrimSpace(link.Direction), "bidirectional") {
			b.out.bidirectionalCount++
		}
		if strings.EqualFold(strings.TrimSpace(link.Protocol), "fdb") {
			b.out.endpointLinksEmitted++
		}
	}
	b.out.links = filteredLinks
}
