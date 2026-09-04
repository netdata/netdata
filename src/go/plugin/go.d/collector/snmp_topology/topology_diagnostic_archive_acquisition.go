// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

type topologyDiagnosticArchiveAcquisitionEvidenceV1 struct {
	Device             topologyDiagnosticArchiveDeviceInputV1       `json:"device"`
	Target             topologyDiagnosticArchiveTargetV1            `json:"target"`
	Client             topologyDiagnosticArchivePhaseV1             `json:"client"`
	Connect            topologyDiagnosticArchivePhaseV1             `json:"connect"`
	Profiles           topologyDiagnosticArchivePhaseV1             `json:"profiles"`
	Collection         topologyDiagnosticArchivePhaseV1             `json:"collection"`
	SysUptime          topologyDiagnosticArchivePhaseV1             `json:"sys_uptime"`
	VLANProfiles       topologyDiagnosticArchivePhaseV1             `json:"vlan_profiles"`
	CollectedAt        time.Time                                    `json:"collected_at"`
	FreshForNanos      int64                                        `json:"fresh_for_ns"`
	SysUptimeValue     int64                                        `json:"sys_uptime_value"`
	CollectionContexts []topologyDiagnosticArchiveContextEvidenceV1 `json:"collection_contexts,omitempty"`
}

type topologyDiagnosticArchiveDeviceInputV1 struct {
	Hostname    string            `json:"hostname"`
	SysObjectID string            `json:"sys_object_id"`
	SysName     string            `json:"sys_name"`
	SysDescr    string            `json:"sys_descr"`
	SysContact  string            `json:"sys_contact"`
	SysLocation string            `json:"sys_location"`
	Vendor      string            `json:"vendor"`
	Model       string            `json:"model"`
	VnodeGUID   string            `json:"vnode_guid"`
	VnodeLabels map[string]string `json:"vnode_labels,omitempty"`
}

type topologyDiagnosticArchiveTargetV1 struct {
	Outcome   string   `json:"outcome"`
	Addresses []string `json:"addresses,omitempty"`
}

type topologyDiagnosticArchivePhaseV1 struct {
	Outcome string `json:"outcome"`
	Failure string `json:"failure"`
}

type topologyDiagnosticArchiveContextEvidenceV1 struct {
	Ordinal    uint32                                       `json:"ordinal"`
	VLANID     string                                       `json:"vlan_id"`
	VLANName   string                                       `json:"vlan_name"`
	Client     topologyDiagnosticArchivePhaseV1             `json:"client"`
	Connect    topologyDiagnosticArchivePhaseV1             `json:"connect"`
	Collection topologyDiagnosticArchivePhaseV1             `json:"collection"`
	Profiles   []topologyDiagnosticArchiveProfileEvidenceV1 `json:"profiles,omitempty"`
}

type topologyDiagnosticArchiveProfileEvidenceV1 struct {
	Identity     topologyDiagnosticArchiveProfileIdentityV1 `json:"identity"`
	Outcome      string                                     `json:"outcome"`
	FailurePhase string                                     `json:"failure_phase"`
	Stats        topologyDiagnosticArchiveCollectionStatsV1 `json:"stats"`
	Execution    *topologyDiagnosticArchiveExecutionV1      `json:"execution,omitempty"`
	Routes       []topologyDiagnosticArchiveRouteV1         `json:"routes,omitempty"`
	Values       topologyDiagnosticArchiveProfileValuesV1   `json:"values"`
}

type topologyDiagnosticArchiveProfileIdentityV1 struct {
	Ordinal     uint32 `json:"ordinal"`
	RouteDigest string `json:"route_digest"`
}

type topologyDiagnosticArchiveRouteV1 struct {
	Ordinal      uint32 `json:"ordinal"`
	Kind         string `json:"kind"`
	RootOID      string `json:"root_oid"`
	Source       string `json:"source"`
	Outcome      string `json:"outcome"`
	FailureClass string `json:"failure_class"`
	Rows         uint64 `json:"rows"`
	Values       uint64 `json:"values"`
	Missing      uint64 `json:"missing"`
	Rejected     uint64 `json:"rejected"`
}

type topologyDiagnosticArchiveProfileValuesV1 struct {
	Metadata  map[string]topologyDiagnosticArchiveMetaTagV1 `json:"metadata,omitempty"`
	Tags      map[string]string                             `json:"tags,omitempty"`
	Metrics   []topologyDiagnosticArchiveMetricValueV1      `json:"metrics,omitempty"`
	BGPRows   []topologyDiagnosticArchiveBGPRowValueV1      `json:"bgp_rows,omitempty"`
	BGPFailed bool                                          `json:"bgp_failed"`
}

type topologyDiagnosticArchiveMetaTagV1 struct {
	Value        string `json:"value"`
	IsExactMatch bool   `json:"is_exact_match"`
}

type topologyDiagnosticArchiveMetricValueV1 struct {
	RouteOrdinal uint32            `json:"route_ordinal"`
	RowOrdinal   uint32            `json:"row_ordinal"`
	ValueOrdinal uint32            `json:"value_ordinal"`
	Kind         string            `json:"kind"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type topologyDiagnosticArchiveBGPRowValueV1 struct {
	RouteOrdinal uint32 `json:"route_ordinal"`
	RowOrdinal   uint32 `json:"row_ordinal"`
	ValueOrdinal uint32 `json:"value_ordinal"`

	OriginProfileID string `json:"origin_profile_id"`
	Table           string `json:"table"`
	RowKey          string `json:"row_key"`
	StructuralID    string `json:"structural_id"`
	Kind            string `json:"kind"`

	RoutingInstance string `json:"routing_instance"`
	Neighbor        string `json:"neighbor"`
	RemoteAS        string `json:"remote_as"`
	LocalAddress    string `json:"local_address"`
	LocalAS         string `json:"local_as"`
	LocalIdentifier string `json:"local_identifier"`
	PeerIdentifier  string `json:"peer_identifier"`
	PeerType        string `json:"peer_type"`
	BGPVersion      string `json:"bgp_version"`
	Description     string `json:"description"`

	AdminHas       bool              `json:"admin_has"`
	AdminEnabled   bool              `json:"admin_enabled"`
	StateHas       bool              `json:"state_has"`
	State          string            `json:"state"`
	StateRaw       string            `json:"state_raw"`
	EstablishedHas bool              `json:"established_has"`
	Established    int64             `json:"established"`
	UpdateAgeHas   bool              `json:"update_age_has"`
	UpdateAge      int64             `json:"update_age"`
	Tags           map[string]string `json:"tags,omitempty"`
}

type topologyDiagnosticArchiveCollectionStatsV1 struct {
	Timing     topologyDiagnosticArchiveTimingStatsV1     `json:"timing"`
	SNMP       topologyDiagnosticArchiveSNMPStatsV1       `json:"snmp"`
	Metrics    topologyDiagnosticArchiveMetricStatsV1     `json:"metrics"`
	TableCache topologyDiagnosticArchiveTableCacheStatsV1 `json:"table_cache"`
	Errors     topologyDiagnosticArchiveErrorStatsV1      `json:"errors"`
}

type topologyDiagnosticArchiveTimingStatsV1 struct {
	PreparationNanos    int64 `json:"preparation_ns,omitempty"`
	ScalarNanos         int64 `json:"scalar_ns"`
	TableNanos          int64 `json:"table_ns"`
	LicensingNanos      int64 `json:"licensing_ns"`
	BGPNanos            int64 `json:"bgp_ns"`
	VirtualMetricsNanos int64 `json:"virtual_metrics_ns"`
}

type topologyDiagnosticArchiveSNMPStatsV1 struct {
	GetRequests  int64 `json:"get_requests"`
	GetOIDs      int64 `json:"get_oids"`
	WalkRequests int64 `json:"walk_requests"`
	WalkPDUs     int64 `json:"walk_pdus"`
	TablesWalked int64 `json:"tables_walked"`
	TablesCached int64 `json:"tables_cached"`
}

type topologyDiagnosticArchiveMetricStatsV1 struct {
	Scalar    int64 `json:"scalar"`
	Table     int64 `json:"table"`
	Virtual   int64 `json:"virtual"`
	Licensing int64 `json:"licensing"`
	BGP       int64 `json:"bgp"`
	Tables    int64 `json:"tables"`
	Rows      int64 `json:"rows"`
}

type topologyDiagnosticArchiveTableCacheStatsV1 struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
}

type topologyDiagnosticArchiveErrorStatsV1 struct {
	ProcessingPreparation int64 `json:"processing_preparation,omitempty"`
	SNMP                  int64 `json:"snmp"`
	ProcessingScalar      int64 `json:"processing_scalar"`
	ProcessingTable       int64 `json:"processing_table"`
	ProcessingLicensing   int64 `json:"processing_licensing"`
	ProcessingBGP         int64 `json:"processing_bgp"`
	MissingOIDs           int64 `json:"missing_oids"`
}

func newTopologyDiagnosticArchiveAcquisitionEvidenceV1(
	evidence *topologyAcquisitionAttemptEvidence,
) (topologyDiagnosticArchiveAcquisitionEvidenceV1, error) {
	targetOutcome, err := topologyDiagnosticArchiveTargetOutcomeName(evidence.target.outcome)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, err
	}
	client, err := newTopologyDiagnosticArchivePhaseV1(evidence.client)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newTopologyDiagnosticArchivePhaseV1(evidence.connect)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := newTopologyDiagnosticArchivePhaseV1(evidence.profiles)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := newTopologyDiagnosticArchivePhaseV1(evidence.collection)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := newTopologyDiagnosticArchivePhaseV1(evidence.sysUptime)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := newTopologyDiagnosticArchivePhaseV1(evidence.vlanProfiles)
	if err != nil {
		return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("vlan_profiles phase: %w", err)
	}
	result := topologyDiagnosticArchiveAcquisitionEvidenceV1{
		Device: topologyDiagnosticArchiveDeviceInputV1{
			Hostname:    evidence.device.hostname,
			SysObjectID: evidence.device.sysObjectID,
			SysName:     evidence.device.sysName,
			SysDescr:    evidence.device.sysDescr,
			SysContact:  evidence.device.sysContact,
			SysLocation: evidence.device.sysLocation,
			Vendor:      evidence.device.vendor,
			Model:       evidence.device.model,
			VnodeGUID:   evidence.device.vnodeGUID,
			VnodeLabels: evidence.device.vnodeLabels,
		},
		Target: topologyDiagnosticArchiveTargetV1{
			Outcome:   targetOutcome,
			Addresses: make([]string, 0, len(evidence.target.addresses)),
		},
		Client:             client,
		Connect:            connect,
		Profiles:           profiles,
		Collection:         collection,
		SysUptime:          sysUptime,
		VLANProfiles:       vlanProfiles,
		CollectedAt:        evidence.collectedAt,
		FreshForNanos:      int64(evidence.freshFor),
		SysUptimeValue:     evidence.sysUptimeValue,
		CollectionContexts: make([]topologyDiagnosticArchiveContextEvidenceV1, 0, len(evidence.collectionContexts)),
	}
	for _, address := range evidence.target.addresses {
		result.Target.Addresses = append(result.Target.Addresses, address.String())
	}
	for _, context := range evidence.collectionContexts {
		archived, err := newTopologyDiagnosticArchiveContextEvidenceV1(context)
		if err != nil {
			return topologyDiagnosticArchiveAcquisitionEvidenceV1{}, fmt.Errorf("collection context %d: %w", context.ordinal, err)
		}
		result.CollectionContexts = append(result.CollectionContexts, archived)
	}
	return result, nil
}

func newTopologyDiagnosticArchiveContextEvidenceV1(
	context topologyAcquisitionContextEvidence,
) (topologyDiagnosticArchiveContextEvidenceV1, error) {
	client, err := newTopologyDiagnosticArchivePhaseV1(context.client)
	if err != nil {
		return topologyDiagnosticArchiveContextEvidenceV1{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := newTopologyDiagnosticArchivePhaseV1(context.connect)
	if err != nil {
		return topologyDiagnosticArchiveContextEvidenceV1{}, fmt.Errorf("connect phase: %w", err)
	}
	collection, err := newTopologyDiagnosticArchivePhaseV1(context.collection)
	if err != nil {
		return topologyDiagnosticArchiveContextEvidenceV1{}, fmt.Errorf("collection phase: %w", err)
	}
	result := topologyDiagnosticArchiveContextEvidenceV1{
		Ordinal:    context.ordinal,
		VLANID:     context.vlanID,
		VLANName:   context.vlanName,
		Client:     client,
		Connect:    connect,
		Collection: collection,
		Profiles:   make([]topologyDiagnosticArchiveProfileEvidenceV1, 0, len(context.profiles)),
	}
	for _, profile := range context.profiles {
		archived, err := newTopologyDiagnosticArchiveProfileEvidenceV1(profile)
		if err != nil {
			return topologyDiagnosticArchiveContextEvidenceV1{}, fmt.Errorf("profile %d: %w", profile.identity.Ordinal, err)
		}
		result.Profiles = append(result.Profiles, archived)
	}
	return result, nil
}

func newTopologyDiagnosticArchiveProfileEvidenceV1(
	profile topologyAcquisitionProfileEvidence,
) (topologyDiagnosticArchiveProfileEvidenceV1, error) {
	outcome, err := topologyDiagnosticArchiveProfileOutcomeName(profile.outcome)
	if err != nil {
		return topologyDiagnosticArchiveProfileEvidenceV1{}, err
	}
	failurePhase, err := topologyDiagnosticArchiveProfileFailurePhaseName(profile.failurePhase)
	if err != nil {
		return topologyDiagnosticArchiveProfileEvidenceV1{}, err
	}
	result := topologyDiagnosticArchiveProfileEvidenceV1{
		Identity: topologyDiagnosticArchiveProfileIdentityV1{
			Ordinal:     profile.identity.Ordinal,
			RouteDigest: hex.EncodeToString(profile.identity.RouteDigest[:]),
		},
		Outcome:      outcome,
		FailurePhase: failurePhase,
		Stats:        newTopologyDiagnosticArchiveCollectionStatsV1(profile.stats),
		Execution:    newTopologyDiagnosticArchiveExecutionV1(profile.execution),
		Routes:       make([]topologyDiagnosticArchiveRouteV1, 0, len(profile.routes)),
		Values:       newTopologyDiagnosticArchiveProfileValuesV1(profile.values),
	}
	for _, route := range profile.routes {
		archived, err := newTopologyDiagnosticArchiveRouteV1(route)
		if err != nil {
			return topologyDiagnosticArchiveProfileEvidenceV1{}, fmt.Errorf("route %d: %w", route.Ordinal, err)
		}
		result.Routes = append(result.Routes, archived)
	}
	return result, nil
}

func newTopologyDiagnosticArchiveRouteV1(
	route ddsnmpcollector.AcquisitionRouteReport,
) (topologyDiagnosticArchiveRouteV1, error) {
	kind, err := topologyDiagnosticArchiveRouteKindName(route.Kind)
	if err != nil {
		return topologyDiagnosticArchiveRouteV1{}, err
	}
	source, err := topologyDiagnosticArchiveRouteSourceName(route.Source)
	if err != nil {
		return topologyDiagnosticArchiveRouteV1{}, err
	}
	outcome, err := topologyDiagnosticArchiveRouteOutcomeName(route.Outcome)
	if err != nil {
		return topologyDiagnosticArchiveRouteV1{}, err
	}
	failureClass, err := topologyDiagnosticArchiveRouteFailureClassName(route.FailureClass)
	if err != nil {
		return topologyDiagnosticArchiveRouteV1{}, err
	}
	return topologyDiagnosticArchiveRouteV1{
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

func newTopologyDiagnosticArchiveProfileValuesV1(
	values topologyAcquisitionProfileValues,
) topologyDiagnosticArchiveProfileValuesV1 {
	result := topologyDiagnosticArchiveProfileValuesV1{
		Tags:      values.tags,
		Metrics:   make([]topologyDiagnosticArchiveMetricValueV1, 0, len(values.metrics)),
		BGPRows:   make([]topologyDiagnosticArchiveBGPRowValueV1, 0, len(values.bgpRows)),
		BGPFailed: values.bgpFailed,
	}
	if values.metadata != nil {
		result.Metadata = make(map[string]topologyDiagnosticArchiveMetaTagV1, len(values.metadata))
		for key, value := range values.metadata {
			result.Metadata[key] = topologyDiagnosticArchiveMetaTagV1{
				Value:        value.Value,
				IsExactMatch: value.IsExactMatch,
			}
		}
	}
	for _, metric := range values.metrics {
		result.Metrics = append(result.Metrics, topologyDiagnosticArchiveMetricValueV1{
			RouteOrdinal: metric.routeOrdinal,
			RowOrdinal:   metric.rowOrdinal,
			ValueOrdinal: metric.valueOrdinal,
			Kind:         string(metric.kind),
			Tags:         metric.tags,
		})
	}
	for _, row := range values.bgpRows {
		result.BGPRows = append(result.BGPRows, topologyDiagnosticArchiveBGPRowValueV1{
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
			Tags:            row.tags,
		})
	}
	return result
}

func newTopologyDiagnosticArchiveCollectionStatsV1(
	stats ddsnmp.CollectionStats,
) topologyDiagnosticArchiveCollectionStatsV1 {
	return topologyDiagnosticArchiveCollectionStatsV1{
		Timing: topologyDiagnosticArchiveTimingStatsV1{
			PreparationNanos:    int64(stats.Timing.Preparation),
			ScalarNanos:         int64(stats.Timing.Scalar),
			TableNanos:          int64(stats.Timing.Table),
			LicensingNanos:      int64(stats.Timing.Licensing),
			BGPNanos:            int64(stats.Timing.BGP),
			VirtualMetricsNanos: int64(stats.Timing.VirtualMetrics),
		},
		SNMP: topologyDiagnosticArchiveSNMPStatsV1{
			GetRequests:  stats.SNMP.GetRequests,
			GetOIDs:      stats.SNMP.GetOIDs,
			WalkRequests: stats.SNMP.WalkRequests,
			WalkPDUs:     stats.SNMP.WalkPDUs,
			TablesWalked: stats.SNMP.TablesWalked,
			TablesCached: stats.SNMP.TablesCached,
		},
		Metrics: topologyDiagnosticArchiveMetricStatsV1{
			Scalar:    stats.Metrics.Scalar,
			Table:     stats.Metrics.Table,
			Virtual:   stats.Metrics.Virtual,
			Licensing: stats.Metrics.Licensing,
			BGP:       stats.Metrics.BGP,
			Tables:    stats.Metrics.Tables,
			Rows:      stats.Metrics.Rows,
		},
		TableCache: topologyDiagnosticArchiveTableCacheStatsV1{
			Hits:   stats.TableCache.Hits,
			Misses: stats.TableCache.Misses,
		},
		Errors: topologyDiagnosticArchiveErrorStatsV1{
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

func (e topologyDiagnosticArchiveAcquisitionEvidenceV1) evidence(
	id topologyAcquisitionAttemptID,
) (*topologyAcquisitionAttemptEvidence, error) {
	targetOutcome, err := topologyDiagnosticArchiveParseTargetOutcome(e.Target.Outcome)
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
	client, err := e.Client.phase()
	if err != nil {
		return nil, fmt.Errorf("client phase: %w", err)
	}
	connect, err := e.Connect.phase()
	if err != nil {
		return nil, fmt.Errorf("connect phase: %w", err)
	}
	profiles, err := e.Profiles.phase()
	if err != nil {
		return nil, fmt.Errorf("profiles phase: %w", err)
	}
	collection, err := e.Collection.phase()
	if err != nil {
		return nil, fmt.Errorf("collection phase: %w", err)
	}
	sysUptime, err := e.SysUptime.phase()
	if err != nil {
		return nil, fmt.Errorf("sys_uptime phase: %w", err)
	}
	vlanProfiles, err := e.VLANProfiles.phase()
	if err != nil {
		return nil, fmt.Errorf("vlan_profiles phase: %w", err)
	}
	result := &topologyAcquisitionAttemptEvidence{
		id: id,
		device: topologySemanticDeviceInput{
			hostname:    e.Device.Hostname,
			sysObjectID: e.Device.SysObjectID,
			sysName:     e.Device.SysName,
			sysDescr:    e.Device.SysDescr,
			sysContact:  e.Device.SysContact,
			sysLocation: e.Device.SysLocation,
			vendor:      e.Device.Vendor,
			model:       e.Device.Model,
			vnodeGUID:   e.Device.VnodeGUID,
			vnodeLabels: e.Device.VnodeLabels,
		},
		target: topologyTargetResolutionEvidence{
			outcome:   targetOutcome,
			addresses: addresses,
		},
		client:             client,
		connect:            connect,
		profiles:           profiles,
		collection:         collection,
		sysUptime:          sysUptime,
		vlanProfiles:       vlanProfiles,
		collectedAt:        e.CollectedAt,
		freshFor:           time.Duration(e.FreshForNanos),
		sysUptimeValue:     e.SysUptimeValue,
		collectionContexts: make([]topologyAcquisitionContextEvidence, 0, len(e.CollectionContexts)),
	}
	for _, context := range e.CollectionContexts {
		reconstructed, err := context.context()
		if err != nil {
			return nil, fmt.Errorf("collection context %d: %w", context.Ordinal, err)
		}
		result.collectionContexts = append(result.collectionContexts, reconstructed)
	}
	return result, nil
}

func (c topologyDiagnosticArchiveContextEvidenceV1) context() (topologyAcquisitionContextEvidence, error) {
	client, err := c.Client.phase()
	if err != nil {
		return topologyAcquisitionContextEvidence{}, fmt.Errorf("client phase: %w", err)
	}
	connect, err := c.Connect.phase()
	if err != nil {
		return topologyAcquisitionContextEvidence{}, fmt.Errorf("connect phase: %w", err)
	}
	collection, err := c.Collection.phase()
	if err != nil {
		return topologyAcquisitionContextEvidence{}, fmt.Errorf("collection phase: %w", err)
	}
	result := topologyAcquisitionContextEvidence{
		ordinal:    c.Ordinal,
		vlanID:     c.VLANID,
		vlanName:   c.VLANName,
		client:     client,
		connect:    connect,
		collection: collection,
		profiles:   make([]topologyAcquisitionProfileEvidence, 0, len(c.Profiles)),
	}
	for _, profile := range c.Profiles {
		reconstructed, err := profile.profile()
		if err != nil {
			return topologyAcquisitionContextEvidence{}, fmt.Errorf("profile %d: %w", profile.Identity.Ordinal, err)
		}
		result.profiles = append(result.profiles, reconstructed)
	}
	return result, nil
}

func (p topologyDiagnosticArchiveProfileEvidenceV1) profile() (topologyAcquisitionProfileEvidence, error) {
	digest, err := hex.DecodeString(p.Identity.RouteDigest)
	if err != nil || len(digest) != 32 {
		return topologyAcquisitionProfileEvidence{}, errors.New("route digest must be 32 bytes of hexadecimal")
	}
	var routeDigest [32]byte
	copy(routeDigest[:], digest)
	outcome, err := topologyDiagnosticArchiveParseProfileOutcome(p.Outcome)
	if err != nil {
		return topologyAcquisitionProfileEvidence{}, err
	}
	failurePhase, err := topologyDiagnosticArchiveParseProfileFailurePhase(p.FailurePhase)
	if err != nil {
		return topologyAcquisitionProfileEvidence{}, err
	}
	result := topologyAcquisitionProfileEvidence{
		identity: ddsnmpcollector.AcquisitionProfileIdentity{
			Ordinal:     p.Identity.Ordinal,
			RouteDigest: routeDigest,
		},
		outcome:      outcome,
		failurePhase: failurePhase,
		stats:        p.Stats.stats(),
		routes:       make([]ddsnmpcollector.AcquisitionRouteReport, 0, len(p.Routes)),
	}
	result.execution, err = p.Execution.execution()
	if err != nil {
		return topologyAcquisitionProfileEvidence{}, err
	}
	routeOrdinals := make(map[uint32]struct{}, len(p.Routes))
	for _, route := range p.Routes {
		if _, ok := routeOrdinals[route.Ordinal]; ok {
			return topologyAcquisitionProfileEvidence{}, fmt.Errorf("duplicate route ordinal %d", route.Ordinal)
		}
		reconstructed, err := route.route()
		if err != nil {
			return topologyAcquisitionProfileEvidence{}, fmt.Errorf("route %d: %w", route.Ordinal, err)
		}
		routeOrdinals[route.Ordinal] = struct{}{}
		result.routes = append(result.routes, reconstructed)
	}
	values, err := p.Values.values(routeOrdinals)
	if err != nil {
		return topologyAcquisitionProfileEvidence{}, err
	}
	result.values = values
	return result, nil
}

func (r topologyDiagnosticArchiveRouteV1) route() (ddsnmpcollector.AcquisitionRouteReport, error) {
	kind, err := topologyDiagnosticArchiveParseRouteKind(r.Kind)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	source, err := topologyDiagnosticArchiveParseRouteSource(r.Source)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	outcome, err := topologyDiagnosticArchiveParseRouteOutcome(r.Outcome)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	failureClass, err := topologyDiagnosticArchiveParseRouteFailureClass(r.FailureClass)
	if err != nil {
		return ddsnmpcollector.AcquisitionRouteReport{}, err
	}
	return ddsnmpcollector.AcquisitionRouteReport{
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

func (v topologyDiagnosticArchiveProfileValuesV1) values(
	routeOrdinals map[uint32]struct{},
) (topologyAcquisitionProfileValues, error) {
	result := topologyAcquisitionProfileValues{
		tags:      v.Tags,
		metrics:   make([]topologyAcquisitionMetricValue, 0, len(v.Metrics)),
		bgpRows:   make([]topologyAcquisitionBGPRowValue, 0, len(v.BGPRows)),
		bgpFailed: v.BGPFailed,
	}
	if v.Metadata != nil {
		result.metadata = make(map[string]ddsnmp.MetaTag, len(v.Metadata))
		for key, value := range v.Metadata {
			result.metadata[key] = ddsnmp.MetaTag{Value: value.Value, IsExactMatch: value.IsExactMatch}
		}
	}
	for _, metric := range v.Metrics {
		if _, ok := routeOrdinals[metric.RouteOrdinal]; !ok {
			return topologyAcquisitionProfileValues{}, fmt.Errorf("metric references unknown route ordinal %d", metric.RouteOrdinal)
		}
		kind := ddsnmp.TopologyKind(metric.Kind)
		if !ddprofiledefinition.IsValidTopologyKind(kind) {
			return topologyAcquisitionProfileValues{}, fmt.Errorf("unknown topology kind %q", metric.Kind)
		}
		result.metrics = append(result.metrics, topologyAcquisitionMetricValue{
			routeOrdinal: metric.RouteOrdinal,
			rowOrdinal:   metric.RowOrdinal,
			valueOrdinal: metric.ValueOrdinal,
			kind:         kind,
			tags:         metric.Tags,
		})
	}
	for _, row := range v.BGPRows {
		if _, ok := routeOrdinals[row.RouteOrdinal]; !ok {
			return topologyAcquisitionProfileValues{}, fmt.Errorf("BGP row references unknown route ordinal %d", row.RouteOrdinal)
		}
		kind := ddprofiledefinition.BGPRowKind(row.Kind)
		if !ddprofiledefinition.IsValidBGPRowKind(kind) {
			return topologyAcquisitionProfileValues{}, fmt.Errorf("unknown BGP row kind %q", row.Kind)
		}
		state := ddprofiledefinition.BGPPeerState(row.State)
		result.bgpRows = append(result.bgpRows, topologyAcquisitionBGPRowValue{
			routeOrdinal:    row.RouteOrdinal,
			rowOrdinal:      row.RowOrdinal,
			valueOrdinal:    row.ValueOrdinal,
			originProfileID: row.OriginProfileID,
			table:           row.Table,
			rowKey:          row.RowKey,
			structuralID:    row.StructuralID,
			kind:            kind,
			routingInstance: row.RoutingInstance,
			neighbor:        row.Neighbor,
			remoteAS:        row.RemoteAS,
			localAddress:    row.LocalAddress,
			localAS:         row.LocalAS,
			localIdentifier: row.LocalIdentifier,
			peerIdentifier:  row.PeerIdentifier,
			peerType:        row.PeerType,
			bgpVersion:      row.BGPVersion,
			description:     row.Description,
			adminHas:        row.AdminHas,
			adminEnabled:    row.AdminEnabled,
			stateHas:        row.StateHas,
			state:           state,
			stateRaw:        row.StateRaw,
			establishedHas:  row.EstablishedHas,
			established:     row.Established,
			updateAgeHas:    row.UpdateAgeHas,
			updateAge:       row.UpdateAge,
			tags:            row.Tags,
		})
	}
	return result, nil
}

func (s topologyDiagnosticArchiveCollectionStatsV1) stats() ddsnmp.CollectionStats {
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

func newTopologyDiagnosticArchivePhaseV1(
	phase topologyAcquisitionPhaseEvidence,
) (topologyDiagnosticArchivePhaseV1, error) {
	outcome, err := topologyDiagnosticArchivePhaseOutcomeName(phase.outcome)
	if err != nil {
		return topologyDiagnosticArchivePhaseV1{}, err
	}
	failure, err := topologyDiagnosticArchivePhaseFailureName(phase.failure)
	if err != nil {
		return topologyDiagnosticArchivePhaseV1{}, err
	}
	return topologyDiagnosticArchivePhaseV1{Outcome: outcome, Failure: failure}, nil
}

func (p topologyDiagnosticArchivePhaseV1) phase() (topologyAcquisitionPhaseEvidence, error) {
	outcome, err := topologyDiagnosticArchiveParsePhaseOutcome(p.Outcome)
	if err != nil {
		return topologyAcquisitionPhaseEvidence{}, err
	}
	failure, err := topologyDiagnosticArchiveParsePhaseFailure(p.Failure)
	if err != nil {
		return topologyAcquisitionPhaseEvidence{}, err
	}
	return topologyAcquisitionPhaseEvidence{outcome: outcome, failure: failure}, nil
}

var (
	topologyDiagnosticArchiveTargetOutcomeNames = []string{
		"unknown", "literal", "resolved", "empty", "unavailable", "failed",
	}
	topologyDiagnosticArchivePhaseOutcomeNames = []string{
		"unknown", "success", "empty", "failed", "not_observed",
	}
	topologyDiagnosticArchivePhaseFailureNames = []string{
		"none", "client_configuration", "connect", "collection", "sys_uptime", "vlan_identifier",
	}
	topologyDiagnosticArchiveProfileOutcomeNames = []string{
		"unknown", "success", "partial", "failed",
	}
	topologyDiagnosticArchiveProfileFailurePhaseNames = []string{
		"none", "prepare", "tables",
	}
	topologyDiagnosticArchiveRouteKindNames = []string{
		"unknown", "profile_tag_scalar", "metadata_scalar", "topology_scalar", "topology_table",
		"bgp_scalar", "bgp_table",
	}
	topologyDiagnosticArchiveRouteSourceNames = []string{
		"none", "get", "walk", "cache",
	}
	topologyDiagnosticArchiveRouteOutcomeNames = []string{
		"not_observed", "missing", "failed", "empty", "values", "rejected", "partial",
	}
	topologyDiagnosticArchiveRouteFailureClassNames = []string{
		"none", "transport", "processing", "dependency",
	}
)

func topologyDiagnosticArchiveTargetOutcomeName(value topologyTargetResolutionOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveTargetOutcomeNames)
}

func topologyDiagnosticArchiveParseTargetOutcome(value string) (topologyTargetResolutionOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[topologyTargetResolutionOutcome](value, topologyDiagnosticArchiveTargetOutcomeNames)
}

func topologyDiagnosticArchivePhaseOutcomeName(value topologyAcquisitionPhaseOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchivePhaseOutcomeNames)
}

func topologyDiagnosticArchiveParsePhaseOutcome(value string) (topologyAcquisitionPhaseOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[topologyAcquisitionPhaseOutcome](value, topologyDiagnosticArchivePhaseOutcomeNames)
}

func topologyDiagnosticArchivePhaseFailureName(value topologyAcquisitionFailureClass) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchivePhaseFailureNames)
}

func topologyDiagnosticArchiveParsePhaseFailure(value string) (topologyAcquisitionFailureClass, error) {
	return topologyDiagnosticArchiveParseEnum[topologyAcquisitionFailureClass](value, topologyDiagnosticArchivePhaseFailureNames)
}

func topologyDiagnosticArchiveProfileOutcomeName(value ddsnmpcollector.AcquisitionProfileOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveProfileOutcomeNames)
}

func topologyDiagnosticArchiveParseProfileOutcome(value string) (ddsnmpcollector.AcquisitionProfileOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionProfileOutcome](value, topologyDiagnosticArchiveProfileOutcomeNames)
}

func topologyDiagnosticArchiveProfileFailurePhaseName(value ddsnmpcollector.AcquisitionFailurePhase) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveProfileFailurePhaseNames)
}

func topologyDiagnosticArchiveParseProfileFailurePhase(value string) (ddsnmpcollector.AcquisitionFailurePhase, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionFailurePhase](value, topologyDiagnosticArchiveProfileFailurePhaseNames)
}

func topologyDiagnosticArchiveRouteKindName(value ddsnmpcollector.AcquisitionRouteKind) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveRouteKindNames)
}

func topologyDiagnosticArchiveParseRouteKind(value string) (ddsnmpcollector.AcquisitionRouteKind, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionRouteKind](value, topologyDiagnosticArchiveRouteKindNames)
}

func topologyDiagnosticArchiveRouteSourceName(value ddsnmpcollector.AcquisitionRouteSource) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveRouteSourceNames)
}

func topologyDiagnosticArchiveParseRouteSource(value string) (ddsnmpcollector.AcquisitionRouteSource, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionRouteSource](value, topologyDiagnosticArchiveRouteSourceNames)
}

func topologyDiagnosticArchiveRouteOutcomeName(value ddsnmpcollector.AcquisitionRouteOutcome) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveRouteOutcomeNames)
}

func topologyDiagnosticArchiveParseRouteOutcome(value string) (ddsnmpcollector.AcquisitionRouteOutcome, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionRouteOutcome](value, topologyDiagnosticArchiveRouteOutcomeNames)
}

func topologyDiagnosticArchiveRouteFailureClassName(value ddsnmpcollector.AcquisitionFailureClass) (string, error) {
	return topologyDiagnosticArchiveEnumName(value, topologyDiagnosticArchiveRouteFailureClassNames)
}

func topologyDiagnosticArchiveParseRouteFailureClass(value string) (ddsnmpcollector.AcquisitionFailureClass, error) {
	return topologyDiagnosticArchiveParseEnum[ddsnmpcollector.AcquisitionFailureClass](value, topologyDiagnosticArchiveRouteFailureClassNames)
}
