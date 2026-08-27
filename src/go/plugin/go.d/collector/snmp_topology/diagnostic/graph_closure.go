// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
)

func GenerationCapabilityV1() CapabilityKey {
	return CapabilityKey{Name: CapabilityGenerationSnapshot, Revision: 1}
}

func GraphCapabilityV1() CapabilityKey {
	return CapabilityKey{Name: CapabilityGraphReplay, Revision: 1}
}

func GenerationClosureV1() Closure {
	capability := GenerationCapabilityV1()
	return Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			{Kind: KindGeneration, Schema: SchemaV1}:     DecodeGenerationV1,
			{Kind: KindObservation, Schema: SchemaV1}:    DecodeLeaf[ObservationV1](),
		},
		ValidateGraph: validateGenerationCapabilityGraphV1,
	}
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
	}
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

func validateGenerationCapabilityGraphV1(root CapabilityRootV1, source MemberSource, limits ReaderLimits) error {
	if len(root.Sections) != 1 || root.Sections[0].Name != GenerationSectionGeneration {
		return errors.New("generation_snapshot@1 requires exactly one generation section")
	}
	section := root.Sections[0]
	if section.State != StateSuccess || section.ExpectedRecords != 1 || len(section.Members) != 1 {
		return errors.New("generation section must contain one successful record")
	}
	generation, err := decodeGenerationSection(section, source, limits)
	if err != nil {
		return err
	}
	return validateGenerationObservations(generation, source, limits)
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

	dnsSection := root.Sections[0]
	generationSection := root.Sections[1]
	ouiSection := root.Sections[2]
	querySection := root.Sections[3]

	if generationSection.State != StateSuccess || generationSection.ExpectedRecords != 1 || len(generationSection.Members) != 1 {
		return errors.New("graph generation section must contain one successful record")
	}
	generation, err := decodeGenerationSection(generationSection, source, limits)
	if err != nil {
		return err
	}
	if err := validateGenerationObservations(generation, source, limits); err != nil {
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
	if query.Generation != generationSection.Members[0] || query.GenerationSequence != generation.Sequence {
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
	if section.Members[0].Type() != (MemberType{Kind: KindGeneration, Schema: SchemaV1}) {
		return GenerationV1{}, errors.New("generation section contains the wrong member type")
	}
	var generation GenerationV1
	if err := decodeGraphMember(source, section.Members[0], limits, &generation); err != nil {
		return GenerationV1{}, err
	}
	if err := generation.Validate(); err != nil {
		return GenerationV1{}, err
	}
	if generation.DeviceCount > limits.MaxDevices || generation.RenderableDevices > limits.MaxDevices {
		return GenerationV1{}, fmt.Errorf("generation device count exceeds limit %d", limits.MaxDevices)
	}
	return generation, nil
}

func validateGenerationObservations(generation GenerationV1, source MemberSource, limits ReaderLimits) error {
	var rows uint64
	var tags uint64
	for i, ref := range generation.Observations {
		var observation ObservationV1
		if err := decodeGraphMember(source, ref, limits, &observation); err != nil {
			return fmt.Errorf("generation observation %d: %w", i, err)
		}
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("generation observation %d: %w", i, err)
		}
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
