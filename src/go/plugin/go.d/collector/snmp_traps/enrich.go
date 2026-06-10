// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
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
}

var trapTopologyEnrichmentForIP = snmptopology.TrapEnrichmentForIP

func resolveDeviceEnrichment(sourceIP string) deviceEnrichment {
	if dev, ok := ddsnmp.DeviceRegistry.DeviceByHostname(sourceIP); ok {
		enrich := deviceEnrichment{}

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

	return deviceEnrichment{}
}

func enrichTrapEntry(entry *TrapEntry, useReverseDNS bool, dns *reverseDNSResolver) {
	if entry == nil {
		return
	}

	sourceIP := entry.SourceIP
	if sourceIP == "" {
		sourceIP = entry.SourceUDPPeer
	}
	if sourceIP == "" {
		return
	}

	enrich := resolveDeviceEnrichment(sourceIP)

	if enrich.deviceHostname != "" {
		entry.DeviceHostname = enrich.deviceHostname
	}
	if enrich.deviceVendor != "" {
		entry.DeviceVendor = enrich.deviceVendor
	}
	if enrich.vnodeID != "" {
		entry.SourceVnodeID = enrich.vnodeID
	}

	if topo := trapTopologyEnrichmentForIP(sourceIP); topo != nil {
		if entry.DeviceHostname == "" && !isUnresolvedSysName(topo.DeviceHostname) {
			entry.DeviceHostname = topo.DeviceHostname
		}
		if entry.DeviceVendor == "" && topo.DeviceVendor != "" {
			entry.DeviceVendor = topo.DeviceVendor
		}
		if entry.SourceVnodeID == "" && topo.SourceVnodeID != "" {
			entry.SourceVnodeID = topo.SourceVnodeID
		}
		if topo.Interface != "" {
			entry.TopologyInterface = topo.Interface
		}
		if len(topo.Neighbors) > 0 {
			entry.TopologyNeighbors = strings.Join(topo.Neighbors, ",")
		}
	}

	if useReverseDNS && entry.DeviceHostname == "" {
		if name := dns.lookupCached(sourceIP); name != "" {
			entry.DeviceHostname = name
		} else {
			dns.resolveAsync(sourceIP)
		}
	}
}
