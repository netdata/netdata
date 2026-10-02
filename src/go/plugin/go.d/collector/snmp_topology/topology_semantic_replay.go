// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

// replayTopologyAcquisitionEvidence receives evidence validated by topologydiag.
func replayTopologyAcquisitionEvidence(evidence *topologydiag.AcquisitionAttemptEvidence) (topologymodel.ObservationSnapshot, bool) {
	builder := newTopologyBuilderFromSemanticInput(
		evidence.Device,
		evidence.Target.Addresses,
		evidence.CollectedAt,
		evidence.FreshFor,
	)
	profiles := topologydiag.SemanticProfiles(evidence.CollectionContexts[0].Profiles)
	applyTopologySemanticEvent(builder, topologySemanticEvent{
		kind: topologySemanticEventSysUptime, sysUptime: evidence.SysUptimeValue,
	})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventProfileTags, profiles: profiles})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventTopologyMetrics, profiles: profiles})
	applyTopologySemanticEvent(builder, topologySemanticEvent{kind: topologySemanticEventBGPPeers, profiles: profiles})
	for _, context := range evidence.CollectionContexts {
		if context.Ordinal == 0 || context.Collection.Outcome != topologydiag.AcquisitionPhaseSuccess {
			continue
		}
		applyTopologySemanticEvent(builder, topologySemanticEvent{
			kind:     topologySemanticEventVLANContext,
			profiles: topologydiag.SemanticProfiles(context.Profiles),
			vlanID:   context.VLANID,
			vlanName: context.VLANName,
		})
	}
	snapshot, _ := freezeTopologyBuilder(builder)
	if snapshot == nil {
		return topologymodel.ObservationSnapshot{}, false
	}
	return snapshot.observation, snapshot.hasObservation
}

func replayTopologyGraph(snapshots []topologymodel.ObservationSnapshot, scope string, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	sortTopologyObservationSnapshots(snapshots)
	return buildTopologyObservationSnapshot(snapshots, scope, options, topologyGraphBuildEnvironment{})
}
