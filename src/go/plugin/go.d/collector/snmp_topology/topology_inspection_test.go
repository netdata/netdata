// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestInspectTopologyDeviceSeparatesLatestAttemptFromRetainedSuccess(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)

	row := &diagnostics.topology.devices[0]
	retained := row.acquisition
	row.latestAttempt = &topologyAcquisitionCapture{
		attemptID: topologyAcquisitionAttemptID{registrationID: 1, ordinal: 2},
		state:     diagnosticCaptureUnavailable,
		reason:    diagnosticCaptureReasonProjectionError,
	}
	aborted := &topologyAbortedSweepDiagnostic{sequence: 2, phase: topologyDiagnosticSweepPhaseDeviceRefresh}
	diagnostics.lastAborted = aborted

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), report.lifecycle.sequence)
	require.Equal(t, topologyScenarioCollectedAt, report.lifecycle.capturedAt)
	require.Equal(t, diagnostics.topology.sequence, report.sweep.sequence)
	require.Equal(t, diagnostics.topology.startedAt, report.sweep.startedAt)
	require.Equal(t, diagnostics.topology.publishedAt, report.sweep.publishedAt)
	require.Same(t, aborted, report.lastAborted)
	require.Equal(t, topologyInspectionPresent, report.lifecycle.membership.state)
	require.Equal(t, topologyInspectionPresent, report.sweep.membership.state)
	require.Equal(t, topologyInspectionPresent, report.latestAttempt.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.latestAttempt.evidence.state)
	require.Equal(t, topologyInspectionPresent, report.retainedSuccess.membership.state)
	require.Equal(t, topologyInspectionPresent, report.retainedSuccess.evidence.state)
	require.False(t, report.sameAttempt)
	require.Same(t, retained, report.retainedSuccess.capture)
	require.Equal(t, topologyInspectionPresent, report.observation.state)
	require.Equal(t, topologyInspectionPresent, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionPresent, report.typedIdentity.state)
	require.Equal(t, 1, report.typedIdentity.candidates)
}

func TestInspectTopologyDeviceKeepsIncompleteCutsUndetermined(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	diagnostics.lifecycle = topologyJobLifecycleDiagnosticCut{
		state:  diagnosticCaptureUnavailable,
		reason: diagnosticCaptureReasonProjectionError,
	}

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionUndetermined, report.lifecycle.membership.state)
	require.Equal(t, diagnosticCaptureUnavailable, report.lifecycle.captureState)
	require.Equal(t, diagnosticCaptureReasonProjectionError, report.lifecycle.captureReason)
	require.Equal(t, topologyInspectionPresent, report.sweep.membership.state)

	diagnostics.topology.captureState = diagnosticCaptureUnavailable
	report, err = inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionUndetermined, report.sweep.membership.state)
	require.Equal(t, diagnosticCaptureUnavailable, report.sweep.captureState)
	require.Equal(t, topologyInspectionUndetermined, report.latestAttempt.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.retainedSuccess.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.observation.state)
	require.Equal(t, topologyInspectionUndetermined, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.typedIdentity.state)
}

func TestInspectTopologyDevicePreservesLimitedLifecycleCutIdentity(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	store.RegisterJob("job-a", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	store.RegisterJob("job-b", ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.20"})
	registrationID := store.LifecycleCut().Entries[0].RegistrationID
	coll.diagnosticGlobalLimits = topologyAcquisitionLimits{maxRecords: 2, maxLogicalBytes: 1 << 20}
	diagnostics := coll.acquireTopologyDiagnostics()
	require.Equal(t, diagnosticCaptureLimitExceeded, diagnostics.lifecycle.state)
	require.NotZero(t, diagnostics.lifecycle.cut.Sequence)
	require.False(t, diagnostics.lifecycle.cut.CapturedAt.IsZero())

	report, err := inspectTopologyDevice(diagnostics, newLLDPDirectScenario().opts, registrationID)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionUndetermined, report.lifecycle.membership.state)
	require.Equal(t, diagnosticCaptureLimitExceeded, report.lifecycle.captureState)
	require.Equal(t, diagnosticCaptureReasonGlobalRecordLimit, report.lifecycle.captureReason)
	require.Equal(t, diagnostics.lifecycle.cut.Sequence, report.lifecycle.sequence)
	require.Equal(t, diagnostics.lifecycle.cut.CapturedAt, report.lifecycle.capturedAt)
}

func TestInspectTopologyDeviceDoesNotFlattenWholeGraphReplayFailure(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)
	diagnostics.topology.devices[1].acquisition = nil

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, report.observation.state)
	require.Equal(t, topologyInspectionUndetermined, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.typedIdentity.state)
}

func TestInspectTopologyDeviceReportsExactMissingRegistration(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)
	diagnostics.topology.removed = append(diagnostics.topology.removed, topologyRemovedDeviceDiagnostic{registrationID: 99})

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 99)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionAbsent, report.lifecycle.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.sweep.membership.state)
	require.Equal(t, topologyInspectionPresent, report.removed.membership.state)
	require.Equal(t, ddsnmp.DeviceRegistrationID(99), report.removed.device.registrationID)
	require.Equal(t, topologyInspectionAbsent, report.latestAttempt.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.retainedSuccess.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.observation.state)
	require.Equal(t, topologyInspectionUndetermined, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.typedIdentity.state)
}

func TestInspectTopologyDeviceKeepsPreRegistrationSeparateFromSweep(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2, 99)

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 99)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, report.lifecycle.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.sweep.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.observation.state)
	require.Equal(t, topologyInspectionUndetermined, report.graphIdentity.membership.state)
}

func TestInspectTopologyDeviceReportsIdentityRepresentationAfterExpiry(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)
	diagnostics.topology.devices[0].renderable = false
	diagnostics.topology.devices[0].expired = true

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.True(t, report.sweep.device.expired)
	require.Equal(t, topologyInspectionPresent, report.observation.state)
	require.Equal(t, topologyInspectionPresent, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionPresent, report.typedIdentity.state)
}

func TestInspectTopologyDeviceUsesOneCollapsedIdentityRepresentation(t *testing.T) {
	scenario := newTopologyScenario("inspection-collapsed-identity")
	left := scenario.Switch("switch-left", "192.0.2.50", "02:00:00:00:50:01")
	right := scenario.Switch("switch-right", "192.0.2.50", "02:00:00:00:50:02")
	scenario.LLDP(left.Port("left-right", 1), right.Port("right-left", 1))
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)

	leftReport, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	rightReport, err := inspectTopologyDevice(diagnostics, scenario.opts, 2)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, leftReport.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionPresent, rightReport.graphIdentity.membership.state)
	require.Equal(t, leftReport.graphIdentity.index, rightReport.graphIdentity.index)
}

func TestInspectTopologyDeviceUsesLocalDeviceIDFallbackRepresentation(t *testing.T) {
	scenario := newTopologyScenario("inspection-local-device-id-fallback")
	scenario.Switch("switch-fallback", "192.0.2.55", "02:00:00:00:55:01")
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1)

	capture := diagnostics.topology.devices[0].acquisition
	require.NotNil(t, capture)
	require.NotNil(t, capture.evidence)
	capture.evidence.device = topologySemanticDeviceInput{}
	for contextIndex := range capture.evidence.collectionContexts {
		context := &capture.evidence.collectionContexts[contextIndex]
		for profileIndex := range context.profiles {
			profile := &context.profiles[profileIndex]
			profile.values = topologyAcquisitionProfileValues{}
		}
	}

	report, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, report.observation.state)
	require.Equal(t, topologyInspectionPresent, report.graphIdentity.membership.state)
	require.Equal(t, topologyInspectionPresent, report.typedIdentity.state)
	require.Equal(t, "local-device", report.graphIdentity.actors[0].ActorID)
}

func TestInspectTopologyActorIdentityRequiresOneMatch(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		{ActorID: "actor-a", Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
		{ActorID: "actor-b", Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
	}}

	got := inspectTopologyActorIdentity(data, "ip:192.0.2.1")
	require.Equal(t, topologyInspectionUndetermined, got.membership.state)
	require.Equal(t, 2, got.membership.candidates)
	require.Len(t, got.actors, 2)
}

func TestInspectTopologyLinkUsesCandidateSubjectAndSingleRenderedRow(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)

	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.Equal(t, topologyInspectionPresent, replay.graph.state)
	require.NotEmpty(t, replay.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
	require.True(t, ok)

	report, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Len(t, report.source.contexts, len(diagnostics.topology.devices))
	require.NotZero(t, topologyInspectionSourceFactCount(report.source))
	require.Equal(t, topologyInspectionPresent, report.graphLink.membership.state)
	require.Equal(t, 1, report.graphLink.membership.candidates)
	require.Equal(t, topologyInspectionPresent, report.typedLink.state)
	require.Equal(t, 1, report.typedLink.candidates)
	require.Equal(t, report.graphLink.index, report.typedLink.row)
	sourceContext := report.source

	reversed := subject
	reversed.srcIdentity, reversed.dstIdentity = reversed.dstIdentity, reversed.srcIdentity
	report, err = inspectTopologyLink(diagnostics, scenario.opts, reversed)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, report.graphLink.membership.state)
	require.Equal(t, topologyInspectionPresent, report.typedLink.state)
	require.Equal(t, 1, report.typedLink.candidates)

	subject.dstIdentity = "ip:192.0.2.254"
	report, err = inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, sourceContext, report.source)
	require.Equal(t, topologyInspectionAbsent, report.graphLink.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.typedLink.state)
	require.Zero(t, report.typedLink.candidates)
}

func TestInspectTopologyLinkAmbiguousIdentityIsUndetermined(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
			{Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
			{Match: topologymodel.Match{IPAddresses: []string{"192.0.2.2"}}},
		},
	}
	allocator := graph.NewActorHandleAllocator()
	for i := range data.Actors {
		data.Actors[i].ActorHandle = allocator.Next()
	}
	data.Links = []topologymodel.Link{
		{
			Protocol:       "lldp",
			Direction:      "bidirectional",
			SrcActorHandle: data.Actors[0].ActorHandle,
			DstActorHandle: data.Actors[2].ActorHandle,
		},
		{
			Protocol:       "lldp",
			Direction:      "bidirectional",
			SrcActorHandle: data.Actors[1].ActorHandle,
			DstActorHandle: data.Actors[2].ActorHandle,
		},
	}
	subject := topologyInspectionLinkSubject{
		srcIdentity: "ip:192.0.2.1",
		dstIdentity: "ip:192.0.2.2",
		family:      "lldp",
		protocol:    "lldp",
		direction:   "bidirectional",
	}

	got := inspectTopologyGraphLink(data, subject)
	require.Equal(t, topologyInspectionUndetermined, got.membership.state)
	require.Equal(t, 2, got.srcActors.membership.candidates)
	require.Equal(t, 2, got.membership.candidates)
	require.Equal(t, data.Links, got.links)
	require.Equal(t, -1, got.index)
}

func TestInspectTopologyGraphLinkPreservesOrderedEndpointRoles(t *testing.T) {
	tests := map[string]struct {
		scenario func() *topologyScenario
		family   string
	}{
		"stp": {
			scenario: newSTPInferredScenario,
			family:   "stp",
		},
		"l3 subnet membership": {
			scenario: newMixedL2L3ControlScenario,
			family:   topologymodel.L3SubnetMembershipLinkType,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scenario := tc.scenario()
			_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
			replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
			require.Equal(t, topologyInspectionPresent, replay.graph.state)

			var subject topologyInspectionLinkSubject
			for i, link := range replay.data.Links {
				if topologyInspectionLinkFamily(link) != tc.family {
					continue
				}
				var ok bool
				subject, ok = topologyInspectionSubjectFromLink(replay.data, i)
				require.True(t, ok)
				break
			}
			require.NotEmpty(t, subject.family)

			subject.srcIdentity, subject.dstIdentity = subject.dstIdentity, subject.srcIdentity
			match := inspectTopologyGraphLink(replay.data, subject)
			require.Equal(t, topologyInspectionAbsent, match.membership.state)
		})
	}
}

func TestInspectTopologyLinkSourceContextIsFamilyWideAndKeepsCaptureAvailability(t *testing.T) {
	scenario := newTopologyScenario("inspection-family-source-context")
	left := scenario.Switch("switch-left", "192.0.2.51", "02:00:00:00:51:01")
	right := scenario.Switch("switch-right", "192.0.2.52", "02:00:00:00:52:01")
	otherLeft := scenario.Switch("switch-other-left", "192.0.2.53", "02:00:00:00:53:01")
	otherRight := scenario.Switch("switch-other-right", "192.0.2.54", "02:00:00:00:54:01")
	scenario.LLDP(left.Port("left-right", 1), right.Port("right-left", 1))
	scenario.LLDP(otherLeft.Port("other-left-right", 1), otherRight.Port("other-right-left", 1))
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.NotEmpty(t, replay.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
	require.True(t, ok)

	var unrelatedRegistrationID ddsnmp.DeviceRegistrationID
	for _, device := range replay.devices {
		if device.snapshot == nil {
			continue
		}
		actor, ok := topologyLocalActorFromCache(
			device.snapshot.observation.LocalDeviceID,
			device.snapshot.observation.LocalDevice,
		)
		if ok && !topologyInspectionActorHasIdentity(actor, subject.srcIdentity) &&
			!topologyInspectionActorHasIdentity(actor, subject.dstIdentity) {
			unrelatedRegistrationID = device.registrationID
			break
		}
	}
	require.NotZero(t, unrelatedRegistrationID)
	var retained *topologyAcquisitionCapture
	var latest *topologyAcquisitionCapture
	for i := range diagnostics.topology.devices {
		device := &diagnostics.topology.devices[i]
		if device.registrationID != unrelatedRegistrationID {
			continue
		}
		retained = device.acquisition
		latest = &topologyAcquisitionCapture{
			attemptID: topologyAcquisitionAttemptID{registrationID: unrelatedRegistrationID, ordinal: 2},
			state:     diagnosticCaptureUnavailable,
			reason:    diagnosticCaptureReasonProjectionError,
		}
		device.latestAttempt = latest
		break
	}
	require.NotNil(t, retained)
	require.NotNil(t, latest)

	unavailableCapture := &topologyAcquisitionCapture{
		state:  diagnosticCaptureUnavailable,
		reason: diagnosticCaptureReasonProjectionError,
	}
	diagnostics.topology.devices = append(diagnostics.topology.devices, topologySweepDeviceDiagnostic{
		registrationID: 99,
		acquisition:    unavailableCapture,
		latestAttempt:  unavailableCapture,
	})
	report, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Len(t, report.source.contexts, len(diagnostics.topology.devices))

	unrelated := topologyInspectionSourceContextByRegistration(report.source, unrelatedRegistrationID)
	require.NotNil(t, unrelated)
	require.False(t, unrelated.sameAttempt)
	require.Same(t, latest, unrelated.latestAttempt.capture)
	require.Equal(t, topologyInspectionUndetermined, unrelated.latestAttempt.evidence.state)
	require.Same(t, retained, unrelated.retainedSuccess.capture)
	require.Equal(t, topologyInspectionPresent, unrelated.retainedSuccess.evidence.state)
	require.Len(t, unrelated.captures, 2)
	require.True(t, unrelated.captures[0].latestAttempt)
	require.False(t, unrelated.captures[0].retainedSuccess)
	require.Empty(t, unrelated.captures[0].facts)
	require.False(t, unrelated.captures[1].latestAttempt)
	require.True(t, unrelated.captures[1].retainedSuccess)
	require.NotEmpty(t, unrelated.captures[1].facts)

	unavailable := topologyInspectionSourceContextByRegistration(report.source, 99)
	require.NotNil(t, unavailable)
	require.True(t, unavailable.sameAttempt)
	require.Equal(t, topologyInspectionPresent, unavailable.latestAttempt.membership.state)
	require.Equal(t, topologyInspectionUndetermined, unavailable.latestAttempt.evidence.state)
	require.Same(t, unavailableCapture, unavailable.latestAttempt.capture)
	require.Equal(t, unavailable.latestAttempt, unavailable.retainedSuccess)
	require.Len(t, unavailable.captures, 1)
	require.True(t, unavailable.captures[0].latestAttempt)
	require.True(t, unavailable.captures[0].retainedSuccess)
	require.Same(t, unavailableCapture, unavailable.captures[0].capture.capture)
	require.Empty(t, unavailable.captures[0].facts)
}

func TestInspectTopologyLinkPreservesDiagnosticCutFailure(t *testing.T) {
	scenario := newLLDPDirectScenario()
	limitedCut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{
		sequence:    7,
		startedAt:   topologyScenarioCollectedAt,
		publishedAt: topologyScenarioCollectedAt,
		entries: []ddsnmp.DeviceEntry{
			{RegistrationID: 1},
		},
		limits: topologyAcquisitionLimits{maxRecords: 1, maxLogicalBytes: 1024},
	})
	require.NoError(t, err)
	require.Equal(t, diagnosticCaptureLimitExceeded, limitedCut.captureState)

	subject := topologyInspectionLinkSubject{
		srcIdentity: "ip:192.0.2.1",
		dstIdentity: "ip:192.0.2.2",
		family:      "lldp",
		protocol:    "lldp",
		direction:   "bidirectional",
	}
	report, err := inspectTopologyLink(topologyDiagnostics{topology: limitedCut}, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, diagnosticCaptureLimitExceeded, report.diagnosticCut.captureState)
	require.Equal(t, diagnosticCaptureReasonRecordLimit, report.diagnosticCut.captureReason)
	require.Equal(t, uint64(7), report.diagnosticCut.sequence)
	require.Equal(t, limitedCut.startedAt, report.diagnosticCut.startedAt)
	require.Equal(t, limitedCut.publishedAt, report.diagnosticCut.publishedAt)
	require.Empty(t, report.source.contexts)
	require.Equal(t, topologyInspectionUndetermined, report.graphLink.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.typedLink.state)

	availableCut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{
		sequence:    8,
		startedAt:   topologyScenarioCollectedAt,
		publishedAt: topologyScenarioCollectedAt,
		limits:      topologyAcquisitionLimits{maxRecords: 1, maxLogicalBytes: 1024},
	})
	require.NoError(t, err)
	report, err = inspectTopologyLink(topologyDiagnostics{topology: availableCut}, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, diagnosticCaptureAvailable, report.diagnosticCut.captureState)
	require.Equal(t, diagnosticCaptureReasonNone, report.diagnosticCut.captureReason)
	require.Empty(t, report.source.contexts)
	require.Equal(t, topologyInspectionUndetermined, report.graphLink.membership.state)
	require.Equal(t, topologyInspectionUndetermined, report.typedLink.state)
}

func TestTopologyInspectionSubjectsReturnEveryRenderedLinkAsCandidate(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.Equal(t, topologyInspectionPresent, replay.graph.state)
	require.NotEmpty(t, replay.data.Links)

	families := make(map[string]bool)
	for i := range replay.data.Links {
		subject, ok := topologyInspectionSubjectFromLink(replay.data, i)
		require.Truef(t, ok, "link=%d", i)
		match := inspectTopologyGraphLink(replay.data, subject)
		require.Containsf(t, match.links, replay.data.Links[i], "link=%d family=%s", i, subject.family)
		require.Equal(t, len(match.links), match.membership.candidates)
		if len(match.links) == 1 {
			require.Equal(t, topologyInspectionPresent, match.membership.state)
			require.Equal(t, i, match.index)
		} else {
			require.Equal(t, topologyInspectionUndetermined, match.membership.state)
			require.Equal(t, -1, match.index)
		}
		if topologyInspectionLinkSubjectUnordered(subject) {
			subject.srcIdentity, subject.dstIdentity = subject.dstIdentity, subject.srcIdentity
			reversed := inspectTopologyGraphLink(replay.data, subject)
			require.Containsf(t, reversed.links, replay.data.Links[i], "reversed link=%d family=%s", i, subject.family)
			require.Equal(t, match.membership, reversed.membership)
			require.Equal(t, match.index, reversed.index)
		}
		families[subject.family] = true
	}
	require.True(t, families["lldp"])
	require.True(t, families[topologymodel.L3SubnetLinkType] || families[topologymodel.L3SubnetMembershipLinkType])
	require.True(t, families[topologymodel.OSPFAdjacencyLinkType])
	require.True(t, families[topologymodel.BGPAdjacencyLinkType])
}

func TestInspectTopologyGraphLinkReturnsDuplicateCandidates(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.NotEmpty(t, replay.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
	require.True(t, ok)

	replay.data.Links = append(replay.data.Links, replay.data.Links[0])
	match := inspectTopologyGraphLink(replay.data, subject)
	require.Equal(t, topologyInspectionUndetermined, match.membership.state)
	require.Equal(t, 2, match.membership.candidates)
	require.Equal(t, -1, match.index)
}

func TestInspectTopologyGraphLinkAtSelectsNULIdentity(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		{
			ActorID: "segment-a",
			Match:   topologymodel.Match{Hostnames: []string{"segment:bridge\x00device\x00port"}},
		},
		{
			ActorID: "device-b",
			Match:   topologymodel.Match{IPAddresses: []string{"192.0.2.2"}},
		},
	}}
	allocator := graph.NewActorHandleAllocator()
	for i := range data.Actors {
		data.Actors[i].ActorHandle = allocator.Next()
	}
	data.Links = []topologymodel.Link{{
		LinkType:       "bridge",
		Protocol:       "fdb",
		Direction:      "observed",
		SrcActorHandle: data.Actors[0].ActorHandle,
		DstActorHandle: data.Actors[1].ActorHandle,
	}}

	subject, ok := topologyInspectionSubjectFromLink(data, 0)
	require.True(t, ok)
	require.Contains(t, subject.srcIdentity, "\x00")

	match := inspectTopologyGraphLinkAt(data, 0)
	require.Equal(t, topologyInspectionPresent, match.membership.state)
	require.Equal(t, 1, match.membership.candidates)
	require.Equal(t, 0, match.index)
	require.Equal(t, data.Links, match.links)
	require.Equal(t, topologyInspectionPresent, match.srcActors.membership.state)
	require.Equal(t, topologyInspectionPresent, match.dstActors.membership.state)
}

func TestTopologyInspectionLinkSubjectReturnsParallelBGPRoutingInstancesAsCandidates(t *testing.T) {
	scenario := newTopologyScenario("inspection-parallel-bgp")
	left := scenario.Router("router-left", "192.0.2.61", "02:00:00:00:61:01", "192.0.2.61", "65001")
	right := scenario.Router("router-right", "192.0.2.62", "02:00:00:00:62:01", "192.0.2.62", "65002")
	left.Port("left-right", 1).IPv4("198.51.100.1/30")
	right.Port("right-left", 1).IPv4("198.51.100.2/30")
	scenario.BGP(left, right, "blue")
	scenario.BGP(left, right, "red")
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)

	var indexes []int
	var subjects []topologyInspectionLinkSubject
	for i, link := range replay.data.Links {
		if topologyInspectionLinkFamily(link) != topologymodel.BGPAdjacencyLinkType {
			continue
		}
		subject, ok := topologyInspectionSubjectFromLink(replay.data, i)
		require.True(t, ok)
		indexes = append(indexes, i)
		subjects = append(subjects, subject)
	}
	require.Len(t, indexes, 2)
	require.Len(t, subjects, 2)
	require.Equal(t, subjects[0], subjects[1])
	match := inspectTopologyGraphLink(replay.data, subjects[0])
	require.Equal(t, topologyInspectionUndetermined, match.membership.state)
	require.Equal(t, 2, match.membership.candidates)
	require.Len(t, match.links, 2)
	routingInstances := make(map[string]bool)
	for _, link := range match.links {
		require.NotNil(t, link.Detail.BGP)
		routingInstances[link.Detail.BGP.RoutingInstance] = true
	}
	require.Equal(t, map[string]bool{"blue": true, "red": true}, routingInstances)

	for _, index := range indexes {
		report, err := inspectTopologyLinkAt(diagnostics, scenario.opts, index)
		require.NoError(t, err)
		require.Equal(t, topologyInspectionPresent, report.graphLink.membership.state)
		require.Equal(t, 1, report.graphLink.membership.candidates)
		require.Equal(t, index, report.graphLink.index)
		require.Equal(t, []topologymodel.Link{replay.data.Links[index]}, report.graphLink.links)
		require.Equal(t, topologyInspectionPresent, report.typedLink.state)
		require.Equal(t, index, report.typedLink.row)
		require.Equal(t, subjects[0], report.subject)
	}
}

func setTopologyInspectionLifecycleCut(diagnostics *topologyDiagnostics, registrationIDs ...ddsnmp.DeviceRegistrationID) {
	diagnostics.lifecycle = topologyJobLifecycleDiagnosticCut{
		state: diagnosticCaptureAvailable,
		cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   1,
			CapturedAt: topologyScenarioCollectedAt,
		},
	}
	for _, registrationID := range registrationIDs {
		diagnostics.lifecycle.cut.Entries = append(diagnostics.lifecycle.cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: registrationID,
			TopologyReady:  true,
		})
	}
}

func topologyInspectionSourceContextByRegistration(
	result topologyInspectionSourceResult,
	registrationID ddsnmp.DeviceRegistrationID,
) *topologyInspectionSourceContext {
	for i := range result.contexts {
		if result.contexts[i].registrationID == registrationID {
			return &result.contexts[i]
		}
	}
	return nil
}

func topologyInspectionSourceFactCount(result topologyInspectionSourceResult) int {
	var count int
	for _, context := range result.contexts {
		for _, capture := range context.captures {
			count += len(capture.facts)
		}
	}
	return count
}
