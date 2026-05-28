// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sync"
)

type topologyQueryOptions struct {
	CollapseActorsByIP     bool
	EliminateNonIPInferred bool
	MapType                string
	InferenceStrategy      string
	ManagedDeviceFocus     string
	Depth                  int
	ResolveDNSName         func(ip string) string
}

type topologyRegistry struct {
	mu     sync.RWMutex
	caches map[*topologyCache]struct{}
}

type topologyManagedFocusTarget struct {
	Value string
	Name  string
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		caches: make(map[*topologyCache]struct{}),
	}
}

var snmpTopologyRegistry = newTopologyRegistry()

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

func (r *topologyRegistry) snapshot() (topologyData, bool) {
	return r.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
		ResolveDNSName:         resolveTopologyReverseDNSName, // live resolver — warms the cache
	})
}

func (r *topologyRegistry) snapshotWithOptions(options topologyQueryOptions) (topologyData, bool) {
	if r == nil {
		return topologyData{}, false
	}
	options = normalizeTopologyQueryOptions(options)

	aggregate, ok := aggregateTopologyObservationSnapshots(r.observationSnapshots())
	if !ok {
		return topologyData{}, false
	}

	return buildSNMPTopologySnapshot(aggregate, options)
}

func normalizeTopologyQueryOptions(options topologyQueryOptions) topologyQueryOptions {
	options.MapType = normalizeTopologyMapType(options.MapType)
	if options.MapType == "" {
		options.MapType = topologyMapTypeLLDPCDPManaged
	}
	options.InferenceStrategy = normalizeTopologyInferenceStrategy(options.InferenceStrategy)
	if options.InferenceStrategy == "" {
		options.InferenceStrategy = topologyInferenceStrategyFDBMinimumKnowledge
	}
	options.ManagedDeviceFocus = formatTopologyManagedFocuses(parseTopologyManagedFocuses(options.ManagedDeviceFocus))
	if options.Depth != topologyDepthAllInternal {
		if options.Depth < topologyDepthMin {
			options.Depth = topologyDepthMin
		} else if options.Depth > topologyDepthMax {
			options.Depth = topologyDepthMax
		}
	}
	return options
}

func (r *topologyRegistry) managedDeviceFocusTargets() []topologyManagedFocusTarget {
	if r == nil {
		return nil
	}
	return buildTopologyManagedFocusTargets(r.observationSnapshots())
}
