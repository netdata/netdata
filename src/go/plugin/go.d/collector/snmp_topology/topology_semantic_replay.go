// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

var errTopologySemanticBGPCollection = errors.New("captured BGP collection failure")

func replayTopologySemanticEvidence(evidence *topologySemanticEvidence) (*topologyDeviceSnapshot, error) {
	if err := validateTopologySemanticEvidence(evidence); err != nil {
		return nil, err
	}
	builder := newTopologyBuilderFromSemanticInput(
		evidence.device,
		evidence.targets,
		evidence.collectedAt,
		evidence.freshFor,
	)
	for _, event := range evidence.events {
		applyTopologySemanticEvent(builder, topologySemanticEventFromEvidence(event))
	}
	snapshot, _ := freezeTopologyBuilder(builder)
	return snapshot, nil
}

func validateTopologySemanticEvidence(evidence *topologySemanticEvidence) error {
	if evidence == nil {
		return errors.New("semantic evidence is missing")
	}
	if evidence.collectedAt.IsZero() {
		return errors.New("semantic collected time is missing")
	}
	if evidence.freshFor <= 0 {
		return errors.New("semantic freshness duration must be positive")
	}
	want := []topologySemanticEventKind{
		topologySemanticEventSysUptime,
		topologySemanticEventProfileTags,
		topologySemanticEventTopologyMetrics,
		topologySemanticEventBGPPeers,
	}
	if len(evidence.events) < len(want) {
		return fmt.Errorf("semantic event order: got %d events, need at least %d", len(evidence.events), len(want))
	}
	for i, kind := range want {
		if evidence.events[i].kind != kind {
			return fmt.Errorf("semantic event order: event %d is %d, expected %d", i, evidence.events[i].kind, kind)
		}
	}
	for i := len(want); i < len(evidence.events); i++ {
		if evidence.events[i].kind != topologySemanticEventVLANContext {
			return fmt.Errorf("semantic event order: event %d is not a VLAN context", i)
		}
	}
	return nil
}

func topologySemanticEventFromEvidence(event topologySemanticEventEvidence) topologySemanticEvent {
	profiles := make([]*ddsnmp.ProfileMetrics, 0, len(event.profiles))
	for _, profile := range event.profiles {
		profiles = append(profiles, topologySemanticProfileFromEvidence(profile))
	}
	return topologySemanticEvent{
		kind:      event.kind,
		sysUptime: event.sysUptime,
		profiles:  profiles,
		vlanID:    event.vlanID,
		vlanName:  event.vlanName,
	}
}

func topologySemanticProfileFromEvidence(profile topologySemanticProfileEvidence) *ddsnmp.ProfileMetrics {
	result := &ddsnmp.ProfileMetrics{
		DeviceMetadata: maps.Clone(profile.metadata),
		Tags:           maps.Clone(profile.tags),
	}
	for _, metric := range profile.metrics {
		result.TopologyMetrics = append(result.TopologyMetrics, ddsnmp.Metric{
			TopologyKind: metric.kind,
			Tags:         maps.Clone(metric.tags),
		})
	}
	for _, row := range profile.bgpRows {
		result.BGPRows = append(result.BGPRows, topologySemanticBGPRowFromEvidence(row))
	}
	if profile.bgpFailed {
		result.BGPCollectError = errTopologySemanticBGPCollection
	}
	return result
}

func topologySemanticBGPRowFromEvidence(row topologySemanticBGPRowEvidence) ddsnmp.BGPRow {
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
		Tags: maps.Clone(row.tags),
	}
}
