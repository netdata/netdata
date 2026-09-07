// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
)

func newDiagnosticDeviceInspection(
	report deviceInspection,
) (DiagnosticDeviceInspection, error) {
	lifecycle, err := newDiagnosticLifecycleInspection(report.lifecycle)
	if err != nil {
		return DiagnosticDeviceInspection{}, fmt.Errorf("project lifecycle inspection: %w", err)
	}
	sweep, err := newDiagnosticSweepInspection(report.sweep)
	if err != nil {
		return DiagnosticDeviceInspection{}, fmt.Errorf("project sweep inspection: %w", err)
	}
	latest, err := newDiagnosticDeviceCaptureInspection(report.latestAttempt)
	if err != nil {
		return DiagnosticDeviceInspection{}, fmt.Errorf("project latest-attempt inspection: %w", err)
	}
	retained, err := newDiagnosticDeviceCaptureInspection(report.retainedSuccess)
	if err != nil {
		return DiagnosticDeviceInspection{}, fmt.Errorf("project retained-success inspection: %w", err)
	}
	aborted, err := newDiagnosticAbortedSweep(report.lastAborted)
	if err != nil {
		return DiagnosticDeviceInspection{}, fmt.Errorf("project last aborted sweep: %w", err)
	}

	result := DiagnosticDeviceInspection{
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
	report linkInspection,
) (DiagnosticLinkInspection, error) {
	cut, err := newDiagnosticCutInspection(report.diagnosticCut)
	if err != nil {
		return DiagnosticLinkInspection{}, fmt.Errorf("project diagnostic cut: %w", err)
	}
	source, err := newDiagnosticSourceInspection(report.source)
	if err != nil {
		return DiagnosticLinkInspection{}, fmt.Errorf("project source inspection: %w", err)
	}
	aborted, err := newDiagnosticAbortedSweep(report.lastAborted)
	if err != nil {
		return DiagnosticLinkInspection{}, fmt.Errorf("project last aborted sweep: %w", err)
	}
	return DiagnosticLinkInspection{
		Subject: DiagnosticLinkSubject{
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
	result inspectionLifecycleResult,
) (diagnosticLifecycleInspection, error) {
	capture, err := newDiagnosticCaptureStatus(result.captureState, result.captureReason)
	if err != nil {
		return diagnosticLifecycleInspection{}, err
	}
	converted := diagnosticLifecycleInspection{
		Membership: diagnosticStage(result.membership),
		Capture:    capture,
		Sequence:   result.sequence,
		CapturedAt: result.capturedAt,
	}
	if result.entry != nil {
		entry, err := newDiagnosticLifecycleRegistration(*result.entry)
		if err != nil {
			return diagnosticLifecycleInspection{}, err
		}
		converted.Entry = &entry
	}
	return converted, nil
}

func newDiagnosticCutInspection(
	result inspectionDiagnosticCutResult,
) (diagnosticCutInspection, error) {
	capture, err := newDiagnosticCaptureStatus(result.captureState, result.captureReason)
	if err != nil {
		return diagnosticCutInspection{}, err
	}
	return diagnosticCutInspection{
		Capture:     capture,
		Sequence:    result.sequence,
		StartedAt:   result.startedAt,
		PublishedAt: result.publishedAt,
	}, nil
}

func newDiagnosticSweepInspection(
	result inspectionSweepResult,
) (diagnosticSweepInspection, error) {
	cut, err := newDiagnosticCutInspection(result.inspectionDiagnosticCutResult)
	if err != nil {
		return diagnosticSweepInspection{}, err
	}
	converted := diagnosticSweepInspection{
		diagnosticCutInspection: cut,
		Membership:              diagnosticStage(result.membership),
	}
	if result.device != nil {
		device, err := newDiagnosticSweepRegistration(result.device)
		if err != nil {
			return diagnosticSweepInspection{}, err
		}
		converted.Device = &device
	}
	return converted, nil
}

func newDiagnosticRemovedInspection(
	result inspectionRemovedResult,
) diagnosticRemovedInspection {
	converted := diagnosticRemovedInspection{
		Membership: diagnosticStage(result.membership),
	}
	if result.device != nil {
		device := newDiagnosticRemovedRegistration(result.device)
		converted.Device = &device
	}
	return converted
}

func newDiagnosticCaptureInspection(
	result inspectionCaptureResult,
) (diagnosticCaptureInspection, error) {
	capture, err := newDiagnosticCaptureSummary(result.capture)
	if err != nil {
		return diagnosticCaptureInspection{}, err
	}
	return diagnosticCaptureInspection{
		Membership: diagnosticStage(result.membership),
		Evidence:   diagnosticStage(result.evidence),
		Capture:    capture,
	}, nil
}

func newDiagnosticDeviceCaptureInspection(result inspectionCaptureResult) (diagnosticDeviceCaptureInspection, error) {
	capture, err := newDiagnosticCaptureInspection(result)
	if err != nil {
		return diagnosticDeviceCaptureInspection{}, err
	}
	converted := diagnosticDeviceCaptureInspection{diagnosticCaptureInspection: capture}
	if result.capture == nil || result.capture.Evidence == nil {
		return converted, nil
	}
	for _, context := range result.capture.Evidence.CollectionContexts {
		client, err := newDiagnosticPhaseStatus(context.Client)
		if err != nil {
			return diagnosticDeviceCaptureInspection{}, err
		}
		connect, err := newDiagnosticPhaseStatus(context.Connect)
		if err != nil {
			return diagnosticDeviceCaptureInspection{}, err
		}
		collection, err := newDiagnosticPhaseStatus(context.Collection)
		if err != nil {
			return diagnosticDeviceCaptureInspection{}, err
		}
		accounting := diagnosticContextAccounting{
			Sources:      context.Sources,
			Interruption: context.Interruption, Failures: context.Failures,
			Client: client, Connect: connect, Collection: collection,
			Ordinal: context.Ordinal, VLANID: context.VLANID, VLANName: context.VLANName,
			Profiles: make([]diagnosticProfileAccounting, 0, len(context.Profiles)),
		}
		for _, profile := range context.Profiles {
			outcome, err := archiveProfileOutcomeName(profile.Outcome)
			if err != nil {
				return diagnosticDeviceCaptureInspection{}, err
			}
			phase, err := archiveProfileFailurePhaseName(profile.FailurePhase)
			if err != nil {
				return diagnosticDeviceCaptureInspection{}, err
			}
			routes := make([]snmpdiag.Route, 0, len(profile.Routes))
			for _, route := range profile.Routes {
				wire, err := newArchiveRouteV1(route)
				if err != nil {
					return diagnosticDeviceCaptureInspection{}, err
				}
				routes = append(routes, wire)
			}
			accounting.Profiles = append(accounting.Profiles, diagnosticProfileAccounting{
				Routes: routes,
				Identity: snmpdiag.ProfileIdentity{
					Ordinal: profile.Identity.Ordinal, RouteDigest: hex.EncodeToString(profile.Identity.RouteDigest[:]),
				},
				Outcome: outcome, FailurePhase: phase,
				Stats:     newArchiveCollectionStatsV1(profile.Stats),
				Execution: newArchiveExecutionV1(profile.Execution),
			})
		}
		converted.CollectionContexts = append(converted.CollectionContexts, accounting)
	}
	return converted, nil
}

func newDiagnosticActorInspection(
	result inspectionActorResult,
) diagnosticActorInspection {
	converted := diagnosticActorInspection{
		Membership:    diagnosticStage(result.membership),
		SelectedIndex: result.index,
		Candidates:    make([]diagnosticGraphActor, 0, len(result.actors)),
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

func newDiagnosticGraphActor(index int, actor topologymodel.Actor) diagnosticGraphActor {
	converted := diagnosticGraphActor{
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

func newDiagnosticActorDetails(actor topologymodel.Actor) (diagnosticActorDetails, bool) {
	arrayLabels := topologymodel.ActorDetailArrayLabelValues(actor)
	details := diagnosticActorDetails{
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

func newDiagnosticRowInspection(result inspectionRowResult) diagnosticRowInspection {
	return diagnosticRowInspection{
		Membership: diagnosticStage(result.inspectionStage),
		Row:        result.row,
	}
}

func newDiagnosticSourceInspection(
	result inspectionSourceResult,
) (diagnosticSourceInspection, error) {
	converted := diagnosticSourceInspection{
		Contexts: make([]diagnosticSourceContext, 0, len(result.contexts)),
	}
	for i := range result.contexts {
		context, err := newDiagnosticSourceContext(result.contexts[i])
		if err != nil {
			return diagnosticSourceInspection{}, err
		}
		converted.Contexts = append(converted.Contexts, context)
	}
	return converted, nil
}

func newDiagnosticSourceContext(
	result inspectionSourceContext,
) (diagnosticSourceContext, error) {
	latest, err := newDiagnosticCaptureInspection(result.latestAttempt)
	if err != nil {
		return diagnosticSourceContext{}, err
	}
	retained, err := newDiagnosticCaptureInspection(result.retainedSuccess)
	if err != nil {
		return diagnosticSourceContext{}, err
	}
	converted := diagnosticSourceContext{
		RegistrationID:  uint64(result.registrationID),
		LatestAttempt:   latest,
		RetainedSuccess: retained,
		SameAttempt:     result.sameAttempt,
		Captures:        make([]diagnosticSourceCaptureContext, 0, len(result.captures)),
	}
	for i := range result.captures {
		capture, err := newDiagnosticSourceCaptureContext(result.captures[i])
		if err != nil {
			return diagnosticSourceContext{}, err
		}
		converted.Captures = append(converted.Captures, capture)
	}
	return converted, nil
}

func newDiagnosticSourceCaptureContext(
	result inspectionSourceCaptureContext,
) (diagnosticSourceCaptureContext, error) {
	capture, err := newDiagnosticCaptureInspection(result.capture)
	if err != nil {
		return diagnosticSourceCaptureContext{}, err
	}
	converted := diagnosticSourceCaptureContext{
		LatestAttempt:   result.latestAttempt,
		RetainedSuccess: result.retainedSuccess,
		Capture:         capture,
		Facts:           make([]diagnosticSourceFact, 0, len(result.facts)),
	}
	for i := range result.facts {
		converted.Facts = append(converted.Facts, newDiagnosticSourceFact(result.facts[i]))
	}
	return converted, nil
}

func newDiagnosticSourceFact(result inspectionSourceFact) diagnosticSourceFact {
	converted := diagnosticSourceFact{
		RegistrationID: uint64(result.registrationID),
		ContextOrdinal: result.contextOrdinal,
		ProfileOrdinal: result.profileOrdinal,
	}
	if result.metric != nil {
		converted.Metric = &diagnosticMetricFact{
			RowIndex:     result.metric.RowIndex,
			Field:        result.metric.Field,
			RouteOrdinal: result.metric.RouteOrdinal,
			RowOrdinal:   result.metric.RowOrdinal,
			ValueOrdinal: result.metric.ValueOrdinal,
			Kind:         string(result.metric.Kind),
			Tags:         cloneStringMap(result.metric.Tags),
		}
	}
	if result.bgp != nil {
		converted.BGP = newDiagnosticBGPFact(result.bgp)
	}
	return converted
}

func newDiagnosticBGPFact(row *AcquisitionBGPRowValue) *diagnosticBGPFact {
	if row == nil {
		return nil
	}
	return &diagnosticBGPFact{
		RouteOrdinal:    row.RouteOrdinal,
		RowOrdinal:      row.RowOrdinal,
		ValueOrdinal:    row.ValueOrdinal,
		OriginProfileID: row.OriginProfileID,
		Table:           row.Table,
		RowKey:          row.RowKey,
		StructuralID:    row.StructuralID,
		Kind:            string(row.Kind),
		RoutingInstance: row.RoutingInstance,
		Neighbor:        row.Neighbor,
		RemoteAS:        row.RemoteAS,
		LocalAddress:    row.LocalAddress,
		LocalAS:         row.LocalAS,
		LocalIdentifier: row.LocalIdentifier,
		PeerIdentifier:  row.PeerIdentifier,
		PeerType:        row.PeerType,
		BGPVersion:      row.BGPVersion,
		Description:     row.Description,
		AdminHas:        row.AdminHas,
		AdminEnabled:    row.AdminEnabled,
		StateHas:        row.StateHas,
		State:           string(row.State),
		StateRaw:        row.StateRaw,
		EstablishedHas:  row.EstablishedHas,
		Established:     row.Established,
		UpdateAgeHas:    row.UpdateAgeHas,
		UpdateAge:       row.UpdateAge,
		Tags:            cloneStringMap(row.Tags),
	}
}

func newDiagnosticGraphLinkInspection(
	result inspectionGraphLinkResult,
) diagnosticGraphLinkInspection {
	converted := diagnosticGraphLinkInspection{
		Membership:        diagnosticStage(result.membership),
		SourceActors:      newDiagnosticActorInspection(result.srcActors),
		DestinationActors: newDiagnosticActorInspection(result.dstActors),
		SelectedIndex:     result.index,
		Candidates:        make([]diagnosticGraphLink, 0, len(result.links)),
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
) diagnosticGraphLink {
	converted := diagnosticGraphLink{
		Family:             inspectionLinkFamily(link),
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

func newDiagnosticLinkEndpoint(endpoint topologymodel.LinkEndpoint) diagnosticLinkEndpoint {
	endpoint.Match = cloneDiagnosticMatch(endpoint.Match)
	return endpoint
}

func newDiagnosticLinkDetail(detail topologymodel.LinkDetail) *diagnosticLinkDetail {
	if detail.L3Subnet == nil && detail.L3SubnetMembership == nil && detail.OSPF == nil && detail.BGP == nil {
		return nil
	}
	converted := &diagnosticLinkDetail{}
	if detail.L3Subnet != nil {
		converted.L3Subnet = &diagnosticL3SubnetLinkDetail{
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
		membership := &diagnosticL3SubnetMembershipLinkDetail{
			Source:  detail.L3SubnetMembership.Source,
			Subnet:  detail.L3SubnetMembership.Subnet,
			Network: detail.L3SubnetMembership.Network,
			Netmask: detail.L3SubnetMembership.Netmask,
			Prefix:  detail.L3SubnetMembership.Prefix,
		}
		for _, iface := range detail.L3SubnetMembership.Interfaces {
			membership.Interfaces = append(membership.Interfaces, diagnosticL3SubnetMembershipInterface{
				MemberIP: iface.MemberIP,
				IfIndex:  iface.IfIndex,
				IfName:   iface.IfName,
				IfDescr:  iface.IfDescr,
			})
		}
		converted.L3SubnetMembership = membership
	}
	if detail.OSPF != nil {
		converted.OSPF = &diagnosticOSPFAdjacencyLinkDetail{
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
		converted.BGP = &diagnosticBGPAdjacencyLinkDetail{
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
