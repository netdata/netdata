// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type topologyRegistry struct {
	mu     sync.RWMutex
	caches map[*topologyCache]struct{}
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		caches: make(map[*topologyCache]struct{}),
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

	return buildSNMPTopologySnapshot(aggregate, options)
}

func (r *topologyRegistry) managedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget {
	if r == nil {
		return nil
	}
	return buildTopologyManagedFocusTargets(r.observationSnapshots())
}
