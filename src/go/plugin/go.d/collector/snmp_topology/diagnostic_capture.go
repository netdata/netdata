// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net/netip"
	"path/filepath"
	"reflect"
	"slices"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
)

const diagnosticSemanticShardRecords = 256

type topologyDiagnosticCapture struct {
	transaction  *diagnostic.CaptureTransaction
	registration uint64
	mainProfiles []*ddsnmp.Profile
	profiles     []diagnostic.SemanticProfileV1
	groups       []diagnosticSemanticGroup
	groupIndex   map[diagnosticSemanticGroupKey]int
	device       *diagnostic.SemanticDeviceV1
	err          error
	finished     bool
}

type diagnosticSemanticGroupKey struct {
	phase   uint32
	context uint32
	profile uint32
}

type diagnosticSemanticGroup struct {
	key     diagnosticSemanticGroupKey
	records []diagnostic.SemanticRecordV1
}

func (c *Collector) beginTopologyDiagnosticCapture(registrationID ddsnmp.DeviceRegistrationID) *topologyDiagnosticCapture {
	if c == nil || c.diagnosticRecorder == nil {
		return nil
	}
	transaction, err := c.diagnosticRecorder.Begin(diagnostic.SemanticCapabilityV1())
	if err != nil {
		return nil
	}
	return &topologyDiagnosticCapture{
		transaction: transaction, registration: uint64(registrationID),
		groupIndex: make(map[diagnosticSemanticGroupKey]int),
	}
}

func (c *topologyDiagnosticCapture) setDevice(
	builder *topologyBuilder,
	targetManagementIPs []netip.Addr,
	sysUptime int64,
	profiles []*ddsnmp.Profile,
) {
	if c == nil || builder == nil {
		return
	}
	targets := make([]string, 0, len(targetManagementIPs))
	for _, addr := range targetManagementIPs {
		targets = append(targets, addr.String())
	}
	diagnostic.SortCanonicalIPs(targets)
	c.mainProfiles = profiles
	c.device = &diagnostic.SemanticDeviceV1{
		CaptureID:           c.transaction.CaptureID(),
		Registration:        c.registration,
		CollectedAt:         canonicalDiagnosticTime(builder.updateTime),
		FreshForNanoseconds: int64(builder.staleAfter),
		AgentID:             builder.agentID,
		LocalDevice:         diagnosticDeviceFromModel(builder.localDevice),
		TargetManagementIPs: targets,
		SysUptime:           sysUptime,
	}
}

func (c *topologyDiagnosticCapture) observe(event topologySemanticEvent) {
	if c == nil {
		return
	}
	switch event.kind {
	case topologySemanticEventProfileTags:
		c.captureProfiles("main", "", diagnostic.SemanticPhaseProfileTags, 0, event.profiles, c.mainProfiles)
	case topologySemanticEventTopologyMetrics:
		c.captureTopologyMetrics(diagnostic.SemanticPhaseTopologyMetrics, 0, "", "", event.profiles)
	case topologySemanticEventBGPPeers:
		c.captureBGPPeers(event.profiles)
	case topologySemanticEventVLANContext:
		c.captureVLANContext(event.vlan)
	}
}

func (c *topologyDiagnosticCapture) captureProfiles(
	role, vlanID string,
	phase, context uint32,
	pms []*ddsnmp.ProfileMetrics,
	definitions []*ddsnmp.Profile,
) {
	bySource := make(map[string]*ddsnmp.Profile, len(definitions))
	for _, profile := range definitions {
		if profile != nil {
			bySource[profile.SourceFile] = profile
		}
	}
	for ordinal, pm := range pms {
		if pm == nil {
			c.err = errors.Join(c.err, errors.New("nil profile metrics in semantic input"))
			continue
		}
		profile, ok := bySource[pm.Source]
		if !ok || profile.Definition == nil {
			c.err = errors.Join(c.err, fmt.Errorf("effective profile evidence missing for %q", filepath.Base(pm.Source)))
		} else {
			evidence, err := diagnosticProfileEvidence(
				c.transaction.CaptureID(), c.registration, role, vlanID, uint32(ordinal), profile,
			)
			if err != nil {
				c.err = errors.Join(c.err, err)
			} else {
				c.profiles = append(c.profiles, evidence)
			}
		}
		c.appendRecord(diagnosticSemanticGroupKey{phase: phase, context: context, profile: uint32(ordinal)}, diagnostic.SemanticRecordV1{
			Kind: "profile_tags", Profile: uint32(ordinal), VLANID: vlanID,
			Metadata: diagnosticMetadata(pm.DeviceMetadata), Tags: maps.Clone(pm.Tags),
		})
	}
}

func (c *topologyDiagnosticCapture) captureTopologyMetrics(
	phase, context uint32,
	vlanID, vlanName string,
	pms []*ddsnmp.ProfileMetrics,
) {
	for profileOrdinal, pm := range pms {
		if pm == nil {
			c.err = errors.Join(c.err, errors.New("nil profile metrics in topology metric input"))
			continue
		}
		key := diagnosticSemanticGroupKey{phase: phase, context: context, profile: uint32(profileOrdinal)}
		for _, metric := range pm.TopologyMetrics {
			if vlanID != "" && !isTopologyVLANContextMetric(metric.TopologyKind) {
				continue
			}
			c.appendRecord(key, diagnostic.SemanticRecordV1{
				Kind: "topology_metric", Profile: uint32(profileOrdinal), VLANID: vlanID, VLANName: vlanName,
				TopologyKind: string(metric.TopologyKind), Tags: maps.Clone(metric.Tags),
			})
		}
	}
}

func (c *topologyDiagnosticCapture) captureBGPPeers(pms []*ddsnmp.ProfileMetrics) {
	for profileOrdinal, pm := range pms {
		if pm == nil {
			c.err = errors.Join(c.err, errors.New("nil profile metrics in BGP input"))
			continue
		}
		key := diagnosticSemanticGroupKey{phase: diagnostic.SemanticPhaseBGPPeers, profile: uint32(profileOrdinal)}
		outcome := diagnostic.SemanticRecordV1{Kind: "bgp_outcome", Profile: uint32(profileOrdinal), State: diagnostic.StateSuccess}
		if pm.BGPCollectError != nil {
			outcome.State = diagnostic.StateFailed
			outcome.Reason = "collection_failed"
		}
		c.appendRecord(key, outcome)
		if pm.BGPCollectError != nil {
			continue
		}
		for _, row := range pm.BGPRows {
			c.appendRecord(key, diagnostic.SemanticRecordV1{
				Kind: "bgp_peer", Profile: uint32(profileOrdinal), BGP: diagnosticBGPPeer(row),
			})
		}
	}
}

func (c *topologyDiagnosticCapture) captureVLANContext(result topologyVLANContextResult) {
	contextOrdinal := result.ordinal
	state := diagnostic.StateIncomplete
	switch result.state {
	case topologyVLANContextSuccess:
		state = diagnostic.StateSuccess
	case topologyVLANContextFailed:
		state = diagnostic.StateFailed
	case topologyVLANContextIncomplete:
		state = diagnostic.StateIncomplete
	}
	c.appendRecord(diagnosticSemanticGroupKey{phase: diagnostic.SemanticPhaseVLANOutcome, context: contextOrdinal}, diagnostic.SemanticRecordV1{
		Kind: "vlan_outcome", VLANID: result.vlanID, VLANName: result.vlanName, State: state, Reason: result.reason,
	})
	if result.state != topologyVLANContextSuccess {
		return
	}
	c.captureProfiles(
		"vlan", result.vlanID, diagnostic.SemanticPhaseVLANProfileTags, contextOrdinal,
		result.profiles, result.profileDefinitions,
	)
	c.captureTopologyMetrics(
		diagnostic.SemanticPhaseVLANMetrics, contextOrdinal, result.vlanID, result.vlanName, result.profiles,
	)
}

func (c *topologyDiagnosticCapture) appendRecord(key diagnosticSemanticGroupKey, record diagnostic.SemanticRecordV1) {
	index, ok := c.groupIndex[key]
	if !ok {
		index = len(c.groups)
		c.groupIndex[key] = index
		c.groups = append(c.groups, diagnosticSemanticGroup{key: key})
	}
	record.Ordinal = uint64(len(c.groups[index].records))
	c.groups[index].records = append(c.groups[index].records, record)
}

func (c *topologyDiagnosticCapture) commit(snapshot *topologyDeviceSnapshot) {
	if c == nil || c.finished {
		return
	}
	if c.device == nil || snapshot == nil || !snapshot.hasObservation {
		if err := c.transaction.Abort(errors.New("semantic capture has no observation")); err == nil {
			c.finished = true
		}
		return
	}

	c.defineSection(diagnostic.SemanticSectionDevice, diagnostic.StateSuccess, 1)
	c.addMember(diagnostic.SemanticSectionDevice, diagnostic.MemberType{Kind: diagnostic.KindSemanticDevice, Schema: diagnostic.SchemaV1}, *c.device)

	profileState := diagnostic.StateSuccess
	if len(c.profiles) == 0 {
		profileState = diagnostic.StateEmpty
	}
	if c.err != nil {
		profileState = diagnostic.StateIncomplete
	}
	c.defineSection(diagnostic.SemanticSectionProfiles, profileState, uint64(len(c.profiles)))
	for _, profile := range c.profiles {
		c.addMember(diagnostic.SemanticSectionProfiles, diagnostic.MemberType{Kind: diagnostic.KindSemanticProfile, Schema: diagnostic.SchemaV1}, profile)
	}

	var eventRecords uint64
	for _, group := range c.groups {
		eventRecords += uint64(len(group.records))
	}
	eventState := diagnostic.StateSuccess
	if eventRecords == 0 {
		eventState = diagnostic.StateEmpty
	}
	c.defineSection(diagnostic.SemanticSectionEvents, eventState, eventRecords)
	for _, group := range c.groups {
		shards := semanticGroupShards(c.transaction.CaptureID(), c.registration, group)
		for _, shard := range shards {
			c.addMember(diagnostic.SemanticSectionEvents, diagnostic.MemberType{Kind: diagnostic.KindSemanticShard, Schema: diagnostic.SchemaV1}, shard)
		}
	}

	observation := diagnosticObservationFromSnapshot(c.transaction.CaptureID(), c.registration, snapshot.observation)
	c.defineSection(diagnostic.SemanticSectionObservation, diagnostic.StateSuccess, 1)
	observationHandle := c.addMember(diagnostic.SemanticSectionObservation, diagnostic.MemberType{Kind: diagnostic.KindObservation, Schema: diagnostic.SchemaV1}, observation)
	c.defineSection(diagnostic.SemanticSectionCheckpoint, diagnostic.StateSuccess, 1)
	_, err := c.transaction.AddDerivedOwned(
		diagnostic.SemanticSectionCheckpoint,
		diagnostic.MemberType{Kind: diagnostic.KindObservationCheckpoint, Schema: diagnostic.SchemaV1},
		[]diagnostic.MemberHandle{observationHandle},
		func(refs []diagnostic.ContentRef) (any, error) {
			if len(refs) != 1 {
				return nil, errors.New("observation checkpoint requires one observation")
			}
			return diagnostic.ObservationCheckpointV1{
				CaptureID: c.transaction.CaptureID(), Registration: c.registration,
				Canonicalization: "netdata.snmp-topology-observation/v1",
				LogicalLength:    refs[0].LogicalLength, SHA256: refs[0].SHA256, Counts: observation.Counts,
			}, nil
		},
		512,
	)
	if err != nil {
		c.err = errors.Join(c.err, err)
	}

	state := diagnostic.StateSuccess
	if c.err != nil {
		state = diagnostic.StateIncomplete
		_ = c.transaction.MarkIncomplete(c.err)
	}
	if err := c.transaction.Commit(state); err != nil {
		return
	}
	c.finished = true
	snapshot.diagnosticObservation = observationHandle
}

func (c *topologyDiagnosticCapture) abort() {
	if c == nil || c.finished {
		return
	}
	if err := c.transaction.Abort(errors.New("topology refresh did not produce a semantic generation")); err == nil {
		c.finished = true
	}
}

func (c *topologyDiagnosticCapture) defineSection(name string, state diagnostic.TerminalState, records uint64) {
	if err := c.transaction.DefineSection(name, state, records); err != nil {
		c.err = errors.Join(c.err, err)
	}
}

func (c *topologyDiagnosticCapture) addMember(
	section string,
	memberType diagnostic.MemberType,
	value any,
) diagnostic.MemberHandle {
	retained := diagnosticRetainedBytes(reflect.ValueOf(value))
	handle, err := c.transaction.AddOwned(section, memberType, value, retained)
	if err != nil {
		c.err = errors.Join(c.err, err)
		return diagnostic.MemberHandle{}
	}
	return handle
}

func semanticGroupShards(captureID, registration uint64, group diagnosticSemanticGroup) []diagnostic.SemanticShardV1 {
	count := (len(group.records) + diagnosticSemanticShardRecords - 1) / diagnosticSemanticShardRecords
	shards := make([]diagnostic.SemanticShardV1, 0, count)
	for shardIndex := range count {
		first := shardIndex * diagnosticSemanticShardRecords
		last := min(first+diagnosticSemanticShardRecords, len(group.records))
		shards = append(shards, diagnostic.SemanticShardV1{
			Geometry: diagnostic.ShardGeometryV1{
				CaptureID: captureID, Registration: registration, Section: diagnostic.SemanticSectionEvents,
				Phase: group.key.phase, Context: group.key.context, Profile: group.key.profile,
				Shard: uint32(shardIndex), ShardCount: uint32(count), FirstOrdinal: uint64(first), RecordCount: uint64(last - first),
			},
			Records: slices.Clone(group.records[first:last]),
		})
	}
	return shards
}

func diagnosticProfileEvidence(
	captureID, registration uint64,
	role, vlanID string,
	ordinal uint32,
	profile *ddsnmp.Profile,
) (diagnostic.SemanticProfileV1, error) {
	definition := profile.Definition
	// This explicit projection is the profile-identity boundary. It includes
	// only inputs that can affect topology collection after profile selection.
	// Extends has already been merged; selectors and regular metrics no longer
	// participate in the executed topology projection.
	projected := diagnosticTopologyProfileEvidence{
		Metadata:            definition.Metadata,
		SysobjectIDMetadata: definition.SysobjectIDMetadata,
		Topology:            definition.Topology,
		BGP:                 definition.BGP,
		MetricTags:          definition.MetricTags,
		StaticTags:          definition.StaticTags,
	}
	encoded, err := yaml.Marshal(projected)
	if err != nil {
		return diagnostic.SemanticProfileV1{}, fmt.Errorf("encode effective profile %q: %w", filepath.Base(profile.SourceFile), err)
	}
	origins := make([]string, 0, len(projected.BGP))
	for _, row := range projected.BGP {
		origin := row.OriginProfileID
		if origin != "" {
			origin = filepath.Base(origin)
		}
		origins = append(origins, origin)
	}
	evidence := diagnostic.ProfileDefinitionEvidenceV1{
		Encoding: "netdata.ddsnmp-topology-profile-evidence-yaml/v1", EffectiveDefinition: string(encoded),
		BGPOriginProfileIDs: origins,
	}
	digest, err := diagnostic.ProfileDefinitionDigest(evidence)
	if err != nil {
		return diagnostic.SemanticProfileV1{}, err
	}
	return diagnostic.SemanticProfileV1{
		CaptureID: captureID, Registration: registration, Role: role, VLANID: vlanID, Ordinal: ordinal,
		Origin: filepath.Base(profile.SourceFile), Projection: diagnosticProfileProjection(role),
		Definition: evidence, DefinitionSHA256: digest,
	}, nil
}

// diagnosticTopologyProfileEvidence is deliberately not ProfileDefinition.
// Adding a field to the runtime profile cannot silently expand diagnostic v1.
type diagnosticTopologyProfileEvidence struct {
	Metadata            ddprofiledefinition.MetadataConfig                   `yaml:"metadata,omitempty"`
	SysobjectIDMetadata []ddprofiledefinition.SysobjectIDMetadataEntryConfig `yaml:"sysobjectid_metadata,omitempty"`
	Topology            []ddprofiledefinition.TopologyConfig                 `yaml:"topology,omitempty"`
	BGP                 []ddprofiledefinition.BGPConfig                      `yaml:"bgp,omitempty"`
	MetricTags          []ddprofiledefinition.GlobalMetricTagConfig          `yaml:"metric_tags,omitempty"`
	StaticTags          []ddprofiledefinition.StaticMetricTagConfig          `yaml:"static_tags,omitempty"`
}

func diagnosticProfileProjection(role string) string {
	if role == "vlan" {
		return "topology_vlan"
	}
	return "topology_bgp"
}

func diagnosticMetadata(values map[string]ddsnmp.MetaTag) map[string]diagnostic.SemanticMetaTagV1 {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]diagnostic.SemanticMetaTagV1, len(values))
	for key, value := range values {
		result[key] = diagnostic.SemanticMetaTagV1{Value: value.Value, IsExactMatch: value.IsExactMatch}
	}
	return result
}

func diagnosticBGPPeer(row ddsnmp.BGPRow) *diagnostic.SemanticBGPPeerV1 {
	return &diagnostic.SemanticBGPPeerV1{
		OriginProfileID: row.OriginProfileID, Table: row.Table, RowKey: row.RowKey, StructuralID: row.StructuralID,
		Kind: string(row.Kind), RoutingInstance: row.Identity.RoutingInstance, Neighbor: row.Identity.Neighbor,
		RemoteAS: row.Identity.RemoteAS, LocalAddress: row.Descriptors.LocalAddress, LocalAS: row.Descriptors.LocalAS,
		LocalIdentifier: row.Descriptors.LocalIdentifier, PeerIdentifier: row.Descriptors.PeerIdentifier,
		PeerType: row.Descriptors.PeerType, BGPVersion: row.Descriptors.BGPVersion, Description: row.Descriptors.Description,
		AdminEnabled:          diagnostic.SemanticOptionalBoolV1{Has: row.Admin.Enabled.Has, Value: row.Admin.Enabled.Value},
		State:                 diagnostic.SemanticOptionalTextV1{Has: row.State.Has, Value: string(row.State.State), Raw: row.State.Raw},
		EstablishedUptime:     diagnostic.SemanticOptionalInt64V1{Has: row.Connection.EstablishedUptime.Has, Value: row.Connection.EstablishedUptime.Value},
		LastReceivedUpdateAge: diagnostic.SemanticOptionalInt64V1{Has: row.Connection.LastReceivedUpdateAge.Has, Value: row.Connection.LastReceivedUpdateAge.Value},
		Tags:                  maps.Clone(row.Tags),
	}
}

func diagnosticRetainedBytes(value reflect.Value) uint64 {
	if !value.IsValid() {
		return 1
	}
	var total uint64
	add := func(value uint64) {
		if math.MaxUint64-total < value {
			total = math.MaxUint64
			return
		}
		total += value
	}
	var visit func(reflect.Value)
	visit = func(current reflect.Value) {
		if !current.IsValid() || total == math.MaxUint64 {
			return
		}
		add(uint64(current.Type().Size()))
		switch current.Kind() {
		case reflect.Interface, reflect.Pointer:
			if !current.IsNil() {
				visit(current.Elem())
			}
		case reflect.String:
			add(uint64(current.Len()))
		case reflect.Slice, reflect.Array:
			for i := 0; i < current.Len(); i++ {
				visit(current.Index(i))
			}
		case reflect.Map:
			iterator := current.MapRange()
			for iterator.Next() {
				// Conservatively cover bucket/overflow and pointer ownership.
				add(256)
				visit(iterator.Key())
				visit(iterator.Value())
			}
		case reflect.Struct:
			for i := 0; i < current.NumField(); i++ {
				visit(current.Field(i))
			}
		}
	}
	visit(value)
	if total == 0 {
		return 1
	}
	return total
}
