// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "sync/atomic"

type engineStats struct {
	routeCacheHits   uint64
	routeCacheMisses uint64
}

// statsSnapshot is a lock-free snapshot of planner/runtime counters.
type statsSnapshot struct {
	RouteCacheHits   uint64
	RouteCacheMisses uint64
}

// stats returns a lock-free snapshot of engine counters.
func (e *Engine) stats() statsSnapshot {
	if e == nil {
		return statsSnapshot{}
	}
	return statsSnapshot{
		RouteCacheHits:   atomic.LoadUint64(&e.state.stats.routeCacheHits),
		RouteCacheMisses: atomic.LoadUint64(&e.state.stats.routeCacheMisses),
	}
}

func (e *Engine) addRouteCacheHit() {
	atomic.AddUint64(&e.state.stats.routeCacheHits, 1)
}

func (e *Engine) addRouteCacheMiss() {
	atomic.AddUint64(&e.state.stats.routeCacheMisses, 1)
}
