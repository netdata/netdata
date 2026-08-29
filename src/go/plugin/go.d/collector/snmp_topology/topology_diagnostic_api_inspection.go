// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/snmptopologydiagnostics"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func newDiagnosticDeviceInspection(
	report topologyDeviceInspection,
) (snmptopologydiagnostics.DeviceInspection, error) {
	lifecycle, err := newDiagnosticLifecycleInspection(report.lifecycle)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, fmt.Errorf("project lifecycle inspection: %w", err)
	}
	sweep, err := newDiagnosticSweepInspection(report.sweep)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, fmt.Errorf("project sweep inspection: %w", err)
	}
	latest, err := newDiagnosticCaptureInspection(report.latestAttempt)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, fmt.Errorf("project latest-attempt inspection: %w", err)
	}
	retained, err := newDiagnosticCaptureInspection(report.retainedSuccess)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, fmt.Errorf("project retained-success inspection: %w", err)
	}
	aborted, err := newDiagnosticAbortedSweep(report.lastAborted)
	if err != nil {
		return snmptopologydiagnostics.DeviceInspection{}, fmt.Errorf("project last aborted sweep: %w", err)
	}

	result := snmptopologydiagnostics.DeviceInspection{
		RegistrationID:  uint64(report.registrationID),
		Query:           diagnosticQueryOptionsFromInternal(report.options),
		Lifecycle:       lifecycle,
		Sweep:           sweep,
		Removed:         newDiagnosticRemovedInspection(report.removed),
		LatestAttempt:   latest,
		RetainedSuccess: retained,
		SameAttempt:     report.sameAttempt,
		Observation:     diagnosticStage(report.observation),
		GraphIdentity:   newDiagnosticActorInspection(report.graphIdentity),
		TypedIdentity:   newDiagnosticRowInspection(report.typedIdentity),
		LastAborted:     aborted,
	}
	if report.hasGraphStats {
		result.GraphStats = topologyv1renderer.RenderStats(report.graphStats)
	}
	return result, nil
}

func newDiagnosticLinkInspection(
	report topologyLinkInspection,
) (snmptopologydiagnostics.LinkInspection, error) {
	cut, err := newDiagnosticCutInspection(report.diagnosticCut)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, fmt.Errorf("project diagnostic cut: %w", err)
	}
	source, err := newDiagnosticSourceInspection(report.source)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, fmt.Errorf("project source inspection: %w", err)
	}
	aborted, err := newDiagnosticAbortedSweep(report.lastAborted)
	if err != nil {
		return snmptopologydiagnostics.LinkInspection{}, fmt.Errorf("project last aborted sweep: %w", err)
	}
	return snmptopologydiagnostics.LinkInspection{
		Subject: snmptopologydiagnostics.LinkSubject{
			SourceIdentity:      report.subject.srcIdentity,
			DestinationIdentity: report.subject.dstIdentity,
			Family:              report.subject.family,
			Protocol:            report.subject.protocol,
			Direction:           report.subject.direction,
		},
		Query:         diagnosticQueryOptionsFromInternal(report.options),
		DiagnosticCut: cut,
		Source:        source,
		GraphLink:     newDiagnosticGraphLinkInspection(report.graphLink),
		TypedLink:     newDiagnosticRowInspection(report.typedLink),
		GraphStats:    diagnosticStage(report.graphStats),
		Stats:         topologyv1renderer.RenderStats(report.stats),
		LastAborted:   aborted,
	}, nil
}

func newDiagnosticLifecycleInspection(
	result topologyInspectionLifecycleResult,
) (snmptopologydiagnostics.LifecycleInspection, error) {
	capture, err := newDiagnosticCaptureStatus(result.captureState, result.captureReason)
	if err != nil {
		return snmptopologydiagnostics.LifecycleInspection{}, err
	}
	converted := snmptopologydiagnostics.LifecycleInspection{
		Membership: diagnosticStage(result.membership),
		Capture:    capture,
		Sequence:   result.sequence,
		CapturedAt: result.capturedAt,
	}
	if result.entry != nil {
		entry, err := newDiagnosticLifecycleRegistration(*result.entry)
		if err != nil {
			return snmptopologydiagnostics.LifecycleInspection{}, err
		}
		converted.Entry = &entry
	}
	return converted, nil
}

func newDiagnosticCutInspection(
	result topologyInspectionDiagnosticCutResult,
) (snmptopologydiagnostics.DiagnosticCutInspection, error) {
	capture, err := newDiagnosticCaptureStatus(result.captureState, result.captureReason)
	if err != nil {
		return snmptopologydiagnostics.DiagnosticCutInspection{}, err
	}
	return snmptopologydiagnostics.DiagnosticCutInspection{
		Capture:     capture,
		Sequence:    result.sequence,
		StartedAt:   result.startedAt,
		PublishedAt: result.publishedAt,
	}, nil
}

func newDiagnosticSweepInspection(
	result topologyInspectionSweepResult,
) (snmptopologydiagnostics.SweepInspection, error) {
	cut, err := newDiagnosticCutInspection(result.topologyInspectionDiagnosticCutResult)
	if err != nil {
		return snmptopologydiagnostics.SweepInspection{}, err
	}
	converted := snmptopologydiagnostics.SweepInspection{
		DiagnosticCutInspection: cut,
		Membership:              diagnosticStage(result.membership),
	}
	if result.device != nil {
		device, err := newDiagnosticSweepRegistration(result.device)
		if err != nil {
			return snmptopologydiagnostics.SweepInspection{}, err
		}
		converted.Device = &device
	}
	return converted, nil
}

func newDiagnosticRemovedInspection(
	result topologyInspectionRemovedResult,
) snmptopologydiagnostics.RemovedInspection {
	converted := snmptopologydiagnostics.RemovedInspection{
		Membership: diagnosticStage(result.membership),
	}
	if result.device != nil {
		device := newDiagnosticRemovedRegistration(result.device)
		converted.Device = &device
	}
	return converted
}

func newDiagnosticCaptureInspection(
	result topologyInspectionCaptureResult,
) (snmptopologydiagnostics.CaptureInspection, error) {
	capture, err := newDiagnosticCaptureSummary(result.capture)
	if err != nil {
		return snmptopologydiagnostics.CaptureInspection{}, err
	}
	return snmptopologydiagnostics.CaptureInspection{
		Membership: diagnosticStage(result.membership),
		Evidence:   diagnosticStage(result.evidence),
		Capture:    capture,
	}, nil
}

func newDiagnosticActorInspection(
	result topologyInspectionActorResult,
) snmptopologydiagnostics.ActorInspection {
	converted := snmptopologydiagnostics.ActorInspection{
		Membership:    diagnosticStage(result.membership),
		SelectedIndex: result.index,
		Candidates:    make([]snmptopologydiagnostics.GraphActor, 0, len(result.actors)),
	}
	for i := range result.actors {
		index := -1
		if i < len(result.indexes) {
			index = result.indexes[i]
		}
		converted.Candidates = append(converted.Candidates, newDiagnosticGraphActor(index, result.actors[i]))
	}
	return converted
}

func newDiagnosticGraphActor(index int, actor topologymodel.Actor) snmptopologydiagnostics.GraphActor {
	converted := snmptopologydiagnostics.GraphActor{
		Index:        index,
		ActorID:      actor.ActorID,
		ActorType:    actor.ActorType,
		SegmentKind:  actor.SegmentKind,
		Layer:        actor.Layer,
		Source:       actor.Source,
		IdentityKeys: topologymodel.MatchIdentityKeys(actor.Match),
		Match:        cloneDiagnosticMatch(actor.Match),
		Labels:       cloneStringMap(actor.Labels),
	}
	if actor.ParentMatch != nil {
		match := cloneDiagnosticMatch(*actor.ParentMatch)
		converted.ParentMatch = &match
	}
	if details, ok := newDiagnosticActorDetails(actor); ok {
		converted.Details = &details
	}
	return converted
}

func newDiagnosticActorDetails(actor topologymodel.Actor) (snmptopologydiagnostics.ActorDetails, bool) {
	arrayLabels := topologymodel.ActorDetailArrayLabelValues(actor)
	details := snmptopologydiagnostics.ActorDetails{
		DisplayName:           topologymodel.ActorDetailDisplayName(actor),
		DisplaySource:         topologymodel.ActorDetailDisplaySource(actor),
		ParentDevices:         slices.Clone(topologymodel.ActorDetailParentDevices(actor)),
		ManagementIP:          topologymodel.ActorDetailManagementIP(actor),
		ManagementAddresses:   topologymodel.ActorDetailManagementIPs(actor),
		Protocols:             slices.Clone(topologymodel.ActorDetailProtocols(actor)),
		Capabilities:          slices.Clone(topologymodel.ActorDetailCapabilities(actor)),
		CapabilitiesSupported: slices.Clone(arrayLabels["capabilities_supported"]),
		CapabilitiesEnabled:   slices.Clone(arrayLabels["capabilities_enabled"]),
		SysDescr:              topologymodel.ActorDetailSysDescr(actor),
		SysContact:            topologymodel.ActorDetailSysContact(actor),
		SysLocation:           topologymodel.ActorDetailSysLocation(actor),
		Vendor:                topologymodel.ActorDetailVendor(actor),
		Model:                 topologymodel.ActorDetailModel(actor),
		OSPFRouterID:          topologymodel.ActorDetailOSPFRouterID(actor),
	}
	present := details.DisplayName != "" || details.DisplaySource != "" || len(details.ParentDevices) > 0 ||
		details.ManagementIP != "" || len(details.ManagementAddresses) > 0 || len(details.Protocols) > 0 ||
		len(details.Capabilities) > 0 || len(details.CapabilitiesSupported) > 0 ||
		len(details.CapabilitiesEnabled) > 0 || details.SysDescr != "" || details.SysContact != "" ||
		details.SysLocation != "" || details.Vendor != "" || details.Model != "" ||
		details.OSPFRouterID != ""
	return details, present
}

func newDiagnosticRowInspection(result topologyInspectionRowResult) snmptopologydiagnostics.RowInspection {
	return snmptopologydiagnostics.RowInspection{
		Membership: diagnosticStage(topologyInspectionStage{state: result.state}),
		Row:        result.row,
	}
}

func newDiagnosticSourceInspection(
	result topologyInspectionSourceResult,
) (snmptopologydiagnostics.SourceInspection, error) {
	converted := snmptopologydiagnostics.SourceInspection{
		Contexts: make([]snmptopologydiagnostics.SourceContext, 0, len(result.contexts)),
	}
	for i := range result.contexts {
		context, err := newDiagnosticSourceContext(result.contexts[i])
		if err != nil {
			return snmptopologydiagnostics.SourceInspection{}, err
		}
		converted.Contexts = append(converted.Contexts, context)
	}
	return converted, nil
}

func newDiagnosticSourceContext(
	result topologyInspectionSourceContext,
) (snmptopologydiagnostics.SourceContext, error) {
	latest, err := newDiagnosticCaptureInspection(result.latestAttempt)
	if err != nil {
		return snmptopologydiagnostics.SourceContext{}, err
	}
	retained, err := newDiagnosticCaptureInspection(result.retainedSuccess)
	if err != nil {
		return snmptopologydiagnostics.SourceContext{}, err
	}
	converted := snmptopologydiagnostics.SourceContext{
		RegistrationID:  uint64(result.registrationID),
		LatestAttempt:   latest,
		RetainedSuccess: retained,
		SameAttempt:     result.sameAttempt,
		Captures:        make([]snmptopologydiagnostics.SourceCaptureContext, 0, len(result.captures)),
	}
	for i := range result.captures {
		capture, err := newDiagnosticSourceCaptureContext(result.captures[i])
		if err != nil {
			return snmptopologydiagnostics.SourceContext{}, err
		}
		converted.Captures = append(converted.Captures, capture)
	}
	return converted, nil
}

func newDiagnosticSourceCaptureContext(
	result topologyInspectionSourceCaptureContext,
) (snmptopologydiagnostics.SourceCaptureContext, error) {
	capture, err := newDiagnosticCaptureInspection(result.capture)
	if err != nil {
		return snmptopologydiagnostics.SourceCaptureContext{}, err
	}
	converted := snmptopologydiagnostics.SourceCaptureContext{
		LatestAttempt:   result.latestAttempt,
		RetainedSuccess: result.retainedSuccess,
		Capture:         capture,
		Facts:           make([]snmptopologydiagnostics.SourceFact, 0, len(result.facts)),
	}
	for i := range result.facts {
		converted.Facts = append(converted.Facts, newDiagnosticSourceFact(result.facts[i]))
	}
	return converted, nil
}

func newDiagnosticSourceFact(result topologyInspectionSourceFact) snmptopologydiagnostics.SourceFact {
	converted := snmptopologydiagnostics.SourceFact{
		RegistrationID: uint64(result.registrationID),
		ContextOrdinal: result.contextOrdinal,
		ProfileOrdinal: result.profileOrdinal,
	}
	if result.metric != nil {
		converted.Metric = &snmptopologydiagnostics.MetricFact{
			RouteOrdinal: result.metric.routeOrdinal,
			RowOrdinal:   result.metric.rowOrdinal,
			ValueOrdinal: result.metric.valueOrdinal,
			Kind:         string(result.metric.kind),
			Tags:         cloneStringMap(result.metric.tags),
		}
	}
	if result.bgp != nil {
		converted.BGP = newDiagnosticBGPFact(result.bgp)
	}
	return converted
}

func newDiagnosticBGPFact(row *topologyAcquisitionBGPRowValue) *snmptopologydiagnostics.BGPFact {
	if row == nil {
		return nil
	}
	return &snmptopologydiagnostics.BGPFact{
		RouteOrdinal:    row.routeOrdinal,
		RowOrdinal:      row.rowOrdinal,
		ValueOrdinal:    row.valueOrdinal,
		OriginProfileID: row.originProfileID,
		Table:           row.table,
		RowKey:          row.rowKey,
		StructuralID:    row.structuralID,
		Kind:            string(row.kind),
		RoutingInstance: row.routingInstance,
		Neighbor:        row.neighbor,
		RemoteAS:        row.remoteAS,
		LocalAddress:    row.localAddress,
		LocalAS:         row.localAS,
		LocalIdentifier: row.localIdentifier,
		PeerIdentifier:  row.peerIdentifier,
		PeerType:        row.peerType,
		BGPVersion:      row.bgpVersion,
		Description:     row.description,
		AdminHas:        row.adminHas,
		AdminEnabled:    row.adminEnabled,
		StateHas:        row.stateHas,
		State:           string(row.state),
		StateRaw:        row.stateRaw,
		EstablishedHas:  row.establishedHas,
		Established:     row.established,
		UpdateAgeHas:    row.updateAgeHas,
		UpdateAge:       row.updateAge,
		Tags:            cloneStringMap(row.tags),
	}
}

func newDiagnosticGraphLinkInspection(
	result topologyInspectionGraphLinkResult,
) snmptopologydiagnostics.GraphLinkInspection {
	converted := snmptopologydiagnostics.GraphLinkInspection{
		Membership:        diagnosticStage(result.membership),
		SourceActors:      newDiagnosticActorInspection(result.srcActors),
		DestinationActors: newDiagnosticActorInspection(result.dstActors),
		SelectedIndex:     result.index,
		Candidates:        make([]snmptopologydiagnostics.GraphLink, 0, len(result.links)),
	}
	actorIDs := make(map[topologymodel.ActorHandle]string, len(result.srcActors.actors)+len(result.dstActors.actors))
	for _, actor := range result.srcActors.actors {
		actorIDs[actor.ActorHandle] = actor.ActorID
	}
	for _, actor := range result.dstActors.actors {
		actorIDs[actor.ActorHandle] = actor.ActorID
	}
	for i := range result.links {
		converted.Candidates = append(converted.Candidates, newDiagnosticGraphLink(result.links[i], actorIDs))
	}
	return converted
}

func newDiagnosticGraphLink(
	link topologymodel.Link,
	actorIDs map[topologymodel.ActorHandle]string,
) snmptopologydiagnostics.GraphLink {
	converted := snmptopologydiagnostics.GraphLink{
		Family:             topologyInspectionLinkFamily(link),
		Layer:              link.Layer,
		Protocol:           link.Protocol,
		Direction:          link.Direction,
		State:              link.State,
		SourceActorID:      actorIDs[link.SrcActorHandle],
		DestinationActorID: actorIDs[link.DstActorHandle],
		Source:             newDiagnosticLinkEndpoint(link.Src),
		Destination:        newDiagnosticLinkEndpoint(link.Dst),
		DiscoveredAt:       link.DiscoveredAt,
		LastSeen:           link.LastSeen,
		Detail:             newDiagnosticLinkDetail(link.Detail),
	}
	if link.Display != nil {
		display := *link.Display
		converted.Display = &display
	}
	if link.L2 != nil {
		l2 := *link.L2
		converted.L2 = &l2
	}
	if link.Inference != nil {
		inference := *link.Inference
		converted.Inference = &inference
	}
	return converted
}

func newDiagnosticLinkEndpoint(endpoint topologymodel.LinkEndpoint) snmptopologydiagnostics.LinkEndpoint {
	endpoint.Match = cloneDiagnosticMatch(endpoint.Match)
	return endpoint
}

func newDiagnosticLinkDetail(detail topologymodel.LinkDetail) *snmptopologydiagnostics.LinkDetail {
	if detail.L3Subnet == nil && detail.L3SubnetMembership == nil && detail.OSPF == nil && detail.BGP == nil {
		return nil
	}
	converted := &snmptopologydiagnostics.LinkDetail{}
	if detail.L3Subnet != nil {
		converted.L3Subnet = &snmptopologydiagnostics.L3SubnetLinkDetail{
			Source:  detail.L3Subnet.Source,
			SrcIP:   detail.L3Subnet.SrcIP,
			DstIP:   detail.L3Subnet.DstIP,
			Subnet:  detail.L3Subnet.Subnet,
			Network: detail.L3Subnet.Network,
			Netmask: detail.L3Subnet.Netmask,
			Prefix:  detail.L3Subnet.Prefix,
		}
	}
	if detail.L3SubnetMembership != nil {
		membership := &snmptopologydiagnostics.L3SubnetMembershipLinkDetail{
			Source:  detail.L3SubnetMembership.Source,
			Subnet:  detail.L3SubnetMembership.Subnet,
			Network: detail.L3SubnetMembership.Network,
			Netmask: detail.L3SubnetMembership.Netmask,
			Prefix:  detail.L3SubnetMembership.Prefix,
		}
		for _, iface := range detail.L3SubnetMembership.Interfaces {
			membership.Interfaces = append(membership.Interfaces, snmptopologydiagnostics.L3SubnetMembershipInterface{
				MemberIP: iface.MemberIP,
				IfIndex:  iface.IfIndex,
				IfName:   iface.IfName,
				IfDescr:  iface.IfDescr,
			})
		}
		converted.L3SubnetMembership = membership
	}
	if detail.OSPF != nil {
		converted.OSPF = &snmptopologydiagnostics.OSPFAdjacencyLinkDetail{
			Source:           detail.OSPF.Source,
			LocalRouterID:    detail.OSPF.LocalRouterID,
			NeighborRouterID: detail.OSPF.NeighborRouterID,
			LocalIP:          detail.OSPF.LocalIP,
			NeighborIP:       detail.OSPF.NeighborIP,
			AddresslessIndex: detail.OSPF.AddresslessIndex,
			Subnet:           detail.OSPF.Subnet,
			Network:          detail.OSPF.Network,
			Netmask:          detail.OSPF.Netmask,
			Prefix:           detail.OSPF.Prefix,
		}
	}
	if detail.BGP != nil {
		converted.BGP = &snmptopologydiagnostics.BGPAdjacencyLinkDetail{
			Source:          detail.BGP.Source,
			RoutingInstance: detail.BGP.RoutingInstance,
			LocalIP:         detail.BGP.LocalIP,
			NeighborIP:      detail.BGP.NeighborIP,
			LocalAS:         detail.BGP.LocalAS,
			RemoteAS:        detail.BGP.RemoteAS,
			LocalIdentifier: detail.BGP.LocalIdentifier,
			PeerIdentifier:  detail.BGP.PeerIdentifier,
		}
	}
	return converted
}

func cloneDiagnosticMatch(match graph.Match) graph.Match {
	match.ChassisIDs = cloneStrings(match.ChassisIDs)
	match.MacAddresses = cloneStrings(match.MacAddresses)
	match.IPAddresses = cloneStrings(match.IPAddresses)
	match.Hostnames = cloneStrings(match.Hostnames)
	match.DNSNames = cloneStrings(match.DNSNames)
	match.ContainerIDs = cloneStrings(match.ContainerIDs)
	match.PodNames = cloneStrings(match.PodNames)
	match.NamespaceIDs = cloneStrings(match.NamespaceIDs)
	return match
}
