// SPDX-License-Identifier: GPL-3.0-or-later

// Package snmptopologydiagnostics defines the repository-internal request and
// JSON report contract for the SNMP topology diagnostic tool.
package snmptopologydiagnostics

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const (
	StateUndetermined = "undetermined"
	StatePresent      = "present"
	StateAbsent       = "absent"
)

type ReadLimits struct {
	MaxCompressedBytes int64
	MaxDecodedBytes    int64
}

type QueryOptions struct {
	CollapseActorsByIP     bool   `json:"collapse_actors_by_ip"`
	EliminateNonIPInferred bool   `json:"eliminate_non_ip_inferred"`
	MapType                string `json:"map_type"`
	InferenceStrategy      string `json:"inference_strategy"`
	ManagedDeviceFocus     string `json:"managed_device_focus"`
	Depth                  string `json:"depth"`
}

type LinkSubject struct {
	SourceIdentity      string `json:"source_identity"`
	DestinationIdentity string `json:"destination_identity"`
	Family              string `json:"family"`
	Protocol            string `json:"protocol"`
	Direction           string `json:"direction"`
}

type ArchiveIdentity struct {
	Format               string `json:"format"`
	Version              uint64 `json:"version"`
	ProducerAgentVersion string `json:"producer_agent_version"`
}

type Validation struct {
	Valid   bool            `json:"valid"`
	Archive ArchiveIdentity `json:"archive"`
}

type CaptureStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type Stage struct {
	State      string `json:"state"`
	Candidates int    `json:"candidates"`
}

type LifecycleCutSummary struct {
	Capture       CaptureStatus `json:"capture"`
	Sequence      uint64        `json:"sequence"`
	CapturedAt    time.Time     `json:"captured_at"`
	Registrations int           `json:"registrations"`
}

type TopologyCutSummary struct {
	Capture      CaptureStatus `json:"capture"`
	Sequence     uint64        `json:"sequence"`
	StartedAt    time.Time     `json:"started_at"`
	PublishedAt  time.Time     `json:"published_at"`
	RecordCount  uint64        `json:"record_count"`
	LogicalBytes uint64        `json:"logical_bytes"`
	Devices      int           `json:"devices"`
	Removed      int           `json:"removed"`
	Selected     int           `json:"selected"`
	Renderable   int           `json:"renderable"`
	Expired      int           `json:"expired"`
}

type Summary struct {
	Archive         ArchiveIdentity       `json:"archive"`
	ProducerScopeID string                `json:"producer_scope_id"`
	Lifecycle       LifecycleCutSummary   `json:"job_lifecycle_cut"`
	Topology        *TopologyCutSummary   `json:"topology_sweep_cut,omitempty"`
	LastAborted     *AbortedSweep         `json:"last_aborted_sweep,omitempty"`
	Registrations   []RegistrationSummary `json:"registrations"`
}

type LifecycleRegistration struct {
	Hostname      string    `json:"hostname"`
	Port          int       `json:"port"`
	SNMPVersion   string    `json:"snmp_version"`
	Phase         string    `json:"phase"`
	Outcome       string    `json:"outcome"`
	CompletedAt   time.Time `json:"completed_at"`
	TopologyReady bool      `json:"topology_ready"`
}

type EvidenceReference struct {
	RegistrationID uint64 `json:"registration_id"`
	Generation     uint64 `json:"generation"`
}

type AcquisitionEvidenceSummary struct {
	Hostname        string      `json:"hostname"`
	SysObjectID     string      `json:"sys_object_id"`
	SysName         string      `json:"sys_name"`
	Vendor          string      `json:"vendor"`
	Model           string      `json:"model"`
	TargetOutcome   string      `json:"target_outcome"`
	TargetAddresses []string    `json:"target_addresses,omitempty"`
	CollectedAt     time.Time   `json:"collected_at"`
	FreshForNanos   int64       `json:"fresh_for_ns"`
	Client          PhaseStatus `json:"client"`
	Connect         PhaseStatus `json:"connect"`
	Profiles        PhaseStatus `json:"profiles"`
	Collection      PhaseStatus `json:"collection"`
	SysUptime       PhaseStatus `json:"sys_uptime"`
	VLANProfiles    PhaseStatus `json:"vlan_profiles"`
	Contexts        int         `json:"contexts"`
	ProfileRuns     int         `json:"profile_runs"`
}

type PhaseStatus struct {
	Outcome string `json:"outcome"`
	Failure string `json:"failure"`
}

type CaptureSummary struct {
	AttemptOrdinal uint64                      `json:"attempt_ordinal"`
	Capture        CaptureStatus               `json:"capture"`
	RecordCount    uint64                      `json:"record_count"`
	LogicalBytes   uint64                      `json:"logical_bytes"`
	Evidence       *AcquisitionEvidenceSummary `json:"evidence,omitempty"`
}

type SweepRegistration struct {
	Selected           bool               `json:"selected"`
	Outcome            string             `json:"outcome"`
	LastAttempt        time.Time          `json:"last_attempt"`
	LastSuccess        time.Time          `json:"last_success"`
	NextRetry          time.Time          `json:"next_retry"`
	RetainedSuccessRef *EvidenceReference `json:"retained_success_ref,omitempty"`
	LatestAttempt      *CaptureSummary    `json:"latest_attempt,omitempty"`
	RetainedSuccess    *CaptureSummary    `json:"retained_success,omitempty"`
	SameAttempt        bool               `json:"same_attempt"`
	HasObservation     bool               `json:"has_observation"`
	ExpiresAt          time.Time          `json:"expires_at"`
	Renderable         bool               `json:"renderable"`
	Expired            bool               `json:"expired"`
}

type RemovedRegistration struct {
	RetainedSuccessRef *EvidenceReference `json:"retained_success_ref,omitempty"`
}

type RegistrationSummary struct {
	RegistrationID uint64                 `json:"registration_id"`
	Lifecycle      *LifecycleRegistration `json:"lifecycle,omitempty"`
	Sweep          *SweepRegistration     `json:"sweep,omitempty"`
	Removed        *RemovedRegistration   `json:"removed,omitempty"`
}

type AbortedSweep struct {
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

type DiagnosticCutInspection struct {
	Capture     CaptureStatus `json:"capture"`
	Sequence    uint64        `json:"sequence"`
	StartedAt   time.Time     `json:"started_at"`
	PublishedAt time.Time     `json:"published_at"`
}

type LifecycleInspection struct {
	Membership Stage                  `json:"membership"`
	Capture    CaptureStatus          `json:"capture"`
	Sequence   uint64                 `json:"sequence"`
	CapturedAt time.Time              `json:"captured_at"`
	Entry      *LifecycleRegistration `json:"entry,omitempty"`
}

type SweepInspection struct {
	DiagnosticCutInspection
	Membership Stage              `json:"membership"`
	Device     *SweepRegistration `json:"device,omitempty"`
}

type RemovedInspection struct {
	Membership Stage                `json:"membership"`
	Device     *RemovedRegistration `json:"device,omitempty"`
}

type CaptureInspection struct {
	Membership Stage           `json:"membership"`
	Evidence   Stage           `json:"evidence"`
	Capture    *CaptureSummary `json:"capture,omitempty"`
}

type GraphActor struct {
	Index        int               `json:"index"`
	ActorID      string            `json:"actor_id"`
	ActorType    string            `json:"actor_type"`
	SegmentKind  string            `json:"segment_kind"`
	Layer        string            `json:"layer"`
	Source       string            `json:"source"`
	IdentityKeys []string          `json:"identity_keys"`
	Match        graph.Match       `json:"match"`
	ParentMatch  *graph.Match      `json:"parent_match,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Details      *ActorDetails     `json:"details,omitempty"`
}

type ActorDetails struct {
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

type ActorInspection struct {
	Membership    Stage        `json:"membership"`
	SelectedIndex int          `json:"selected_index"`
	Candidates    []GraphActor `json:"candidates,omitempty"`
}

type RowInspection struct {
	Membership Stage `json:"membership"`
	Row        int   `json:"row"`
}

type DeviceInspection struct {
	RegistrationID  uint64              `json:"registration_id"`
	Query           QueryOptions        `json:"query"`
	Lifecycle       LifecycleInspection `json:"lifecycle"`
	Sweep           SweepInspection     `json:"sweep"`
	Removed         RemovedInspection   `json:"removed"`
	LatestAttempt   CaptureInspection   `json:"latest_attempt"`
	RetainedSuccess CaptureInspection   `json:"retained_success"`
	SameAttempt     bool                `json:"same_attempt"`
	Observation     Stage               `json:"observation"`
	GraphIdentity   ActorInspection     `json:"graph_identity"`
	TypedIdentity   RowInspection       `json:"typed_identity"`
	GraphStats      map[string]any      `json:"graph_stats,omitempty"`
	LastAborted     *AbortedSweep       `json:"last_aborted_sweep,omitempty"`
}

type SourceFact struct {
	RegistrationID uint64      `json:"registration_id"`
	ContextOrdinal uint32      `json:"context_ordinal"`
	ProfileOrdinal uint32      `json:"profile_ordinal"`
	Metric         *MetricFact `json:"metric,omitempty"`
	BGP            *BGPFact    `json:"bgp,omitempty"`
}

type MetricFact struct {
	RouteOrdinal uint32            `json:"route_ordinal"`
	RowOrdinal   uint32            `json:"row_ordinal"`
	ValueOrdinal uint32            `json:"value_ordinal"`
	Kind         string            `json:"kind"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type BGPFact struct {
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

type SourceCaptureContext struct {
	LatestAttempt   bool              `json:"latest_attempt"`
	RetainedSuccess bool              `json:"retained_success"`
	Capture         CaptureInspection `json:"capture"`
	Facts           []SourceFact      `json:"facts,omitempty"`
}

type SourceContext struct {
	RegistrationID  uint64                 `json:"registration_id"`
	LatestAttempt   CaptureInspection      `json:"latest_attempt"`
	RetainedSuccess CaptureInspection      `json:"retained_success"`
	SameAttempt     bool                   `json:"same_attempt"`
	Captures        []SourceCaptureContext `json:"captures,omitempty"`
}

type SourceInspection struct {
	Contexts []SourceContext `json:"contexts"`
}

type LinkEndpoint = graph.LinkEndpoint

type GraphLink struct {
	Family             string               `json:"family"`
	Layer              string               `json:"layer"`
	Protocol           string               `json:"protocol"`
	Direction          string               `json:"direction"`
	State              string               `json:"state"`
	SourceActorID      string               `json:"source_actor_id"`
	DestinationActorID string               `json:"destination_actor_id"`
	Source             LinkEndpoint         `json:"source"`
	Destination        LinkEndpoint         `json:"destination"`
	DiscoveredAt       *time.Time           `json:"discovered_at,omitempty"`
	LastSeen           *time.Time           `json:"last_seen,omitempty"`
	Display            *graph.LinkDisplay   `json:"display,omitempty"`
	L2                 *graph.LinkL2        `json:"l2,omitempty"`
	Inference          *graph.LinkInference `json:"inference,omitempty"`
	Detail             *LinkDetail          `json:"detail,omitempty"`
}

type LinkDetail struct {
	L3Subnet           *L3SubnetLinkDetail           `json:"l3_subnet,omitempty"`
	L3SubnetMembership *L3SubnetMembershipLinkDetail `json:"l3_subnet_membership,omitempty"`
	OSPF               *OSPFAdjacencyLinkDetail      `json:"ospf,omitempty"`
	BGP                *BGPAdjacencyLinkDetail       `json:"bgp,omitempty"`
}

type L3SubnetLinkDetail struct {
	Source  string `json:"source"`
	SrcIP   string `json:"source_ip"`
	DstIP   string `json:"destination_ip"`
	Subnet  string `json:"subnet"`
	Network string `json:"network"`
	Netmask string `json:"netmask"`
	Prefix  int    `json:"prefix"`
}

type L3SubnetMembershipLinkDetail struct {
	Source     string                        `json:"source"`
	Subnet     string                        `json:"subnet"`
	Network    string                        `json:"network"`
	Netmask    string                        `json:"netmask"`
	Prefix     int                           `json:"prefix"`
	Interfaces []L3SubnetMembershipInterface `json:"interfaces,omitempty"`
}

type L3SubnetMembershipInterface struct {
	MemberIP string `json:"member_ip"`
	IfIndex  int    `json:"if_index"`
	IfName   string `json:"if_name"`
	IfDescr  string `json:"if_descr"`
}

type OSPFAdjacencyLinkDetail struct {
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

type BGPAdjacencyLinkDetail struct {
	Source          string `json:"source"`
	RoutingInstance string `json:"routing_instance"`
	LocalIP         string `json:"local_ip"`
	NeighborIP      string `json:"neighbor_ip"`
	LocalAS         string `json:"local_as"`
	RemoteAS        string `json:"remote_as"`
	LocalIdentifier string `json:"local_identifier"`
	PeerIdentifier  string `json:"peer_identifier"`
}

type GraphLinkInspection struct {
	Membership        Stage           `json:"membership"`
	SourceActors      ActorInspection `json:"source_actors"`
	DestinationActors ActorInspection `json:"destination_actors"`
	SelectedIndex     int             `json:"selected_index"`
	Candidates        []GraphLink     `json:"candidates,omitempty"`
}

type LinkInspection struct {
	Subject       LinkSubject             `json:"subject"`
	Query         QueryOptions            `json:"query"`
	DiagnosticCut DiagnosticCutInspection `json:"diagnostic_cut"`
	Source        SourceInspection        `json:"source"`
	GraphLink     GraphLinkInspection     `json:"graph_link"`
	TypedLink     RowInspection           `json:"typed_link"`
	GraphStats    Stage                   `json:"graph_stats_state"`
	Stats         map[string]any          `json:"graph_stats,omitempty"`
	LastAborted   *AbortedSweep           `json:"last_aborted_sweep,omitempty"`
}
