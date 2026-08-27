// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

const (
	CapabilityGenerationSnapshot = "generation_snapshot"
	CapabilityGraphReplay        = "graph_replay"

	KindGeneration = "generation"
	KindGraphQuery = "graph_query"
	KindDNSTrace   = "dns_trace"
	KindOUITrace   = "oui_trace"
)

const (
	GenerationSectionGeneration = "generation"

	GraphSectionDNSTrace   = "dns_trace"
	GraphSectionGeneration = "generation"
	GraphSectionOUITrace   = "oui_trace"
	GraphSectionQuery      = "query"
)

const (
	DNSStateMiss           = "miss"
	DNSStatePositive       = "positive"
	DNSStateCachedNegative = "cached_negative"
)

type GenerationV1 struct {
	Sequence          uint64        `json:"sequence"`
	PublishedAt       string        `json:"published_at"`
	ProducerScopeID   string        `json:"producer_scope_id"`
	Kernel            GraphKernelV1 `json:"kernel"`
	DeviceCount       uint64        `json:"device_count"`
	RenderableDevices uint64        `json:"renderable_devices"`
	Observations      []ContentRef  `json:"observations"`
}

func (g GenerationV1) Validate() error {
	if g.Sequence == 0 {
		return errors.New("generation sequence must be nonzero")
	}
	if err := validateCanonicalTime(g.PublishedAt); err != nil {
		return fmt.Errorf("published_at: %w", err)
	}
	if strings.ContainsRune(g.ProducerScopeID, 0) {
		return errors.New("producer_scope_id contains NUL")
	}
	if err := g.Kernel.Validate(); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}
	if g.RenderableDevices != uint64(len(g.Observations)) {
		return errors.New("renderable device count does not match observations")
	}
	if g.RenderableDevices > g.DeviceCount {
		return errors.New("renderable device count exceeds device count")
	}
	seen := make(map[string]struct{}, len(g.Observations))
	for i, ref := range g.Observations {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("observations[%d]: %w", i, err)
		}
		if ref.Type() != (MemberType{Kind: KindObservation, Schema: SchemaV1}) {
			return fmt.Errorf("observations[%d] has type %s@%s", i, ref.Kind, ref.Schema)
		}
		if _, exists := seen[ref.Key()]; exists {
			return fmt.Errorf("observations[%d] duplicates an earlier observation", i)
		}
		seen[ref.Key()] = struct{}{}
	}
	return nil
}

func (g GenerationV1) References() []ContentRef {
	return slices.Clone(g.Observations)
}

type GraphKernelV1 struct {
	Name         string `json:"name"`
	Revision     uint32 `json:"revision"`
	ModelSchema  string `json:"model_schema"`
	OutputSchema string `json:"output_schema"`
}

func (k GraphKernelV1) Validate() error {
	if k.Name != "snmp_topology_graph" || k.Revision != 1 {
		return fmt.Errorf("unsupported graph kernel %s@%d", k.Name, k.Revision)
	}
	if k.ModelSchema != "2.0" {
		return fmt.Errorf("unsupported graph model schema %q", k.ModelSchema)
	}
	if k.OutputSchema != "netdata.topology.v1" {
		return fmt.Errorf("unsupported graph output schema %q", k.OutputSchema)
	}
	return nil
}

type GraphQueryV1 struct {
	CaptureID          uint64              `json:"capture_id"`
	GenerationSequence uint64              `json:"generation_sequence"`
	Generation         ContentRef          `json:"generation"`
	Options            GraphQueryOptionsV1 `json:"options"`
}

func (q GraphQueryV1) Validate() error {
	if q.CaptureID == 0 || q.GenerationSequence == 0 {
		return errors.New("query owner identifiers must be nonzero")
	}
	if err := q.Generation.Validate(); err != nil {
		return fmt.Errorf("generation: %w", err)
	}
	if q.Generation.Type() != (MemberType{Kind: KindGeneration, Schema: SchemaV1}) {
		return fmt.Errorf("query generation has type %s@%s", q.Generation.Kind, q.Generation.Schema)
	}
	return q.Options.Validate()
}

func (q GraphQueryV1) References() []ContentRef { return []ContentRef{q.Generation} }

type GraphQueryOptionsV1 struct {
	CollapseActorsByIP     bool   `json:"collapse_actors_by_ip"`
	EliminateNonIPInferred bool   `json:"eliminate_non_ip_inferred"`
	MapType                string `json:"map_type"`
	InferenceStrategy      string `json:"inference_strategy"`
	ManagedDeviceFocus     string `json:"managed_device_focus"`
	Depth                  int    `json:"depth"`
}

func (o GraphQueryOptionsV1) Validate() error {
	if !oneOf(o.MapType, "managed_fabric", "lldp_cdp_managed", "high_confidence_inferred", "all_devices_low_confidence") {
		return fmt.Errorf("unsupported map_type %q", o.MapType)
	}
	if !oneOf(o.InferenceStrategy,
		"fdb_minimum_knowledge", "stp_parent_tree", "fdb_pairwise_minimum_knowledge", "stp_fdb_correlated", "cdp_fdb_hybrid") {
		return fmt.Errorf("unsupported inference_strategy %q", o.InferenceStrategy)
	}
	if o.Depth < -1 || o.Depth > 10 {
		return fmt.Errorf("depth %d is outside [-1,10]", o.Depth)
	}
	if err := validateManagedDeviceFocus(o.ManagedDeviceFocus); err != nil {
		return err
	}
	return nil
}

func validateManagedDeviceFocus(value string) error {
	if value == "all_devices" {
		return nil
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return errors.New("managed_device_focus is empty")
	}
	previous := ""
	for i, part := range parts {
		if !strings.HasPrefix(part, "ip:") {
			return fmt.Errorf("managed_device_focus[%d] is not an IP focus", i)
		}
		ip := strings.TrimPrefix(part, "ip:")
		addr, err := netip.ParseAddr(ip)
		if err != nil || addr.String() != ip {
			return fmt.Errorf("managed_device_focus[%d] is not canonical", i)
		}
		if previous >= part {
			return errors.New("managed_device_focus entries must be strictly ordered")
		}
		previous = part
	}
	return nil
}

type DNSTraceV1 struct {
	CaptureID uint64        `json:"capture_id"`
	Records   []DNSRecordV1 `json:"records"`
}

func (t DNSTraceV1) Validate() error {
	if t.CaptureID == 0 || len(t.Records) == 0 {
		return errors.New("DNS trace owner and records are required")
	}
	for i, record := range t.Records {
		if record.Ordinal != uint64(i) {
			return fmt.Errorf("DNS record %d has ordinal %d", i, record.Ordinal)
		}
		if err := record.Validate(); err != nil {
			return fmt.Errorf("DNS record %d: %w", i, err)
		}
	}
	return nil
}

type DNSRecordV1 struct {
	Ordinal uint64 `json:"ordinal"`
	IP      string `json:"ip"`
	State   string `json:"state"`
	Name    string `json:"name,omitempty"`
}

func (r DNSRecordV1) Validate() error {
	addr, err := netip.ParseAddr(r.IP)
	if err != nil || addr.String() != r.IP {
		return errors.New("IP is not canonical")
	}
	switch r.State {
	case DNSStatePositive:
		if strings.TrimSpace(r.Name) == "" {
			return errors.New("positive DNS record requires a name")
		}
	case DNSStateMiss, DNSStateCachedNegative:
		if r.Name != "" {
			return errors.New("non-positive DNS record must not contain a name")
		}
	default:
		return fmt.Errorf("unsupported DNS state %q", r.State)
	}
	return nil
}

type OUITraceV1 struct {
	CaptureID uint64        `json:"capture_id"`
	Records   []OUIRecordV1 `json:"records"`
}

func (t OUITraceV1) Validate() error {
	if t.CaptureID == 0 || len(t.Records) == 0 {
		return errors.New("OUI trace owner and records are required")
	}
	for i, record := range t.Records {
		if record.Ordinal != uint64(i) {
			return fmt.Errorf("OUI record %d has ordinal %d", i, record.Ordinal)
		}
		if err := record.Validate(); err != nil {
			return fmt.Errorf("OUI record %d: %w", i, err)
		}
	}
	return nil
}

type OUIRecordV1 struct {
	Ordinal uint64 `json:"ordinal"`
	MAC     string `json:"mac"`
	Vendor  string `json:"vendor,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
}

func (r OUIRecordV1) Validate() error {
	mac, err := net.ParseMAC(r.MAC)
	if err != nil || mac.String() != r.MAC {
		return errors.New("MAC is not canonical")
	}
	if (r.Vendor == "") != (r.Prefix == "") {
		return errors.New("OUI vendor and prefix must be both present or both absent")
	}
	if r.Prefix != "" {
		if len(r.Prefix) < 6 || len(r.Prefix) > 12 || strings.ToUpper(r.Prefix) != r.Prefix {
			return errors.New("OUI prefix is not canonical")
		}
		for _, value := range r.Prefix {
			if !strings.ContainsRune("0123456789ABCDEF", value) {
				return errors.New("OUI prefix is not hexadecimal")
			}
		}
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	return slices.Contains(choices, value)
}
