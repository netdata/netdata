// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"cmp"
	"errors"
	"fmt"
	"time"
)

func GraphCapabilityV1() CapabilityKey {
	return CapabilityKey{Name: CapabilityGraphReplay, Revision: 1}
}

func GraphClosureV1() Closure {
	capability := GraphCapabilityV1()
	return Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			{Kind: KindGeneration, Schema: SchemaV1}:     DecodeGenerationV1,
			{Kind: KindGraphQuery, Schema: SchemaV1}:     DecodeGraphQueryV1,
			{Kind: KindDNSTrace, Schema: SchemaV1}:       DecodeLeaf[DNSTraceV1](),
			{Kind: KindOUITrace, Schema: SchemaV1}:       DecodeLeaf[OUITraceV1](),
			{Kind: KindObservation, Schema: SchemaV1}:    DecodeLeaf[ObservationV1](),
		},
		ValidateGraph: validateGraphCapabilityGraphV1,
		AssessGraph:   assessGraphCapabilityV1,
	}
}

func assessGraphCapabilityV1(root CapabilityRootV1, source MemberSource, limits ReaderLimits) (bool, bool, error) {
	complete := root.State == StateSuccess || root.State == StateEmpty
	if !complete {
		return false, false, nil
	}
	generationSection := root.Sections[1]
	if len(generationSection.Members) != 1 {
		return false, false, errors.New("generation section must contain one member")
	}
	generation, err := decodeGeneration(generationSection.Members[0], source, limits)
	if err != nil {
		return false, false, err
	}
	return true, generation.Replayable(), nil
}

func DecodeGenerationV1(data []byte, limits ReaderLimits) ([]ContentRef, error) {
	var generation GenerationV1
	if err := DecodeCanonical(data, limits, &generation); err != nil {
		return nil, err
	}
	if err := generation.Validate(); err != nil {
		return nil, err
	}
	return generation.References(), nil
}

func DecodeGraphQueryV1(data []byte, limits ReaderLimits) ([]ContentRef, error) {
	var query GraphQueryV1
	if err := DecodeCanonical(data, limits, &query); err != nil {
		return nil, err
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return query.References(), nil
}

func validateGraphCapabilityGraphV1(root CapabilityRootV1, source MemberSource, limits ReaderLimits) error {
	wantSections := []string{
		GraphSectionDNSTrace,
		GraphSectionGeneration,
		GraphSectionOUITrace,
		GraphSectionQuery,
	}
	if len(root.Sections) != len(wantSections) {
		return fmt.Errorf("graph_replay@1 requires %d sections, found %d", len(wantSections), len(root.Sections))
	}
	for i, name := range wantSections {
		if root.Sections[i].Name != name {
			return fmt.Errorf("graph_replay@1 section %d is %q, expected %q", i, root.Sections[i].Name, name)
		}
	}
	if isEmptyIncompleteShape(root) {
		return nil
	}

	dnsSection := root.Sections[0]
	generationSection := root.Sections[1]
	ouiSection := root.Sections[2]
	querySection := root.Sections[3]

	if generationSection.State != StateSuccess || generationSection.ExpectedRecords != 1 || len(generationSection.Members) != 1 {
		return errors.New("graph generation section must contain one successful record")
	}
	_, err := decodeGenerationSection(generationSection, source, limits)
	if err != nil {
		return err
	}

	if querySection.State != StateSuccess || querySection.ExpectedRecords != 1 || len(querySection.Members) != 1 ||
		querySection.Members[0].Type() != (MemberType{Kind: KindGraphQuery, Schema: SchemaV1}) {
		return errors.New("graph query section must contain one successful record")
	}
	var query GraphQueryV1
	if err := decodeGraphMember(source, querySection.Members[0], limits, &query); err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	if query.Generation != generationSection.Members[0] {
		return errors.New("graph query does not identify the inventoried generation")
	}

	_, err = validateDNSTraceSection(dnsSection, query.CaptureID, source, limits)
	if err != nil {
		return err
	}
	_, err = validateOUITraceSection(ouiSection, query.CaptureID, source, limits)
	if err != nil {
		return err
	}

	switch root.State {
	case StateSuccess, StateEmpty, StateFailed, StateIncomplete:
	default:
		return fmt.Errorf("unsupported graph capability state %q", root.State)
	}
	return nil
}

func decodeGenerationSection(section SectionInventoryV1, source MemberSource, limits ReaderLimits) (GenerationV1, error) {
	if len(section.Members) != 1 {
		return GenerationV1{}, errors.New("generation section must contain one member")
	}
	return decodeAndValidateGeneration(section.Members[0], source, limits)
}

func decodeAndValidateGeneration(ref ContentRef, source MemberSource, limits ReaderLimits) (GenerationV1, error) {
	generation, err := decodeGeneration(ref, source, limits)
	if err != nil {
		return GenerationV1{}, err
	}
	if err := validateGenerationObservations(generation, source, limits); err != nil {
		return GenerationV1{}, err
	}
	return generation, nil
}

func decodeGeneration(ref ContentRef, source MemberSource, limits ReaderLimits) (GenerationV1, error) {
	if ref.Type() != (MemberType{Kind: KindGeneration, Schema: SchemaV1}) {
		return GenerationV1{}, errors.New("generation section contains the wrong member type")
	}
	var generation GenerationV1
	if err := decodeGraphMember(source, ref, limits, &generation); err != nil {
		return GenerationV1{}, err
	}
	if err := generation.Validate(); err != nil {
		return GenerationV1{}, err
	}
	if uint64(len(generation.Devices)) > limits.MaxDevices {
		return GenerationV1{}, fmt.Errorf("generation device count exceeds limit %d", limits.MaxDevices)
	}
	return generation, nil
}

func validateGenerationObservations(generation GenerationV1, source MemberSource, limits ReaderLimits) error {
	var rows uint64
	var tags uint64
	available := make(map[string]uint64)
	for _, device := range generation.Devices {
		if device.Observation != nil {
			available[device.Observation.Key()] = device.Registration
		}
	}
	var previous *ObservationV1
	for i, ref := range generation.Observations {
		var observation ObservationV1
		if err := decodeGraphMember(source, ref, limits, &observation); err != nil {
			return fmt.Errorf("generation observation %d: %w", i, err)
		}
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("generation observation %d: %w", i, err)
		}
		registration, ok := available[ref.Key()]
		if !ok || observation.Registration != registration {
			return fmt.Errorf("generation observation %d does not match its registration-owned device row", i)
		}
		if previous != nil && compareObservationV1(*previous, observation) >= 0 {
			return errors.New("generation observations must be strictly semantic ordered")
		}
		value := observation
		previous = &value
		observationRows, observationTags, err := validateObservationLimits(observation, limits)
		if err != nil {
			return fmt.Errorf("generation observation %d: %w", i, err)
		}
		rows, err = checkedAdd(rows, observationRows)
		if err != nil || rows > limits.MaxRows {
			return fmt.Errorf("generation observation rows exceed limit %d", limits.MaxRows)
		}
		tags, err = checkedAdd(tags, observationTags)
		if err != nil || tags > limits.MaxTags {
			return fmt.Errorf("generation observation tags exceed limit %d", limits.MaxTags)
		}
	}
	return nil
}

func compareObservationV1(left, right ObservationV1) int {
	if value := cmp.Compare(left.LocalDeviceID, right.LocalDeviceID); value != 0 {
		return value
	}
	leftManagement, leftHostname := observationIdentityV1(left)
	rightManagement, rightHostname := observationIdentityV1(right)
	if value := cmp.Compare(leftManagement, rightManagement); value != 0 {
		return value
	}
	if value := cmp.Compare(leftHostname, rightHostname); value != 0 {
		return value
	}
	leftCollected, _ := time.Parse(time.RFC3339Nano, left.CollectedAt)
	rightCollected, _ := time.Parse(time.RFC3339Nano, right.CollectedAt)
	if leftCollected.Before(rightCollected) {
		return -1
	}
	if leftCollected.After(rightCollected) {
		return 1
	}
	return cmp.Compare(left.Registration, right.Registration)
}

func observationIdentityV1(observation ObservationV1) (managementIP, hostname string) {
	if len(observation.L2) == 0 {
		return "", ""
	}
	return observation.L2[0].ManagementIP, observation.L2[0].Hostname
}

func validateDNSTraceSection(section SectionInventoryV1, captureID uint64, source MemberSource, limits ReaderLimits) (uint64, error) {
	if section.State == StateEmpty {
		if section.ExpectedRecords != 0 || len(section.Members) != 0 {
			return 0, errors.New("empty DNS trace section contains records")
		}
		return 0, nil
	}
	if section.State != StateSuccess || len(section.Members) != 1 ||
		section.Members[0].Type() != (MemberType{Kind: KindDNSTrace, Schema: SchemaV1}) {
		return 0, errors.New("DNS trace section must be empty or contain one successful trace")
	}
	var trace DNSTraceV1
	if err := decodeGraphMember(source, section.Members[0], limits, &trace); err != nil {
		return 0, err
	}
	if err := trace.Validate(); err != nil {
		return 0, err
	}
	count := uint64(len(trace.Records))
	if trace.CaptureID != captureID || count != section.ExpectedRecords || count > limits.MaxDNSRecords {
		return 0, fmt.Errorf("DNS trace ownership or count exceeds limit %d", limits.MaxDNSRecords)
	}
	return count, nil
}

func validateOUITraceSection(section SectionInventoryV1, captureID uint64, source MemberSource, limits ReaderLimits) (uint64, error) {
	if section.State == StateEmpty {
		if section.ExpectedRecords != 0 || len(section.Members) != 0 {
			return 0, errors.New("empty OUI trace section contains records")
		}
		return 0, nil
	}
	if section.State != StateSuccess || len(section.Members) != 1 ||
		section.Members[0].Type() != (MemberType{Kind: KindOUITrace, Schema: SchemaV1}) {
		return 0, errors.New("OUI trace section must be empty or contain one successful trace")
	}
	var trace OUITraceV1
	if err := decodeGraphMember(source, section.Members[0], limits, &trace); err != nil {
		return 0, err
	}
	if err := trace.Validate(); err != nil {
		return 0, err
	}
	count := uint64(len(trace.Records))
	if trace.CaptureID != captureID || count != section.ExpectedRecords || count > limits.MaxOUIRecords {
		return 0, fmt.Errorf("OUI trace ownership or count exceeds limit %d", limits.MaxOUIRecords)
	}
	return count, nil
}
