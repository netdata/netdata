// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import "time"

type Document struct {
	Format   string   `json:"format"`
	Version  uint64   `json:"version"`
	Producer Producer `json:"producer"`
	Snapshot Snapshot `json:"snapshot"`
}

type Producer struct {
	AgentVersion string `json:"agent_version"`
}

type Snapshot struct {
	Lifecycle       Lifecycle `json:"job_lifecycle_cut"`
	ProducerScopeID string    `json:"producer_scope_id"`
	Topology        *Sweep    `json:"topology_sweep_cut,omitempty"`
	LastAborted     *Abort    `json:"last_aborted_sweep,omitempty"`
}

type Lifecycle struct {
	State  string       `json:"capture_state"`
	Reason string       `json:"capture_reason"`
	Cut    LifecycleCut `json:"cut"`
}

type LifecycleCut struct {
	Sequence   uint64           `json:"sequence"`
	CapturedAt time.Time        `json:"captured_at"`
	Entries    []LifecycleEntry `json:"entries,omitempty"`
}

type LifecycleEntry struct {
	RegistrationID uint64          `json:"registration_id"`
	Hostname       string          `json:"hostname"`
	Port           int             `json:"port"`
	SNMPVersion    string          `json:"snmp_version"`
	LastCompleted  LifecycleStatus `json:"last_completed"`
	TopologyReady  bool            `json:"topology_ready"`
}

type LifecycleStatus struct {
	Phase       string    `json:"phase"`
	Outcome     string    `json:"outcome"`
	CompletedAt time.Time `json:"completed_at"`
}

type Sweep struct {
	Sequence      uint64    `json:"sequence"`
	StartedAt     time.Time `json:"started_at"`
	PublishedAt   time.Time `json:"published_at"`
	CaptureState  string    `json:"capture_state"`
	CaptureReason string    `json:"capture_reason"`
	RecordCount   uint64    `json:"record_count"`
	LogicalBytes  uint64    `json:"logical_bytes"`
	Devices       []Device  `json:"devices,omitempty"`
	Removed       []Removed `json:"removed_devices,omitempty"`
}

type Device struct {
	RegistrationID  uint64       `json:"registration_id"`
	Selected        bool         `json:"selected"`
	Outcome         string       `json:"outcome"`
	LastAttempt     time.Time    `json:"last_attempt"`
	LastSuccess     time.Time    `json:"last_success"`
	NextRetry       time.Time    `json:"next_retry"`
	RetainedSuccess *EvidenceRef `json:"retained_success,omitempty"`
	Captures        []Capture    `json:"captures,omitempty"`
	HasObservation  bool         `json:"has_observation"`
	ExpiresAt       time.Time    `json:"expires_at"`
	Renderable      bool         `json:"renderable"`
	Expired         bool         `json:"expired"`
}

type Removed struct {
	RegistrationID  uint64       `json:"registration_id"`
	RetainedSuccess *EvidenceRef `json:"retained_success,omitempty"`
}

type EvidenceRef struct {
	RegistrationID uint64 `json:"registration_id"`
	Generation     uint64 `json:"generation"`
}

type Capture struct {
	Roles          []string             `json:"roles"`
	AttemptOrdinal uint64               `json:"attempt_ordinal"`
	State          string               `json:"capture_state"`
	Reason         string               `json:"capture_reason"`
	RecordCount    uint64               `json:"record_count"`
	LogicalBytes   uint64               `json:"logical_bytes"`
	Evidence       *AcquisitionEvidence `json:"evidence,omitempty"`
}

type Abort struct {
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

type AcquisitionEvidence struct {
	Device             DeviceInput       `json:"device"`
	Target             Target            `json:"target"`
	Client             Phase             `json:"client"`
	Connect            Phase             `json:"connect"`
	Profiles           Phase             `json:"profiles"`
	Collection         Phase             `json:"collection"`
	SysUptime          Phase             `json:"sys_uptime"`
	VLANProfiles       Phase             `json:"vlan_profiles"`
	CollectedAt        time.Time         `json:"collected_at"`
	FreshForNanos      int64             `json:"fresh_for_ns"`
	SysUptimeValue     int64             `json:"sys_uptime_value"`
	CollectionContexts []ContextEvidence `json:"collection_contexts,omitempty"`
}

type DeviceInput struct {
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

type Target struct {
	Outcome   string   `json:"outcome"`
	Addresses []string `json:"addresses,omitempty"`
}

type Phase struct {
	Outcome string `json:"outcome"`
	Failure string `json:"failure"`
}

type ContextEvidence struct {
	Ordinal    uint32            `json:"ordinal"`
	VLANID     string            `json:"vlan_id"`
	VLANName   string            `json:"vlan_name"`
	Client     Phase             `json:"client"`
	Connect    Phase             `json:"connect"`
	Collection Phase             `json:"collection"`
	Profiles   []ProfileEvidence `json:"profiles,omitempty"`
}

type ProfileEvidence struct {
	Identity     ProfileIdentity `json:"identity"`
	Outcome      string          `json:"outcome"`
	FailurePhase string          `json:"failure_phase"`
	Stats        CollectionStats `json:"stats"`
	Execution    *Execution      `json:"execution,omitempty"`
	Routes       []Route         `json:"routes,omitempty"`
	Values       ProfileValues   `json:"values"`
}

type ProfileIdentity struct {
	Ordinal     uint32 `json:"ordinal"`
	RouteDigest string `json:"route_digest"`
}

type Route struct {
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

type ProfileValues struct {
	Metadata  map[string]MetaTag `json:"metadata,omitempty"`
	Tags      map[string]string  `json:"tags,omitempty"`
	Metrics   []MetricValue      `json:"metrics,omitempty"`
	BGPRows   []BGPRowValue      `json:"bgp_rows,omitempty"`
	BGPFailed bool               `json:"bgp_failed"`
}

type MetaTag struct {
	Value        string `json:"value"`
	IsExactMatch bool   `json:"is_exact_match"`
}

type MetricValue struct {
	RouteOrdinal uint32            `json:"route_ordinal"`
	RowOrdinal   uint32            `json:"row_ordinal"`
	ValueOrdinal uint32            `json:"value_ordinal"`
	Kind         string            `json:"kind"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type BGPRowValue struct {
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

type CollectionStats struct {
	Timing     TimingStats     `json:"timing"`
	SNMP       SNMPStats       `json:"snmp"`
	Metrics    MetricStats     `json:"metrics"`
	TableCache TableCacheStats `json:"table_cache"`
	Errors     ErrorStats      `json:"errors"`
}

type TimingStats struct {
	PreparationNanos    int64 `json:"preparation_ns,omitempty"`
	ScalarNanos         int64 `json:"scalar_ns"`
	TableNanos          int64 `json:"table_ns"`
	LicensingNanos      int64 `json:"licensing_ns"`
	BGPNanos            int64 `json:"bgp_ns"`
	VirtualMetricsNanos int64 `json:"virtual_metrics_ns"`
}

type SNMPStats struct {
	GetRequests  int64 `json:"get_requests"`
	GetOIDs      int64 `json:"get_oids"`
	WalkRequests int64 `json:"walk_requests"`
	WalkPDUs     int64 `json:"walk_pdus"`
	TablesWalked int64 `json:"tables_walked"`
	TablesCached int64 `json:"tables_cached"`
}

type MetricStats struct {
	Scalar    int64 `json:"scalar"`
	Table     int64 `json:"table"`
	Virtual   int64 `json:"virtual"`
	Licensing int64 `json:"licensing"`
	BGP       int64 `json:"bgp"`
	Tables    int64 `json:"tables"`
	Rows      int64 `json:"rows"`
}

type TableCacheStats struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
}

type ErrorStats struct {
	ProcessingPreparation int64 `json:"processing_preparation,omitempty"`
	SNMP                  int64 `json:"snmp"`
	ProcessingScalar      int64 `json:"processing_scalar"`
	ProcessingTable       int64 `json:"processing_table"`
	ProcessingLicensing   int64 `json:"processing_licensing"`
	ProcessingBGP         int64 `json:"processing_bgp"`
	MissingOIDs           int64 `json:"missing_oids"`
}

type Execution struct {
	Preparation Preparation `json:"preparation"`
	Walks       []Walk      `json:"walks"`
}

type Preparation struct {
	ElapsedNanos     int64 `json:"elapsed_ns"`
	GetRequests      int64 `json:"get_requests"`
	GetOIDs          int64 `json:"get_oids"`
	SNMPErrors       int64 `json:"snmp_errors"`
	MissingOIDs      int64 `json:"missing_oids"`
	ProcessingErrors int64 `json:"processing_errors"`
}

type Walk struct {
	RootOID      string `json:"root_oid"`
	ElapsedNanos int64  `json:"elapsed_ns"`
	Failed       bool   `json:"failed"`
}
