// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
)

func incompleteCapabilityRoot(capability CapabilityKey) (CapabilityRootV1, bool) {
	var names []string
	switch capability {
	case SemanticCapabilityV1():
		names = []string{SemanticSectionDevice, SemanticSectionObservation, SemanticSectionProfiles, SemanticSectionEvents}
	case RefreshCapabilityV1():
		names = []string{RefreshSectionGeneration, RefreshSectionSweep}
	case GraphCapabilityV1():
		names = []string{GraphSectionDNSTrace, GraphSectionGeneration, GraphSectionOUITrace, GraphSectionQuery}
	default:
		return CapabilityRootV1{}, false
	}
	root := CapabilityRootV1{Capability: capability, State: StateIncomplete}
	for _, name := range names {
		root.Sections = append(root.Sections, SectionInventoryV1{Name: name, State: StateIncomplete})
	}
	return root, true
}

func isEmptyIncompleteShape(root CapabilityRootV1) bool {
	if root.State != StateIncomplete {
		return false
	}
	for _, section := range root.Sections {
		if section.State != StateIncomplete || section.ExpectedRecords != 0 || len(section.Members) != 0 {
			return false
		}
	}
	return true
}

func validateKnownCapabilityTerminalShape(root CapabilityRootV1) error {
	switch root.Capability {
	case SemanticCapabilityV1():
		return validateSemanticTerminalShape(root)
	case RefreshCapabilityV1():
		return validateRefreshTerminalShape(root)
	case GraphCapabilityV1():
		return validateGraphTerminalShape(root)
	default:
		return nil
	}
}

func validateSemanticTerminalShape(root CapabilityRootV1) error {
	want := []string{SemanticSectionDevice, SemanticSectionObservation, SemanticSectionProfiles, SemanticSectionEvents}
	if err := requireTerminalSections(root, want); err != nil {
		return err
	}
	if isEmptyIncompleteShape(root) {
		return nil
	}
	device, observation, profiles, events := root.Sections[0], root.Sections[1], root.Sections[2], root.Sections[3]
	if !sectionHasSingleSuccessfulMember(device) {
		return errors.New("semantic terminal shape requires one device member")
	}
	if !sectionIsCompleteOptionalSingleton(observation) {
		return errors.New("semantic terminal shape has an invalid observation section")
	}
	if !sectionHasCompleteInventory(profiles, true) || !sectionHasCompleteRecordInventory(events) {
		return errors.New("semantic terminal shape has an invalid semantic inventory")
	}
	if root.State != StateSuccess && root.State != StateEmpty && root.State != StateIncomplete {
		return fmt.Errorf("semantic terminal shape has unsupported state %q", root.State)
	}
	if root.State == StateSuccess && observation.State != StateSuccess {
		return errors.New("successful semantic terminal shape requires an observation")
	}
	if root.State == StateEmpty && observation.State != StateEmpty {
		return errors.New("empty semantic terminal shape must not contain an observation")
	}
	return nil
}

func validateRefreshTerminalShape(root CapabilityRootV1) error {
	if err := requireTerminalSections(root, []string{RefreshSectionGeneration, RefreshSectionSweep}); err != nil {
		return err
	}
	if isEmptyIncompleteShape(root) {
		return nil
	}
	generation, sweep := root.Sections[0], root.Sections[1]
	if !sectionHasSingleSuccessfulMember(sweep) {
		return errors.New("refresh terminal shape requires one sweep member")
	}
	switch root.State {
	case StateSuccess:
		if !sectionHasSingleSuccessfulMember(generation) {
			return errors.New("published refresh terminal shape requires one generation")
		}
	case StateFailed:
		if generation.State != StateNotApplicable || generation.ExpectedRecords != 0 || len(generation.Members) != 0 {
			return errors.New("unpublished refresh terminal shape must not contain a generation")
		}
	default:
		return fmt.Errorf("refresh terminal shape has unsupported state %q", root.State)
	}
	return nil
}

func validateGraphTerminalShape(root CapabilityRootV1) error {
	want := []string{GraphSectionDNSTrace, GraphSectionGeneration, GraphSectionOUITrace, GraphSectionQuery}
	if err := requireTerminalSections(root, want); err != nil {
		return err
	}
	if isEmptyIncompleteShape(root) {
		return nil
	}
	if !sectionHasSingleSuccessfulMember(root.Sections[1]) || !sectionHasSingleSuccessfulMember(root.Sections[3]) {
		return errors.New("graph terminal shape requires one generation and query member")
	}
	if !sectionIsCompleteTrace(root.Sections[0]) || !sectionIsCompleteTrace(root.Sections[2]) {
		return errors.New("graph terminal shape has an invalid trace inventory")
	}
	if root.State != StateSuccess && root.State != StateEmpty && root.State != StateIncomplete && root.State != StateFailed {
		return fmt.Errorf("graph terminal shape has unsupported state %q", root.State)
	}
	return nil
}

func requireTerminalSections(root CapabilityRootV1, want []string) error {
	if len(root.Sections) != len(want) {
		return fmt.Errorf("%s@%d terminal shape requires %d sections", root.Capability.Name, root.Capability.Revision, len(want))
	}
	for i, name := range want {
		if root.Sections[i].Name != name {
			return fmt.Errorf("%s@%d terminal section %d is %q, expected %q",
				root.Capability.Name, root.Capability.Revision, i, root.Sections[i].Name, name)
		}
	}
	return nil
}

func sectionHasSingleSuccessfulMember(section SectionInventoryV1) bool {
	return section.State == StateSuccess && section.ExpectedRecords == 1 && len(section.Members) == 1
}

func sectionIsCompleteOptionalSingleton(section SectionInventoryV1) bool {
	return sectionHasSingleSuccessfulMember(section) ||
		(section.State == StateEmpty && section.ExpectedRecords == 0 && len(section.Members) == 0)
}

func sectionHasCompleteInventory(section SectionInventoryV1, allowIncomplete bool) bool {
	if section.State == StateEmpty {
		return section.ExpectedRecords == 0 && len(section.Members) == 0
	}
	if section.State != StateSuccess && (!allowIncomplete || section.State != StateIncomplete) {
		return false
	}
	return section.ExpectedRecords == uint64(len(section.Members))
}

func sectionHasCompleteRecordInventory(section SectionInventoryV1) bool {
	if section.State == StateEmpty {
		return section.ExpectedRecords == 0 && len(section.Members) == 0
	}
	return section.State == StateSuccess || section.State == StateIncomplete
}

func sectionIsCompleteTrace(section SectionInventoryV1) bool {
	if section.State == StateEmpty {
		return section.ExpectedRecords == 0 && len(section.Members) == 0
	}
	return (section.State == StateSuccess || section.State == StateIncomplete) && len(section.Members) == 1
}
