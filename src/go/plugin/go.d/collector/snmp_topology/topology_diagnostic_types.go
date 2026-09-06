// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

const (
	diagnosticStateUndetermined = "undetermined"
	diagnosticStatePresent      = "present"
	diagnosticStateAbsent       = "absent"
)

type DiagnosticQueryOptions struct {
	CollapseActorsByIP     bool   `json:"collapse_actors_by_ip"`
	EliminateNonIPInferred bool   `json:"eliminate_non_ip_inferred"`
	MapType                string `json:"map_type"`
	InferenceStrategy      string `json:"inference_strategy"`
	ManagedDeviceFocus     string `json:"managed_device_focus"`
	Depth                  string `json:"depth"`
}

type DiagnosticLinkSubject struct {
	SourceIdentity      string `json:"source_identity"`
	DestinationIdentity string `json:"destination_identity"`
	Family              string `json:"family"`
	Protocol            string `json:"protocol"`
	Direction           string `json:"direction"`
}

type DiagnosticArchiveIdentity struct {
	Format               string `json:"format"`
	Version              uint64 `json:"version"`
	ProducerAgentVersion string `json:"producer_agent_version"`
}

type DiagnosticValidation struct {
	Valid   bool                      `json:"valid"`
	Archive DiagnosticArchiveIdentity `json:"archive"`
}

type diagnosticCaptureStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type diagnosticStageReport struct {
	State      string `json:"state"`
	Candidates int    `json:"candidates"`
}

type diagnosticLifecycleCutSummary struct {
	Capture       diagnosticCaptureStatus `json:"capture"`
	Sequence      uint64                  `json:"sequence"`
	CapturedAt    time.Time               `json:"captured_at"`
	Registrations int                     `json:"registrations"`
}

type diagnosticTopologyCutSummary struct {
	Capture      diagnosticCaptureStatus `json:"capture"`
	Sequence     uint64                  `json:"sequence"`
	StartedAt    time.Time               `json:"started_at"`
	PublishedAt  time.Time               `json:"published_at"`
	RecordCount  uint64                  `json:"record_count"`
	LogicalBytes uint64                  `json:"logical_bytes"`
	Devices      int                     `json:"devices"`
	Removed      int                     `json:"removed"`
	Selected     int                     `json:"selected"`
	Renderable   int                     `json:"renderable"`
	Expired      int                     `json:"expired"`
}

type DiagnosticSummary struct {
	Archive         DiagnosticArchiveIdentity       `json:"archive"`
	ProducerScopeID string                          `json:"producer_scope_id"`
	Lifecycle       diagnosticLifecycleCutSummary   `json:"job_lifecycle_cut"`
	Topology        *diagnosticTopologyCutSummary   `json:"topology_sweep_cut,omitempty"`
	LastAborted     *diagnosticAbortedSweep         `json:"last_aborted_sweep,omitempty"`
	Registrations   []diagnosticRegistrationSummary `json:"registrations"`
}

type diagnosticLifecycleRegistration struct {
	Hostname      string    `json:"hostname"`
	Port          int       `json:"port"`
	SNMPVersion   string    `json:"snmp_version"`
	Phase         string    `json:"phase"`
	Outcome       string    `json:"outcome"`
	CompletedAt   time.Time `json:"completed_at"`
	TopologyReady bool      `json:"topology_ready"`
}

type diagnosticEvidenceReference struct {
	RegistrationID uint64 `json:"registration_id"`
	Generation     uint64 `json:"generation"`
}

type diagnosticAcquisitionEvidenceSummary struct {
	Hostname        string                `json:"hostname"`
	SysObjectID     string                `json:"sys_object_id"`
	SysName         string                `json:"sys_name"`
	Vendor          string                `json:"vendor"`
	Model           string                `json:"model"`
	TargetOutcome   string                `json:"target_outcome"`
	TargetAddresses []string              `json:"target_addresses,omitempty"`
	CollectedAt     time.Time             `json:"collected_at"`
	FreshForNanos   int64                 `json:"fresh_for_ns"`
	Client          diagnosticPhaseStatus `json:"client"`
	Connect         diagnosticPhaseStatus `json:"connect"`
	Profiles        diagnosticPhaseStatus `json:"profiles"`
	Collection      diagnosticPhaseStatus `json:"collection"`
	SysUptime       diagnosticPhaseStatus `json:"sys_uptime"`
	VLANProfiles    diagnosticPhaseStatus `json:"vlan_profiles"`
	Contexts        int                   `json:"contexts"`
	ProfileRuns     int                   `json:"profile_runs"`
}

type diagnosticPhaseStatus struct {
	Outcome string `json:"outcome"`
	Failure string `json:"failure"`
}

type diagnosticCaptureSummary struct {
	AttemptOrdinal uint64                                `json:"attempt_ordinal"`
	Capture        diagnosticCaptureStatus               `json:"capture"`
	RecordCount    uint64                                `json:"record_count"`
	LogicalBytes   uint64                                `json:"logical_bytes"`
	Evidence       *diagnosticAcquisitionEvidenceSummary `json:"evidence,omitempty"`
}

type diagnosticSweepRegistration struct {
	Selected           bool                         `json:"selected"`
	Outcome            string                       `json:"outcome"`
	LastAttempt        time.Time                    `json:"last_attempt"`
	LastSuccess        time.Time                    `json:"last_success"`
	NextRetry          time.Time                    `json:"next_retry"`
	RetainedSuccessRef *diagnosticEvidenceReference `json:"retained_success_ref,omitempty"`
	LatestAttempt      *diagnosticCaptureSummary    `json:"latest_attempt,omitempty"`
	RetainedSuccess    *diagnosticCaptureSummary    `json:"retained_success,omitempty"`
	SameAttempt        bool                         `json:"same_attempt"`
	HasObservation     bool                         `json:"has_observation"`
	ExpiresAt          time.Time                    `json:"expires_at"`
	Renderable         bool                         `json:"renderable"`
	Expired            bool                         `json:"expired"`
}

type diagnosticRemovedRegistration struct {
	RetainedSuccessRef *diagnosticEvidenceReference `json:"retained_success_ref,omitempty"`
}

type diagnosticRegistrationSummary struct {
	RegistrationID uint64                           `json:"registration_id"`
	Lifecycle      *diagnosticLifecycleRegistration `json:"lifecycle,omitempty"`
	Sweep          *diagnosticSweepRegistration     `json:"sweep,omitempty"`
	Removed        *diagnosticRemovedRegistration   `json:"removed,omitempty"`
}

type diagnosticAbortedSweep struct {
	Sequence              uint64    `json:"sequence"`
	StartedAt             time.Time `json:"started_at"`
	AbortedAt             time.Time `json:"aborted_at"`
	Reason                string    `json:"reason"`
	Phase                 string    `json:"phase"`
	ActiveRegistrationID  uint64    `json:"active_registration_id,omitempty"`
	HasActiveRegistration bool      `json:"has_active_registration"`
	RegistrationCount     int       `json:"registration_count"`
	SelectedCount         int       `json:"selected_count"`
}

type diagnosticCutInspection struct {
	Capture     diagnosticCaptureStatus `json:"capture"`
	Sequence    uint64                  `json:"sequence"`
	StartedAt   time.Time               `json:"started_at"`
	PublishedAt time.Time               `json:"published_at"`
}

type diagnosticLifecycleInspection struct {
	Membership diagnosticStageReport            `json:"membership"`
	Capture    diagnosticCaptureStatus          `json:"capture"`
	Sequence   uint64                           `json:"sequence"`
	CapturedAt time.Time                        `json:"captured_at"`
	Entry      *diagnosticLifecycleRegistration `json:"entry,omitempty"`
}

type diagnosticSweepInspection struct {
	diagnosticCutInspection
	Membership diagnosticStageReport        `json:"membership"`
	Device     *diagnosticSweepRegistration `json:"device,omitempty"`
}

type diagnosticRemovedInspection struct {
	Membership diagnosticStageReport          `json:"membership"`
	Device     *diagnosticRemovedRegistration `json:"device,omitempty"`
}

type diagnosticCaptureInspection struct {
	Membership diagnosticStageReport     `json:"membership"`
	Evidence   diagnosticStageReport     `json:"evidence"`
	Capture    *diagnosticCaptureSummary `json:"capture,omitempty"`
}

type diagnosticDeviceCaptureInspection struct {
	diagnosticCaptureInspection
	CollectionContexts []diagnosticContextAccounting `json:"collection_contexts,omitempty"`
}

type diagnosticContextAccounting struct {
	Ordinal  uint32                        `json:"ordinal"`
	VLANID   string                        `json:"vlan_id"`
	VLANName string                        `json:"vlan_name"`
	Profiles []diagnosticProfileAccounting `json:"profiles"`
}

type diagnosticProfileAccounting struct {
	Identity     snmpdiag.ProfileIdentity `json:"identity"`
	Outcome      string                   `json:"outcome"`
	FailurePhase string                   `json:"failure_phase"`
	Stats        snmpdiag.CollectionStats `json:"stats"`
	Execution    *snmpdiag.Execution      `json:"execution,omitempty"`
}

type diagnosticGraphActor struct {
	Index        int                     `json:"index"`
	ActorID      string                  `json:"actor_id"`
	ActorType    string                  `json:"actor_type"`
	SegmentKind  string                  `json:"segment_kind"`
	Layer        string                  `json:"layer"`
	Source       string                  `json:"source"`
	IdentityKeys []string                `json:"identity_keys"`
	Match        graph.Match             `json:"match"`
	ParentMatch  *graph.Match            `json:"parent_match,omitempty"`
	Labels       map[string]string       `json:"labels,omitempty"`
	Details      *diagnosticActorDetails `json:"details,omitempty"`
}

type diagnosticActorDetails struct {
	DisplayName           string   `json:"display_name,omitempty"`
	DisplaySource         string   `json:"display_source,omitempty"`
	ParentDevices         []string `json:"parent_devices,omitempty"`
	ManagementIP          string   `json:"management_ip,omitempty"`
	ManagementAddresses   []string `json:"management_addresses,omitempty"`
	Protocols             []string `json:"protocols,omitempty"`
	Capabilities          []string `json:"capabilities,omitempty"`
	CapabilitiesSupported []string `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string `json:"capabilities_enabled,omitempty"`
	SysDescr              string   `json:"sys_descr,omitempty"`
	SysContact            string   `json:"sys_contact,omitempty"`
	SysLocation           string   `json:"sys_location,omitempty"`
	Vendor                string   `json:"vendor,omitempty"`
	Model                 string   `json:"model,omitempty"`
	OSPFRouterID          string   `json:"ospf_router_id,omitempty"`
}

type diagnosticActorInspection struct {
	Membership    diagnosticStageReport  `json:"membership"`
	SelectedIndex int                    `json:"selected_index"`
	Candidates    []diagnosticGraphActor `json:"candidates,omitempty"`
}

type diagnosticRowInspection struct {
	Membership diagnosticStageReport `json:"membership"`
	Row        int                   `json:"row"`
}

type DiagnosticDeviceInspection struct {
	RegistrationID  uint64                            `json:"registration_id"`
	Query           DiagnosticQueryOptions            `json:"query"`
	Lifecycle       diagnosticLifecycleInspection     `json:"lifecycle"`
	Sweep           diagnosticSweepInspection         `json:"sweep"`
	Removed         diagnosticRemovedInspection       `json:"removed"`
	LatestAttempt   diagnosticDeviceCaptureInspection `json:"latest_attempt"`
	RetainedSuccess diagnosticDeviceCaptureInspection `json:"retained_success"`
	SameAttempt     bool                              `json:"same_attempt"`
	Observation     diagnosticStageReport             `json:"observation"`
	GraphIdentity   diagnosticActorInspection         `json:"graph_identity"`
	TypedIdentity   diagnosticRowInspection           `json:"typed_identity"`
	GraphStats      map[string]any                    `json:"graph_stats,omitempty"`
	LastAborted     *diagnosticAbortedSweep           `json:"last_aborted_sweep,omitempty"`
}

type diagnosticSourceFact struct {
	RegistrationID uint64                `json:"registration_id"`
	ContextOrdinal uint32                `json:"context_ordinal"`
	ProfileOrdinal uint32                `json:"profile_ordinal"`
	Metric         *diagnosticMetricFact `json:"metric,omitempty"`
	BGP            *diagnosticBGPFact    `json:"bgp,omitempty"`
}

type diagnosticMetricFact struct {
	RouteOrdinal uint32            `json:"route_ordinal"`
	RowOrdinal   uint32            `json:"row_ordinal"`
	ValueOrdinal uint32            `json:"value_ordinal"`
	Kind         string            `json:"kind"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type diagnosticBGPFact struct {
	RouteOrdinal    uint32            `json:"route_ordinal"`
	RowOrdinal      uint32            `json:"row_ordinal"`
	ValueOrdinal    uint32            `json:"value_ordinal"`
	OriginProfileID string            `json:"origin_profile_id"`
	Table           string            `json:"table"`
	RowKey          string            `json:"row_key"`
	StructuralID    string            `json:"structural_id"`
	Kind            string            `json:"kind"`
	RoutingInstance string            `json:"routing_instance"`
	Neighbor        string            `json:"neighbor"`
	RemoteAS        string            `json:"remote_as"`
	LocalAddress    string            `json:"local_address"`
	LocalAS         string            `json:"local_as"`
	LocalIdentifier string            `json:"local_identifier"`
	PeerIdentifier  string            `json:"peer_identifier"`
	PeerType        string            `json:"peer_type"`
	BGPVersion      string            `json:"bgp_version"`
	Description     string            `json:"description"`
	AdminHas        bool              `json:"admin_has"`
	AdminEnabled    bool              `json:"admin_enabled"`
	StateHas        bool              `json:"state_has"`
	State           string            `json:"state"`
	StateRaw        string            `json:"state_raw"`
	EstablishedHas  bool              `json:"established_has"`
	Established     int64             `json:"established"`
	UpdateAgeHas    bool              `json:"update_age_has"`
	UpdateAge       int64             `json:"update_age"`
	Tags            map[string]string `json:"tags,omitempty"`
}

type diagnosticSourceCaptureContext struct {
	LatestAttempt   bool                        `json:"latest_attempt"`
	RetainedSuccess bool                        `json:"retained_success"`
	Capture         diagnosticCaptureInspection `json:"capture"`
	Facts           []diagnosticSourceFact      `json:"facts,omitempty"`
}

type diagnosticSourceContext struct {
	RegistrationID  uint64                           `json:"registration_id"`
	LatestAttempt   diagnosticCaptureInspection      `json:"latest_attempt"`
	RetainedSuccess diagnosticCaptureInspection      `json:"retained_success"`
	SameAttempt     bool                             `json:"same_attempt"`
	Captures        []diagnosticSourceCaptureContext `json:"captures,omitempty"`
}

type diagnosticSourceInspection struct {
	Contexts []diagnosticSourceContext `json:"contexts"`
}

type diagnosticLinkEndpoint = graph.LinkEndpoint

type diagnosticGraphLink struct {
	Family             string                 `json:"family"`
	Layer              string                 `json:"layer"`
	Protocol           string                 `json:"protocol"`
	Direction          string                 `json:"direction"`
	State              string                 `json:"state"`
	SourceActorID      string                 `json:"source_actor_id"`
	DestinationActorID string                 `json:"destination_actor_id"`
	Source             diagnosticLinkEndpoint `json:"source"`
	Destination        diagnosticLinkEndpoint `json:"destination"`
	DiscoveredAt       *time.Time             `json:"discovered_at,omitempty"`
	LastSeen           *time.Time             `json:"last_seen,omitempty"`
	Display            *graph.LinkDisplay     `json:"display,omitempty"`
	L2                 *graph.LinkL2          `json:"l2,omitempty"`
	Inference          *graph.LinkInference   `json:"inference,omitempty"`
	Detail             *diagnosticLinkDetail  `json:"detail,omitempty"`
}

type diagnosticLinkDetail struct {
	L3Subnet           *diagnosticL3SubnetLinkDetail           `json:"l3_subnet,omitempty"`
	L3SubnetMembership *diagnosticL3SubnetMembershipLinkDetail `json:"l3_subnet_membership,omitempty"`
	OSPF               *diagnosticOSPFAdjacencyLinkDetail      `json:"ospf,omitempty"`
	BGP                *diagnosticBGPAdjacencyLinkDetail       `json:"bgp,omitempty"`
}

type diagnosticL3SubnetLinkDetail struct {
	Source  string `json:"source"`
	SrcIP   string `json:"source_ip"`
	DstIP   string `json:"destination_ip"`
	Subnet  string `json:"subnet"`
	Network string `json:"network"`
	Netmask string `json:"netmask"`
	Prefix  int    `json:"prefix"`
}

type diagnosticL3SubnetMembershipLinkDetail struct {
	Source     string                                  `json:"source"`
	Subnet     string                                  `json:"subnet"`
	Network    string                                  `json:"network"`
	Netmask    string                                  `json:"netmask"`
	Prefix     int                                     `json:"prefix"`
	Interfaces []diagnosticL3SubnetMembershipInterface `json:"interfaces,omitempty"`
}

type diagnosticL3SubnetMembershipInterface struct {
	MemberIP string `json:"member_ip"`
	IfIndex  int    `json:"if_index"`
	IfName   string `json:"if_name"`
	IfDescr  string `json:"if_descr"`
}

type diagnosticOSPFAdjacencyLinkDetail struct {
	Source           string `json:"source"`
	LocalRouterID    string `json:"local_router_id"`
	NeighborRouterID string `json:"neighbor_router_id"`
	LocalIP          string `json:"local_ip"`
	NeighborIP       string `json:"neighbor_ip"`
	AddresslessIndex string `json:"addressless_index"`
	Subnet           string `json:"subnet"`
	Network          string `json:"network"`
	Netmask          string `json:"netmask"`
	Prefix           int    `json:"prefix"`
}

type diagnosticBGPAdjacencyLinkDetail struct {
	Source          string `json:"source"`
	RoutingInstance string `json:"routing_instance"`
	LocalIP         string `json:"local_ip"`
	NeighborIP      string `json:"neighbor_ip"`
	LocalAS         string `json:"local_as"`
	RemoteAS        string `json:"remote_as"`
	LocalIdentifier string `json:"local_identifier"`
	PeerIdentifier  string `json:"peer_identifier"`
}

type diagnosticGraphLinkInspection struct {
	Membership        diagnosticStageReport     `json:"membership"`
	SourceActors      diagnosticActorInspection `json:"source_actors"`
	DestinationActors diagnosticActorInspection `json:"destination_actors"`
	SelectedIndex     int                       `json:"selected_index"`
	Candidates        []diagnosticGraphLink     `json:"candidates,omitempty"`
}

type DiagnosticLinkInspection struct {
	Subject       DiagnosticLinkSubject         `json:"subject"`
	Query         DiagnosticQueryOptions        `json:"query"`
	DiagnosticCut diagnosticCutInspection       `json:"diagnostic_cut"`
	Source        diagnosticSourceInspection    `json:"source"`
	GraphLink     diagnosticGraphLinkInspection `json:"graph_link"`
	TypedLink     diagnosticRowInspection       `json:"typed_link"`
	GraphStats    diagnosticStageReport         `json:"graph_stats_state"`
	Stats         map[string]any                `json:"graph_stats,omitempty"`
	LastAborted   *diagnosticAbortedSweep       `json:"last_aborted_sweep,omitempty"`
}
