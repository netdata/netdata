// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
)

func SemanticCapabilityV1() CapabilityKey {
	return CapabilityKey{Name: CapabilitySemanticReplay, Revision: 1}
}

func SemanticClosureV1() Closure {
	capability := SemanticCapabilityV1()
	return Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}:        DecodeCapabilityRoot(capability),
			{Kind: KindSemanticDevice, Schema: SchemaV1}:        DecodeLeaf[SemanticDeviceV1](),
			{Kind: KindSemanticProfile, Schema: SchemaV1}:       DecodeLeaf[SemanticProfileV1](),
			{Kind: KindSemanticShard, Schema: SchemaV1}:         DecodeLeaf[SemanticShardV1](),
			{Kind: KindObservation, Schema: SchemaV1}:           DecodeLeaf[ObservationV1](),
			{Kind: KindObservationCheckpoint, Schema: SchemaV1}: DecodeLeaf[ObservationCheckpointV1](),
		},
		ValidateGraph: validateSemanticGraphV1,
	}
}

type semanticOwnerV1 struct {
	captureID    uint64
	registration uint64
}

type semanticShardGroupV1 struct {
	key        semanticShardGroupKeyV1
	geometries []ShardGeometryV1
	records    []SemanticRecordV1
}

type semanticShardGroupKeyV1 struct {
	phase   uint32
	context uint32
	profile uint32
}

func validateSemanticGraphV1(root CapabilityRootV1, source MemberSource, limits ReaderLimits) error {
	wantSections := []string{
		SemanticSectionDevice,
		SemanticSectionObservation,
		SemanticSectionCheckpoint,
		SemanticSectionProfiles,
		SemanticSectionEvents,
	}
	if len(root.Sections) != len(wantSections) {
		return fmt.Errorf("semantic_replay@1 requires %d sections, found %d", len(wantSections), len(root.Sections))
	}
	for i, want := range wantSections {
		if root.Sections[i].Name != want {
			return fmt.Errorf("semantic_replay@1 section %d is %q, expected %q", i, root.Sections[i].Name, want)
		}
	}

	var owner semanticOwnerV1
	deviceSection := root.Sections[0]
	if deviceSection.State != StateSuccess || deviceSection.ExpectedRecords != 1 || len(deviceSection.Members) != 1 {
		return errors.New("semantic device section must contain one successful record")
	}
	if deviceSection.Members[0].Type() != (MemberType{Kind: KindSemanticDevice, Schema: SchemaV1}) {
		return errors.New("semantic device section contains the wrong member type")
	}
	var device SemanticDeviceV1
	if err := decodeGraphMember(source, deviceSection.Members[0], limits, &device); err != nil {
		return err
	}
	if err := device.Validate(); err != nil {
		return err
	}
	owner = semanticOwnerV1{captureID: device.CaptureID, registration: device.Registration}

	observationSection := root.Sections[1]
	if observationSection.State != StateSuccess || observationSection.ExpectedRecords != 1 || len(observationSection.Members) != 1 {
		return errors.New("semantic observation section must contain one successful record")
	}
	if observationSection.Members[0].Type() != (MemberType{Kind: KindObservation, Schema: SchemaV1}) {
		return errors.New("semantic observation section contains the wrong member type")
	}
	var observation ObservationV1
	if err := decodeGraphMember(source, observationSection.Members[0], limits, &observation); err != nil {
		return err
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	if _, _, err := validateObservationLimits(observation, limits); err != nil {
		return err
	}
	if err := requireSemanticOwner(owner, observation.CaptureID, observation.Registration); err != nil {
		return fmt.Errorf("observation: %w", err)
	}
	checkpointSection := root.Sections[2]
	if checkpointSection.State != StateSuccess || checkpointSection.ExpectedRecords != 1 || len(checkpointSection.Members) != 1 {
		return errors.New("semantic observation checkpoint section must contain one successful record")
	}
	if checkpointSection.Members[0].Type() != (MemberType{Kind: KindObservationCheckpoint, Schema: SchemaV1}) {
		return errors.New("semantic observation checkpoint section contains the wrong member type")
	}
	var checkpoint ObservationCheckpointV1
	if err := decodeGraphMember(source, checkpointSection.Members[0], limits, &checkpoint); err != nil {
		return err
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	if err := requireSemanticOwner(owner, checkpoint.CaptureID, checkpoint.Registration); err != nil {
		return fmt.Errorf("observation checkpoint: %w", err)
	}
	if checkpoint.LogicalLength != observationSection.Members[0].LogicalLength ||
		checkpoint.SHA256 != observationSection.Members[0].SHA256 || checkpoint.Counts != observation.Counts {
		return errors.New("observation checkpoint does not identify the captured observation")
	}

	profileSection := root.Sections[3]
	if uint64(len(profileSection.Members)) > limits.MaxProfiles || profileSection.ExpectedRecords > limits.MaxProfiles {
		return fmt.Errorf("profile count exceeds limit %d", limits.MaxProfiles)
	}
	if uint64(len(profileSection.Members)) != profileSection.ExpectedRecords {
		return errors.New("profile inventory count does not match expected records")
	}
	profiles := make([]SemanticProfileV1, 0, len(profileSection.Members))
	var mainProfiles uint32
	for i, ref := range profileSection.Members {
		if ref.Type() != (MemberType{Kind: KindSemanticProfile, Schema: SchemaV1}) {
			return fmt.Errorf("profiles[%d] contains the wrong member type", i)
		}
		var profile SemanticProfileV1
		if err := decodeGraphMember(source, ref, limits, &profile); err != nil {
			return err
		}
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("profiles[%d]: %w", i, err)
		}
		if err := requireSemanticOwner(owner, profile.CaptureID, profile.Registration); err != nil {
			return fmt.Errorf("profiles[%d]: %w", i, err)
		}
		profiles = append(profiles, profile)
		if profile.Role == "main" {
			mainProfiles++
		}
	}

	eventSection := root.Sections[4]
	if uint64(len(eventSection.Members)) > limits.MaxMembers {
		return fmt.Errorf("semantic shard count exceeds limit %d", limits.MaxMembers)
	}
	groups := make([]semanticShardGroupV1, 0)
	seenGroups := make(map[semanticShardGroupKeyV1]struct{})
	var totalRows uint64
	var totalTags uint64
	for i, ref := range eventSection.Members {
		if ref.Type() != (MemberType{Kind: KindSemanticShard, Schema: SchemaV1}) {
			return fmt.Errorf("semantic_events[%d] contains the wrong member type", i)
		}
		var shard SemanticShardV1
		if err := decodeGraphMember(source, ref, limits, &shard); err != nil {
			return err
		}
		if err := shard.Validate(); err != nil {
			return fmt.Errorf("semantic_events[%d]: %w", i, err)
		}
		if err := requireSemanticOwner(owner, shard.Geometry.CaptureID, shard.Geometry.Registration); err != nil {
			return fmt.Errorf("semantic_events[%d]: %w", i, err)
		}
		if shard.Geometry.Section != SemanticSectionEvents {
			return fmt.Errorf("semantic_events[%d] declares section %q", i, shard.Geometry.Section)
		}
		var err error
		totalRows, err = checkedAdd(totalRows, uint64(len(shard.Records)))
		if err != nil || totalRows > limits.MaxRows {
			return fmt.Errorf("semantic row count exceeds limit %d", limits.MaxRows)
		}
		for _, record := range shard.Records {
			totalTags, err = checkedAdd(totalTags, uint64(len(record.Tags)+len(record.Metadata)))
			if err != nil || totalTags > limits.MaxTags {
				return fmt.Errorf("semantic tag count exceeds limit %d", limits.MaxTags)
			}
			if record.BGP != nil {
				totalTags, err = checkedAdd(totalTags, uint64(len(record.BGP.Tags)))
				if err != nil || totalTags > limits.MaxTags {
					return fmt.Errorf("semantic tag count exceeds limit %d", limits.MaxTags)
				}
			}
		}
		key := semanticShardGroupKeyV1{
			phase: shard.Geometry.Phase, context: shard.Geometry.Context, profile: shard.Geometry.Profile,
		}
		if len(groups) == 0 || groups[len(groups)-1].key != key {
			if _, exists := seenGroups[key]; exists {
				return errors.New("semantic shard group is split into non-contiguous ranges")
			}
			seenGroups[key] = struct{}{}
			groups = append(groups, semanticShardGroupV1{key: key})
		}
		groups[len(groups)-1].geometries = append(groups[len(groups)-1].geometries, shard.Geometry)
		groups[len(groups)-1].records = append(groups[len(groups)-1].records, shard.Records...)
	}
	if totalRows != eventSection.ExpectedRecords {
		return fmt.Errorf("semantic event inventory covers %d records, expected %d", totalRows, eventSection.ExpectedRecords)
	}
	for i, group := range groups {
		var records uint64
		for _, geometry := range group.geometries {
			var err error
			records, err = checkedAdd(records, geometry.RecordCount)
			if err != nil {
				return err
			}
		}
		if err := ValidateShardSequence(group.geometries, records); err != nil {
			return fmt.Errorf("semantic shard group %d: %w", i, err)
		}
	}
	if root.State == StateSuccess {
		if err := validateSemanticGroupOrderV1(groups, profiles, mainProfiles); err != nil {
			return err
		}
	}
	return nil
}

func validateSemanticGroupOrderV1(groups []semanticShardGroupV1, profiles []SemanticProfileV1, mainProfiles uint32) error {
	if int(mainProfiles) > len(profiles) {
		return errors.New("main profile count exceeds profile evidence")
	}
	for i, profile := range profiles[:mainProfiles] {
		if profile.Role != "main" || profile.Ordinal != uint32(i) {
			return fmt.Errorf("profile evidence %d is not the expected ordered main profile", i)
		}
	}
	for i := mainProfiles; int(i) < len(profiles); i++ {
		if profiles[i].Role != "vlan" {
			return errors.New("main profile evidence is not contiguous")
		}
	}
	profilePosition := int(mainProfiles)
	position := 0
	for profile := uint32(0); profile < mainProfiles; profile++ {
		if position >= len(groups) || !semanticGroupIs(groups[position], SemanticPhaseProfileTags, 0, profile, "profile_tags") ||
			len(groups[position].records) != 1 || groups[position].records[0].Profile != profile ||
			groups[position].records[0].VLANID != "" {
			return fmt.Errorf("main profile %d is missing its ordered profile_tags group", profile)
		}
		position++
	}
	var previousMetrics *uint32
	for position < len(groups) && groups[position].key.phase == SemanticPhaseTopologyMetrics {
		group := groups[position]
		if group.key.context != 0 || group.key.profile >= mainProfiles || len(group.records) == 0 {
			return errors.New("invalid main topology_metrics group coordinates")
		}
		if previousMetrics != nil && *previousMetrics >= group.key.profile {
			return errors.New("main topology_metrics groups are not in profile order")
		}
		for _, record := range group.records {
			if record.Kind != "topology_metric" || record.Profile != group.key.profile || record.VLANID != "" {
				return errors.New("invalid main topology_metrics record")
			}
		}
		value := group.key.profile
		previousMetrics = &value
		position++
	}
	for profile := uint32(0); profile < mainProfiles; profile++ {
		if position >= len(groups) || !semanticGroupIs(groups[position], SemanticPhaseBGPPeers, 0, profile, "bgp_outcome") {
			return fmt.Errorf("main profile %d is missing its ordered BGP group", profile)
		}
		if groups[position].records[0].Profile != profile || groups[position].records[0].VLANID != "" {
			return errors.New("ordered BGP outcome does not match its profile")
		}
		for i, record := range groups[position].records {
			if i > 0 && (record.Kind != "bgp_peer" || record.Profile != profile) {
				return errors.New("invalid ordered BGP peer record")
			}
		}
		position++
	}

	for context := uint32(0); position < len(groups); context++ {
		outcomeGroup := groups[position]
		if !semanticGroupIs(outcomeGroup, SemanticPhaseVLANOutcome, context, 0, "vlan_outcome") ||
			len(outcomeGroup.records) != 1 {
			return fmt.Errorf("VLAN context %d is missing its ordered outcome group", context)
		}
		outcome := outcomeGroup.records[0]
		position++
		if outcome.State != StateSuccess {
			continue
		}

		var vlanProfiles uint32
		for position < len(groups) && groups[position].key.phase == SemanticPhaseVLANProfileTags &&
			groups[position].key.context == context {
			if !semanticGroupIs(groups[position], SemanticPhaseVLANProfileTags, context, vlanProfiles, "profile_tags") ||
				len(groups[position].records) != 1 || groups[position].records[0].Profile != vlanProfiles ||
				groups[position].records[0].VLANID != outcome.VLANID {
				return fmt.Errorf("VLAN context %d has an invalid profile_tags group", context)
			}
			if profilePosition >= len(profiles) || profiles[profilePosition].Role != "vlan" ||
				profiles[profilePosition].VLANID != outcome.VLANID || profiles[profilePosition].Ordinal != vlanProfiles {
				return fmt.Errorf("VLAN context %d profile %d has no matching ordered evidence", context, vlanProfiles)
			}
			profilePosition++
			vlanProfiles++
			position++
		}
		var previousVLANMetrics *uint32
		for position < len(groups) && groups[position].key.phase == SemanticPhaseVLANMetrics &&
			groups[position].key.context == context {
			group := groups[position]
			if group.key.profile >= vlanProfiles || len(group.records) == 0 {
				return fmt.Errorf("VLAN context %d has invalid topology_metrics coordinates", context)
			}
			if previousVLANMetrics != nil && *previousVLANMetrics >= group.key.profile {
				return fmt.Errorf("VLAN context %d topology_metrics groups are not in profile order", context)
			}
			for _, record := range group.records {
				if record.Kind != "topology_metric" || record.Profile != group.key.profile ||
					record.VLANID != outcome.VLANID || record.VLANName != outcome.VLANName {
					return fmt.Errorf("VLAN context %d has an invalid topology_metrics record", context)
				}
			}
			value := group.key.profile
			previousVLANMetrics = &value
			position++
		}
	}
	if profilePosition != len(profiles) {
		return errors.New("profile evidence is not consumed by the ordered semantic stream")
	}
	return nil
}

func semanticGroupIs(group semanticShardGroupV1, phase, context, profile uint32, firstKind string) bool {
	return group.key == (semanticShardGroupKeyV1{phase: phase, context: context, profile: profile}) &&
		len(group.records) > 0 && group.records[0].Kind == firstKind
}

func decodeGraphMember(source MemberSource, ref ContentRef, limits ReaderLimits, dst any) error {
	data, err := readMember(source, ref, limits)
	if err != nil {
		return err
	}
	return DecodeCanonical(data, limits, dst)
}

func requireSemanticOwner(want semanticOwnerV1, captureID, registration uint64) error {
	if captureID != want.captureID || registration != want.registration {
		return fmt.Errorf("owner is capture %d registration %d, expected capture %d registration %d",
			captureID, registration, want.captureID, want.registration)
	}
	return nil
}
