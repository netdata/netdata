// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	CapabilitySemanticReplay = "semantic_replay"

	KindSemanticDevice  = "semantic_device"
	KindSemanticProfile = "semantic_profile"
	KindSemanticShard   = "semantic_shard"
	KindObservation     = "observation"
)

const (
	SemanticSectionDevice      = "device"
	SemanticSectionProfiles    = "profiles"
	SemanticSectionEvents      = "semantic_events"
	SemanticSectionObservation = "observation"
)

const (
	SemanticPhaseProfileTags uint32 = iota + 1
	SemanticPhaseTopologyMetrics
	SemanticPhaseBGPPeers
	SemanticPhaseVLANOutcome
	SemanticPhaseVLANProfileTags
	SemanticPhaseVLANMetrics
)

type SemanticDeviceV1 struct {
	CaptureID           uint64            `json:"capture_id"`
	Registration        uint64            `json:"registration"`
	CollectedAt         string            `json:"collected_at"`
	FreshForNanoseconds int64             `json:"fresh_for_nanoseconds"`
	AgentID             string            `json:"agent_id"`
	LocalDevice         SemanticDeviceDTO `json:"local_device"`
	TargetManagementIPs []string          `json:"target_management_ips"`
	SysUptime           int64             `json:"sys_uptime"`
}

func (d SemanticDeviceV1) Validate() error {
	if d.CaptureID == 0 || d.Registration == 0 {
		return errors.New("semantic device owner identifiers must be nonzero")
	}
	if err := validateCanonicalTime(d.CollectedAt); err != nil {
		return fmt.Errorf("collected_at: %w", err)
	}
	if d.FreshForNanoseconds <= 0 {
		return errors.New("fresh_for_nanoseconds must be positive")
	}
	if strings.TrimSpace(d.AgentID) == "" {
		return errors.New("agent_id is required")
	}
	if err := d.LocalDevice.Validate(); err != nil {
		return fmt.Errorf("local_device: %w", err)
	}
	previous := ""
	for i, value := range d.TargetManagementIPs {
		addr, err := netip.ParseAddr(value)
		if err != nil || addr.String() != value {
			return fmt.Errorf("target_management_ips[%d] is not a canonical IP address", i)
		}
		if previous >= value {
			return errors.New("target_management_ips must be strictly ordered")
		}
		previous = value
	}
	return nil
}

type SemanticDeviceDTO struct {
	ChassisID             string                        `json:"chassis_id"`
	ChassisIDType         string                        `json:"chassis_id_type"`
	SysObjectID           string                        `json:"sys_object_id,omitempty"`
	SysName               string                        `json:"sys_name,omitempty"`
	SysDescr              string                        `json:"sys_descr,omitempty"`
	SysContact            string                        `json:"sys_contact,omitempty"`
	SysLocation           string                        `json:"sys_location,omitempty"`
	SysUptime             int64                         `json:"sys_uptime,omitempty"`
	SerialNumber          string                        `json:"serial_number,omitempty"`
	SoftwareVersion       string                        `json:"software_version,omitempty"`
	FirmwareVersion       string                        `json:"firmware_version,omitempty"`
	HardwareVersion       string                        `json:"hardware_version,omitempty"`
	ManagementIP          string                        `json:"management_ip,omitempty"`
	ManagementAddresses   []SemanticManagementAddressV1 `json:"management_addresses,omitempty"`
	AgentID               string                        `json:"agent_id,omitempty"`
	AgentJobID            string                        `json:"agent_job_id,omitempty"`
	NetdataHostID         string                        `json:"netdata_host_id,omitempty"`
	ChartIDPrefix         string                        `json:"chart_id_prefix,omitempty"`
	ChartContextPrefix    string                        `json:"chart_context_prefix,omitempty"`
	DeviceCharts          map[string]string             `json:"device_charts,omitempty"`
	InterfaceCharts       map[string]SemanticChartRefV1 `json:"interface_charts,omitempty"`
	Vendor                string                        `json:"vendor,omitempty"`
	Model                 string                        `json:"model,omitempty"`
	OSPFRouterID          string                        `json:"ospf_router_id,omitempty"`
	Capabilities          []string                      `json:"capabilities,omitempty"`
	CapabilitiesSupported []string                      `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string                      `json:"capabilities_enabled,omitempty"`
	Labels                map[string]string             `json:"labels,omitempty"`
	Discovered            bool                          `json:"discovered,omitempty"`
}

func (d SemanticDeviceDTO) Validate() error {
	return nil
}

type SemanticManagementAddressV1 struct {
	Address     string `json:"address"`
	AddressType string `json:"address_type,omitempty"`
	IfSubtype   string `json:"if_subtype,omitempty"`
	IfID        string `json:"if_id,omitempty"`
	OID         string `json:"oid,omitempty"`
	Source      string `json:"source,omitempty"`
}

type SemanticChartRefV1 struct {
	ChartIDSuffix    string   `json:"chart_id_suffix,omitempty"`
	AvailableMetrics []string `json:"available_metrics,omitempty"`
}

type SemanticProfileV1 struct {
	CaptureID        uint64                      `json:"capture_id"`
	Registration     uint64                      `json:"registration"`
	Role             string                      `json:"role"`
	VLANID           string                      `json:"vlan_id,omitempty"`
	Ordinal          uint32                      `json:"ordinal"`
	Origin           string                      `json:"origin"`
	Definition       ProfileDefinitionEvidenceV1 `json:"definition"`
	DefinitionSHA256 string                      `json:"definition_sha256"`
}

func (p SemanticProfileV1) Validate() error {
	if p.CaptureID == 0 || p.Registration == 0 {
		return errors.New("semantic profile owner identifiers must be nonzero")
	}
	if p.Role != "main" && p.Role != "vlan" {
		return fmt.Errorf("unsupported profile role %q", p.Role)
	}
	if p.Role == "vlan" && strings.TrimSpace(p.VLANID) == "" {
		return errors.New("vlan profile requires vlan_id")
	}
	if p.Role == "main" && p.VLANID != "" {
		return errors.New("main profile must not declare vlan_id")
	}
	if err := validatePortableOrigin(p.Origin); err != nil {
		return err
	}
	if err := p.Definition.Validate(); err != nil {
		return err
	}
	data, err := CanonicalBytes(p.Definition)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != p.DefinitionSHA256 {
		return errors.New("profile definition digest mismatch")
	}
	return nil
}

type ProfileDefinitionEvidenceV1 struct {
	Encoding            string   `json:"encoding"`
	EffectiveDefinition string   `json:"effective_definition"`
	BGPOriginProfileIDs []string `json:"bgp_origin_profile_ids,omitempty"`
}

func (e ProfileDefinitionEvidenceV1) Validate() error {
	if e.Encoding != "netdata.ddsnmp-topology-profile-evidence-yaml/v1" {
		return fmt.Errorf("unsupported profile evidence encoding %q", e.Encoding)
	}
	if strings.TrimSpace(e.EffectiveDefinition) == "" {
		return errors.New("effective profile definition is empty")
	}
	if strings.ContainsRune(e.EffectiveDefinition, 0) {
		return errors.New("effective profile definition contains NUL")
	}
	for i, origin := range e.BGPOriginProfileIDs {
		if origin == "" {
			continue
		}
		if err := validatePortableOrigin(origin); err != nil {
			return fmt.Errorf("bgp_origin_profile_ids[%d]: %w", i, err)
		}
	}
	return nil
}

func ProfileDefinitionDigest(evidence ProfileDefinitionEvidenceV1) (string, error) {
	if err := evidence.Validate(); err != nil {
		return "", err
	}
	data, err := CanonicalBytes(evidence)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

type SemanticShardV1 struct {
	Geometry ShardGeometryV1    `json:"geometry"`
	Records  []SemanticRecordV1 `json:"records"`
}

func (s SemanticShardV1) Validate() error {
	if err := s.Geometry.Validate(); err != nil {
		return err
	}
	if uint64(len(s.Records)) != s.Geometry.RecordCount {
		return fmt.Errorf("record count %d does not match geometry %d", len(s.Records), s.Geometry.RecordCount)
	}
	for i, record := range s.Records {
		if record.Ordinal != s.Geometry.FirstOrdinal+uint64(i) {
			return fmt.Errorf("records[%d] has ordinal %d, expected %d", i, record.Ordinal, s.Geometry.FirstOrdinal+uint64(i))
		}
		if err := record.Validate(); err != nil {
			return fmt.Errorf("records[%d]: %w", i, err)
		}
	}
	return nil
}

type SemanticRecordV1 struct {
	Ordinal      uint64                       `json:"ordinal"`
	Kind         string                       `json:"kind"`
	Profile      uint32                       `json:"profile"`
	Metadata     map[string]SemanticMetaTagV1 `json:"metadata,omitempty"`
	Tags         map[string]string            `json:"tags,omitempty"`
	TopologyKind string                       `json:"topology_kind,omitempty"`
	BGP          *SemanticBGPPeerV1           `json:"bgp,omitempty"`
	VLANID       string                       `json:"vlan_id,omitempty"`
	VLANName     string                       `json:"vlan_name,omitempty"`
	State        TerminalState                `json:"state,omitempty"`
	Reason       string                       `json:"reason,omitempty"`
}

func (r SemanticRecordV1) Validate() error {
	switch r.Kind {
	case "profile_tags":
		if r.TopologyKind != "" || r.BGP != nil || r.State != "" || r.Reason != "" || r.VLANName != "" {
			return errors.New("profile_tags record contains fields for another record kind")
		}
	case "topology_metric":
		if r.TopologyKind == "" || r.Metadata != nil || r.BGP != nil || r.State != "" || r.Reason != "" {
			return errors.New("invalid topology_metric record")
		}
	case "bgp_peer":
		if r.BGP == nil || r.Metadata != nil || r.Tags != nil || r.TopologyKind != "" || r.State != "" || r.Reason != "" ||
			r.VLANID != "" || r.VLANName != "" {
			return errors.New("invalid bgp_peer record")
		}
		if err := r.BGP.Validate(); err != nil {
			return err
		}
	case "bgp_outcome":
		if (r.State != StateSuccess && r.State != StateFailed) || r.Metadata != nil || r.Tags != nil ||
			r.TopologyKind != "" || r.BGP != nil || r.VLANID != "" || r.VLANName != "" {
			return errors.New("invalid bgp_outcome record")
		}
		if r.State == StateSuccess && r.Reason != "" {
			return errors.New("successful BGP outcome must not contain a reason")
		}
		if r.State == StateFailed && r.Reason != "collection_failed" {
			return errors.New("failed BGP outcome requires collection_failed reason")
		}
	case "vlan_outcome":
		if strings.TrimSpace(r.VLANID) == "" || !r.State.valid() || r.Profile != 0 || r.Metadata != nil || r.Tags != nil ||
			r.TopologyKind != "" || r.BGP != nil {
			return errors.New("invalid vlan_outcome record")
		}
		if r.State == StateSuccess && r.Reason != "" {
			return errors.New("successful vlan outcome must not contain a reason")
		}
		if r.State != StateSuccess && r.Reason == "" {
			return errors.New("unsuccessful vlan outcome requires a reason")
		}
		if r.Reason != "" {
			if err := validateID("vlan outcome reason", r.Reason); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported semantic record kind %q", r.Kind)
	}
	return nil
}

type SemanticMetaTagV1 struct {
	Value        string `json:"value"`
	IsExactMatch bool   `json:"is_exact_match"`
}

type SemanticBGPPeerV1 struct {
	OriginProfileID       string                  `json:"origin_profile_id,omitempty"`
	Table                 string                  `json:"table,omitempty"`
	RowKey                string                  `json:"row_key,omitempty"`
	StructuralID          string                  `json:"structural_id,omitempty"`
	Kind                  string                  `json:"kind"`
	RoutingInstance       string                  `json:"routing_instance,omitempty"`
	Neighbor              string                  `json:"neighbor,omitempty"`
	RemoteAS              string                  `json:"remote_as,omitempty"`
	LocalAddress          string                  `json:"local_address,omitempty"`
	LocalAS               string                  `json:"local_as,omitempty"`
	LocalIdentifier       string                  `json:"local_identifier,omitempty"`
	PeerIdentifier        string                  `json:"peer_identifier,omitempty"`
	PeerType              string                  `json:"peer_type,omitempty"`
	BGPVersion            string                  `json:"bgp_version,omitempty"`
	Description           string                  `json:"description,omitempty"`
	AdminEnabled          SemanticOptionalBoolV1  `json:"admin_enabled"`
	State                 SemanticOptionalTextV1  `json:"state"`
	EstablishedUptime     SemanticOptionalInt64V1 `json:"established_uptime"`
	LastReceivedUpdateAge SemanticOptionalInt64V1 `json:"last_received_update_age"`
	Tags                  map[string]string       `json:"tags,omitempty"`
}

func (p SemanticBGPPeerV1) Validate() error {
	if p.Kind == "" {
		return errors.New("BGP row kind is required")
	}
	if !p.AdminEnabled.Has && p.AdminEnabled.Value {
		return errors.New("absent BGP admin_enabled must carry the zero value")
	}
	if !p.State.Has && (p.State.Value != "" || p.State.Raw != "") {
		return errors.New("absent BGP state must carry zero values")
	}
	if !p.EstablishedUptime.Has && p.EstablishedUptime.Value != 0 {
		return errors.New("absent BGP established_uptime must carry the zero value")
	}
	if !p.LastReceivedUpdateAge.Has && p.LastReceivedUpdateAge.Value != 0 {
		return errors.New("absent BGP last_received_update_age must carry the zero value")
	}
	return nil
}

type SemanticOptionalBoolV1 struct {
	Has   bool `json:"has"`
	Value bool `json:"value"`
}

type SemanticOptionalTextV1 struct {
	Has   bool   `json:"has"`
	Value string `json:"value,omitempty"`
	Raw   string `json:"raw,omitempty"`
}

type SemanticOptionalInt64V1 struct {
	Has   bool  `json:"has"`
	Value int64 `json:"value"`
}

type ObservationCountsV1 struct {
	L2Observations uint64 `json:"l2_observations"`
	L3Interfaces   uint64 `json:"l3_interfaces"`
	OSPFNeighbors  uint64 `json:"ospf_neighbors"`
	BGPPeers       uint64 `json:"bgp_peers"`
}

func validateCanonicalTime(value string) error {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return err
	}
	if parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != value {
		return errors.New("timestamp must be canonical UTC RFC3339Nano")
	}
	if parsed.IsZero() {
		return errors.New("timestamp must be nonzero")
	}
	return nil
}

func validatePortableOrigin(value string) error {
	if value == "" || filepath.Base(value) != value || strings.Contains(value, "..") || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("profile origin %q is not portable", value)
	}
	return nil
}

func SortCanonicalIPs(values []string) {
	slices.Sort(values)
}
