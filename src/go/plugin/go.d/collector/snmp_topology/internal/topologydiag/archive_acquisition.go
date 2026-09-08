// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

func newArchiveAcquisitionEvidenceV1(
	evidence *AcquisitionAttemptEvidence,
) (snmpdiag.AcquisitionEvidence, error) {
	targetOutcome, err := archiveTargetOutcomeName(evidence.Target.Outcome)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, err
	}
	client, err := newArchivePhaseV1(evidence.Client)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newArchivePhaseV1(evidence.Connect)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := newArchivePhaseV1(evidence.Profiles)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := newArchivePhaseV1(evidence.Collection)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := newArchivePhaseV1(evidence.SysUptime)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := newArchivePhaseV1(evidence.VLANProfiles)
	if err != nil {
		return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("vlan_profiles phase: %w", err)
	}
	result := snmpdiag.AcquisitionEvidence{
		Interruption:       evidence.Interruption,
		ProfileContext:     evidence.ProfileContext.Snapshot(),
		VLANProfileContext: evidence.VLANProfileContext.Snapshot(),
		Device: snmpdiag.DeviceInput{
			Hostname:    evidence.Device.Hostname,
			SysObjectID: evidence.Device.SysObjectID,
			SysName:     evidence.Device.SysName,
			SysDescr:    evidence.Device.SysDescr,
			SysContact:  evidence.Device.SysContact,
			SysLocation: evidence.Device.SysLocation,
			Vendor:      evidence.Device.Vendor,
			Model:       evidence.Device.Model,
			VnodeGUID:   evidence.Device.VnodeGUID,
			VnodeLabels: evidence.Device.VnodeLabels,
		},
		Target: snmpdiag.Target{
			Outcome:   targetOutcome,
			Addresses: make([]string, 0, len(evidence.Target.Addresses)),
		},
		Client:             client,
		Connect:            connect,
		Profiles:           profiles,
		Collection:         collection,
		SysUptime:          sysUptime,
		VLANProfiles:       vlanProfiles,
		CollectedAt:        evidence.CollectedAt,
		FreshForNanos:      int64(evidence.FreshFor),
		SysUptimeValue:     evidence.SysUptimeValue,
		CollectionContexts: make([]snmpdiag.ContextEvidence, 0, len(evidence.CollectionContexts)),
	}
	for _, address := range evidence.Target.Addresses {
		result.Target.Addresses = append(result.Target.Addresses, address.String())
	}
	for _, context := range evidence.CollectionContexts {
		archived, err := newArchiveContextEvidenceV1(context)
		if err != nil {
			return snmpdiag.AcquisitionEvidence{}, fmt.Errorf("collection context %d: %w", context.Ordinal, err)
		}
		result.CollectionContexts = append(result.CollectionContexts, archived)
	}
	return result, nil
}

func newArchiveContextEvidenceV1(
	context AcquisitionContextEvidence,
) (snmpdiag.ContextEvidence, error) {
	client, err := newArchivePhaseV1(context.Client)
	if err != nil {
		return snmpdiag.ContextEvidence{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newArchivePhaseV1(context.Connect)
	if err != nil {
		return snmpdiag.ContextEvidence{}, fmt.Errorf("connect phase: %w", err)
	}
	collection, err := newArchivePhaseV1(context.Collection)
	if err != nil {
		return snmpdiag.ContextEvidence{}, fmt.Errorf("collection phase: %w", err)
	}
	result := snmpdiag.ContextEvidence{
		Sources:      context.Sources,
		Interruption: context.Interruption,
		Failures:     context.Failures,
		Ordinal:      context.Ordinal,
		VLANID:       context.VLANID,
		VLANName:     context.VLANName,
		Client:       client,
		Connect:      connect,
		Collection:   collection,
		Profiles:     make([]snmpdiag.ProfileEvidence, 0, len(context.Profiles)),
	}
	for _, profile := range context.Profiles {
		archived, err := newArchiveProfileEvidenceV1(profile)
		if err != nil {
			return snmpdiag.ContextEvidence{}, fmt.Errorf("profile %d: %w", profile.Identity.Ordinal, err)
		}
		result.Profiles = append(result.Profiles, archived)
	}
	return result, nil
}

func newArchiveProfileEvidenceV1(
	profile AcquisitionProfileEvidence,
) (snmpdiag.ProfileEvidence, error) {
	outcome, err := archiveProfileOutcomeName(profile.Outcome)
	if err != nil {
		return snmpdiag.ProfileEvidence{}, err
	}
	failurePhase, err := archiveProfileFailurePhaseName(profile.FailurePhase)
	if err != nil {
		return snmpdiag.ProfileEvidence{}, err
	}
	result := snmpdiag.ProfileEvidence{
		Identity: snmpdiag.ProfileIdentity{
			Ordinal:     profile.Identity.Ordinal,
			RouteDigest: hex.EncodeToString(profile.Identity.RouteDigest[:]),
		},
		Outcome:      outcome,
		FailurePhase: failurePhase,
		Stats:        newArchiveCollectionStatsV1(profile.Stats),
		Execution:    newArchiveExecutionV1(profile.Execution),
		Routes:       make([]snmpdiag.Route, 0, len(profile.Routes)),
		Values:       newArchiveProfileValuesV1(profile.Values),
	}
	for _, route := range profile.Routes {
		archived, err := newArchiveRouteV1(route)
		if err != nil {
			return snmpdiag.ProfileEvidence{}, fmt.Errorf("route %d: %w", route.Ordinal, err)
		}
		result.Routes = append(result.Routes, archived)
	}
	return result, nil
}

func newArchiveRouteV1(
	route ddsnmpcollector.AcquisitionRouteReport,
) (snmpdiag.Route, error) {
	kind, err := archiveRouteKindName(route.Kind)
	if err != nil {
		return snmpdiag.Route{}, err
	}
	source, err := archiveRouteSourceName(route.Source)
	if err != nil {
		return snmpdiag.Route{}, err
	}
	outcome, err := archiveRouteOutcomeName(route.Outcome)
	if err != nil {
		return snmpdiag.Route{}, err
	}
	failureClass, err := archiveRouteFailureClassName(route.FailureClass)
	if err != nil {
		return snmpdiag.Route{}, err
	}
	return snmpdiag.Route{
		Sources: route.Sources, Processing: route.Processing,
		Ordinal:      route.Ordinal,
		Kind:         kind,
		RootOID:      route.RootOID,
		Source:       source,
		Outcome:      outcome,
		FailureClass: failureClass,
		Rows:         route.Rows,
		Values:       route.Values,
		Missing:      route.Missing,
		Rejected:     route.Rejected,
	}, nil
}

func newArchiveProfileValuesV1(
	values AcquisitionProfileValues,
) snmpdiag.ProfileValues {
	result := snmpdiag.ProfileValues{
		Tags:      values.Tags,
		Metrics:   make([]snmpdiag.MetricValue, 0, len(values.TopologyMetrics)),
		BGPRows:   make([]snmpdiag.BGPRowValue, 0, len(values.BGPRows)),
		BGPFailed: values.BGPFailed,
	}
	if values.Metadata != nil {
		result.Metadata = make(map[string]snmpdiag.MetaTag, len(values.Metadata))
		for key, value := range values.Metadata {
			result.Metadata[key] = snmpdiag.MetaTag{
				Value:        value.Value,
				IsExactMatch: value.IsExactMatch,
			}
		}
	}
	for _, metric := range values.TopologyMetrics {
		result.Metrics = append(result.Metrics, snmpdiag.MetricValue{
			RowIndex: metric.RowIndex, Field: metric.Field,
			RouteOrdinal: metric.RouteOrdinal,
			RowOrdinal:   metric.RowOrdinal,
			ValueOrdinal: metric.ValueOrdinal,
			Kind:         string(metric.Kind),
			Tags:         metric.Tags,
		})
	}
	for _, row := range values.BGPRows {
		result.BGPRows = append(result.BGPRows, snmpdiag.BGPRowValue{
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
			Tags:            row.Tags,
		})
	}
	return result
}

func newArchiveCollectionStatsV1(
	stats ddsnmp.CollectionStats,
) snmpdiag.CollectionStats {
	return snmpdiag.CollectionStats{
		Timing: snmpdiag.TimingStats{
			PreparationNanos:    int64(stats.Timing.Preparation),
			ScalarNanos:         int64(stats.Timing.Scalar),
			TableNanos:          int64(stats.Timing.Table),
			LicensingNanos:      int64(stats.Timing.Licensing),
			BGPNanos:            int64(stats.Timing.BGP),
			VirtualMetricsNanos: int64(stats.Timing.VirtualMetrics),
		},
		SNMP: snmpdiag.SNMPStats{
			GetRequests:  stats.SNMP.GetRequests,
			GetOIDs:      stats.SNMP.GetOIDs,
			WalkRequests: stats.SNMP.WalkRequests,
			WalkPDUs:     stats.SNMP.WalkPDUs,
			TablesWalked: stats.SNMP.TablesWalked,
			TablesCached: stats.SNMP.TablesCached,
		},
		Metrics: snmpdiag.MetricStats{
			Scalar:    stats.Metrics.Scalar,
			Table:     stats.Metrics.Table,
			Virtual:   stats.Metrics.Virtual,
			Licensing: stats.Metrics.Licensing,
			BGP:       stats.Metrics.BGP,
			Tables:    stats.Metrics.Tables,
			Rows:      stats.Metrics.Rows,
		},
		TableCache: snmpdiag.TableCacheStats{
			Hits:   stats.TableCache.Hits,
			Misses: stats.TableCache.Misses,
		},
		Errors: snmpdiag.ErrorStats{
			ProcessingPreparation: stats.Errors.Processing.Preparation,
			SNMP:                  stats.Errors.SNMP,
			ProcessingScalar:      stats.Errors.Processing.Scalar,
			ProcessingTable:       stats.Errors.Processing.Table,
			ProcessingLicensing:   stats.Errors.Processing.Licensing,
			ProcessingBGP:         stats.Errors.Processing.BGP,
			MissingOIDs:           stats.Errors.MissingOIDs,
		},
	}
}

func restoreArchiveAcquisitionEvidence(e snmpdiag.AcquisitionEvidence,
	id AcquisitionAttemptID,
) (*AcquisitionAttemptEvidence, error) {
	targetOutcome, err := archiveParseTargetOutcome(e.Target.Outcome)
	if err != nil {
		return nil, err
	}
	var addresses []netip.Addr
	if len(e.Target.Addresses) > 0 {
		addresses = make([]netip.Addr, 0, len(e.Target.Addresses))
	}
	for _, raw := range e.Target.Addresses {
		address, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, fmt.Errorf("target address %q: %w", raw, err)
		}
		addresses = append(addresses, address)
	}
	client, err := restoreArchivePhase(e.Client)
	if err != nil {
		return nil, fmt.Errorf("client phase: %w", err)
	}
	connect, err := restoreArchivePhase(e.Connect)
	if err != nil {
		return nil, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := restoreArchivePhase(e.Profiles)
	if err != nil {
		return nil, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := restoreArchivePhase(e.Collection)
	if err != nil {
		return nil, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := restoreArchivePhase(e.SysUptime)
	if err != nil {
		return nil, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := restoreArchivePhase(e.VLANProfiles)
	if err != nil {
		return nil, fmt.Errorf("vlan_profiles phase: %w", err)
	}
	if !e.Interruption.Valid() {
		return nil, fmt.Errorf("invalid interruption")
	}
	result := &AcquisitionAttemptEvidence{
		Interruption: e.Interruption,
		ID:           id,
		Device: DeviceInput{
			Hostname:    e.Device.Hostname,
			SysObjectID: e.Device.SysObjectID,
			SysName:     e.Device.SysName,
			SysDescr:    e.Device.SysDescr,
			SysContact:  e.Device.SysContact,
			SysLocation: e.Device.SysLocation,
			Vendor:      e.Device.Vendor,
			Model:       e.Device.Model,
			VnodeGUID:   e.Device.VnodeGUID,
			VnodeLabels: e.Device.VnodeLabels,
		},
		Target: TargetResolutionEvidence{
			Outcome:   targetOutcome,
			Addresses: addresses,
		},
		Client:             client,
		Connect:            connect,
		Profiles:           profiles,
		Collection:         collection,
		SysUptime:          sysUptime,
		VLANProfiles:       vlanProfiles,
		CollectedAt:        e.CollectedAt,
		FreshFor:           time.Duration(e.FreshForNanos),
		SysUptimeValue:     e.SysUptimeValue,
		CollectionContexts: make([]AcquisitionContextEvidence, 0, len(e.CollectionContexts)),
	}

	for _, context := range e.CollectionContexts {
		reconstructed, err := restoreArchiveContextEvidence(context)
		if err != nil {
			return nil, fmt.Errorf("collection context %d: %w", context.Ordinal, err)
		}
		result.CollectionContexts = append(result.CollectionContexts, reconstructed)
	}
	profileContext, err := ddsnmp.RestoreProfileContext(e.ProfileContext)
	if err != nil {
		return nil, err
	}
	vlanProfileContext, err := ddsnmp.RestoreProfileContext(e.VLANProfileContext)
	if err != nil {
		return nil, err
	}
	result.ProfileContext, result.VLANProfileContext = profileContext, vlanProfileContext
	return result, nil
}

func restoreArchiveContextEvidence(c snmpdiag.ContextEvidence) (AcquisitionContextEvidence, error) {

	client, err := restoreArchivePhase(c.Client)
	if err != nil {
		return AcquisitionContextEvidence{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := restoreArchivePhase(c.Connect)
	if err != nil {
		return AcquisitionContextEvidence{}, fmt.Errorf("connect phase: %w", err)
	}
	collection, err := restoreArchivePhase(c.Collection)
	if err != nil {
		return AcquisitionContextEvidence{}, fmt.Errorf("collection phase: %w", err)
	}
	if !c.Interruption.Valid() || !c.Failures.Valid() {
		return AcquisitionContextEvidence{}, fmt.Errorf("invalid context failure")
	}
	if err := ddsnmp.ValidateSourceOperations(c.Sources); err != nil {
		return AcquisitionContextEvidence{}, err
	}
	result := AcquisitionContextEvidence{
		Sources:      c.Sources,
		Interruption: c.Interruption,
		Failures:     c.Failures,
		Ordinal:      c.Ordinal,
		VLANID:       c.VLANID,
		VLANName:     c.VLANName,
		Client:       client,
		Connect:      connect,
		Collection:   collection,
		Profiles:     make([]AcquisitionProfileEvidence, 0, len(c.Profiles)),
	}
	sourceRequests := ddsnmp.SourceRequestIndex(c.Sources)
	chargedWalks := make(map[uint64]struct{})
	for _, profile := range c.Profiles {
		for _, route := range profile.Routes {
			if err := ddsnmp.ValidateSourceBindings(route.Sources, sourceRequests); err != nil {
				return AcquisitionContextEvidence{}, err
			}
			if err := ddsnmp.ValidateProcessingEvents(route.Processing); err != nil {
				return AcquisitionContextEvidence{}, err
			}
		}
		if profile.Execution != nil {
			for _, id := range profile.Execution.WalkOperations {
				if id == 0 || id > uint64(len(c.Sources)) || c.Sources[id-1].Method == "get" {
					return AcquisitionContextEvidence{}, errors.New("invalid executed WALK reference")
				}
				if _, duplicate := chargedWalks[id]; duplicate {
					return AcquisitionContextEvidence{}, errors.New("WALK execution charged more than once")
				}
				chargedWalks[id] = struct{}{}
			}
		}
		reconstructed, err := restoreArchiveProfileEvidence(profile)
		if err != nil {
			return AcquisitionContextEvidence{}, fmt.Errorf("profile %d: %w", profile.Identity.Ordinal, err)
		}

		result.Profiles = append(result.Profiles, reconstructed)
	}
	return result, nil
}

func restoreArchiveProfileEvidence(p snmpdiag.ProfileEvidence) (AcquisitionProfileEvidence, error) {
	digest, err := hex.DecodeString(p.Identity.RouteDigest)
	if err != nil || len(digest) != 32 {
		return AcquisitionProfileEvidence{}, errors.New("route digest must be 32 bytes of hexadecimal")
	}
	var routeDigest [32]byte
	copy(routeDigest[:], digest)
	outcome, err := archiveParseProfileOutcome(p.Outcome)
	if err != nil {
		return AcquisitionProfileEvidence{}, err
	}
	failurePhase, err := archiveParseProfileFailurePhase(p.FailurePhase)
	if err != nil {
		return AcquisitionProfileEvidence{}, err
	}
	result := AcquisitionProfileEvidence{
		Identity: ddsnmpcollector.AcquisitionProfileIdentity{
			Ordinal:     p.Identity.Ordinal,
			RouteDigest: routeDigest,
		},
		Outcome:      outcome,
		FailurePhase: failurePhase,
		Stats:        restoreArchiveCollectionStats(p.Stats),
		Routes:       make([]ddsnmpcollector.AcquisitionRouteReport, 0, len(p.Routes)),
	}
	result.Execution, err = restoreArchiveExecution(p.Execution)
	if err != nil {
		return AcquisitionProfileEvidence{}, err
	}
	routeOrdinals := make(map[uint32]struct{}, len(p.Routes))
	for _, route := range p.Routes {
		if _, ok := routeOrdinals[route.Ordinal]; ok {
			return AcquisitionProfileEvidence{}, fmt.Errorf("duplicate route ordinal %d", route.Ordinal)
		}
		reconstructed, err := restoreArchiveRoute(route)
		if err != nil {
			return AcquisitionProfileEvidence{}, fmt.Errorf("route %d: %w", route.Ordinal, err)
		}
		routeOrdinals[route.Ordinal] = struct{}{}
		result.Routes = append(result.Routes, reconstructed)
	}
	values, err := restoreArchiveProfileValues(p.Values, routeOrdinals)
	if err != nil {
		return AcquisitionProfileEvidence{}, err
	}
	result.Values = values
	return result, nil
}

func restoreArchiveRoute(r snmpdiag.Route) (ddsnmpcollector.AcquisitionRouteReport, error) {
	kind, err := archiveParseRouteKind(r.Kind)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	source, err := archiveParseRouteSource(r.Source)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	outcome, err := archiveParseRouteOutcome(r.Outcome)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	failureClass, err := archiveParseRouteFailureClass(r.FailureClass)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	return ddsnmpcollector.AcquisitionRouteReport{
		Sources: r.Sources, Processing: r.Processing,
		Ordinal:      r.Ordinal,
		Kind:         kind,
		RootOID:      r.RootOID,
		Source:       source,
		Outcome:      outcome,
		FailureClass: failureClass,
		Rows:         r.Rows,
		Values:       r.Values,
		Missing:      r.Missing,
		Rejected:     r.Rejected,
	}, nil
}

func restoreArchiveProfileValues(v snmpdiag.ProfileValues,
	routeOrdinals map[uint32]struct{},
) (AcquisitionProfileValues, error) {
	result := AcquisitionProfileValues{
		Tags:            v.Tags,
		TopologyMetrics: make([]AcquisitionMetricValue, 0, len(v.Metrics)),
		BGPRows:         make([]AcquisitionBGPRowValue, 0, len(v.BGPRows)),
		BGPFailed:       v.BGPFailed,
	}
	if v.Metadata != nil {
		result.Metadata = make(map[string]ddsnmp.MetaTag, len(v.Metadata))
		for key, value := range v.Metadata {
			result.Metadata[key] = ddsnmp.MetaTag{Value: value.Value, IsExactMatch: value.IsExactMatch}
		}
	}
	for _, metric := range v.Metrics {
		if _, ok := routeOrdinals[metric.RouteOrdinal]; !ok {
			return AcquisitionProfileValues{}, fmt.Errorf("metric references unknown route ordinal %d", metric.RouteOrdinal)
		}
		kind := ddsnmp.TopologyKind(metric.Kind)
		if !ddprofiledefinition.IsValidTopologyKind(kind) {
			return AcquisitionProfileValues{}, fmt.Errorf("unknown topology kind %q", metric.Kind)
		}
		result.TopologyMetrics = append(result.TopologyMetrics, AcquisitionMetricValue{
			RouteOrdinal: metric.RouteOrdinal,
			RowIndex:     metric.RowIndex, Field: metric.Field,
			RowOrdinal:   metric.RowOrdinal,
			ValueOrdinal: metric.ValueOrdinal,
			Kind:         kind,
			Tags:         metric.Tags,
		})
	}
	for _, row := range v.BGPRows {
		if _, ok := routeOrdinals[row.RouteOrdinal]; !ok {
			return AcquisitionProfileValues{}, fmt.Errorf("BGP row references unknown route ordinal %d", row.RouteOrdinal)
		}
		kind := ddprofiledefinition.BGPRowKind(row.Kind)
		if !ddprofiledefinition.IsValidBGPRowKind(kind) {
			return AcquisitionProfileValues{}, fmt.Errorf("unknown BGP row kind %q", row.Kind)
		}
		state := ddprofiledefinition.BGPPeerState(row.State)
		result.BGPRows = append(result.BGPRows, AcquisitionBGPRowValue{
			RouteOrdinal:    row.RouteOrdinal,
			RowOrdinal:      row.RowOrdinal,
			ValueOrdinal:    row.ValueOrdinal,
			OriginProfileID: row.OriginProfileID,
			Table:           row.Table,
			RowKey:          row.RowKey,
			StructuralID:    row.StructuralID,
			Kind:            kind,
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
			State:           state,
			StateRaw:        row.StateRaw,
			EstablishedHas:  row.EstablishedHas,
			Established:     row.Established,
			UpdateAgeHas:    row.UpdateAgeHas,
			UpdateAge:       row.UpdateAge,
			Tags:            row.Tags,
		})
	}
	return result, nil
}

func restoreArchiveCollectionStats(s snmpdiag.CollectionStats) ddsnmp.CollectionStats {
	var result ddsnmp.CollectionStats
	result.Timing.Preparation = time.Duration(s.Timing.PreparationNanos)
	result.Timing.Scalar = time.Duration(s.Timing.ScalarNanos)
	result.Timing.Table = time.Duration(s.Timing.TableNanos)
	result.Timing.Licensing = time.Duration(s.Timing.LicensingNanos)
	result.Timing.BGP = time.Duration(s.Timing.BGPNanos)
	result.Timing.VirtualMetrics = time.Duration(s.Timing.VirtualMetricsNanos)
	result.SNMP.GetRequests = s.SNMP.GetRequests
	result.SNMP.GetOIDs = s.SNMP.GetOIDs
	result.SNMP.WalkRequests = s.SNMP.WalkRequests
	result.SNMP.WalkPDUs = s.SNMP.WalkPDUs
	result.SNMP.TablesWalked = s.SNMP.TablesWalked
	result.SNMP.TablesCached = s.SNMP.TablesCached
	result.Metrics.Scalar = s.Metrics.Scalar
	result.Metrics.Table = s.Metrics.Table
	result.Metrics.Virtual = s.Metrics.Virtual
	result.Metrics.Licensing = s.Metrics.Licensing
	result.Metrics.BGP = s.Metrics.BGP
	result.Metrics.Tables = s.Metrics.Tables
	result.Metrics.Rows = s.Metrics.Rows
	result.TableCache.Hits = s.TableCache.Hits
	result.TableCache.Misses = s.TableCache.Misses
	result.Errors.SNMP = s.Errors.SNMP
	result.Errors.Processing.Preparation = s.Errors.ProcessingPreparation
	result.Errors.Processing.Scalar = s.Errors.ProcessingScalar
	result.Errors.Processing.Table = s.Errors.ProcessingTable
	result.Errors.Processing.Licensing = s.Errors.ProcessingLicensing
	result.Errors.Processing.BGP = s.Errors.ProcessingBGP
	result.Errors.MissingOIDs = s.Errors.MissingOIDs
	return result
}

func newArchivePhaseV1(
	phase AcquisitionPhaseEvidence,
) (snmpdiag.Phase, error) {
	outcome, err := archivePhaseOutcomeName(phase.Outcome)
	if err != nil {
		return snmpdiag.Phase{}, err
	}
	failure, err := archivePhaseFailureName(phase.Failure)
	if err != nil {
		return snmpdiag.Phase{}, err
	}
	return snmpdiag.Phase{Outcome: outcome, Failure: failure, Detail: phase.Detail}, nil
}

func restoreArchivePhase(p snmpdiag.Phase) (AcquisitionPhaseEvidence, error) {
	outcome, err := archiveParsePhaseOutcome(p.Outcome)
	if err != nil {
		return AcquisitionPhaseEvidence{}, err
	}
	failure, err := archiveParsePhaseFailure(p.Failure)
	if err != nil {
		return AcquisitionPhaseEvidence{}, err
	}
	if !p.Detail.Valid() {
		return AcquisitionPhaseEvidence{}, fmt.Errorf("invalid phase failure detail")
	}
	return AcquisitionPhaseEvidence{Outcome: outcome, Failure: failure, Detail: p.Detail}, nil
}

var (
	archiveTargetOutcomeNames = []string{
		"unknown", "literal", "resolved", "empty", "unavailable", "failed",
	}
	archivePhaseOutcomeNames = []string{
		"unknown", "success", "empty", "failed", "not_observed",
	}
	archivePhaseFailureNames = []string{
		"none", "client_configuration", "connect", "collection", "sys_uptime", "vlan_identifier",
	}
	archiveProfileOutcomeNames = []string{
		"unknown", "success", "partial", "failed",
	}
	archiveProfileFailurePhaseNames = []string{
		"none", "prepare", "tables",
	}
	archiveRouteKindNames = []string{
		"unknown", "profile_tag_scalar", "metadata_scalar", "topology_scalar", "topology_table",
		"bgp_scalar", "bgp_table",
	}
	archiveRouteSourceNames = []string{
		"none", "get", "walk", "cache",
	}
	archiveRouteOutcomeNames = []string{
		"not_observed", "missing", "failed", "empty", "values", "rejected", "partial",
	}
	archiveRouteFailureClassNames = []string{
		"none", "transport", "processing", "dependency",
	}
)

func archiveTargetOutcomeName(value TargetResolutionOutcome) (string, error) {
	return archiveEnumName(value, archiveTargetOutcomeNames)
}

func archiveParseTargetOutcome(value string) (TargetResolutionOutcome, error) {
	return archiveParseEnum[TargetResolutionOutcome](value, archiveTargetOutcomeNames)
}

func archivePhaseOutcomeName(value AcquisitionPhaseOutcome) (string, error) {
	return archiveEnumName(value, archivePhaseOutcomeNames)
}

func archiveParsePhaseOutcome(value string) (AcquisitionPhaseOutcome, error) {
	return archiveParseEnum[AcquisitionPhaseOutcome](value, archivePhaseOutcomeNames)
}

func archivePhaseFailureName(value AcquisitionFailureClass) (string, error) {
	return archiveEnumName(value, archivePhaseFailureNames)
}

func archiveParsePhaseFailure(value string) (AcquisitionFailureClass, error) {
	return archiveParseEnum[AcquisitionFailureClass](value, archivePhaseFailureNames)
}

func archiveProfileOutcomeName(value ddsnmpcollector.AcquisitionProfileOutcome) (string, error) {
	return archiveEnumName(value, archiveProfileOutcomeNames)
}

func archiveParseProfileOutcome(value string) (ddsnmpcollector.AcquisitionProfileOutcome, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionProfileOutcome](
		value,
		archiveProfileOutcomeNames,
	)
}

func archiveProfileFailurePhaseName(value ddsnmpcollector.AcquisitionFailurePhase) (string, error) {
	return archiveEnumName(value, archiveProfileFailurePhaseNames)
}

func archiveParseProfileFailurePhase(value string) (ddsnmpcollector.AcquisitionFailurePhase, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionFailurePhase](
		value,
		archiveProfileFailurePhaseNames,
	)
}

func archiveRouteKindName(value ddsnmpcollector.AcquisitionRouteKind) (string, error) {
	return archiveEnumName(value, archiveRouteKindNames)
}

func archiveParseRouteKind(value string) (ddsnmpcollector.AcquisitionRouteKind, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionRouteKind](value, archiveRouteKindNames)
}

func archiveRouteSourceName(value ddsnmpcollector.AcquisitionRouteSource) (string, error) {
	return archiveEnumName(value, archiveRouteSourceNames)
}

func archiveParseRouteSource(value string) (ddsnmpcollector.AcquisitionRouteSource, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionRouteSource](value, archiveRouteSourceNames)
}

func archiveRouteOutcomeName(value ddsnmpcollector.AcquisitionRouteOutcome) (string, error) {
	return archiveEnumName(value, archiveRouteOutcomeNames)
}

func archiveParseRouteOutcome(value string) (ddsnmpcollector.AcquisitionRouteOutcome, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionRouteOutcome](value, archiveRouteOutcomeNames)
}

func archiveRouteFailureClassName(value ddsnmpcollector.AcquisitionFailureClass) (string, error) {
	return archiveEnumName(value, archiveRouteFailureClassNames)
}

func archiveParseRouteFailureClass(value string) (ddsnmpcollector.AcquisitionFailureClass, error) {
	return archiveParseEnum[ddsnmpcollector.AcquisitionFailureClass](
		value,
		archiveRouteFailureClassNames,
	)
}
