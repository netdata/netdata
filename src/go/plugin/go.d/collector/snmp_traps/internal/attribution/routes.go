// SPDX-License-Identifier: GPL-3.0-or-later

package attribution

import (
	"slices"
	"strings"
	"time"
)

// RouteTracker detects changes between raw and effective source identities.
// Its caller owns synchronization.
type RouteTracker struct {
	limit  int
	routes map[string]string
	seen   map[string]time.Time
}

func NewRouteTracker(limit int) *RouteTracker {
	return &RouteTracker{
		limit:  limit,
		routes: make(map[string]string),
		seen:   make(map[string]time.Time),
	}
}

// Observe records an effective route and reports whether it changed.
func (t *RouteTracker) Observe(rawRouteKey, routeKey string, now time.Time) bool {
	if t == nil || rawRouteKey == "" || routeKey == "" {
		return false
	}
	transitioned := t.routes[rawRouteKey] != "" && t.routes[rawRouteKey] != routeKey
	t.routes[rawRouteKey] = routeKey
	t.seen[rawRouteKey] = now
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
	type routeAge struct {
		key  string
		seen time.Time
	}
	ages := make([]routeAge, 0, len(t.routes))
	for rawRouteKey := range t.routes {
		ages = append(ages, routeAge{key: rawRouteKey, seen: t.seen[rawRouteKey]})
	}
	slices.SortFunc(ages, func(a, b routeAge) int {
		if a.seen.Equal(b.seen) {
			return strings.Compare(a.key, b.key)
		}
		if a.seen.Before(b.seen) {
			return -1
		}
		return 1
	})
	for _, age := range ages {
		if len(t.routes) <= limit {
			break
		}
		delete(t.routes, age.key)
		delete(t.seen, age.key)
	}
}

func (t *RouteTracker) Len() int {
	if t == nil {
		return 0
	}
	return len(t.routes)
}
