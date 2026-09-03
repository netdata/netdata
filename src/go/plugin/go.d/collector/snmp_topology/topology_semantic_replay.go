// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

var errTopologySemanticBGPCollection = errors.New("captured BGP collection failure")

func replayTopologyAcquisitionEvidence(evidence *topologyAcquisitionAttemptEvidence) (*topologyDeviceSnapshot, error) {
	if err := validateTopologyAcquisitionEvidence(evidence); err != nil {
		return nil, err
	}
	builder := newTopologyBuilderFromSemanticInput(
		evidence.device,
		evidence.target.addresses,
		evidence.collectedAt,
		evidence.freshFor,
	)
	main := acquisitionContextByOrdinal(evidence, 0)
	profiles := topologySemanticProfilesFromAcquisition(main.profiles)
	applyTopologySemanticEvent(builder, topologySemanticEvent{
		kind: topologySemanticEventSysUptime, sysUptime: evidence.sysUptimeValue,
	})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: profiles})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: profiles})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: profiles})
	for _, context := range evidence.collectionContexts {
		if context.ordinal == 0 || context.collection.outcome != topologyAcquisitionPhaseSuccess {
			continue
		}
		applyTopologySemanticEvent(builder, topologySemanticEvent{
			kind:     topologySemanticEventVLANContext,
			profiles: topologySemanticProfilesFromAcquisition(context.profiles),
			vlanID:   context.vlanID,
			vlanName: context.vlanName,
		})
	}
	snapshot, _ := freezeTopologyBuilder(builder)
	return snapshot, nil
}

func validateTopologyAcquisitionEvidence(evidence *topologyAcquisitionAttemptEvidence) error {
	if evidence == nil {
		return errors.New("acquisition evidence is missing")
	}
	if evidence.collectedAt.IsZero() {
		return errors.New("acquisition collected time is missing")
	}
	if evidence.freshFor <= 0 {
		return errors.New("acquisition freshness duration must be positive")
	}
	main := acquisitionContextByOrdinal(evidence, 0)
	if main == nil || main.collection.outcome != topologyAcquisitionPhaseSuccess {
		return errors.New("successful main acquisition context is missing")
	}
	var previousContext uint32
	for i, context := range evidence.collectionContexts {
		if i > 0 && context.ordinal <= previousContext {
			return fmt.Errorf("acquisition context order: ordinal %d follows %d", context.ordinal, previousContext)
		}
		previousContext = context.ordinal
		var previousProfile uint32
		for j, profile := range context.profiles {
			if j > 0 && profile.identity.Ordinal <= previousProfile {
				return fmt.Errorf("acquisition profile order: ordinal %d follows %d", profile.identity.Ordinal, previousProfile)
			}
			previousProfile = profile.identity.Ordinal
		}
	}
	return nil
}

func acquisitionContextByOrdinal(
	evidence *topologyAcquisitionAttemptEvidence,
	ordinal uint32,
) *topologyAcquisitionContextEvidence {
	if evidence == nil {
		return nil
	}
	for i := range evidence.collectionContexts {
		if evidence.collectionContexts[i].ordinal == ordinal {
			return &evidence.collectionContexts[i]
		}
	}
	return nil
}

func topologySemanticProfilesFromAcquisition(
	profiles []topologyAcquisitionProfileEvidence,
) []*ddsnmp.ProfileMetrics {
	result := make([]*ddsnmp.ProfileMetrics, 0, len(profiles))
	for _, profile := range profiles {
		if profile.outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed ||
			profile.outcome == ddsnmpcollector.AcquisitionProfileOutcomeUnknown {
			continue
		}
		result = append(result, topologySemanticProfileFromAcquisition(profile.values))
	}
	return result
}

func topologySemanticProfileFromAcquisition(profile topologyAcquisitionProfileValues) *ddsnmp.ProfileMetrics {
	// The semantic builder only reads these immutable evidence maps; borrowing
	// them avoids rebuilding the acquisition value store during replay.
	result := &ddsnmp.ProfileMetrics{
		DeviceMetadata: profile.metadata,
		Tags:           profile.tags,
	}
	for _, metric := range profile.metrics {
		result.TopologyMetrics = append(result.TopologyMetrics, ddsnmp.Metric{
			TopologyKind: metric.kind,
			Tags:         metric.tags,
		})
	}
	for _, row := range profile.bgpRows {
		result.BGPRows = append(result.BGPRows, topologySemanticBGPRowFromAcquisition(row))
	}
	if profile.bgpFailed {
		result.BGPCollectError = errTopologySemanticBGPCollection
	}
	return result
}

func topologySemanticBGPRowFromAcquisition(row topologyAcquisitionBGPRowValue) ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		OriginProfileID: row.originProfileID,
		Table:           row.table,
		RowKey:          row.rowKey,
		StructuralID:    row.structuralID,
		Kind:            row.kind,
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: row.routingInstance,
			Neighbor:        row.neighbor,
			RemoteAS:        row.remoteAS,
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress:    row.localAddress,
			LocalAS:         row.localAS,
			LocalIdentifier: row.localIdentifier,
			PeerIdentifier:  row.peerIdentifier,
			PeerType:        row.peerType,
			BGPVersion:      row.bgpVersion,
			Description:     row.description,
		},
		Admin: ddsnmp.BGPAdmin{Enabled: ddsnmp.BGPBool{
			Has:   row.adminHas,
			Value: row.adminEnabled,
		}},
		State: ddsnmp.BGPState{
			Has:   row.stateHas,
			State: row.state,
			Raw:   row.stateRaw,
		},
		Connection: ddsnmp.BGPConnection{
			EstablishedUptime: ddsnmp.BGPInt64{
				Has:   row.establishedHas,
				Value: row.established,
			},
			LastReceivedUpdateAge: ddsnmp.BGPInt64{
				Has:   row.updateAgeHas,
				Value: row.updateAge,
			},
		},
		Tags: row.tags,
	}
}
