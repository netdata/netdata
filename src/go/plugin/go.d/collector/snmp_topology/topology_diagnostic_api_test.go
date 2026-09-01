// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticArchiveAPIReusesArchiveReplayAndInspection(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchiveWithProducerVersion(&encoded, diagnostics, "v-test"))

	archive, err := ReadDiagnosticArchive(
		bytes.NewReader(encoded.Bytes()),
		DefaultDiagnosticArchiveReadLimits(),
	)
	require.NoError(t, err)
	require.Equal(t, DiagnosticArchiveIdentity{
		Format:               topologyDiagnosticArchiveFormat,
		Version:              topologyDiagnosticArchiveVersion,
		ProducerAgentVersion: "v-test",
	}, archive.Identity())

	summary, err := archive.Summary()
	require.NoError(t, err)
	require.Equal(t, archive.Identity(), summary.Archive)
	require.NotEmpty(t, summary.Registrations)
	require.Equal(t, uint64(1), summary.Registrations[0].RegistrationID)

	query := diagnosticQueryOptionsFromInternal(scenario.opts)
	gotReplay, err := archive.Replay(query)
	require.NoError(t, err)
	wantReplay, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, topologyv1test.NormalizeData(t, wantReplay), topologyv1test.NormalizeData(t, gotReplay))

	device, err := archive.InspectDevice(query, 1)
	require.NoError(t, err)
	directDeviceReport, err := inspectTopologyDevice(diagnostics, scenario.opts, 1)
	require.NoError(t, err)
	directDevice, err := newDiagnosticDeviceInspection(directDeviceReport)
	require.NoError(t, err)
	require.Equal(t, directDevice, device)
	require.Equal(t, uint64(1), device.RegistrationID)
	require.Equal(t, diagnosticStatePresent, device.Sweep.Membership.State)
	require.Equal(t, diagnosticStatePresent, device.Observation.State)
	require.Equal(t, diagnosticStatePresent, device.GraphIdentity.Membership.State)
	require.NotEmpty(t, device.GraphIdentity.Candidates)
	require.Equal(t, diagnosticStatePresent, device.TypedIdentity.Membership.State)
	require.Equal(t, 1, device.TypedIdentity.Membership.Candidates)
	require.Equal(t, wantReplay.Stats, device.GraphStats)

	stages := replayTopologyDiagnosticStages(diagnostics, scenario.opts)
	subject, ok := topologyInspectionSubjectFromLink(stages.data, 0)
	require.True(t, ok)
	link, err := archive.InspectLink(query, DiagnosticLinkSubject{
		SourceIdentity:      subject.srcIdentity,
		DestinationIdentity: subject.dstIdentity,
		Family:              subject.family,
		Protocol:            subject.protocol,
		Direction:           subject.direction,
	})
	require.NoError(t, err)
	directLinkReport, err := inspectTopologyLink(diagnostics, scenario.opts, subject)
	require.NoError(t, err)
	directLink, err := newDiagnosticLinkInspection(directLinkReport)
	require.NoError(t, err)
	require.Equal(t, directLink, link)
	require.Equal(t, diagnosticStatePresent, link.GraphLink.Membership.State)
	require.NotEmpty(t, link.GraphLink.Candidates)
	require.Equal(t, diagnosticStatePresent, link.TypedLink.Membership.State)
	require.Equal(t, 1, link.TypedLink.Membership.Candidates)
	require.NotEmpty(t, link.Source.Contexts)
	require.Equal(t, wantReplay.Stats, link.Stats)

	linkAt, err := archive.InspectLinkAt(query, 0)
	require.NoError(t, err)
	directLinkAtReport, err := inspectTopologyLinkAt(diagnostics, scenario.opts, 0)
	require.NoError(t, err)
	directLinkAt, err := newDiagnosticLinkInspection(directLinkAtReport)
	require.NoError(t, err)
	require.Equal(t, directLinkAt, linkAt)
	require.Equal(t, 0, linkAt.GraphLink.SelectedIndex)
	require.Equal(t, 0, linkAt.TypedLink.Row)
}

func TestDiagnosticArchiveAPIRejectsInvalidExactLinkIndexes(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchive(&encoded, diagnostics))
	archive, err := ReadDiagnosticArchive(bytes.NewReader(encoded.Bytes()), DefaultDiagnosticArchiveReadLimits())
	require.NoError(t, err)

	query := diagnosticQueryOptionsFromInternal(newLLDPDirectScenario().opts)
	for _, index := range []int{-1, 1_000_000} {
		_, err := archive.InspectLinkAt(query, index)
		require.ErrorContains(t, err, "link index")
	}
}

func TestDiagnosticArchiveAPIRejectsInvalidExternalSelectors(t *testing.T) {
	_, diagnostics := newTopologyScenarioReplayFixture(t, newLLDPDirectScenario())
	completeTopologyDiagnosticArchiveFixture(&diagnostics)

	var encoded bytes.Buffer
	require.NoError(t, writeTopologyDiagnosticArchive(&encoded, diagnostics))
	archive, err := ReadDiagnosticArchive(bytes.NewReader(encoded.Bytes()), DefaultDiagnosticArchiveReadLimits())
	require.NoError(t, err)

	tests := map[string]struct {
		query   DiagnosticQueryOptions
		subject DiagnosticLinkSubject
		link    bool
		want    string
	}{
		"map type": {
			query: DiagnosticQueryOptions{MapType: "other"},
			want:  "map type",
		},
		"inference strategy": {
			query: DiagnosticQueryOptions{InferenceStrategy: "other"},
			want:  "inference strategy",
		},
		"managed focus": {
			query: DiagnosticQueryOptions{ManagedDeviceFocus: "hostname:router"},
			want:  "managed device focus",
		},
		"empty managed focus list": {
			query: DiagnosticQueryOptions{ManagedDeviceFocus: ","},
			want:  "managed device focus",
		},
		"depth": {
			query: DiagnosticQueryOptions{Depth: "eleven"},
			want:  "depth",
		},
		"link family": {
			query: diagnosticQueryOptionsFromInternal(newLLDPDirectScenario().opts),
			subject: DiagnosticLinkSubject{
				SourceIdentity:      "actor:a",
				DestinationIdentity: "actor:b",
				Family:              "other",
			},
			link: true,
			want: "link family",
		},
		"link direction": {
			query: diagnosticQueryOptionsFromInternal(newLLDPDirectScenario().opts),
			subject: DiagnosticLinkSubject{
				SourceIdentity:      "actor:a",
				DestinationIdentity: "actor:b",
				Family:              "lldp",
				Direction:           "",
			},
			link: true,
			want: "link direction",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			if tc.link {
				_, err = archive.InspectLink(tc.query, tc.subject)
			} else {
				_, err = archive.Replay(tc.query)
			}
			require.ErrorContains(t, err, tc.want)
		})
	}
}
