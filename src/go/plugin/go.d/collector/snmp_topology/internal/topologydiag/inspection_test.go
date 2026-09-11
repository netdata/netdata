// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestInspectTopologyActorIdentityRequiresOneMatch(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		{ActorID: "actor-a", Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
		{ActorID: "actor-b", Match: topologymodel.Match{IPAddresses: []string{"192.0.2.1"}}},
	}}

	got := inspectActorIdentity(data, "ip:192.0.2.1")
	require.Equal(t, inspectionUndetermined, got.membership.state)
	require.Equal(t, 2, got.membership.candidates)
	require.Len(t, got.actors, 2)
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
	subject := inspectionLinkSubject{
		srcIdentity: "ip:192.0.2.1",
		dstIdentity: "ip:192.0.2.2",
		family:      "lldp",
		protocol:    "lldp",
		direction:   "bidirectional",
	}

	got := inspectGraphLink(data, subject)
	require.Equal(t, inspectionUndetermined, got.membership.state)
	require.Equal(t, 2, got.srcActors.membership.candidates)
	require.Equal(t, 2, got.membership.candidates)
	require.Equal(t, data.Links, got.links)
	require.Equal(t, -1, got.index)
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

	subject, ok := inspectionSubjectFromLink(data, 0)
	require.True(t, ok)
	require.Contains(t, subject.srcIdentity, "\x00")

	match := inspectGraphLinkAt(data, 0)
	require.Equal(t, inspectionPresent, match.membership.state)
	require.Equal(t, 1, match.membership.candidates)
	require.Equal(t, 0, match.index)
	require.Equal(t, data.Links, match.links)
	require.Equal(t, inspectionPresent, match.srcActors.membership.state)
	require.Equal(t, inspectionPresent, match.dstActors.membership.state)
}

func TestInspectionKeepsDeviceObservationWhenAnotherDeviceFails(t *testing.T) {
	evidence := &AcquisitionAttemptEvidence{CollectedAt: time.Unix(1, 0), FreshFor: time.Minute, CollectionContexts: []AcquisitionContextEvidence{{Collection: AcquisitionPhaseEvidence{Outcome: AcquisitionPhaseSuccess}}}}
	capture := &AcquisitionCapture{State: CaptureAvailable, Evidence: evidence}
	cut := Cut{Topology: &SweepCut{CaptureState: CaptureAvailable, Devices: []SweepDevice{
		{RegistrationID: 1, Acquisition: capture, LatestAttempt: capture, Renderable: true},
		{RegistrationID: 2, Renderable: true},
	}}}
	semantics := Semantics{ReplayAcquisition: func(*AcquisitionAttemptEvidence) (topologymodel.ObservationSnapshot, bool) {
		return topologymodel.ObservationSnapshot{}, true
	}, BuildGraph: func([]topologymodel.ObservationSnapshot, string, topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
		t.Fatal("incomplete observations must not reach graph construction")
		return topologymodel.Data{}, false, nil
	}}
	report, err := semantics.inspectDevice(cut, topologyoptions.DefaultQueryOptions(), 1)
	require.NoError(t, err)
	require.Equal(t, inspectionPresent, report.observation.state)
	require.Equal(t, inspectionUndetermined, report.graphIdentity.membership.state)
	require.Equal(t, inspectionUndetermined, report.typedIdentity.state)
}

func TestGraphLinkReturnsDuplicateCandidates(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{{ActorID: "left"}, {ActorID: "right"}}}
	allocator := graph.NewActorHandleAllocator()
	for i := range data.Actors {
		data.Actors[i].ActorHandle = allocator.Next()
	}
	link := topologymodel.Link{Protocol: "lldp", Direction: "bidirectional", SrcActorHandle: data.Actors[0].ActorHandle, DstActorHandle: data.Actors[1].ActorHandle}
	data.Links = []topologymodel.Link{link, link}
	subject, ok := inspectionSubjectFromLink(data, 0)
	require.True(t, ok)
	match := inspectGraphLink(data, subject)
	require.Equal(t, inspectionUndetermined, match.membership.state)
	require.Equal(t, 2, match.membership.candidates)
	require.Equal(t, -1, match.index)
}
