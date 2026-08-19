// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

type topologyRegistry struct {
	mu                    sync.RWMutex
	caches                map[*topologyCache]struct{}
	producerScopeID       string
	reverseDNS            *reversedns.Resolver
	reverseDNSWarmer      *topologyReverseDNSWarmer
	reverseDNSWarmCtx     context.Context
	reverseDNSSnapshotRun atomic.Bool
}

func newTopologyRegistryWithResolver(reverseDNS *reversedns.Resolver) *topologyRegistry {
	if reverseDNS == nil {
		panic("snmp_topology registry requires a non-nil reverse DNS resolver")
	}
	return &topologyRegistry{
		caches:           make(map[*topologyCache]struct{}),
		producerScopeID:  strings.TrimSpace(pluginconfig.RegistryUniqueID()),
		reverseDNS:       reverseDNS,
		reverseDNSWarmer: newTopologyReverseDNSWarmer(reverseDNS),
	}
}

func (r *topologyRegistry) register(cache *topologyCache) {
	if r == nil || cache == nil {
		return
	}
	r.mu.Lock()
	r.caches[cache] = struct{}{}
	r.mu.Unlock()
}

func (r *topologyRegistry) unregister(cache *topologyCache) {
	if r == nil || cache == nil {
		return
	}
	r.mu.Lock()
	delete(r.caches, cache)
	r.mu.Unlock()
}

func (r *topologyRegistry) snapshotWithOptions(options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	if r == nil {
		return topologymodel.Data{}, false, nil
	}
	options = topologyoptions.NormalizeQueryOptions(options)

	aggregate, ok := aggregateTopologyObservationSnapshots(r.observationSnapshots())
	if !ok {
		return topologymodel.Data{}, false, nil
	}
	aggregate.ProducerScopeID = r.producerScope()

	return buildSNMPTopologySnapshot(aggregate, options)
}

func (r *topologyRegistry) hasRenderableObservations() bool {
	if r == nil {
		return false
	}
	now := time.Now()
	for _, cache := range r.activeCaches() {
		if cache.hasRenderableObservationAt(now) {
			return true
		}
	}
	return false
}

func (r *topologyRegistry) producerScope() string {
	r.mu.RLock()
	scope := strings.TrimSpace(r.producerScopeID)
	r.mu.RUnlock()
	if scope != "" {
		return scope
	}

	scope = strings.TrimSpace(pluginconfig.RegistryUniqueID())
	if scope == "" {
		return ""
	}

	r.mu.Lock()
	if strings.TrimSpace(r.producerScopeID) == "" {
		r.producerScopeID = scope
	} else {
		scope = strings.TrimSpace(r.producerScopeID)
	}
	r.mu.Unlock()
	return scope
}

func (r *topologyRegistry) managedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget {
	if r == nil {
		return nil
	}
	return buildTopologyManagedFocusTargets(r.observationSnapshots())
}

func (r *topologyRegistry) setReverseDNSWarmContext(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.reverseDNSWarmCtx = ctx
	r.mu.Unlock()
}

func (r *topologyRegistry) reverseDNSContext() context.Context {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ctx := r.reverseDNSWarmCtx
	r.mu.RUnlock()
	return ctx
}

func (r *topologyRegistry) reverseDNSCandidateCollector() *topologyReverseDNSCandidateCollector {
	if r == nil || r.reverseDNS == nil {
		return nil
	}
	return newTopologyReverseDNSCandidateCollector(r.reverseDNS)
}

func (r *topologyRegistry) enqueueReverseDNSWarm(candidates []netip.Addr) bool {
	if r == nil || r.reverseDNSWarmer == nil || len(candidates) == 0 {
		return false
	}
	ctx := r.reverseDNSContext()
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	return r.reverseDNSWarmer.warmAsync(ctx, candidates)
}

func (r *topologyRegistry) enqueueReverseDNSWarmFromDefaultSnapshot() bool {
	if r == nil || r.reverseDNSWarmer == nil {
		return false
	}
	ctx := r.reverseDNSContext()
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	if !r.reverseDNSSnapshotRun.CompareAndSwap(false, true) {
		return false
	}

	go func() {
		defer r.reverseDNSSnapshotRun.Store(false)
		collector := r.reverseDNSCandidateCollector()
		if collector == nil {
			return
		}
		options := topologyoptions.DefaultQueryOptions()
		options.ResolveDNSName = collector.lookupCached
		if _, ok, err := r.snapshotWithOptions(options); err != nil || !ok {
			return
		}
		r.reverseDNSWarmer.warm(ctx, collector.collectedCandidates())
	}()
	return true
}
