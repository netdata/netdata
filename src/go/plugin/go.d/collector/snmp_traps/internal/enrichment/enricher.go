// SPDX-License-Identifier: GPL-3.0-or-later

package enrichment

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

// RegistryResult is the trap enrichment projection of an SNMP registry lookup.
type RegistryResult struct {
	Matches  int
	Hostname string
	Vendor   string
	VnodeID  string
}

// TopologyResult is the trap enrichment projection of a topology lookup.
type TopologyResult struct {
	Status          string
	Method          string
	Matches         int
	Hostname        string
	Vendor          string
	VnodeID         string
	InterfaceIndex  string
	InterfaceStatus string
	InterfaceName   string
	NeighborStatus  string
	NeighborNames   []string
}

type (
	RegistryLookup func(sourceIP string) RegistryResult
	TopologyLookup func(sourceIP, trapIfIndex string) TopologyResult
	ReverseDNS     interface {
		Lookup(netip.Addr) reversedns.Result
		Schedule(netip.Addr) reversedns.ScheduleState
	}
)

// Enricher applies immutable registry, topology, and reverse-DNS dependencies
// to one trap entry. It holds no per-entry mutable state.
type Enricher struct {
	registry   RegistryLookup
	topology   TopologyLookup
	reverseDNS ReverseDNS
}

func New(registry RegistryLookup, topology TopologyLookup, reverseDNS ReverseDNS) *Enricher {
	return &Enricher{registry: registry, topology: topology, reverseDNS: reverseDNS}
}

func (e *Enricher) Enrich(entry *model.TrapEntry, useReverseDNS bool) {
	if entry == nil {
		return
	}

	audit := ensureTrapEnrichmentAudit(entry)
	sourceIP := entry.SourceIP
	if sourceIP == "" {
		sourceIP = entry.SourceUDPPeer
	}
	if sourceIP == "" {
		audit.Registry = &model.TrapEnrichmentLookup{Status: "skipped", Reason: "missing_source"}
		audit.Topology = &model.TrapEnrichmentLookup{Status: "skipped", Reason: "missing_source"}
		return
	}
	if audit.Source == nil {
		audit.Source = &model.TrapSourceAudit{Selected: sourceIP, Method: "entry_source"}
	}

	registry := e.lookupRegistry(sourceIP)
	audit.Registry = &model.TrapEnrichmentLookup{
		Key:     sourceIP,
		Status:  lookupStatus(registry.Matches),
		Method:  "hostname_or_ip",
		Matches: registry.Matches,
	}
	if registry.Matches > 1 {
		audit.Registry.Reason = "ambiguous_source"
	}

	registryMatched := registry.Matches == 1
	if registryMatched && registry.Hostname != "" {
		entry.DeviceHostname = registry.Hostname
		addTrapEnrichmentApplied(audit, "_HOSTNAME", registry.Hostname)
		audit.Registry.Fields = append(audit.Registry.Fields, "_HOSTNAME")
	}
	if registryMatched && registry.Vendor != "" {
		entry.DeviceVendor = registry.Vendor
		addTrapEnrichmentApplied(audit, "TRAP_DEVICE_VENDOR", registry.Vendor)
		audit.Registry.Fields = append(audit.Registry.Fields, "TRAP_DEVICE_VENDOR")
	}
	if registryMatched && registry.VnodeID != "" {
		entry.SourceVnodeID = registry.VnodeID
		addTrapEnrichmentApplied(audit, "ND_NIDL_NODE", registry.VnodeID)
		audit.Registry.Fields = append(audit.Registry.Fields, "ND_NIDL_NODE")
	}

	trapIfIndex := trapIfIndexFromVarbinds(entry.Varbinds)
	if iface, key := trapInterfaceNameFromVarbinds(entry.Varbinds); iface != "" {
		entry.TopologyInterface = iface
		audit.Interface = &model.TrapEnrichmentLookup{
			Key:    key,
			Status: "matched",
			Method: "trap_varbind",
			Fields: []string{"TRAP_INTERFACE"},
		}
		addTrapEnrichmentApplied(audit, "TRAP_INTERFACE", iface)
	}

	topology := e.lookupTopology(sourceIP, trapIfIndex)
	topologyTrusted := topology.Status == "matched"
	if topology.Status != "" {
		audit.Topology = &model.TrapEnrichmentLookup{
			Key:     sourceIP,
			Status:  topology.Status,
			Method:  topology.Method,
			Matches: topology.Matches,
		}
		if topology.Status == "ambiguous" {
			audit.Topology.Reason = "ambiguous_source"
		}
		if registryMatched && entry.SourceVnodeID != "" && topology.VnodeID != "" && entry.SourceVnodeID != topology.VnodeID {
			topologyTrusted = false
			audit.Topology.Status = "conflict"
			audit.Topology.Reason = "vnode_mismatch"
		}
	} else {
		audit.Topology = &model.TrapEnrichmentLookup{Key: sourceIP, Status: "no_match", Matches: 0}
	}

	if topologyTrusted {
		if entry.DeviceHostname == "" && !IsUnresolvedSysName(topology.Hostname) {
			entry.DeviceHostname = topology.Hostname
			addTrapEnrichmentApplied(audit, "_HOSTNAME", topology.Hostname)
			audit.Topology.Fields = append(audit.Topology.Fields, "_HOSTNAME")
		}
		if entry.DeviceVendor == "" && topology.Vendor != "" {
			entry.DeviceVendor = topology.Vendor
			addTrapEnrichmentApplied(audit, "TRAP_DEVICE_VENDOR", topology.Vendor)
			audit.Topology.Fields = append(audit.Topology.Fields, "TRAP_DEVICE_VENDOR")
		}
		if entry.SourceVnodeID == "" && topology.VnodeID != "" {
			entry.SourceVnodeID = topology.VnodeID
			addTrapEnrichmentApplied(audit, "ND_NIDL_NODE", topology.VnodeID)
			audit.Topology.Fields = append(audit.Topology.Fields, "ND_NIDL_NODE")
		}

		if entry.TopologyInterface == "" && topology.InterfaceName != "" {
			entry.TopologyInterface = topology.InterfaceName
			addTrapEnrichmentApplied(audit, "TRAP_INTERFACE", topology.InterfaceName)
		}
		if audit.Interface == nil {
			audit.Interface = topologyInterfaceAudit(topology, entry.TopologyInterface != "")
			if entry.TopologyInterface != "" {
				audit.Interface.Fields = append(audit.Interface.Fields, "TRAP_INTERFACE")
			}
		}
		if len(topology.NeighborNames) > 0 {
			entry.TopologyNeighbors = strings.Join(topology.NeighborNames, ",")
			addTrapEnrichmentApplied(audit, "TRAP_NEIGHBORS", entry.TopologyNeighbors)
			audit.Neighbors = &model.TrapEnrichmentLookup{
				Key:    topology.InterfaceIndex,
				Status: topology.NeighborStatus,
				Method: "topology_ifindex",
				Fields: []string{"TRAP_NEIGHBORS"},
			}
		} else {
			audit.Neighbors = &model.TrapEnrichmentLookup{
				Key:    topology.InterfaceIndex,
				Status: topology.NeighborStatus,
				Method: "topology_ifindex",
			}
			if topology.InterfaceIndex == "" {
				audit.Neighbors.Reason = "missing_trap_ifindex"
			}
		}
	} else if audit.Interface == nil {
		audit.Interface = skippedTopologyIfIndexAudit(trapIfIndex, topologyTrusted)
		audit.Neighbors = skippedTopologyIfIndexAudit(trapIfIndex, topologyTrusted)
	}
	if audit.Neighbors == nil && entry.TopologyNeighbors == "" {
		audit.Neighbors = skippedTopologyIfIndexAudit(trapIfIndex, topologyTrusted)
	}

	if useReverseDNS {
		e.enrichReverseDNS(entry, audit, sourceIP)
	}
}

func (e *Enricher) lookupRegistry(sourceIP string) RegistryResult {
	if e == nil || e.registry == nil {
		return RegistryResult{}
	}
	return e.registry(sourceIP)
}

func (e *Enricher) lookupTopology(sourceIP, trapIfIndex string) TopologyResult {
	if e == nil || e.topology == nil {
		return TopologyResult{}
	}
	return e.topology(sourceIP, trapIfIndex)
}

func (e *Enricher) enrichReverseDNS(entry *model.TrapEntry, audit *model.TrapEnrichmentAudit, sourceIP string) {
	audit.ReverseDNS = &model.TrapEnrichmentLookup{Key: sourceIP, Method: "reverse_dns"}
	addr, _ := netip.ParseAddr(strings.TrimSpace(sourceIP))
	if e != nil && e.reverseDNS != nil {
		if result := e.reverseDNS.Lookup(addr); result.State == reversedns.StatePositive {
			entry.ReverseDNS = result.Name
			audit.ReverseDNS.Status = "matched"
			audit.ReverseDNS.Value = result.Name
			audit.ReverseDNS.Fields = []string{"TRAP_REVERSE_DNS"}
			addTrapEnrichmentApplied(audit, "TRAP_REVERSE_DNS", result.Name)
			return
		}
	}

	audit.ReverseDNS.Status = "pending"
	if e != nil && e.reverseDNS != nil {
		e.reverseDNS.Schedule(addr)
	}
}

func ensureTrapEnrichmentAudit(entry *model.TrapEntry) *model.TrapEnrichmentAudit {
	if entry.Enrichment == nil {
		entry.Enrichment = &model.TrapEnrichmentAudit{}
	}
	return entry.Enrichment
}

func lookupStatus(matches int) string {
	switch matches {
	case 0:
		return "no_match"
	case 1:
		return "matched"
	default:
		return "ambiguous"
	}
}

func addTrapEnrichmentApplied(audit *model.TrapEnrichmentAudit, field, value string) {
	if audit == nil || field == "" || value == "" {
		return
	}
	if audit.Applied == nil {
		audit.Applied = make(map[string]string)
	}
	audit.Applied[field] = value
}

// IsUnresolvedSysName reports whether an SNMP sysName is unusable as a hostname.
func IsUnresolvedSysName(name string) bool {
	s := strings.TrimSpace(name)
	return s == "" || strings.EqualFold(s, "unknown")
}

const (
	ifIndexOIDPrefix = "1.3.6.1.2.1.2.2.1.1"
	ifDescrOIDPrefix = "1.3.6.1.2.1.2.2.1.2"
	ifNameOIDPrefix  = "1.3.6.1.2.1.31.1.1.1.1"
)

func trapIfIndexFromVarbinds(vbs []model.VarbindValue) string {
	for _, vb := range vbs {
		if isIfIndexVarbind(vb) {
			return strings.TrimSpace(varbindScalarString(vb.Value))
		}
	}
	return ""
}

func trapInterfaceNameFromVarbinds(vbs []model.VarbindValue) (string, string) {
	for _, want := range []struct {
		name string
		oid  string
	}{
		{name: "ifName", oid: ifNameOIDPrefix},
		{name: "ifDescr", oid: ifDescrOIDPrefix},
	} {
		for _, vb := range vbs {
			if !isNamedOrOIDPrefixedVarbind(vb, want.name, want.oid) {
				continue
			}
			value := strings.TrimSpace(varbindScalarString(vb.Value))
			if value != "" {
				return value, want.name
			}
		}
	}
	return "", ""
}

func isIfIndexVarbind(vb model.VarbindValue) bool {
	return isNamedOrOIDPrefixedVarbind(vb, "ifIndex", ifIndexOIDPrefix)
}

func isNamedOrOIDPrefixedVarbind(vb model.VarbindValue, name, oidPrefix string) bool {
	vbName := strings.TrimSpace(vb.Name)
	if vbName == name || strings.HasPrefix(vbName, name+".") {
		return true
	}
	oid := model.NormalizeOID(vb.OID)
	return oid == oidPrefix || strings.HasPrefix(oid, oidPrefix+".")
}

func varbindScalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case int8:
		return fmt.Sprintf("%d", v)
	case int16:
		return fmt.Sprintf("%d", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint:
		return fmt.Sprintf("%d", v)
	case uint8:
		return fmt.Sprintf("%d", v)
	case uint16:
		return fmt.Sprintf("%d", v)
	case uint32:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func topologyInterfaceAudit(topology TopologyResult, matched bool) *model.TrapEnrichmentLookup {
	audit := &model.TrapEnrichmentLookup{
		Key:    topology.InterfaceIndex,
		Status: topology.InterfaceStatus,
		Method: "topology_ifindex",
	}
	if topology.InterfaceIndex == "" {
		audit.Reason = "missing_trap_ifindex"
	} else if !matched {
		audit.Reason = "ifindex_not_found"
	}
	return audit
}

func skippedTopologyIfIndexAudit(trapIfIndex string, topologyTrusted bool) *model.TrapEnrichmentLookup {
	audit := &model.TrapEnrichmentLookup{
		Key:    trapIfIndex,
		Status: "skipped",
		Method: "topology_ifindex",
	}
	if trapIfIndex == "" {
		audit.Reason = "missing_trap_ifindex"
	} else if !topologyTrusted {
		audit.Reason = "no_exact_topology_device_match"
	}
	return audit
}
