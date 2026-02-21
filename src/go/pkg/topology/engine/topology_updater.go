// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "sync"

// TopologyBuilderFunc builds the current topology snapshot.
type TopologyBuilderFunc func() (NetworkRouterTopology, error)

// TopologyUpdater ports non-routing runtime semantics from Enlinkd TopologyUpdater.
type TopologyUpdater struct {
	mu sync.RWMutex

	topology   NetworkRouterTopology
	builder    TopologyBuilderFunc
	parseFn    func() bool
	refreshFn  func()
	hasRun     bool
	forceRun   bool
	runFailed  bool
	lastFailed error
}

// NewTopologyUpdater builds an updater. parseFn and refreshFn are optional.
func NewTopologyUpdater(builder TopologyBuilderFunc, parseFn func() bool, refreshFn func()) *TopologyUpdater {
	if builder == nil {
		return &TopologyUpdater{}
	}
	return &TopologyUpdater{
		builder:   builder,
		parseFn:   parseFn,
		refreshFn: refreshFn,
		topology:  NetworkRouterTopology{},
	}
}

// RunSchedulable executes one discovery cycle.
func (u *TopologyUpdater) RunSchedulable() {
	if u == nil || u.builder == nil {
		return
	}

	u.mu.Lock()
	hasRun := u.hasRun
	forceRun := u.forceRun
	if forceRun {
		u.forceRun = false
	}
	u.mu.Unlock()

	if !hasRun {
		topology, err := u.builder()
		u.mu.Lock()
		defer u.mu.Unlock()
		if err != nil {
			u.runFailed = true
			u.lastFailed = err
			return
		}
		u.topology = CloneNetworkRouterTopology(topology)
		u.hasRun = true
		u.runFailed = false
		u.lastFailed = nil
		return
	}

	parseUpdates := false
	if u.parseFn != nil {
		parseUpdates = u.parseFn()
	}
	if !parseUpdates && !forceRun {
		return
	}
	if u.refreshFn != nil {
		u.refreshFn()
	}

	topology, err := u.builder()
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.runFailed = true
		u.lastFailed = err
		return
	}
	u.topology = CloneNetworkRouterTopology(topology)
	u.runFailed = false
	u.lastFailed = nil
}

// GetTopology returns a non-blocking clone of the current topology.
func (u *TopologyUpdater) GetTopology() NetworkRouterTopology {
	if u == nil {
		return NetworkRouterTopology{}
	}
	u.mu.RLock()
	topology := CloneNetworkRouterTopology(u.topology)
	u.mu.RUnlock()
	return topology
}

// ForceRun requests recomputation on the next RunSchedulable call.
func (u *TopologyUpdater) ForceRun() {
	if u == nil {
		return
	}
	u.mu.Lock()
	u.forceRun = true
	u.mu.Unlock()
}

// HasRun reports if the first successful run completed.
func (u *TopologyUpdater) HasRun() bool {
	if u == nil {
		return false
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.hasRun
}

// LastError returns the last builder error, if any.
func (u *TopologyUpdater) LastError() error {
	if u == nil {
		return nil
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.lastFailed
}

// CloneNetworkRouterTopology deep-copies topology structures.
func CloneNetworkRouterTopology(topology NetworkRouterTopology) NetworkRouterTopology {
	out := NetworkRouterTopology{
		Vertices:      append([]NetworkRouterVertex(nil), topology.Vertices...),
		Edges:         append([]NetworkRouterEdge(nil), topology.Edges...),
		DefaultVertex: topology.DefaultVertex,
	}
	return out
}
