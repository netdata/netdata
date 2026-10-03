// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// maxTrackedSessions bounds memory under session-id churn; past this the
// tracker sweeps expired entries before adding more (mirrors the
// aggregator's own session LRU cap).
const maxTrackedSessions = 100000

// sessionTracker decides whether a beacon's session id is new (first
// sighting or TTL-expired), independent of the aggregator's own session
// gauge: the two serve different purposes (event stream vs. a live
// count) and must not share state.
type sessionTracker struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newSessionTracker() *sessionTracker {
	return &sessionTracker{
		seen: map[string]time.Time{},
	}
}

// starts reports whether id counts as a new session at now, and records
// the sighting either way.
func (t *sessionTracker) starts(id string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.seen) >= maxTrackedSessions {
		for k, v := range t.seen {
			if now.Sub(v) > beacon.SessionTTL {
				delete(t.seen, k)
			}
		}
	}
	last, ok := t.seen[id]
	t.seen[id] = now
	return !ok || now.Sub(last) > beacon.SessionTTL
}
