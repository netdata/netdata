// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func inspectTopologyDiagnosticCut(cut *topologySweepDiagnosticCut) topologyInspectionDiagnosticCutResult {
	if cut == nil {
		return topologyInspectionDiagnosticCutResult{}
	}
	return topologyInspectionDiagnosticCutResult{
		captureState:  cut.captureState,
		captureReason: cut.captureReason,
		sequence:      cut.sequence,
		startedAt:     cut.startedAt,
		publishedAt:   cut.publishedAt,
	}
}

func inspectTopologyLinkSourceContext(
	devices []topologyDiagnosticReplayedDevice,
	family string,
) topologyInspectionSourceResult {
	result := topologyInspectionSourceResult{
		contexts: make([]topologyInspectionSourceContext, 0, len(devices)),
	}
	for i := range devices {
		device := &devices[i]
		context := topologyInspectionSourceContext{
			registrationID:  device.registrationID,
			latestAttempt:   inspectTopologyCapture(device.latestAttempt),
			retainedSuccess: inspectTopologyCapture(device.retainedSuccess),
			sameAttempt:     device.latestAttempt != nil && device.latestAttempt == device.retainedSuccess,
		}
		if device.latestAttempt != nil {
			context.captures = append(context.captures, topologyInspectionSourceCaptureContext{
				latestAttempt:   true,
				retainedSuccess: context.sameAttempt,
				capture:         context.latestAttempt,
				facts: topologyInspectionCaptureFacts(
					device.registrationID,
					device.latestAttempt,
					family,
				),
			})
		}
		if device.retainedSuccess != nil && !context.sameAttempt {
			context.captures = append(context.captures, topologyInspectionSourceCaptureContext{
				retainedSuccess: true,
				capture:         context.retainedSuccess,
				facts: topologyInspectionCaptureFacts(
					device.registrationID,
					device.retainedSuccess,
					family,
				),
			})
		}
		result.contexts = append(result.contexts, context)
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
