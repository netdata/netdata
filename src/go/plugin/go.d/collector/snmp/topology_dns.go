// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	topologyReverseDNSTimeout  = 50 * time.Millisecond
	topologyReverseDNSCacheTTL = 10 * time.Minute
	topologyReverseDNSNegTTL   = 30 * time.Second
)

type topologyReverseDNSCacheEntry struct {
	name      string
	expiresAt time.Time
}

type topologyReverseDNSResolver struct {
	mu      sync.RWMutex
	timeout time.Duration
	ttl     time.Duration
	cache   map[string]topologyReverseDNSCacheEntry
}

func newTopologyReverseDNSResolver(timeout, ttl time.Duration) *topologyReverseDNSResolver {
	return &topologyReverseDNSResolver{
		timeout: timeout,
		ttl:     ttl,
		cache:   make(map[string]topologyReverseDNSCacheEntry),
	}
}

func (r *topologyReverseDNSResolver) lookup(ip string) string {
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
	entry, ok := r.cache[ip]
	r.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.name
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	resolved := ""
	if err == nil {
		resolved = topologyNormalizeReverseDNSName(names)
	}
	ttl := r.ttl
	if resolved == "" && topologyReverseDNSNegTTL > 0 {
		ttl = topologyReverseDNSNegTTL
	}

	r.mu.Lock()
	r.cache[ip] = topologyReverseDNSCacheEntry{
		name:      resolved,
		expiresAt: now.Add(ttl),
	}
	r.mu.Unlock()

	return resolved
}

func topologyNormalizeReverseDNSName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		name = strings.TrimSuffix(name, ".")
		name = strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return out[0]
}

var defaultTopologyReverseDNSResolver = newTopologyReverseDNSResolver(topologyReverseDNSTimeout, topologyReverseDNSCacheTTL)

func resolveTopologyReverseDNSName(ip string) string {
	return defaultTopologyReverseDNSResolver.lookup(ip)
}
