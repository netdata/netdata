// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const (
	topologyReverseDNSMaxCandidates  = 1024
	topologyReverseDNSMaxConcurrency = 4
)

type topologyReverseDNSConfig struct {
	maxCandidates int
	concurrency   int
}

type topologyReverseDNSWarmer struct {
	resolver *reversedns.Resolver
	config   topologyReverseDNSConfig
	warming  atomic.Bool
}

type topologyReverseDNSCandidateCollector struct {
	resolver   *reversedns.Resolver
	mu         sync.Mutex
	candidates map[netip.Addr]struct{}
}

func newTopologyReverseDNSWarmer(resolver *reversedns.Resolver) *topologyReverseDNSWarmer {
	return newTopologyReverseDNSWarmerWithConfig(resolver, topologyReverseDNSConfig{})
}

func newTopologyReverseDNSWarmerWithConfig(resolver *reversedns.Resolver, config topologyReverseDNSConfig) *topologyReverseDNSWarmer {
	if resolver == nil {
		panic("snmp_topology reverse DNS warmer requires a non-nil resolver")
	}
	config = normalizeTopologyReverseDNSConfig(config)
	return &topologyReverseDNSWarmer{resolver: resolver, config: config}
}

func normalizeTopologyReverseDNSConfig(config topologyReverseDNSConfig) topologyReverseDNSConfig {
	if config.maxCandidates <= 0 {
		config.maxCandidates = topologyReverseDNSMaxCandidates
	}
	if config.concurrency <= 0 {
		config.concurrency = topologyReverseDNSMaxConcurrency
	}
	return config
}

func newTopologyReverseDNSCandidateCollector(resolver *reversedns.Resolver) *topologyReverseDNSCandidateCollector {
	if resolver == nil {
		return nil
	}
	return &topologyReverseDNSCandidateCollector{
		resolver:   resolver,
		candidates: make(map[netip.Addr]struct{}),
	}
}

func (c *topologyReverseDNSCandidateCollector) lookupCached(ip string) string {
	if c == nil {
		return ""
	}
	addr, ok := normalizeTopologyReverseDNSCandidateIP(ip)
	if !ok {
		return ""
	}

	c.mu.Lock()
	c.candidates[addr] = struct{}{}
	c.mu.Unlock()

	result := c.resolver.Lookup(addr)
	if result.State == reversedns.StatePositive {
		return result.Name
	}
	return ""
}

func (c *topologyReverseDNSCandidateCollector) collectedCandidates() []netip.Addr {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]netip.Addr, 0, len(c.candidates))
	for addr := range c.candidates {
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 })
	return out
}

func (r *topologyReverseDNSWarmer) warm(ctx context.Context, candidates []netip.Addr) {
	if r == nil || ctx == nil || len(candidates) == 0 || ctx.Err() != nil {
		return
	}
	if !r.warming.CompareAndSwap(false, true) {
		return
	}
	defer r.warming.Store(false)
	r.warmStarted(ctx, candidates)
}

func (r *topologyReverseDNSWarmer) warmAsync(ctx context.Context, candidates []netip.Addr) bool {
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

func (r *topologyReverseDNSWarmer) warmStarted(ctx context.Context, candidates []netip.Addr) {
	ips := r.warmCandidates(candidates)
	if len(ips) == 0 {
		return
	}

	workers := min(r.config.concurrency, len(ips))

	jobs := make(chan netip.Addr)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for addr := range jobs {
				_, _ = r.resolver.Resolve(ctx, addr)
			}
		}()
	}

sendJobs:
	for _, addr := range ips {
		select {
		case <-ctx.Done():
			break sendJobs
		case jobs <- addr:
		}
	}
	close(jobs)
	wg.Wait()
}

func (r *topologyReverseDNSWarmer) warmCandidates(candidates []netip.Addr) []netip.Addr {
	seen := make(map[netip.Addr]struct{}, len(candidates))
	out := make([]netip.Addr, 0, len(candidates))
	for _, addr := range candidates {
		if len(out) >= r.config.maxCandidates {
			break
		}
		addr = addr.Unmap()
		if !isEligibleTopologyReverseDNSAddress(addr) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		if r.resolver.Lookup(addr).State != reversedns.StateMiss {
			continue
		}
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Compare(out[j]) < 0 })
	return out
}

func normalizeTopologyReverseDNSCandidateIP(ip string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.IsValid() {
		return netip.Addr{}, false
	}
	addr = addr.Unmap()
	if !isEligibleTopologyReverseDNSAddress(addr) {
		return netip.Addr{}, false
	}
	return addr, true
}

func isEligibleTopologyReverseDNSAddress(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	if addr.Is4() && addr == netip.MustParseAddr("255.255.255.255") {
		return false
	}
	return true
}
