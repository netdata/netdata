// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type topologyRegistry struct {
	mu                    sync.RWMutex
	caches                map[*topologyCache]struct{}
	producerScopeID       string
	reverseDNS            *topologyReverseDNSResolver
	reverseDNSWarmCtx     context.Context
	reverseDNSSnapshotRun atomic.Bool
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		caches:          make(map[*topologyCache]struct{}),
		producerScopeID: strings.TrimSpace(pluginconfig.RegistryUniqueID()),
		reverseDNS:      newTopologyReverseDNSResolver(),
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

func (r *topologyRegistry) snapshotWithOptions(options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	if r == nil {
		return topologymodel.Data{}, false
	}
	options = topologyoptions.NormalizeQueryOptions(options)

	aggregate, ok := aggregateTopologyObservationSnapshots(r.observationSnapshots())
	if !ok {
		return topologymodel.Data{}, false
	}
	aggregate.ProducerScopeID = r.producerScope()

	return buildSNMPTopologySnapshot(aggregate, options)
}

func (r *topologyRegistry) hasRenderableSnapshot() bool {
	_, ok := r.snapshotWithOptions(topologyoptions.DefaultQueryOptions())
	return ok
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
	return r.reverseDNS.newCandidateCollector()
}

func (r *topologyRegistry) enqueueReverseDNSWarm(candidates []string) bool {
	if r == nil || r.reverseDNS == nil || len(candidates) == 0 {
		return false
	}
	ctx := r.reverseDNSContext()
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	return r.reverseDNS.warmAsync(ctx, candidates)
}

func (r *topologyRegistry) enqueueReverseDNSWarmFromDefaultSnapshot() bool {
	if r == nil || r.reverseDNS == nil {
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
		if _, ok := r.snapshotWithOptions(options); !ok {
			return
		}
		r.reverseDNS.warm(ctx, collector.collectedCandidates())
	}()
	return true
}
