// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
)

type reverseDNSResolver struct {
	mu          sync.RWMutex
	ctx         context.Context // NOSONAR: resolver owns async lookup cancellation for its lifetime.
	cancel      context.CancelFunc
	timeout     time.Duration
	ttl         time.Duration
	negativeTTL time.Duration
	maxEntries  int
	lastSweep   time.Time
	closed      bool
	lookupAddr  func(context.Context, string) ([]string, error)
	lookupSem   chan struct{}
	cache       map[string]reverseDNSCacheEntry
	pending     map[string]struct{}
}

type reverseDNSCacheEntry struct {
	name      string
	expiresAt time.Time
}

const (
	defaultReverseDNSTimeout     = 50 * time.Millisecond
	defaultReverseDNSTTL         = 10 * time.Minute
	defaultReverseDNSNegativeTTL = 30 * time.Second
	defaultReverseDNSMaxEntries  = 10000
	defaultReverseDNSConcurrent  = 32
	reverseDNSSweepInterval      = 5 * time.Minute
)

func newReverseDNSResolver() *reverseDNSResolver {
	ctx, cancel := context.WithCancel(context.Background())
	return &reverseDNSResolver{
		ctx:         ctx,
		cancel:      cancel,
		timeout:     defaultReverseDNSTimeout,
		ttl:         defaultReverseDNSTTL,
		negativeTTL: defaultReverseDNSNegativeTTL,
		maxEntries:  defaultReverseDNSMaxEntries,
		lookupAddr:  net.DefaultResolver.LookupAddr,
		lookupSem:   make(chan struct{}, defaultReverseDNSConcurrent),
		cache:       make(map[string]reverseDNSCacheEntry),
		pending:     make(map[string]struct{}),
	}
}

func (r *reverseDNSResolver) Close() {
	if r == nil {
		return
	}

	r.mu.Lock()
	if !r.closed {
		r.closed = true
		if r.cancel != nil {
			r.cancel()
		}
	}
	r.mu.Unlock()
}

func (r *reverseDNSResolver) lookupCached(ip string) string {
	if r == nil {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.IsValid() {
		return ""
	}
	ip = addr.Unmap().String()

	now := time.Now()

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return ""
	}
	entry, ok := r.cache[ip]
	r.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.name
	}
	return ""
}

func (r *reverseDNSResolver) resolveAsync(ip string) {
	if r == nil {
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.IsValid() {
		return
	}
	ip = addr.Unmap().String()

	now := time.Now()

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return
	}
	entry, ok := r.cache[ip]
	_, pending := r.pending[ip]
	r.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return
	}
	if pending {
		return
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	entry, ok = r.cache[ip]
	if ok && now.Before(entry.expiresAt) {
		r.mu.Unlock()
		return
	}
	if _, pending = r.pending[ip]; pending {
		r.mu.Unlock()
		return
	}
	lookupSem := r.lookupSem
	if lookupSem != nil {
		select {
		case lookupSem <- struct{}{}:
		default:
			r.mu.Unlock()
			return
		}
	}
	r.pending[ip] = struct{}{}
	parentCtx := r.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	lookupAddr := r.lookupAddr
	if lookupAddr == nil {
		lookupAddr = net.DefaultResolver.LookupAddr
	}
	r.mu.Unlock()

	go func() {
		defer func() {
			if lookupSem != nil {
				<-lookupSem
			}
		}()
		ctx, cancel := context.WithTimeout(parentCtx, r.timeout)
		defer cancel()

		names, err := lookupAddr(ctx, ip)
		resolved := ""
		if err == nil && len(names) > 0 {
			name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(names[0])), ".")
			if name != "" {
				resolved = name
			}
		}
		ttl := r.ttl
		if resolved == "" {
			ttl = r.negativeTTL
		}
		now := time.Now()
		r.mu.Lock()
		defer r.mu.Unlock()

		delete(r.pending, ip)
		if r.closed {
			return
		}
		r.cache[ip] = reverseDNSCacheEntry{
			name:      resolved,
			expiresAt: now.Add(ttl),
		}
		r.sweepExpiredLocked(now)
		r.trimCacheLocked()
	}()
}

func (r *reverseDNSResolver) maybeSweep(now time.Time) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	overLimit := r.maxEntries > 0 && len(r.cache) > r.maxEntries
	if !overLimit && now.Sub(r.lastSweep) < reverseDNSSweepInterval {
		return
	}
	r.sweepExpiredLocked(now)
	r.trimCacheLocked()
	r.lastSweep = now
}

func (r *reverseDNSResolver) sweepExpiredLocked(now time.Time) {
	for ip, entry := range r.cache {
		if !now.Before(entry.expiresAt) {
			delete(r.cache, ip)
		}
	}
}

func (r *reverseDNSResolver) trimCacheLocked() {
	if r.maxEntries <= 0 {
		return
	}
	if len(r.cache) <= r.maxEntries {
		return
	}

	candidates := make([]reverseDNSCacheCandidate, 0, len(r.cache))
	for ip, entry := range r.cache {
		candidates = append(candidates, reverseDNSCacheCandidate{
			ip:        ip,
			expiresAt: entry.expiresAt,
			negative:  entry.name == "",
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].negative != candidates[j].negative {
			return candidates[i].negative
		}
		if !candidates[i].expiresAt.Equal(candidates[j].expiresAt) {
			return candidates[i].expiresAt.Before(candidates[j].expiresAt)
		}
		return candidates[i].ip < candidates[j].ip
	})

	for _, candidate := range candidates {
		if len(r.cache) <= r.maxEntries {
			return
		}
		delete(r.cache, candidate.ip)
	}
}

type reverseDNSCacheCandidate struct {
	ip        string
	expiresAt time.Time
	negative  bool
}

func isUnresolvedSysName(name string) bool {
	s := strings.TrimSpace(name)
	return s == "" || strings.EqualFold(s, "unknown")
}

type deviceEnrichment struct {
	deviceHostname string
	deviceVendor   string
	vnodeID        string
	matches        int
}

var trapTopologyEnrichmentForSource = snmptopology.TrapEnrichmentForSource

func resolveDeviceEnrichment(sourceIP string) deviceEnrichment {
	devices := ddsnmp.DeviceRegistry.DevicesByHostname(sourceIP)
	enrich := deviceEnrichment{matches: len(devices)}
	if len(devices) != 1 {
		return enrich
	}

	dev := devices[0]

	if dev.VnodeHostname != "" && !isUnresolvedSysName(dev.VnodeHostname) {
		enrich.deviceHostname = dev.VnodeHostname
	} else if dev.SysName != "" && !isUnresolvedSysName(dev.SysName) {
		enrich.deviceHostname = dev.SysName
	}

	if dev.Vendor != "" {
		enrich.deviceVendor = dev.Vendor
	}
	if dev.VnodeGUID != "" {
		enrich.vnodeID = dev.VnodeGUID
	}

	return enrich
}

func enrichTrapEntry(entry *TrapEntry, useReverseDNS bool, dns *reverseDNSResolver) {
	if entry == nil {
		return
	}

	audit := ensureTrapEnrichmentAudit(entry)
	sourceIP := entry.SourceIP
	if sourceIP == "" {
		sourceIP = entry.SourceUDPPeer
	}
	if sourceIP == "" {
		audit.Registry = &TrapEnrichmentLookup{Status: "skipped", Reason: "missing_source"}
		audit.Topology = &TrapEnrichmentLookup{Status: "skipped", Reason: "missing_source"}
		return
	}
	if audit.Source == nil {
		audit.Source = &TrapSourceAudit{Selected: sourceIP, Method: "entry_source"}
	}

	enrich := resolveDeviceEnrichment(sourceIP)
	audit.Registry = &TrapEnrichmentLookup{
		Key:     sourceIP,
		Status:  lookupStatus(enrich.matches),
		Method:  "hostname_or_ip",
		Matches: enrich.matches,
	}
	if enrich.matches > 1 {
		audit.Registry.Reason = "ambiguous_source"
	}

	registryMatched := enrich.matches == 1
	if registryMatched && enrich.deviceHostname != "" {
		entry.DeviceHostname = enrich.deviceHostname
		addTrapEnrichmentApplied(audit, "_HOSTNAME", enrich.deviceHostname)
		audit.Registry.Fields = append(audit.Registry.Fields, "_HOSTNAME")
	}
	if registryMatched && enrich.deviceVendor != "" {
		entry.DeviceVendor = enrich.deviceVendor
		addTrapEnrichmentApplied(audit, "TRAP_DEVICE_VENDOR", enrich.deviceVendor)
		audit.Registry.Fields = append(audit.Registry.Fields, "TRAP_DEVICE_VENDOR")
	}
	if registryMatched && enrich.vnodeID != "" {
		entry.SourceVnodeID = enrich.vnodeID
		addTrapEnrichmentApplied(audit, "ND_NIDL_NODE", enrich.vnodeID)
		audit.Registry.Fields = append(audit.Registry.Fields, "ND_NIDL_NODE")
	}

	trapIfIndex := trapIfIndexFromVarbinds(entry.Varbinds)
	if iface, key := trapInterfaceNameFromVarbinds(entry.Varbinds); iface != "" {
		entry.TopologyInterface = iface
		audit.Interface = &TrapEnrichmentLookup{
			Key:    key,
			Status: "matched",
			Method: "trap_varbind",
			Fields: []string{"TRAP_INTERFACE"},
		}
		addTrapEnrichmentApplied(audit, "TRAP_INTERFACE", iface)
	}

	topo := trapTopologyEnrichmentForSource(sourceIP, trapIfIndex)
	topologyTrusted := topo != nil && topo.DeviceStatus == "matched"
	if topo != nil {
		audit.Topology = &TrapEnrichmentLookup{
			Key:     sourceIP,
			Status:  topo.DeviceStatus,
			Method:  topo.DeviceMethod,
			Matches: topo.DeviceMatches,
		}
		if topo.DeviceStatus == "ambiguous" {
			audit.Topology.Reason = "ambiguous_source"
		}
		if registryMatched && entry.SourceVnodeID != "" && topo.SourceVnodeID != "" && entry.SourceVnodeID != topo.SourceVnodeID {
			topologyTrusted = false
			audit.Topology.Status = "conflict"
			audit.Topology.Reason = "vnode_mismatch"
		}
	} else {
		audit.Topology = &TrapEnrichmentLookup{Key: sourceIP, Status: "no_match", Matches: 0}
	}

	if topologyTrusted {
		if entry.DeviceHostname == "" && !isUnresolvedSysName(topo.DeviceHostname) {
			entry.DeviceHostname = topo.DeviceHostname
			addTrapEnrichmentApplied(audit, "_HOSTNAME", topo.DeviceHostname)
			audit.Topology.Fields = append(audit.Topology.Fields, "_HOSTNAME")
		}
		if entry.DeviceVendor == "" && topo.DeviceVendor != "" {
			entry.DeviceVendor = topo.DeviceVendor
			addTrapEnrichmentApplied(audit, "TRAP_DEVICE_VENDOR", topo.DeviceVendor)
			audit.Topology.Fields = append(audit.Topology.Fields, "TRAP_DEVICE_VENDOR")
		}
		if entry.SourceVnodeID == "" && topo.SourceVnodeID != "" {
			entry.SourceVnodeID = topo.SourceVnodeID
			addTrapEnrichmentApplied(audit, "ND_NIDL_NODE", topo.SourceVnodeID)
			audit.Topology.Fields = append(audit.Topology.Fields, "ND_NIDL_NODE")
		}

		if entry.TopologyInterface == "" && topo.Interface != "" {
			entry.TopologyInterface = topo.Interface
			addTrapEnrichmentApplied(audit, "TRAP_INTERFACE", topo.Interface)
		}
		if audit.Interface == nil {
			audit.Interface = topologyInterfaceAudit(topo, entry.TopologyInterface != "")
			if entry.TopologyInterface != "" {
				audit.Interface.Fields = append(audit.Interface.Fields, "TRAP_INTERFACE")
			}
		}
		if len(topo.Neighbors) > 0 {
			entry.TopologyNeighbors = strings.Join(topo.Neighbors, ",")
			addTrapEnrichmentApplied(audit, "TRAP_NEIGHBORS", entry.TopologyNeighbors)
			audit.Neighbors = &TrapEnrichmentLookup{
				Key:    topo.InterfaceIndex,
				Status: topo.NeighborStatus,
				Method: "topology_ifindex",
				Fields: []string{"TRAP_NEIGHBORS"},
			}
		} else {
			audit.Neighbors = &TrapEnrichmentLookup{
				Key:    topo.InterfaceIndex,
				Status: topo.NeighborStatus,
				Method: "topology_ifindex",
			}
			if topo.InterfaceIndex == "" {
				audit.Neighbors.Reason = "missing_trap_ifindex"
			}
		}
	} else if audit.Interface == nil {
		audit.Interface = skippedTopologyInterfaceAudit(trapIfIndex, topologyTrusted)
		audit.Neighbors = skippedTopologyNeighborsAudit(trapIfIndex, topologyTrusted)
	}
	if audit.Neighbors == nil && entry.TopologyNeighbors == "" {
		audit.Neighbors = skippedTopologyNeighborsAudit(trapIfIndex, topologyTrusted)
	}

	if useReverseDNS && entry.DeviceHostname == "" {
		audit.ReverseDNS = &TrapEnrichmentLookup{Key: sourceIP, Method: "reverse_dns"}
		if name := dns.lookupCached(sourceIP); name != "" {
			entry.DeviceHostname = name
			audit.ReverseDNS.Status = "matched"
			audit.ReverseDNS.Fields = []string{"_HOSTNAME"}
			addTrapEnrichmentApplied(audit, "_HOSTNAME", name)
		} else {
			audit.ReverseDNS.Status = "pending"
			dns.resolveAsync(sourceIP)
		}
	}
}

func ensureTrapEnrichmentAudit(entry *TrapEntry) *TrapEnrichmentAudit {
	if entry.Enrichment == nil {
		entry.Enrichment = &TrapEnrichmentAudit{}
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

func addTrapEnrichmentApplied(audit *TrapEnrichmentAudit, field, value string) {
	if audit == nil || field == "" || value == "" {
		return
	}
	if audit.Applied == nil {
		audit.Applied = make(map[string]string)
	}
	audit.Applied[field] = value
}

const (
	ifIndexOIDPrefix = "1.3.6.1.2.1.2.2.1.1"
	ifDescrOIDPrefix = "1.3.6.1.2.1.2.2.1.2"
	ifNameOIDPrefix  = "1.3.6.1.2.1.31.1.1.1.1"
)

func trapIfIndexFromVarbinds(vbs []VarbindValue) string {
	for _, vb := range vbs {
		if isIfIndexVarbind(vb) {
			return strings.TrimSpace(varbindScalarString(vb.Value))
		}
	}
	return ""
}

func trapInterfaceNameFromVarbinds(vbs []VarbindValue) (string, string) {
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

func isIfIndexVarbind(vb VarbindValue) bool {
	return isNamedOrOIDPrefixedVarbind(vb, "ifIndex", ifIndexOIDPrefix)
}

func isNamedOrOIDPrefixedVarbind(vb VarbindValue, name, oidPrefix string) bool {
	vbName := strings.TrimSpace(vb.Name)
	if vbName == name || strings.HasPrefix(vbName, name+".") {
		return true
	}
	oid := normalizeOID(vb.OID)
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

func topologyInterfaceAudit(topo *snmptopology.TrapTopologyEnrichment, matched bool) *TrapEnrichmentLookup {
	if topo == nil {
		return &TrapEnrichmentLookup{Status: "skipped", Reason: "no_topology_match"}
	}
	audit := &TrapEnrichmentLookup{
		Key:    topo.InterfaceIndex,
		Status: topo.InterfaceStatus,
		Method: "topology_ifindex",
	}
	if topo.InterfaceIndex == "" {
		audit.Reason = "missing_trap_ifindex"
	} else if !matched {
		audit.Reason = "ifindex_not_found"
	}
	return audit
}

func skippedTopologyInterfaceAudit(trapIfIndex string, topologyTrusted bool) *TrapEnrichmentLookup {
	audit := &TrapEnrichmentLookup{
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

func skippedTopologyNeighborsAudit(trapIfIndex string, topologyTrusted bool) *TrapEnrichmentLookup {
	audit := &TrapEnrichmentLookup{
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
