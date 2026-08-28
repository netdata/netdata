// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
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

func TestInspectTopologyLinkUsesStructuralSubjectAndSingleRenderedRow(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)

	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.Equal(t, topologyInspectionPresent, replay.graph.state)
	require.NotEmpty(t, replay.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
	require.True(t, ok)

	report, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionPresent, report.source.membership.state)
	require.NotEmpty(t, report.source.facts)
	require.Equal(t, topologyInspectionPresent, report.graphLink.membership.state)
	require.Equal(t, 1, report.graphLink.membership.candidates)
	require.Equal(t, topologyInspectionPresent, report.typedLink.state)
	require.Equal(t, report.graphLink.index, report.typedLink.row)

	subject.discriminator.srcPortID = "missing-port"
	report, err = inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionAbsent, report.graphLink.membership.state)
	require.Equal(t, topologyInspectionAbsent, report.typedLink.state)
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
	data.Links = []topologymodel.Link{{
		Protocol:       "lldp",
		Direction:      "bidirectional",
		SrcActorHandle: data.Actors[0].ActorHandle,
		DstActorHandle: data.Actors[2].ActorHandle,
	}}
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
}

func TestInspectTopologyLinkSourceAbsenceRequiresCompleteRoutes(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	require.NotEmpty(t, replay.data.Links)
	subject, ok := topologyInspectionSubjectFromLink(replay.data, 0)
	require.True(t, ok)

	setTopologyInspectionSourceRoutes(t, diagnostics, ddsnmpcollector.AcquisitionRouteOutcomePartial)
	report, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionUndetermined, report.source.membership.state)
	require.Empty(t, report.source.facts)

	setTopologyInspectionSourceRoutes(t, diagnostics, ddsnmpcollector.AcquisitionRouteOutcomeEmpty)
	report, err = inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionAbsent, report.source.membership.state)
	require.Empty(t, report.source.facts)

	clearTopologyInspectionSourceRoutes(t, diagnostics)
	report, err = inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	require.Equal(t, topologyInspectionUndetermined, report.source.membership.state)
	require.Empty(t, report.source.facts)
}

func TestTopologyInspectionSubjectsDistinguishEveryRenderedLink(t *testing.T) {
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
		require.Equalf(t, topologyInspectionPresent, match.membership.state, "link=%d family=%s", i, subject.family)
		require.Equalf(t, 1, match.membership.candidates, "link=%d family=%s", i, subject.family)
		require.Equal(t, i, match.index)
		families[subject.family] = true
	}
	require.True(t, families["lldp"])
	require.True(t, families[topologymodel.L3SubnetLinkType] || families[topologymodel.L3SubnetMembershipLinkType])
	require.True(t, families[topologymodel.OSPFAdjacencyLinkType])
	require.True(t, families[topologymodel.BGPAdjacencyLinkType])
}

func TestInspectTopologyGraphLinkRejectsDuplicateStructuralMatches(t *testing.T) {
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

func TestTopologyInspectionLinkSubjectSeparatesParallelBGPRoutingInstances(t *testing.T) {
	scenario := newTopologyScenario("inspection-parallel-bgp")
	left := scenario.Router("router-left", "192.0.2.61", "02:00:00:00:61:01", "192.0.2.61", "65001")
	right := scenario.Router("router-right", "192.0.2.62", "02:00:00:00:62:01", "192.0.2.62", "65002")
	left.Port("left-right", 1).IPv4("198.51.100.1/30")
	right.Port("right-left", 1).IPv4("198.51.100.2/30")
	scenario.BGP(left, right, "blue")
	scenario.BGP(left, right, "red")
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	replay := replayTopologyDiagnosticStages(diagnostics, scenario.opts)

	var subjects []topologyInspectionLinkSubject
	for i, link := range replay.data.Links {
		if topologyInspectionLinkFamily(link) != topologymodel.BGPAdjacencyLinkType {
			continue
		}
		subject, ok := topologyInspectionSubjectFromLink(replay.data, i)
		require.True(t, ok)
		subjects = append(subjects, subject)
	}
	require.Len(t, subjects, 2)
	require.NotEqual(t, subjects[0].discriminator.bgpRoutingInstance, subjects[1].discriminator.bgpRoutingInstance)
	for _, subject := range subjects {
		match := inspectTopologyGraphLink(replay.data, subject)
		require.Equal(t, topologyInspectionPresent, match.membership.state)
		require.Equal(t, 1, match.membership.candidates)
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

func setTopologyInspectionSourceRoutes(
	t *testing.T,
	diagnostics topologyDiagnostics,
	outcome ddsnmpcollector.AcquisitionRouteOutcome,
) {
	t.Helper()
	for deviceIndex := range diagnostics.topology.devices {
		capture := diagnostics.topology.devices[deviceIndex].acquisition
		require.NotNil(t, capture)
		require.NotNil(t, capture.evidence)
		for contextIndex := range capture.evidence.collectionContexts {
			context := &capture.evidence.collectionContexts[contextIndex]
			for profileIndex := range context.profiles {
				profile := &context.profiles[profileIndex]
				profile.values.metrics = nil
				for routeIndex := range profile.routes {
					route := &profile.routes[routeIndex]
					if route.Kind == ddsnmpcollector.AcquisitionRouteKindTopologyScalar ||
						route.Kind == ddsnmpcollector.AcquisitionRouteKindTopologyTable {
						route.Outcome = outcome
					}
				}
			}
		}
	}
}

func clearTopologyInspectionSourceRoutes(t *testing.T, diagnostics topologyDiagnostics) {
	t.Helper()
	for deviceIndex := range diagnostics.topology.devices {
		capture := diagnostics.topology.devices[deviceIndex].acquisition
		require.NotNil(t, capture)
		require.NotNil(t, capture.evidence)
		for contextIndex := range capture.evidence.collectionContexts {
			context := &capture.evidence.collectionContexts[contextIndex]
			for profileIndex := range context.profiles {
				profile := &context.profiles[profileIndex]
				profile.values.metrics = nil
				profile.values.bgpRows = nil
				profile.routes = nil
			}
		}
	}
}
