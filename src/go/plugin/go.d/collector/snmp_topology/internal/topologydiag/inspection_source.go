// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func inspectDiagnosticCut(cut *SweepCut) inspectionDiagnosticCutResult {
	if cut == nil {
		return inspectionDiagnosticCutResult{}
	}
	return inspectionDiagnosticCutResult{
		captureState:  cut.CaptureState,
		captureReason: cut.CaptureReason,
		sequence:      cut.Sequence,
		startedAt:     cut.StartedAt,
		publishedAt:   cut.PublishedAt,
	}
}

func inspectLinkSourceContext(
	devices []diagnosticReplayedDevice,
	family string,
) inspectionSourceResult {
	result := inspectionSourceResult{
		contexts: make([]inspectionSourceContext, 0, len(devices)),
	}
	for i := range devices {
		device := &devices[i]
		context := inspectionSourceContext{
			registrationID:  device.registrationID,
			latestAttempt:   inspectCapture(device.latestAttempt),
			retainedSuccess: inspectCapture(device.retainedSuccess),
			sameAttempt:     device.latestAttempt != nil && device.latestAttempt == device.retainedSuccess,
		}
		if device.latestAttempt != nil {
			context.captures = append(context.captures, inspectionSourceCaptureContext{
				latestAttempt:   true,
				retainedSuccess: context.sameAttempt,
				capture:         context.latestAttempt,
				facts: inspectionCaptureFacts(
					device.registrationID,
					device.latestAttempt,
					family,
				),
			})
		}
		if device.retainedSuccess != nil && !context.sameAttempt {
			context.captures = append(context.captures, inspectionSourceCaptureContext{
				retainedSuccess: true,
				capture:         context.retainedSuccess,
				facts: inspectionCaptureFacts(
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

func inspectionCaptureFacts(
	registrationID ddsnmp.DeviceRegistrationID,
	capture *AcquisitionCapture,
	family string,
) []inspectionSourceFact {
	if capture == nil || capture.State != CaptureAvailable || capture.Evidence == nil {
		return nil
	}
	var facts []inspectionSourceFact
	for contextIndex := range capture.Evidence.CollectionContexts {
		context := &capture.Evidence.CollectionContexts[contextIndex]
		for profileIndex := range context.Profiles {
			profile := &context.Profiles[profileIndex]
			for metricIndex := range profile.Values.TopologyMetrics {
				metric := &profile.Values.TopologyMetrics[metricIndex]
				if inspectionMetricMatchesFamily(metric.Kind, family) {
					facts = append(facts, inspectionSourceFact{
						registrationID: registrationID,
						contextOrdinal: context.Ordinal,
						profileOrdinal: profile.Identity.Ordinal,
						metric:         metric,
					})
				}
			}
			if family == topologymodel.BGPAdjacencyLinkType {
				for rowIndex := range profile.Values.BGPRows {
					facts = append(facts, inspectionSourceFact{
						registrationID: registrationID,
						contextOrdinal: context.Ordinal,
						profileOrdinal: profile.Identity.Ordinal,
						bgp:            &profile.Values.BGPRows[rowIndex],
					})
				}
			}
		}
	}
	return facts
}

func inspectionMetricMatchesFamily(kind ddsnmp.TopologyKind, family string) bool {
	switch family {
	case "lldp":
		return kind == ddsnmp.KindLldpLocPort || kind == ddsnmp.KindLldpLocManAddr ||
			kind == ddsnmp.KindLldpRem || kind == ddsnmp.KindLldpRemManAddr
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
