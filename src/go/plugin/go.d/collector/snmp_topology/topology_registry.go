// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type topologyRegistry struct {
	mu              sync.RWMutex
	caches          map[*topologyCache]struct{}
	producerScopeID string
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		caches:          make(map[*topologyCache]struct{}),
		producerScopeID: strings.TrimSpace(pluginconfig.RegistryUniqueID()),
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
