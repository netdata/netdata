// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"time"
)

// SiteActivity is the live per-site read the rum-sites FUNCTION renders.
// Unlike Snapshot, computing it never touches breakdown ranking/top-N
// state, so polling it has no side effect on chart emission.
type SiteActivity struct {
	BeaconsPerMin  int
	RejectedPerMin int
	BotsPerMin     int // bot beacons filtered out
	LastBeaconAgeS int // -1 = never accepted a beacon
	ActiveSessions int
	// InvestigatedSessions is how many of ActiveSessions are kept in full
	// under investigate sampling.
	InvestigatedSessions int
	PageviewsWindow      int
	JSErrorsWindow       uint64
}

// Activity returns the current read for this site.
func (a *Aggregator) Activity() SiteActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	st := &a.site
	st.evictActivity(now, a.window)
	st.evictSessions(now)
	age := -1
	if !st.lastAccepted.IsZero() {
		age = int(now.Sub(st.lastAccepted) / time.Second)
	}
	var jsErr uint64
	for _, tc := range st.jsErrWindow {
		jsErr += tc.n
	}
	return SiteActivity{
		BeaconsPerMin:        len(st.accepted),
		RejectedPerMin:       len(st.rejected),
		BotsPerMin:           len(st.bots),
		LastBeaconAgeS:       age,
		ActiveSessions:       st.sess.Len(),
		InvestigatedSessions: st.investigatedSessions(),
		PageviewsWindow:      len(st.pvWindow),
		JSErrorsWindow:       jsErr,
	}
}

// evictActivity evicts the rings behind Activity independently of the
// breakdown window logic: beacons_per_min/rejected_per_min always use a
// fixed 60s cutoff, pageviews_window/js_errors_window follow the
// configured aggregation window. lastAccepted is never evicted so a
// long-silent site still reports an accurate age.
func (st *siteState) evictActivity(now time.Time, window time.Duration) {
	oneMinAgo := now.Add(-time.Minute)
	cutoff := now.Add(-window)
	st.accepted = evictTimes(st.accepted, oneMinAgo)
	st.rejected = evictTimes(st.rejected, oneMinAgo)
	st.bots = evictTimes(st.bots, oneMinAgo)
	st.pvWindow = evictTimes(st.pvWindow, cutoff)
	st.jsErrWindow = evictTsCounts(st.jsErrWindow, cutoff)
}
