// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshClosureV1_RejectsOutcomeGenerationContradictions(t *testing.T) {
	tests := map[string]struct {
		outcome string
		state   string
	}{
		"failure reported as refreshed": {
			outcome: RefreshOutcomeCollectionFailed,
			state:   GenerationStateRefreshed,
		},
		"success reported as retained": {
			outcome: RefreshOutcomeSuccess,
			state:   GenerationStateRetained,
		},
		"skipped reported as refreshed": {
			outcome: RefreshOutcomeSkippedNotDue,
			state:   GenerationStateRefreshed,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root, source := publishedRefreshClosureFixture(t, tc.outcome, tc.state, nil,
				"2026-08-27T12:00:01Z", "2026-08-27T12:00:01Z")
			require.ErrorContains(t, validateRefreshCapabilityGraphV1(
				root, source, testReaderLimits(),
			), "refresh outcome does not match resulting generation state")
		})
	}
}

func TestRefreshClosureV1_BindsGenerationChronology(t *testing.T) {
	t.Run("result publication time", func(t *testing.T) {
		root, source := publishedRefreshClosureFixture(t, RefreshOutcomeSuccess, GenerationStateRefreshed, nil,
			"2026-08-27T12:00:01Z", "2026-08-27T12:00:02Z")
		require.ErrorContains(t, validateRefreshCapabilityGraphV1(
			root, source, testReaderLimits(),
		), "published_at does not match sweep finished_at")
	})

	t.Run("previous generation precedes sweep", func(t *testing.T) {
		previous := validTestGeneration()
		previous.PublishedAt = "2026-08-27T12:00:02Z"
		root, source := publishedRefreshClosureFixture(t, RefreshOutcomeSuccess, GenerationStateRefreshed, &previous,
			"2026-08-27T12:00:01Z", "2026-08-27T12:00:01Z")
		require.ErrorContains(t, validateRefreshCapabilityGraphV1(
			root, source, testReaderLimits(),
		), "previous generation was published after the sweep started")
	})

	t.Run("result sequence follows previous generation", func(t *testing.T) {
		previous := validTestGeneration()
		previous.Sequence = 7
		previous.PublishedAt = "2026-08-27T11:59:59Z"
		root, source := publishedRefreshClosureFixture(t, RefreshOutcomeSuccess, GenerationStateRefreshed, &previous,
			"2026-08-27T12:00:01Z", "2026-08-27T12:00:01Z")
		require.ErrorContains(t, validateRefreshCapabilityGraphV1(
			root, source, testReaderLimits(),
		), "sequence does not follow")
	})
}

func TestRefreshClosureV1_EnforcesLimitsOnPreviousGeneration(t *testing.T) {
	previous := validTestGeneration()
	previous.Devices = []GenerationDeviceV1{
		{Registration: 1, State: GenerationStateAbsent, ObservationState: ObservationStateNotApplicable},
		{Registration: 2, State: GenerationStateAbsent, ObservationState: ObservationStateNotApplicable},
	}
	root, source := publishedRefreshClosureFixture(t, RefreshOutcomeSuccess, GenerationStateRefreshed, &previous,
		"2026-08-27T12:00:01Z", "2026-08-27T12:00:01Z")
	limits := testReaderLimits()
	limits.MaxDevices = 1
	require.ErrorContains(t, validateRefreshCapabilityGraphV1(root, source, limits), "device count exceeds limit 1")
}

func TestRefreshClosureV1_BindsUnpublishedOutcomeClass(t *testing.T) {
	tests := map[string]struct {
		publication string
		outcome     string
	}{
		"canceled publication rejects panic outcome": {
			publication: RefreshPublicationCanceled,
			outcome:     RefreshOutcomePanicNotStarted,
		},
		"panic publication rejects canceled outcome": {
			publication: RefreshPublicationPanic,
			outcome:     RefreshOutcomeCanceledInFlight,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root, source := unpublishedRefreshClosureFixture(t, tc.publication, tc.outcome)
			require.ErrorContains(t, validateRefreshCapabilityGraphV1(
				root, source, testReaderLimits(),
			), "does not match publication state")
		})
	}
}

func unpublishedRefreshClosureFixture(t *testing.T, publication, outcome string) (CapabilityRootV1, MemorySource) {
	t.Helper()
	sweep := RefreshSweepV1{
		CaptureID: 1, StartedAt: "2026-08-27T12:00:00Z", FinishedAt: "2026-08-27T12:00:01Z",
		PreviousGeneration: GenerationReferenceV1{State: GenerationReferenceNone},
		Registrations: []RefreshRegistrationV1{{
			Registration: 1,
			Device:       RefreshDeviceIdentityV1{Hostname: "192.0.2.1", Port: 161, SNMPVersion: "2c"},
			Selection:    RefreshSelectionDue, TargetResolution: TargetResolutionNotAttempted, Outcome: outcome,
		}},
		Publication: RefreshPublicationV1{
			State: publication, Generation: GenerationReferenceV1{State: GenerationReferenceNone},
		},
	}
	sweepRef, sweepData, err := Seal(MemberType{Kind: KindRefreshSweep, Schema: SchemaV1}, sweep)
	require.NoError(t, err)
	return CapabilityRootV1{
		Capability: RefreshCapabilityV1(), State: StateFailed,
		Sections: []SectionInventoryV1{
			{Name: RefreshSectionGeneration, State: StateNotApplicable},
			{Name: RefreshSectionSweep, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{sweepRef}},
		},
	}, MemorySource{sweepRef.Key(): sweepData}
}

func publishedRefreshClosureFixture(
	t *testing.T,
	outcome, state string,
	previous *GenerationV1,
	resultPublishedAt, sweepFinishedAt string,
) (CapabilityRootV1, MemorySource) {
	t.Helper()
	result := validTestGeneration()
	result.PublishedAt = resultPublishedAt
	result.Devices = []GenerationDeviceV1{{
		Registration: 1, State: state, ObservationState: ObservationStateNotApplicable,
	}}
	resultRef, resultData, err := Seal(MemberType{Kind: KindGeneration, Schema: SchemaV1}, result)
	require.NoError(t, err)

	previousReference := GenerationReferenceV1{State: GenerationReferenceNone}
	source := MemorySource{resultRef.Key(): resultData}
	if previous != nil {
		previousRef, previousData, err := Seal(MemberType{Kind: KindGeneration, Schema: SchemaV1}, *previous)
		require.NoError(t, err)
		previousReference = GenerationReferenceV1{State: GenerationReferenceAvailable, Ref: &previousRef}
		source[previousRef.Key()] = previousData
	}
	selection := RefreshSelectionDue
	if outcome == RefreshOutcomeSkippedNotDue {
		selection = RefreshSelectionSkippedNotDue
	}
	targetResolution := TargetResolutionLiteral
	if selection == RefreshSelectionSkippedNotDue {
		targetResolution = TargetResolutionNotAttempted
	}
	sweep := RefreshSweepV1{
		CaptureID: 1, StartedAt: "2026-08-27T12:00:00Z", FinishedAt: sweepFinishedAt,
		PreviousGeneration: previousReference,
		Registrations: []RefreshRegistrationV1{{
			Registration: 1,
			Device:       RefreshDeviceIdentityV1{Hostname: "192.0.2.1", Port: 161, SNMPVersion: "2c"},
			Selection:    selection, TargetResolution: targetResolution, Outcome: outcome,
		}},
		Publication: RefreshPublicationV1{
			State:      RefreshPublicationPublished,
			Generation: GenerationReferenceV1{State: GenerationReferenceAvailable, Ref: &resultRef},
		},
	}
	sweepRef, sweepData, err := Seal(MemberType{Kind: KindRefreshSweep, Schema: SchemaV1}, sweep)
	require.NoError(t, err)
	source[sweepRef.Key()] = sweepData
	return CapabilityRootV1{
		Capability: RefreshCapabilityV1(), State: StateSuccess,
		Sections: []SectionInventoryV1{
			{Name: RefreshSectionGeneration, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{resultRef}},
			{Name: RefreshSectionSweep, State: StateSuccess, ExpectedRecords: 1, Members: []ContentRef{sweepRef}},
		},
	}, source
}
