// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
)

type replaySemanticGroup struct {
	phase   uint32
	context uint32
	profile uint32
	records []diagnostic.SemanticRecordV1
}

func replayTopologySemanticV1(
	manifest diagnostic.ManifestV1,
	source diagnostic.MemberSource,
	limits diagnostic.ReaderLimits,
) (*topologyDeviceSnapshot, error) {
	registry := diagnostic.NewRegistry()
	capability := diagnostic.SemanticCapabilityV1()
	if err := registry.Register(capability, diagnostic.SemanticClosureV1()); err != nil {
		return nil, err
	}
	report, err := registry.ValidateCapability(manifest, source, capability, limits)
	if err != nil {
		return nil, err
	}
	if !report.Replayable {
		return nil, fmt.Errorf("semantic capability is not replayable: state=%s", report.State)
	}

	rootRef, ok := semanticCapabilityRoot(manifest)
	if !ok {
		return nil, errors.New("semantic capability root is missing")
	}
	var root diagnostic.CapabilityRootV1
	if err := diagnostic.DecodeReferenced(source, rootRef, limits, &root); err != nil {
		return nil, err
	}
	deviceSection := semanticSection(root, diagnostic.SemanticSectionDevice)
	eventSection := semanticSection(root, diagnostic.SemanticSectionEvents)
	observationSection := semanticSection(root, diagnostic.SemanticSectionObservation)

	var device diagnostic.SemanticDeviceV1
	if err := diagnostic.DecodeReferenced(source, deviceSection.Members[0], limits, &device); err != nil {
		return nil, err
	}
	groups, err := decodeReplaySemanticGroups(source, eventSection.Members, limits)
	if err != nil {
		return nil, err
	}
	work := &replayWorkBudget{limit: limits.MaxReplayWork}
	mainProfiles, vlanResults, err := replaySemanticInputs(groups, work)
	if err != nil {
		return nil, err
	}

	collectedAt, err := time.Parse(time.RFC3339Nano, device.CollectedAt)
	if err != nil || collectedAt.IsZero() {
		return nil, errors.New("semantic device has invalid collected_at")
	}
	builder := newTopologyBuilder()
	builder.updateTime = collectedAt
	builder.staleAfter = time.Duration(device.FreshForNanoseconds)
	builder.agentID = device.AgentID
	builder.localDevice = diagnosticDeviceToModel(device.LocalDevice)
	for _, value := range device.TargetManagementIPs {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, err
		}
		builder.targetManagementIPs = append(builder.targetManagementIPs, addr)
	}

	applyTopologySemanticStream(builder, newTopologyMainSemanticStream(device.SysUptime, mainProfiles), nil)
	if err := work.addSort(uint64(len(builder.vlanNameByID))); err != nil {
		return nil, err
	}
	if err := verifyReplayVLANInventory(builder.vtpVLANContexts(), vlanResults); err != nil {
		return nil, err
	}
	applyTopologySemanticStream(builder, newTopologyVLANSemanticStream(vlanResults), nil)
	if err := chargeSemanticFinalizeWork(work, builder); err != nil {
		return nil, err
	}
	snapshot, _ := freezeTopologyBuilder(builder)
	if snapshot == nil {
		return nil, errors.New("semantic replay produced no snapshot")
	}
	if root.State == diagnostic.StateEmpty {
		if snapshot.hasObservation || observationSection.State != diagnostic.StateEmpty {
			return nil, errors.New("empty semantic replay unexpectedly produced an observation")
		}
		return snapshot, nil
	}
	if !snapshot.hasObservation {
		return nil, errors.New("successful semantic replay produced no observation")
	}

	actual := diagnosticObservationFromSnapshot(device.CaptureID, device.Registration, snapshot.observation)
	actualRef, _, err := diagnostic.Seal(
		diagnostic.MemberType{Kind: diagnostic.KindObservation, Schema: diagnostic.SchemaV1},
		actual,
	)
	if err != nil {
		return nil, err
	}
	wantRef := observationSection.Members[0]
	if actualRef != wantRef {
		return nil, fmt.Errorf("semantic replay observation mismatch: got %s want %s", actualRef.SHA256, wantRef.SHA256)
	}
	return snapshot, nil
}

func decodeReplaySemanticGroups(
	source diagnostic.MemberSource,
	refs []diagnostic.ContentRef,
	limits diagnostic.ReaderLimits,
) ([]replaySemanticGroup, error) {
	var groups []replaySemanticGroup
	for _, ref := range refs {
		var shard diagnostic.SemanticShardV1
		if err := diagnostic.DecodeReferenced(source, ref, limits, &shard); err != nil {
			return nil, err
		}
		if len(groups) == 0 || groups[len(groups)-1].phase != shard.Geometry.Phase ||
			groups[len(groups)-1].context != shard.Geometry.Context || groups[len(groups)-1].profile != shard.Geometry.Profile {
			groups = append(groups, replaySemanticGroup{
				phase: shard.Geometry.Phase, context: shard.Geometry.Context, profile: shard.Geometry.Profile,
			})
		}
		groups[len(groups)-1].records = append(groups[len(groups)-1].records, shard.Records...)
	}
	return groups, nil
}

func replaySemanticInputs(
	groups []replaySemanticGroup,
	work *replayWorkBudget,
) ([]*ddsnmp.ProfileMetrics, []topologyVLANContextResult, error) {
	position := 0
	var profiles []*ddsnmp.ProfileMetrics

	for position < len(groups) && groups[position].phase <= diagnostic.SemanticPhaseBGPPeers {
		group := groups[position]
		if group.context != 0 {
			return nil, nil, errors.New("main semantic group has a VLAN context")
		}
		if err := addSemanticReplayWork(work, group.records); err != nil {
			return nil, nil, err
		}
		switch group.phase {
		case diagnostic.SemanticPhaseProfileTags:
			if int(group.profile) != len(profiles) || len(group.records) != 1 || group.records[0].Kind != "profile_tags" {
				return nil, nil, errors.New("invalid main profile_tags group")
			}
			profiles = append(profiles, profileMetricsFromTags(group.records[0]))
		case diagnostic.SemanticPhaseTopologyMetrics:
			if int(group.profile) >= len(profiles) {
				return nil, nil, errors.New("topology metric group references an unknown profile")
			}
			for _, record := range group.records {
				if record.Kind != "topology_metric" || record.Profile != group.profile || record.VLANID != "" {
					return nil, nil, errors.New("invalid main topology metric record")
				}
				profiles[group.profile].TopologyMetrics = append(profiles[group.profile].TopologyMetrics, metricFromRecord(record))
			}
		case diagnostic.SemanticPhaseBGPPeers:
			if int(group.profile) >= len(profiles) || len(group.records) == 0 || group.records[0].Kind != "bgp_outcome" {
				return nil, nil, errors.New("invalid BGP semantic group")
			}
			if group.records[0].State == diagnostic.StateFailed {
				profiles[group.profile].BGPCollectError = errors.New("captured BGP collection failure")
				if len(group.records) != 1 {
					return nil, nil, errors.New("failed BGP group contains peer rows")
				}
			} else {
				for _, record := range group.records[1:] {
					if record.Kind != "bgp_peer" || record.Profile != group.profile {
						return nil, nil, errors.New("invalid BGP peer record")
					}
					profiles[group.profile].BGPRows = append(profiles[group.profile].BGPRows, bgpRowFromRecord(record))
				}
			}
		default:
			return nil, nil, fmt.Errorf("unsupported main semantic phase %d", group.phase)
		}
		position++
	}

	for profile, metrics := range profiles {
		if metrics == nil {
			return nil, nil, fmt.Errorf("main profile %d is absent", profile)
		}
	}

	var vlanResults []topologyVLANContextResult
	for position < len(groups) {
		outcomeGroup := groups[position]
		if outcomeGroup.phase != diagnostic.SemanticPhaseVLANOutcome || outcomeGroup.profile != 0 ||
			outcomeGroup.context != uint32(len(vlanResults)) || len(outcomeGroup.records) != 1 {
			return nil, nil, errors.New("invalid VLAN outcome group order")
		}
		if err := addSemanticReplayWork(work, outcomeGroup.records); err != nil {
			return nil, nil, err
		}
		outcome := outcomeGroup.records[0]
		if outcome.Kind != "vlan_outcome" {
			return nil, nil, errors.New("VLAN outcome group contains the wrong record")
		}
		result := topologyVLANContextResult{
			ordinal: outcomeGroup.context, vlanID: outcome.VLANID, vlanName: outcome.VLANName,
			reason: outcome.Reason,
		}
		switch outcome.State {
		case diagnostic.StateSuccess:
			result.state = topologyVLANContextSuccess
		case diagnostic.StateFailed:
			result.state = topologyVLANContextFailed
		case diagnostic.StateIncomplete:
			result.state = topologyVLANContextIncomplete
		default:
			return nil, nil, errors.New("unsupported VLAN outcome state")
		}
		position++

		if result.state == topologyVLANContextSuccess {
			for position < len(groups) && groups[position].phase == diagnostic.SemanticPhaseVLANProfileTags &&
				groups[position].context == result.ordinal {
				group := groups[position]
				if int(group.profile) != len(result.profiles) || len(group.records) != 1 || group.records[0].Kind != "profile_tags" {
					return nil, nil, errors.New("invalid VLAN profile_tags group")
				}
				if err := addSemanticReplayWork(work, group.records); err != nil {
					return nil, nil, err
				}
				result.profiles = append(result.profiles, profileMetricsFromTags(group.records[0]))
				position++
			}
			for position < len(groups) && groups[position].phase == diagnostic.SemanticPhaseVLANMetrics &&
				groups[position].context == result.ordinal {
				group := groups[position]
				if int(group.profile) >= len(result.profiles) {
					return nil, nil, errors.New("VLAN metric group references an unknown profile")
				}
				if err := addSemanticReplayWork(work, group.records); err != nil {
					return nil, nil, err
				}
				for _, record := range group.records {
					if record.Kind != "topology_metric" || record.Profile != group.profile || record.VLANID != result.vlanID {
						return nil, nil, errors.New("invalid VLAN topology metric record")
					}
					result.profiles[group.profile].TopologyMetrics = append(
						result.profiles[group.profile].TopologyMetrics,
						metricFromRecord(record),
					)
				}
				position++
			}
		}
		vlanResults = append(vlanResults, result)
	}
	return profiles, vlanResults, nil
}

func addSemanticReplayWork(work *replayWorkBudget, records []diagnostic.SemanticRecordV1) error {
	for _, record := range records {
		if err := work.add(1); err != nil {
			return err
		}
		if err := work.add(uint64(len(record.Tags))); err != nil {
			return err
		}
		if err := work.add(uint64(len(record.Metadata))); err != nil {
			return err
		}
		if record.BGP != nil {
			if err := work.add(uint64(len(record.BGP.Tags))); err != nil {
				return err
			}
		}
	}
	return nil
}

func chargeSemanticFinalizeWork(work *replayWorkBudget, builder *topologyBuilder) error {
	if builder == nil {
		return nil
	}
	interfaceKeys, err := worklimit.Sum(uint64(len(builder.ifNamesByIndex)), uint64(len(builder.ifStatusByIndex)))
	if err != nil {
		return err
	}
	for _, size := range []uint64{
		uint64(len(builder.localDevice.Labels)),
		uint64(len(builder.localDevice.ManagementAddresses)),
		uint64(len(builder.fdbEntries)),
		uint64(len(builder.ifStatusByIndex)),
		interfaceKeys,
		uint64(len(builder.bridgePortToIf)),
		uint64(len(builder.fdbEntries)),
		uint64(len(builder.stpPorts)),
		uint64(len(builder.arpEntries)),
		uint64(len(builder.lldpRemotes)),
		uint64(len(builder.cdpRemotes)),
		uint64(len(builder.l3InterfacesByIP)),
		uint64(len(builder.ospfNeighborsByKey)),
		uint64(len(builder.bgpPeersByKey)),
	} {
		if err := work.addSort(size); err != nil {
			return err
		}
	}
	interfaceSort, err := worklimit.SortEnvelope(uint64(len(builder.l3InterfacesByIP)))
	if err != nil {
		return err
	}
	perNeighbor, err := worklimit.Product(uint64(len(builder.ospfNeighborsByKey)), interfaceSort)
	if err != nil {
		return err
	}
	return work.add(perNeighbor)
}

func profileMetricsFromTags(record diagnostic.SemanticRecordV1) *ddsnmp.ProfileMetrics {
	metadata := make(map[string]ddsnmp.MetaTag, len(record.Metadata))
	for key, value := range record.Metadata {
		metadata[key] = ddsnmp.MetaTag{Value: value.Value, IsExactMatch: value.IsExactMatch}
	}
	return &ddsnmp.ProfileMetrics{DeviceMetadata: metadata, Tags: maps.Clone(record.Tags)}
}

func metricFromRecord(record diagnostic.SemanticRecordV1) ddsnmp.Metric {
	return ddsnmp.Metric{TopologyKind: ddprofiledefinition.TopologyKind(record.TopologyKind), Tags: maps.Clone(record.Tags)}
}

func bgpRowFromRecord(record diagnostic.SemanticRecordV1) ddsnmp.BGPRow {
	value := record.BGP
	return ddsnmp.BGPRow{
		OriginProfileID: value.OriginProfileID, Table: value.Table, RowKey: value.RowKey, StructuralID: value.StructuralID,
		Kind: ddprofiledefinition.BGPRowKind(value.Kind),
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: value.RoutingInstance, Neighbor: value.Neighbor, RemoteAS: value.RemoteAS,
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress: value.LocalAddress, LocalAS: value.LocalAS, LocalIdentifier: value.LocalIdentifier,
			PeerIdentifier: value.PeerIdentifier, PeerType: value.PeerType, BGPVersion: value.BGPVersion,
			Description: value.Description,
		},
		Admin: ddsnmp.BGPAdmin{Enabled: ddsnmp.BGPBool{Has: value.AdminEnabled.Has, Value: value.AdminEnabled.Value}},
		State: ddsnmp.BGPState{Has: value.State.Has, State: ddprofiledefinition.BGPPeerState(value.State.Value), Raw: value.State.Raw},
		Connection: ddsnmp.BGPConnection{
			EstablishedUptime:     ddsnmp.BGPInt64{Has: value.EstablishedUptime.Has, Value: value.EstablishedUptime.Value},
			LastReceivedUpdateAge: ddsnmp.BGPInt64{Has: value.LastReceivedUpdateAge.Has, Value: value.LastReceivedUpdateAge.Value},
		},
		Tags: maps.Clone(value.Tags),
	}
}

func verifyReplayVLANInventory(want []topologyVLANContext, got []topologyVLANContextResult) error {
	if len(want) != len(got) {
		return fmt.Errorf("VLAN context inventory mismatch: derived=%d captured=%d", len(want), len(got))
	}
	for i := range want {
		if want[i].vlanID != got[i].vlanID || want[i].vlanName != got[i].vlanName {
			return fmt.Errorf("VLAN context %d mismatch", i)
		}
	}
	return nil
}

type replayWorkBudget struct {
	used  uint64
	limit uint64
}

func (b *replayWorkBudget) add(value uint64) error {
	if b == nil {
		return errors.New("diagnostic replay work budget is nil")
	}
	if b.used > b.limit || value > b.limit-b.used {
		return fmt.Errorf("diagnostic replay work exceeds limit %d", b.limit)
	}
	b.used += value
	return nil
}

func (b *replayWorkBudget) addSort(items uint64) error {
	units, err := worklimit.SortEnvelope(items)
	if err != nil {
		return err
	}
	return b.add(units)
}

func semanticCapabilityRoot(manifest diagnostic.ManifestV1) (diagnostic.ContentRef, bool) {
	for _, root := range manifest.Roots {
		if root.CapabilityKey == diagnostic.SemanticCapabilityV1() {
			return root.Root, true
		}
	}
	return diagnostic.ContentRef{}, false
}

func semanticSection(root diagnostic.CapabilityRootV1, name string) diagnostic.SectionInventoryV1 {
	for _, section := range root.Sections {
		if section.Name == name {
			return section
		}
	}
	return diagnostic.SectionInventoryV1{}
}
