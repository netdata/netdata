// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import "time"

type SiteActivity struct {
	LastBeaconAt                              time.Time
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
	st.expire(now)
	var documents, views, errors uint64
	for _, el := range st.activity.entries {
		v := el.Value.(*activityObservation)
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
		BeaconsPerMin:          len(st.accepted),
		RejectedPerMin:         len(st.rejected),
		BotsPerMin:             len(st.bots),
		LastBeaconAgeS:         age,
		LastBeaconAt:           st.lastAccepted,
		ObservedSessions:       len(st.sessions),
		InvestigatedSessions:   st.investigatedSessions(),
		PageviewsWindow:        int(documents),
		JSErrorsWindow:         errors,
		ApplicationViewsWindow: views,
		WindowLost:             st.activityLoss.current(now) + st.currentIdentityLoss("document", now) + st.currentIdentityLoss("view", now),
		SessionsLost:           st.sessionLoss.current(now) + st.identitySessionLoss.current(now),
	}
}
func (st *siteState) evictActivity(now time.Time) {
	cutoff := now.Add(-time.Minute)
	st.accepted = evictTimes(st.accepted, cutoff)
	st.rejected = evictTimes(st.rejected, cutoff)
	st.bots = evictTimes(st.bots, cutoff)
}
