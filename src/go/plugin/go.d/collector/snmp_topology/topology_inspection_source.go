// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func inspectTopologyLinkSources(
	devices []topologyDiagnosticReplayedDevice,
	subject topologyInspectionLinkSubject,
) topologyInspectionSourceResult {
	var result topologyInspectionSourceResult
	complete := true
	matchedDevices := 0
	for i := range devices {
		device := &devices[i]
		if device.observation != topologyInspectionPresent || device.snapshot == nil {
			continue
		}
		actor, ok := topologyLocalActorFromCache(
			device.snapshot.observation.LocalDeviceID,
			device.snapshot.observation.LocalDevice,
		)
		if !ok || (!topologyInspectionActorHasIdentity(actor, subject.srcIdentity) &&
			!topologyInspectionActorHasIdentity(actor, subject.dstIdentity)) {
			continue
		}
		matchedDevices++
		if !topologyInspectionCaptureCompleteForFamily(device.capture, subject.family) {
			complete = false
		}
		result.facts = append(result.facts, topologyInspectionCaptureFacts(device.registrationID, device.capture, subject.family)...)
	}
	result.membership.candidates = len(result.facts)
	if len(result.facts) > 0 {
		result.membership.state = topologyInspectionPresent
	} else if matchedDevices > 0 && complete {
		result.membership.state = topologyInspectionAbsent
	}
	return result
}

func topologyInspectionCaptureFacts(
	registrationID ddsnmp.DeviceRegistrationID,
	capture *topologyAcquisitionCapture,
	family string,
) []topologyInspectionSourceFact {
	if capture == nil || capture.state != diagnosticCaptureAvailable || capture.evidence == nil {
		return nil
	}
	var facts []topologyInspectionSourceFact
	for contextIndex := range capture.evidence.collectionContexts {
		context := &capture.evidence.collectionContexts[contextIndex]
		for profileIndex := range context.profiles {
			profile := &context.profiles[profileIndex]
			for metricIndex := range profile.values.metrics {
				metric := &profile.values.metrics[metricIndex]
				if topologyInspectionMetricMatchesFamily(metric.kind, family) {
					facts = append(facts, topologyInspectionSourceFact{
						registrationID: registrationID,
						contextOrdinal: context.ordinal,
						profileOrdinal: profile.identity.Ordinal,
						metric:         metric,
					})
				}
			}
			if family == topologymodel.BGPAdjacencyLinkType {
				for rowIndex := range profile.values.bgpRows {
					facts = append(facts, topologyInspectionSourceFact{
						registrationID: registrationID,
						contextOrdinal: context.ordinal,
						profileOrdinal: profile.identity.Ordinal,
						bgp:            &profile.values.bgpRows[rowIndex],
					})
				}
			}
		}
	}
	return facts
}

func topologyInspectionMetricMatchesFamily(kind ddsnmp.TopologyKind, family string) bool {
	switch family {
	case "lldp":
		return kind == ddsnmp.KindLldpLocPort || kind == ddsnmp.KindLldpLocManAddr ||
			kind == ddsnmp.KindLldpRem || kind == ddsnmp.KindLldpRemManAddr ||
			kind == ddsnmp.KindLldpRemManAddrCompat
	case "cdp":
		return kind == ddsnmp.KindCdpCache
	case "stp":
		return kind == ddsnmp.KindStpPort
	case "arp":
		return kind == ddsnmp.KindArpEntry || kind == ddsnmp.KindArpLegacyEntry
	case "fdb", "bridge":
		return kind == ddsnmp.KindBridgePortIfIndex || kind == ddsnmp.KindFdbEntry ||
			kind == ddsnmp.KindQbridgeFdbEntry || kind == ddsnmp.KindQbridgeVlanEntry ||
			kind == ddsnmp.KindVtpVlan
	case topologymodel.L3SubnetLinkType, topologymodel.L3SubnetMembershipLinkType:
		return kind == ddsnmp.KindIfName || kind == ddsnmp.KindIpIfIndex
	case topologymodel.OSPFAdjacencyLinkType:
		return kind == ddsnmp.KindOSPFNeighbor
	case topologymodel.BGPAdjacencyLinkType:
		return false
	default:
		return true
	}
}

func topologyInspectionCaptureCompleteForFamily(capture *topologyAcquisitionCapture, family string) bool {
	if capture == nil || capture.state != diagnosticCaptureAvailable || capture.evidence == nil {
		return false
	}
	observedRelevantRoute := false
	for _, context := range capture.evidence.collectionContexts {
		if context.collection.outcome != topologyAcquisitionPhaseSuccess {
			return false
		}
		for _, profile := range context.profiles {
			if profile.outcome != ddsnmpcollector.AcquisitionProfileOutcomeSuccess {
				return false
			}
			if family == topologymodel.BGPAdjacencyLinkType && profile.values.bgpFailed {
				return false
			}
			for _, route := range profile.routes {
				if !topologyInspectionRouteMatchesFamily(route.Kind, family) {
					continue
				}
				observedRelevantRoute = true
				switch route.Outcome {
				case ddsnmpcollector.AcquisitionRouteOutcomeMissing,
					ddsnmpcollector.AcquisitionRouteOutcomeEmpty,
					ddsnmpcollector.AcquisitionRouteOutcomeValues:
				default:
					return false
				}
			}
		}
	}
	return observedRelevantRoute
}

func topologyInspectionRouteMatchesFamily(kind ddsnmpcollector.AcquisitionRouteKind, family string) bool {
	if family == topologymodel.BGPAdjacencyLinkType {
		return kind == ddsnmpcollector.AcquisitionRouteKindBGPScalar ||
			kind == ddsnmpcollector.AcquisitionRouteKindBGPTable
	}
	return kind == ddsnmpcollector.AcquisitionRouteKindTopologyScalar ||
		kind == ddsnmpcollector.AcquisitionRouteKindTopologyTable
}
