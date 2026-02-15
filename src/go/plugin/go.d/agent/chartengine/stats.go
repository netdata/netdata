// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "sync/atomic"

type engineStats struct {
	routeCacheHits   uint64
	routeCacheMisses uint64
}

// Stats is a snapshot of planner/runtime counters.
type Stats struct {
	RouteCacheHits   uint64
	RouteCacheMisses uint64
}

// Stats returns a lock-free snapshot of engine counters.
func (e *Engine) Stats() Stats {
	if e == nil {
		return Stats{}
	}
	return Stats{
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
