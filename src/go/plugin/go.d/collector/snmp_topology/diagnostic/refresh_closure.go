// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
	"math"
	"time"
)

func RefreshCapabilityV1() CapabilityKey {
	return CapabilityKey{Name: CapabilityRefreshSweep, Revision: 1}
}

func RefreshClosureV1() Closure {
	capability := RefreshCapabilityV1()
	return Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(capability),
			{Kind: KindRefreshSweep, Schema: SchemaV1}:   DecodeRefreshSweepV1,
			{Kind: KindGeneration, Schema: SchemaV1}:     DecodeGenerationV1,
			{Kind: KindObservation, Schema: SchemaV1}:    DecodeLeaf[ObservationV1](),
		},
		ValidateGraph: validateRefreshCapabilityGraphV1,
		Assess: func(root CapabilityRootV1) (bool, bool) {
			return root.State == StateSuccess || root.State == StateFailed, false
		},
	}
}

func DecodeRefreshSweepV1(data []byte, limits ReaderLimits) ([]ContentRef, error) {
	var sweep RefreshSweepV1
	if err := DecodeCanonical(data, limits, &sweep); err != nil {
		return nil, err
	}
	if err := sweep.Validate(); err != nil {
		return nil, err
	}
	return sweep.References(), nil
}

func validateRefreshCapabilityGraphV1(root CapabilityRootV1, source MemberSource, limits ReaderLimits) error {
	wantSections := [...]string{RefreshSectionGeneration, RefreshSectionSweep}
	if len(root.Sections) != len(wantSections) {
		return fmt.Errorf("refresh_sweep@1 requires %d sections, found %d", len(wantSections), len(root.Sections))
	}
	for i, name := range wantSections {
		if root.Sections[i].Name != name {
			return fmt.Errorf("refresh_sweep@1 section %d is %q, expected %q", i, root.Sections[i].Name, name)
		}
	}
	if isEmptyIncompleteShape(root) {
		return nil
	}
	generationSection := root.Sections[0]
	sweepSection := root.Sections[1]
	if sweepSection.State != StateSuccess || sweepSection.ExpectedRecords != 1 || len(sweepSection.Members) != 1 ||
		sweepSection.Members[0].Type() != (MemberType{Kind: KindRefreshSweep, Schema: SchemaV1}) {
		return errors.New("refresh sweep section must contain one successful sweep member")
	}
	var sweep RefreshSweepV1
	if err := decodeGraphMember(source, sweepSection.Members[0], limits, &sweep); err != nil {
		return err
	}
	if err := sweep.Validate(); err != nil {
		return err
	}
	if uint64(len(sweep.Registrations)) > limits.MaxDevices {
		return fmt.Errorf("refresh registration count exceeds limit %d", limits.MaxDevices)
	}
	var previous *GenerationV1
	if sweep.PreviousGeneration.Ref != nil {
		value, err := decodeAndValidateGeneration(*sweep.PreviousGeneration.Ref, source, limits)
		if err != nil {
			return fmt.Errorf("previous generation: %w", err)
		}
		previous = &value
		publishedAt, _ := time.Parse(time.RFC3339Nano, value.PublishedAt)
		startedAt, _ := time.Parse(time.RFC3339Nano, sweep.StartedAt)
		if publishedAt.After(startedAt) {
			return errors.New("previous generation was published after the sweep started")
		}
	}

	switch sweep.Publication.State {
	case RefreshPublicationPublished:
		if root.State != StateSuccess || generationSection.State != StateSuccess ||
			generationSection.ExpectedRecords != 1 || len(generationSection.Members) != 1 {
			return errors.New("published refresh sweep requires one successful generation member")
		}
		if sweep.Publication.Generation.Ref == nil || *sweep.Publication.Generation.Ref != generationSection.Members[0] {
			return errors.New("refresh publication does not identify the inventoried generation")
		}
		generation, err := decodeGenerationSection(generationSection, source, limits)
		if err != nil {
			return err
		}
		if generation.PublishedAt != sweep.FinishedAt {
			return errors.New("resulting generation published_at does not match sweep finished_at")
		}
		if previous != nil {
			if previous.Sequence == math.MaxUint64 || generation.Sequence != previous.Sequence+1 {
				return errors.New("resulting generation sequence does not follow the previous generation")
			}
		}
		if len(generation.Devices) != len(sweep.Registrations) {
			return errors.New("generation device inventory does not cover the complete refresh plan")
		}
		for i := range generation.Devices {
			if generation.Devices[i].Registration != sweep.Registrations[i].Registration {
				return fmt.Errorf("generation device %d does not match the refresh registration order", i)
			}
			outcome := sweep.Registrations[i].Outcome
			if outcome == RefreshOutcomeCanceledInFlight || outcome == RefreshOutcomeCanceledNotStarted ||
				outcome == RefreshOutcomePanicInFlight || outcome == RefreshOutcomePanicNotStarted {
				return errors.New("published refresh sweep contains an unpublished terminal outcome")
			}
			if (outcome == RefreshOutcomeSuccess) != (generation.Devices[i].State == GenerationStateRefreshed) {
				return errors.New("refresh outcome does not match resulting generation state")
			}
		}
	case RefreshPublicationCanceled, RefreshPublicationPanic:
		if root.State != StateFailed || generationSection.State != StateNotApplicable ||
			generationSection.ExpectedRecords != 0 || len(generationSection.Members) != 0 {
			return errors.New("unpublished refresh sweep has an invalid generation section")
		}
		for _, registration := range sweep.Registrations {
			if !refreshOutcomeMatchesPublication(registration.Outcome, sweep.Publication.State) {
				return fmt.Errorf("refresh outcome %q does not match publication state %q", registration.Outcome, sweep.Publication.State)
			}
		}
	default:
		return fmt.Errorf("unsupported refresh publication state %q", sweep.Publication.State)
	}
	return nil
}

func refreshOutcomeMatchesPublication(outcome, publication string) bool {
	switch publication {
	case RefreshPublicationCanceled:
		return outcome != RefreshOutcomePanicInFlight && outcome != RefreshOutcomePanicNotStarted
	case RefreshPublicationPanic:
		return outcome != RefreshOutcomeCanceledInFlight && outcome != RefreshOutcomeCanceledNotStarted
	default:
		return true
	}
}
