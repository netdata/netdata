// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestInspectTopologyDeviceSeparatesLatestAttemptFromRetainedSuccess(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)

	row := &diagnostics.Topology.Devices[0]
	retained := row.Acquisition
	row.LatestAttempt = &topologydiag.AcquisitionCapture{
		AttemptID: topologydiag.AcquisitionAttemptID{RegistrationID: 1, Ordinal: 2},
		State:     topologydiag.CaptureUnavailable,
		Reason:    topologydiag.CaptureReasonProjectionError,
	}
	aborted := &topologydiag.AbortedSweep{Sequence: 2, Phase: topologydiag.DiagnosticSweepPhaseDeviceRefresh}
	diagnostics.LastAborted = aborted

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), report.Lifecycle.Sequence)
	require.Equal(t, topologyScenarioCollectedAt, report.Lifecycle.CapturedAt)
	require.Equal(t, diagnostics.Topology.Sequence, report.Sweep.Sequence)
	require.Equal(t, diagnostics.Topology.StartedAt, report.Sweep.StartedAt)
	require.Equal(t, diagnostics.Topology.PublishedAt, report.Sweep.PublishedAt)
	require.Equal(t, aborted.Sequence, report.LastAborted.Sequence)
	require.Equal(t, "device_refresh", report.LastAborted.Phase)
	require.Equal(t, "present", report.Lifecycle.Membership.State)
	require.Equal(t, "present", report.Sweep.Membership.State)
	require.Equal(t, "present", report.LatestAttempt.Membership.State)
	require.Equal(t, "undetermined", report.LatestAttempt.Evidence.State)
	require.Equal(t, "present", report.RetainedSuccess.Membership.State)
	require.Equal(t, "present", report.RetainedSuccess.Evidence.State)
	require.False(t, report.SameAttempt)
	require.Equal(t, retained.AttemptID.Ordinal, report.RetainedSuccess.Capture.AttemptOrdinal)
	require.Equal(t, "present", report.Observation.State)
	require.Equal(t, "present", report.GraphIdentity.Membership.State)
	require.Equal(t, "present", report.TypedIdentity.Membership.State)
	require.Equal(t, 1, report.TypedIdentity.Membership.Candidates)
}

func TestInspectTopologyDeviceKeepsIncompleteCutsUndetermined(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	diagnostics.Lifecycle = topologydiag.LifecycleCut{
		State:  topologydiag.CaptureUnavailable,
		Reason: topologydiag.CaptureReasonProjectionError,
	}

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.Equal(t, "undetermined", report.Lifecycle.Membership.State)
	require.Equal(t, "unavailable", report.Lifecycle.Capture.State)
	require.Equal(t, "projection_error", report.Lifecycle.Capture.Reason)
	require.Equal(t, "present", report.Sweep.Membership.State)

	diagnostics.Topology = unavailableTopologyDiagnosticCut(topologyDiagnosticCutInput{sequence: diagnostics.Topology.Sequence}, topologydiag.CaptureReasonProjectionError)
	report, err = openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.Equal(t, "undetermined", report.Sweep.Membership.State)
	require.Equal(t, "unavailable", report.Sweep.Capture.State)
	require.Equal(t, "undetermined", report.LatestAttempt.Membership.State)
	require.Equal(t, "undetermined", report.RetainedSuccess.Membership.State)
	require.Equal(t, "undetermined", report.Observation.State)
	require.Equal(t, "undetermined", report.GraphIdentity.Membership.State)
	require.Equal(t, "undetermined", report.TypedIdentity.Membership.State)
}

func TestInspectTopologyDeviceReportsExactMissingRegistration(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)
	diagnostics.Topology.Removed = append(diagnostics.Topology.Removed, topologydiag.RemovedDevice{RegistrationID: 99})

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 99)
	require.NoError(t, err)
	require.Equal(t, "absent", report.Lifecycle.Membership.State)
	require.Equal(t, "absent", report.Sweep.Membership.State)
	require.Equal(t, "present", report.Removed.Membership.State)
	require.NotNil(t, report.Removed.Device)
	require.Equal(t, "absent", report.LatestAttempt.Membership.State)
	require.Equal(t, "absent", report.RetainedSuccess.Membership.State)
	require.Equal(t, "absent", report.Observation.State)
	require.Equal(t, "undetermined", report.GraphIdentity.Membership.State)
	require.Equal(t, "undetermined", report.TypedIdentity.Membership.State)
}

func TestInspectTopologyDeviceKeepsPreRegistrationSeparateFromSweep(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2, 99)

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 99)
	require.NoError(t, err)
	require.Equal(t, "present", report.Lifecycle.Membership.State)
	require.Equal(t, "absent", report.Sweep.Membership.State)
	require.Equal(t, "absent", report.Observation.State)
	require.Equal(t, "undetermined", report.GraphIdentity.Membership.State)
}

func TestInspectTopologyDeviceReportsIdentityRepresentationAfterExpiry(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)
	diagnostics.Topology.Devices[0].Renderable = false
	diagnostics.Topology.Devices[0].Expired = true

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.True(t, report.Sweep.Device.Expired)
	require.Equal(t, "present", report.Observation.State)
	require.Equal(t, "present", report.GraphIdentity.Membership.State)
	require.Equal(t, "present", report.TypedIdentity.Membership.State)
}

func TestInspectTopologyDeviceUsesOneCollapsedIdentityRepresentation(t *testing.T) {
	scenario := newTopologyScenario("inspection-collapsed-identity")
	left := scenario.Switch("switch-left", "192.0.2.50", "02:00:00:00:50:01")
	right := scenario.Switch("switch-right", "192.0.2.50", "02:00:00:00:50:02")
	scenario.LLDP(left.Port("left-right", 1), right.Port("right-left", 1))
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1, 2)

	leftReport, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	rightReport, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 2)
	require.NoError(t, err)
	require.Equal(t, "present", leftReport.GraphIdentity.Membership.State)
	require.Equal(t, "present", rightReport.GraphIdentity.Membership.State)
	require.Equal(t, leftReport.GraphIdentity.SelectedIndex, rightReport.GraphIdentity.SelectedIndex)
}

func TestInspectTopologyDeviceUsesLocalDeviceIDFallbackRepresentation(t *testing.T) {
	scenario := newTopologyScenario("inspection-local-device-id-fallback")
	scenario.Switch("switch-fallback", "192.0.2.55", "02:00:00:00:55:01")
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	setTopologyInspectionLifecycleCut(&diagnostics, 1)

	capture := diagnostics.Topology.Devices[0].Acquisition
	require.NotNil(t, capture)
	require.NotNil(t, capture.Evidence)
	capture.Evidence.Device = topologydiag.DeviceInput{}
	for contextIndex := range capture.Evidence.CollectionContexts {
		context := &capture.Evidence.CollectionContexts[contextIndex]
		for profileIndex := range context.Profiles {
			profile := &context.Profiles[profileIndex]
			profile.Values = topologydiag.AcquisitionProfileValues{}
		}
	}

	report, err := openTestDiagnosticCut(t, diagnostics).InspectDevice(testDiagnosticQuery(scenario.opts), 1)
	require.NoError(t, err)
	require.Equal(t, "present", report.Observation.State)
	require.Equal(t, "present", report.GraphIdentity.Membership.State)
	require.Equal(t, "present", report.TypedIdentity.Membership.State)
	require.Equal(t, "local-device", report.GraphIdentity.Candidates[0].ActorID)
}

func TestInspectTopologyLinkUsesCandidateSubjectAndSingleRenderedRow(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, cut := newTopologyScenarioReplayFixture(t, scenario)
	archive := openTestDiagnosticCut(t, cut)
	query := testDiagnosticQuery(scenario.opts)
	exact, err := archive.InspectLinkAt(query, 0)
	require.NoError(t, err)
	subject := exact.Subject
	report, err := archive.InspectLink(query, subject)
	require.NoError(t, err)
	require.Len(t, report.Source.Contexts, len(cut.Topology.Devices))
	require.NotZero(t, inspectionSourceFactCount(report))
	require.Equal(t, "present", report.GraphLink.Membership.State)
	require.Equal(t, 1, report.GraphLink.Membership.Candidates)
	require.Equal(t, "present", report.TypedLink.Membership.State)
	require.Equal(t, 1, report.TypedLink.Membership.Candidates)
	require.Equal(t, report.GraphLink.SelectedIndex, report.TypedLink.Row)
	sources := report.Source
	for name, tc := range map[string]struct {
		subject DiagnosticLinkSubject
		state   string
	}{
		"reverse":          {DiagnosticLinkSubject{SourceIdentity: subject.DestinationIdentity, DestinationIdentity: subject.SourceIdentity, Family: subject.Family, Protocol: subject.Protocol, Direction: subject.Direction}, "present"},
		"missing endpoint": {DiagnosticLinkSubject{SourceIdentity: subject.SourceIdentity, DestinationIdentity: "ip:192.0.2.254", Family: subject.Family, Protocol: subject.Protocol, Direction: subject.Direction}, "absent"},
	} {
		t.Run(name, func(t *testing.T) {
			report, err := archive.InspectLink(query, tc.subject)
			require.NoError(t, err)
			require.Equal(t, tc.state, report.GraphLink.Membership.State)
			require.Equal(t, tc.state, report.TypedLink.Membership.State)
			require.Equal(t, sources, report.Source)
		})
	}
}

func TestInspectTopologyGraphLinkPreservesOrderedEndpointRoles(t *testing.T) {
	for name, tc := range map[string]struct {
		scenario func() *topologyScenario
		family   string
	}{
		"stp":                  {newSTPInferredScenario, "stp"},
		"l3 subnet membership": {newMixedL2L3ControlScenario, topologymodel.L3SubnetMembershipLinkType},
	} {
		t.Run(name, func(t *testing.T) {
			scenario := tc.scenario()
			_, cut := newTopologyScenarioReplayFixture(t, scenario)
			archive := openTestDiagnosticCut(t, cut)
			query := testDiagnosticQuery(scenario.opts)
			payload, err := archive.Replay(query)
			require.NoError(t, err)
			var found bool
			for i := 0; i < payload.Links.Rows; i++ {
				exact, err := archive.InspectLinkAt(query, i)
				require.NoError(t, err)
				subject := exact.Subject
				if subject.Family != tc.family {
					continue
				}
				found = true
				subject.SourceIdentity, subject.DestinationIdentity = subject.DestinationIdentity, subject.SourceIdentity
				match, err := archive.InspectLink(query, subject)
				require.NoError(t, err)
				require.Equal(t, "absent", match.GraphLink.Membership.State)
			}
			require.True(t, found)
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
	_, cut := newTopologyScenarioReplayFixture(t, scenario)
	archive := openTestDiagnosticCut(t, cut)
	query := testDiagnosticQuery(scenario.opts)
	exact, err := archive.InspectLinkAt(query, 0)
	require.NoError(t, err)
	var unrelatedID ddsnmp.DeviceRegistrationID
	for _, device := range cut.Topology.Devices {
		report, err := archive.InspectDevice(query, uint64(device.RegistrationID))
		require.NoError(t, err)
		require.Len(t, report.GraphIdentity.Candidates, 1)
		keys := report.GraphIdentity.Candidates[0].IdentityKeys
		if !slices.Contains(keys, exact.Subject.SourceIdentity) && !slices.Contains(keys, exact.Subject.DestinationIdentity) {
			unrelatedID = device.RegistrationID
			break
		}
	}
	require.NotZero(t, unrelatedID)
	for i := range cut.Topology.Devices {
		row := &cut.Topology.Devices[i]
		if row.RegistrationID == unrelatedID {
			row.LatestAttempt = &topologydiag.AcquisitionCapture{AttemptID: topologydiag.AcquisitionAttemptID{RegistrationID: unrelatedID, Ordinal: 2}, State: topologydiag.CaptureUnavailable, Reason: topologydiag.CaptureReasonProjectionError}
		}
	}
	unavailable := &topologydiag.AcquisitionCapture{AttemptID: topologydiag.AcquisitionAttemptID{RegistrationID: 99, Ordinal: 1}, State: topologydiag.CaptureUnavailable, Reason: topologydiag.CaptureReasonProjectionError}
	cut.Topology.Devices = append(cut.Topology.Devices, topologydiag.SweepDevice{RegistrationID: 99, Acquisition: unavailable, LatestAttempt: unavailable, HasRetainedSuccess: true, RetainedSuccess: topologydiag.EvidenceRef{RegistrationID: 99, Generation: cut.Topology.Sequence}})
	report, err := openTestDiagnosticCut(t, cut).InspectLink(query, exact.Subject)
	require.NoError(t, err)
	require.Len(t, report.Source.Contexts, len(cut.Topology.Devices))
	unrelated := report.Source.Contexts[inspectionSourceContextIndex(t, report, uint64(unrelatedID))]
	require.False(t, unrelated.SameAttempt)
	require.Equal(t, uint64(2), unrelated.LatestAttempt.Capture.AttemptOrdinal)
	require.Equal(t, "undetermined", unrelated.LatestAttempt.Evidence.State)
	require.Equal(t, "present", unrelated.RetainedSuccess.Evidence.State)
	require.Len(t, unrelated.Captures, 2)
	require.True(t, unrelated.Captures[0].LatestAttempt)
	require.False(t, unrelated.Captures[0].RetainedSuccess)
	require.Empty(t, unrelated.Captures[0].Facts)
	require.False(t, unrelated.Captures[1].LatestAttempt)
	require.True(t, unrelated.Captures[1].RetainedSuccess)
	require.NotEmpty(t, unrelated.Captures[1].Facts)
	missing := report.Source.Contexts[inspectionSourceContextIndex(t, report, 99)]
	require.True(t, missing.SameAttempt)
	require.Equal(t, "present", missing.LatestAttempt.Membership.State)
	require.Equal(t, "undetermined", missing.LatestAttempt.Evidence.State)
	require.Equal(t, missing.LatestAttempt, missing.RetainedSuccess)
	require.Len(t, missing.Captures, 1)
	require.True(t, missing.Captures[0].LatestAttempt)
	require.True(t, missing.Captures[0].RetainedSuccess)
	require.Equal(t, missing.LatestAttempt, missing.Captures[0].Capture)
	require.Empty(t, missing.Captures[0].Facts)
}

func TestInspectTopologyLinkPreservesDiagnosticCutFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		state                 topologydiag.CaptureState
		reason                topologydiag.CaptureReason
		wantState, wantReason string
	}{
		"unavailable": {topologydiag.CaptureUnavailable, topologydiag.CaptureReasonProjectionError, "unavailable", "projection_error"},
		"empty":       {topologydiag.CaptureAvailable, topologydiag.CaptureReasonNone, "available", "none"},
	} {
		t.Run(name, func(t *testing.T) {
			cut := topologydiag.Cut{Topology: &topologydiag.SweepCut{Sequence: 7, StartedAt: topologyScenarioCollectedAt, PublishedAt: topologyScenarioCollectedAt, CaptureState: tc.state, CaptureReason: tc.reason}}
			subject := DiagnosticLinkSubject{SourceIdentity: "ip:192.0.2.1", DestinationIdentity: "ip:192.0.2.2", Family: "lldp", Protocol: "lldp", Direction: "bidirectional"}
			report, err := openTestDiagnosticCut(t, cut).InspectLink(DefaultDiagnosticQueryOptions(), subject)
			require.NoError(t, err)
			require.Equal(t, tc.wantState, report.DiagnosticCut.Capture.State)
			require.Equal(t, tc.wantReason, report.DiagnosticCut.Capture.Reason)
			require.Equal(t, uint64(7), report.DiagnosticCut.Sequence)
			require.Equal(t, cut.Topology.StartedAt, report.DiagnosticCut.StartedAt)
			require.Equal(t, cut.Topology.PublishedAt, report.DiagnosticCut.PublishedAt)
			require.Empty(t, report.Source.Contexts)
			require.Equal(t, "undetermined", report.GraphLink.Membership.State)
			require.Equal(t, "undetermined", report.TypedLink.Membership.State)
		})
	}
}

func TestTopologyInspectionSubjectsReturnEveryRenderedLinkAsCandidate(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	_, cut := newTopologyScenarioReplayFixture(t, scenario)
	archive := openTestDiagnosticCut(t, cut)
	query := testDiagnosticQuery(scenario.opts)
	payload, err := archive.Replay(query)
	require.NoError(t, err)
	require.Positive(t, payload.Links.Rows)
	families := make(map[string]bool)
	reversedFamilies := make(map[string]bool)
	for i := 0; i < payload.Links.Rows; i++ {
		exact, err := archive.InspectLinkAt(query, i)
		require.NoError(t, err)
		subject := exact.Subject
		match, err := archive.InspectLink(query, subject)
		require.NoError(t, err)
		require.Contains(t, match.GraphLink.Candidates, exact.GraphLink.Candidates[0])
		require.Equal(t, len(match.GraphLink.Candidates), match.GraphLink.Membership.Candidates)
		if len(match.GraphLink.Candidates) == 1 {
			require.Equal(t, "present", match.GraphLink.Membership.State)
			require.Equal(t, i, match.GraphLink.SelectedIndex)
		} else {
			require.Equal(t, "undetermined", match.GraphLink.Membership.State)
			require.Equal(t, -1, match.GraphLink.SelectedIndex)
		}
		unordered := subject.Direction == "bidirectional"
		switch subject.Family {
		case topologymodel.L3SubnetLinkType, topologymodel.OSPFAdjacencyLinkType, topologymodel.BGPAdjacencyLinkType:
			unordered = true
		}
		if unordered {
			subject.SourceIdentity, subject.DestinationIdentity = subject.DestinationIdentity, subject.SourceIdentity
			reversed, err := archive.InspectLink(query, subject)
			require.NoError(t, err)
			require.Contains(t, reversed.GraphLink.Candidates, exact.GraphLink.Candidates[0])
			require.Equal(t, match.GraphLink.Membership, reversed.GraphLink.Membership)
			require.Equal(t, match.GraphLink.SelectedIndex, reversed.GraphLink.SelectedIndex)
			reversedFamilies[subject.Family] = true
		}
		families[subject.Family] = true
	}
	require.True(t, families["lldp"])
	require.True(t, families[topologymodel.L3SubnetLinkType] || families[topologymodel.L3SubnetMembershipLinkType])
	require.True(t, families[topologymodel.OSPFAdjacencyLinkType])
	require.True(t, families[topologymodel.BGPAdjacencyLinkType])
	require.True(t, reversedFamilies[topologymodel.L3SubnetLinkType])
	require.True(t, reversedFamilies[topologymodel.OSPFAdjacencyLinkType])
	require.True(t, reversedFamilies[topologymodel.BGPAdjacencyLinkType])
}

func TestTopologyInspectionLinkSubjectReturnsParallelBGPRoutingInstancesAsCandidates(t *testing.T) {
	scenario := newTopologyScenario("inspection-parallel-bgp")
	left := scenario.Router("router-left", "192.0.2.61", "02:00:00:00:61:01", "192.0.2.61", "65001")
	right := scenario.Router("router-right", "192.0.2.62", "02:00:00:00:62:01", "192.0.2.62", "65002")
	left.Port("left-right", 1).IPv4("198.51.100.1/30")
	right.Port("right-left", 1).IPv4("198.51.100.2/30")
	scenario.BGP(left, right, "blue")
	scenario.BGP(left, right, "red")
	_, cut := newTopologyScenarioReplayFixture(t, scenario)
	archive := openTestDiagnosticCut(t, cut)
	query := testDiagnosticQuery(scenario.opts)
	payload, err := archive.Replay(query)
	require.NoError(t, err)
	var exacts []DiagnosticLinkInspection
	for i := 0; i < payload.Links.Rows; i++ {
		exact, err := archive.InspectLinkAt(query, i)
		require.NoError(t, err)
		if exact.Subject.Family == topologymodel.BGPAdjacencyLinkType {
			exacts = append(exacts, exact)
		}
	}
	require.Len(t, exacts, 2)
	require.Equal(t, exacts[0].Subject, exacts[1].Subject)
	match, err := archive.InspectLink(query, exacts[0].Subject)
	require.NoError(t, err)
	require.Equal(t, "undetermined", match.GraphLink.Membership.State)
	require.Equal(t, 2, match.GraphLink.Membership.Candidates)
	require.Len(t, match.GraphLink.Candidates, 2)
	instances := make(map[string]bool)
	for _, link := range match.GraphLink.Candidates {
		require.NotNil(t, link.Detail.BGP)
		instances[link.Detail.BGP.RoutingInstance] = true
	}
	require.Equal(t, map[string]bool{"blue": true, "red": true}, instances)
	for _, exact := range exacts {
		require.Equal(t, "present", exact.GraphLink.Membership.State)
		require.Equal(t, 1, exact.GraphLink.Membership.Candidates)
		require.Equal(t, "present", exact.TypedLink.Membership.State)
		require.Equal(t, exact.GraphLink.SelectedIndex, exact.TypedLink.Row)
		require.Contains(t, match.GraphLink.Candidates, exact.GraphLink.Candidates[0])
	}
}

func inspectionSourceContextIndex(t *testing.T, report DiagnosticLinkInspection, id uint64) int {
	t.Helper()
	for i, context := range report.Source.Contexts {
		if context.RegistrationID == id {
			return i
		}
	}
	t.Fatalf("missing source context for registration %d", id)
	return -1
}

func inspectionSourceFactCount(report DiagnosticLinkInspection) int {
	var count int
	for _, context := range report.Source.Contexts {
		for _, capture := range context.Captures {
			count += len(capture.Facts)
		}
	}
	return count
}

func setTopologyInspectionLifecycleCut(diagnostics *topologydiag.Cut, registrationIDs ...ddsnmp.DeviceRegistrationID) {
	diagnostics.Lifecycle = topologydiag.LifecycleCut{
		State: topologydiag.CaptureAvailable,
		Cut: ddsnmp.DeviceLifecycleCut{
			Sequence:   1,
			CapturedAt: topologyScenarioCollectedAt,
		},
	}
	for _, registrationID := range registrationIDs {
		diagnostics.Lifecycle.Cut.Entries = append(diagnostics.Lifecycle.Cut.Entries, ddsnmp.DeviceLifecycleEntry{
			RegistrationID: registrationID,
			TopologyReady:  true,
		})
	}
}
