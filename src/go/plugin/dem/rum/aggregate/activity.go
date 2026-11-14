// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import "time"

type SiteActivity struct {
	VnodeGUID                                 string
	LastBeaconAt                              time.Time
	BeaconsPerMin, RejectedPerMin, BotsPerMin int
	LastBeaconAgeS                            int
	ObservedSessions, InvestigatedSessions    int
	PageviewsWindow                           int
	JSErrorsWindow, ApplicationViewsWindow    uint64
}

func (a *Aggregator) Activity() SiteActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	st := &a.site
	st.expire(now)
	var documents, views, errors uint64
	for _, el := range st.activity.entries {
		v := &el.Value
		documents += v.documents
		views += v.views
		errors += v.errors
	}
	age := -1
	if !st.lastAccepted.IsZero() {
		age = int(now.Sub(st.lastAccepted) / time.Second)
		if age < 0 {
			age = 0
		}
	}
	return SiteActivity{
		VnodeGUID:              st.cfg.VnodeGUID,
		BeaconsPerMin:          len(st.accepted),
		RejectedPerMin:         len(st.rejected),
		BotsPerMin:             len(st.bots),
		LastBeaconAgeS:         age,
		LastBeaconAt:           st.lastAccepted,
		ObservedSessions:       st.sessions.len(),
		InvestigatedSessions:   st.investigatedSessions(),
		PageviewsWindow:        int(documents),
		JSErrorsWindow:         errors,
		ApplicationViewsWindow: views,
	}
}

// Cleanup cadence follows the one-minute receiver activity window. This bounds
// timestamp retention during delayed publication without a scan on every beacon.
func (st *siteState) expireActivityIfDue(now time.Time) {
	if st.lastActivityExpiry.IsZero() || now.Sub(st.lastActivityExpiry) >= time.Minute {
		st.evictActivity(now)
	}
}
func (st *siteState) evictActivity(now time.Time) {
	st.lastActivityExpiry = now
	cutoff := now.Add(-time.Minute)
	st.accepted = evictTimes(st.accepted, cutoff)
	st.rejected = evictTimes(st.rejected, cutoff)
	st.bots = evictTimes(st.bots, cutoff)
}
func evictTimes(ts []time.Time, cutoff time.Time) []time.Time {
	n := 0
	for i, t := range ts {
		if !t.Before(cutoff) {
			if n != i {
				ts[n] = t
			}
			n++
		}
	}
	return ts[:n]
}
