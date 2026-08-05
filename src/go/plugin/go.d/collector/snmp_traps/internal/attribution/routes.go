// SPDX-License-Identifier: GPL-3.0-or-later

package attribution

import (
	"time"
)

// RouteTracker detects changes between raw and effective source identities.
// Its caller owns synchronization.
type RouteTracker struct {
	limit  int
	routes map[string]trackedRoute
}

type trackedRoute struct {
	effective string
	seen      time.Time
}

func NewRouteTracker(limit int) *RouteTracker {
	return &RouteTracker{
		limit:  limit,
		routes: make(map[string]trackedRoute),
	}
}

// Observe records an effective route and reports whether it changed.
func (t *RouteTracker) Observe(rawRouteKey, routeKey string, now time.Time) bool {
	if t == nil || rawRouteKey == "" || routeKey == "" {
		return false
	}
	previous := t.routes[rawRouteKey]
	transitioned := previous.effective != "" && previous.effective != routeKey
	t.routes[rawRouteKey] = trackedRoute{effective: routeKey, seen: now}
	t.Prune()
	return transitioned
}

// Prune retains the newest bounded set of raw source routes.
func (t *RouteTracker) Prune() {
	if t == nil || len(t.routes) == 0 {
		return
	}
	limit := t.limit
	if limit <= 0 {
		limit = 2000
	}
	if len(t.routes) <= limit {
		return
	}
	oldestKey := ""
	oldestSeen := time.Time{}
	for key, route := range t.routes {
		if oldestKey == "" || route.seen.Before(oldestSeen) || route.seen.Equal(oldestSeen) && key < oldestKey {
			oldestKey = key
			oldestSeen = route.seen
		}
	}
	if oldestKey != "" {
		delete(t.routes, oldestKey)
	}
}

func (t *RouteTracker) Len() int {
	if t == nil {
		return 0
	}
	return len(t.routes)
}
