// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	topologyReverseDNSLookupTimeout   = 500 * time.Millisecond
	topologyReverseDNSPositiveTTL     = 24 * time.Hour
	topologyReverseDNSNegativeTTL     = 6 * time.Hour
	topologyReverseDNSMaxCandidates   = 1024
	topologyReverseDNSMaxCacheEntries = 4096
	topologyReverseDNSMaxConcurrency  = 4
)

type topologyReverseDNSLookupFunc func(context.Context, string) ([]string, error)

type topologyReverseDNSConfig struct {
	lookup        topologyReverseDNSLookupFunc
	now           func() time.Time
	timeout       time.Duration
	positiveTTL   time.Duration
	negativeTTL   time.Duration
	maxCandidates int
	maxEntries    int
	concurrency   int
}

type topologyReverseDNSCacheEntry struct {
	name       string
	expiresAt  time.Time
	lastAccess time.Time
}

type topologyReverseDNSResolver struct {
	mu      sync.Mutex
	cache   map[string]topologyReverseDNSCacheEntry
	config  topologyReverseDNSConfig
	warming atomic.Bool
}

type topologyReverseDNSCandidateCollector struct {
	resolver   *topologyReverseDNSResolver
	mu         sync.Mutex
	candidates map[string]struct{}
}

func newTopologyReverseDNSResolver() *topologyReverseDNSResolver {
	return newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{})
}

func newTopologyReverseDNSResolverWithConfig(config topologyReverseDNSConfig) *topologyReverseDNSResolver {
	config = normalizeTopologyReverseDNSConfig(config)
	return &topologyReverseDNSResolver{
		cache:  make(map[string]topologyReverseDNSCacheEntry),
		config: config,
	}
}

func normalizeTopologyReverseDNSConfig(config topologyReverseDNSConfig) topologyReverseDNSConfig {
	if config.lookup == nil {
		config.lookup = func(ctx context.Context, ip string) ([]string, error) {
			return net.DefaultResolver.LookupAddr(ctx, ip)
		}
	}
	if config.now == nil {
		config.now = time.Now
	}
	if config.timeout <= 0 {
		config.timeout = topologyReverseDNSLookupTimeout
	}
	if config.positiveTTL <= 0 {
		config.positiveTTL = topologyReverseDNSPositiveTTL
	}
	if config.negativeTTL <= 0 {
		config.negativeTTL = topologyReverseDNSNegativeTTL
	}
	if config.maxCandidates <= 0 {
		config.maxCandidates = topologyReverseDNSMaxCandidates
	}
	if config.maxEntries <= 0 {
		config.maxEntries = topologyReverseDNSMaxCacheEntries
	}
	if config.concurrency <= 0 {
		config.concurrency = topologyReverseDNSMaxConcurrency
	}
	return config
}

func (r *topologyReverseDNSResolver) lookupCached(ip string) string {
	if r == nil {
		return ""
	}
	ip, ok := normalizeTopologyReverseDNSCandidateIP(ip)
	if !ok {
		return ""
	}
	return r.lookupCachedNormalized(ip)
}

func (r *topologyReverseDNSResolver) lookupCachedNormalized(ip string) string {
	now := r.config.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.cache[ip]
	if !ok {
		return ""
	}
	if !now.Before(entry.expiresAt) {
		delete(r.cache, ip)
		return ""
	}
	entry.lastAccess = now
	r.cache[ip] = entry
	return entry.name
}

func (r *topologyReverseDNSResolver) newCandidateCollector() *topologyReverseDNSCandidateCollector {
	if r == nil {
		return nil
	}
	return &topologyReverseDNSCandidateCollector{
		resolver:   r,
		candidates: make(map[string]struct{}),
	}
}

func (c *topologyReverseDNSCandidateCollector) lookupCached(ip string) string {
	if c == nil {
		return ""
	}
	ip, ok := normalizeTopologyReverseDNSCandidateIP(ip)
	if !ok {
		return ""
	}

	c.mu.Lock()
	c.candidates[ip] = struct{}{}
	c.mu.Unlock()

	if c.resolver == nil {
		return ""
	}
	return c.resolver.lookupCachedNormalized(ip)
}

func (c *topologyReverseDNSCandidateCollector) collectedCandidates() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.candidates))
	for ip := range c.candidates {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func (r *topologyReverseDNSResolver) warm(ctx context.Context, candidates []string) {
	if r == nil || ctx == nil || len(candidates) == 0 || ctx.Err() != nil {
		return
	}
	if !r.warming.CompareAndSwap(false, true) {
		return
	}
	defer r.warming.Store(false)
	r.warmStarted(ctx, candidates)
}

func (r *topologyReverseDNSResolver) warmAsync(ctx context.Context, candidates []string) bool {
	if r == nil || ctx == nil || len(candidates) == 0 || ctx.Err() != nil {
		return false
	}
	if !r.warming.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer r.warming.Store(false)
		r.warmStarted(ctx, candidates)
	}()
	return true
}

func (r *topologyReverseDNSResolver) warmStarted(ctx context.Context, candidates []string) {
	ips := r.warmCandidates(candidates)
	if len(ips) == 0 {
		return
	}

	workers := min(r.config.concurrency, len(ips))

	jobs := make(chan string)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for ip := range jobs {
				r.resolveAndStore(ctx, ip)
			}
		}()
	}

sendJobs:
	for _, ip := range ips {
		select {
		case <-ctx.Done():
			break sendJobs
		case jobs <- ip:
		}
	}
	close(jobs)
	wg.Wait()
}

func (r *topologyReverseDNSResolver) warmCandidates(candidates []string) []string {
	now := r.config.now()
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if len(out) >= r.config.maxCandidates {
			break
		}
		ip, ok := normalizeTopologyReverseDNSCandidateIP(candidate)
		if !ok {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		if r.hasFreshEntry(ip, now) {
			continue
		}
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func (r *topologyReverseDNSResolver) hasFreshEntry(ip string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.cache[ip]
	if !ok {
		return false
	}
	if !now.Before(entry.expiresAt) {
		delete(r.cache, ip)
		return false
	}
	entry.lastAccess = now
	r.cache[ip] = entry
	return true
}

func (r *topologyReverseDNSResolver) resolveAndStore(ctx context.Context, ip string) {
	if ctx.Err() != nil {
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, r.config.timeout)
	names, err := r.config.lookup(lookupCtx, ip)
	cancel()
	if ctx.Err() != nil {
		return
	}

	name := ""
	ttl := r.config.negativeTTL
	if err == nil {
		name = normalizeTopologyReverseDNSName(names)
		if name != "" {
			ttl = r.config.positiveTTL
		}
	}
	if ttl <= 0 {
		return
	}
	r.store(ip, name, r.config.now().Add(ttl))
}

func (r *topologyReverseDNSResolver) store(ip, name string, expiresAt time.Time) {
	now := r.config.now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[ip] = topologyReverseDNSCacheEntry{
		name:       name,
		expiresAt:  expiresAt,
		lastAccess: now,
	}
	r.evictLocked(now)
}

func (r *topologyReverseDNSResolver) evictLocked(now time.Time) {
	for ip, entry := range r.cache {
		if !now.Before(entry.expiresAt) {
			delete(r.cache, ip)
		}
	}
	for len(r.cache) > r.config.maxEntries {
		oldestIP := ""
		var oldest time.Time
		for ip, entry := range r.cache {
			if oldestIP == "" || entry.lastAccess.Before(oldest) || (entry.lastAccess.Equal(oldest) && ip < oldestIP) {
				oldestIP = ip
				oldest = entry.lastAccess
			}
		}
		if oldestIP == "" {
			return
		}
		delete(r.cache, oldestIP)
	}
}

func normalizeTopologyReverseDNSName(names []string) string {
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

func normalizeTopologyReverseDNSCandidateIP(ip string) (string, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.IsValid() {
		return "", false
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return "", false
	}
	if addr.Is4() && addr == netip.MustParseAddr("255.255.255.255") {
		return "", false
	}
	return addr.String(), true
}
