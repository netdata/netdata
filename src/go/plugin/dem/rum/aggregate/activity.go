// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import "time"

type SiteActivity struct {
	BeaconsPerMin, RejectedPerMin, BotsPerMin int
	LastBeaconAgeS                            int
	ObservedSessions, InvestigatedSessions    int
	PageviewsWindow                           int
	JSErrorsWindow, ApplicationViewsWindow    uint64
	WindowLost, SessionsLost                  uint64
}

func (a *Aggregator) Activity() SiteActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	st := &a.site
	r := st.readWindow(now)
	age := -1
	if !st.lastAccepted.IsZero() {
		age = int(now.Sub(st.lastAccepted) / time.Second)
		if age < 0 {
			age = 0
		}
	}
	return SiteActivity{
		BeaconsPerMin:          len(st.accepted),
		RejectedPerMin:         len(st.rejected),
		BotsPerMin:             len(st.bots),
		LastBeaconAgeS:         age,
		ObservedSessions:       r.sessions,
		InvestigatedSessions:   st.investigatedSessions(),
		PageviewsWindow:        int(r.global.documents),
		JSErrorsWindow:         r.global.errors,
		ApplicationViewsWindow: r.global.views,
		WindowLost:             r.windowLost,
		SessionsLost:           r.sessionsLost,
	}
}
func (st *siteState) evictActivity(now time.Time) {
	cutoff := now.Add(-time.Minute)
	st.accepted = evictTimes(st.accepted, cutoff)
	st.rejected = evictTimes(st.rejected, cutoff)
	st.bots = evictTimes(st.bots, cutoff)
}
