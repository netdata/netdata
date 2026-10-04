// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Reject counts a collector rejection (beacon.Reject* reasons) and, for
// origin/rate/size, feeds the 60s ring behind rejected_per_min — an invalid body never reached a rate worth surfacing.
func (a *Aggregator) Reject(site, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	if site != st.cfg.Name {
		return
	}
	st.counters[reason]++
	switch reason {
	case beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize:
		st.rejected = append(st.rejected, a.now())
	case beacon.RejectBot:
		st.bots = append(st.bots, a.now())
	}
}

// Add bumps a site-level monotonic counter (used by the OTLP exporter for
// otlp_sent/dropped/errors).
func (a *Aggregator) Add(counter string, n uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.site.counters[counter] += n
}

// Result describes the decisions made while processing a normalized observation.
// Its zero value indicates the observation did not belong to this aggregator.
type Result struct {
	Accepted     bool
	PageView     bool
	Investigated bool
}

// Ingest records measurements and investigation events without modifying b.
// History is enqueued under the state lock before the result reaches export.
func (a *Aggregator) Ingest(b *beacon.Beacon) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	if b.Site != st.cfg.Name {
		return Result{}
	}
	now := b.Received
	if now.IsZero() {
		now = a.now()
	}
	st.counters[CounterAccepted]++
	st.accepted = append(st.accepted, now)
	if now.After(st.lastAccepted) {
		st.lastAccepted = now
	}

	var pageView bool
	// Page view: one per (session, page) within pageviewDedup. Beacons
	// without a session id cannot be de-duplicated and count as views.
	page := b.PageID
	if page == "" {
		page = b.Path
	}
	if b.SessionID == "" {
		pageView = true
	} else {
		key := pageviewKey{
			session: b.SessionID,
			page:    page,
		}
		last, seen := st.dedup[key]
		if !seen || now.Sub(last) > pageviewDedup {
			pageView = true
		}
		if !seen || now.After(last) {
			st.dedup[key] = now
		}
	}

	var frustrations uint64
	for _, e := range b.Events {
		if c, ok := frustrationCounters[e.Name]; ok {
			st.counters[c]++
			frustrations++
		}
	}
	bd := breakdowns(b)
	if pageView {
		st.counters[CounterPageviews]++
		st.pvWindow = append(st.pvWindow, now)
	}
	if n := uint64(len(b.Errors)); n > 0 {
		st.counters[CounterJSErrors] += n
		st.jsErrWindow = append(st.jsErrWindow, tsCount{now, n})
		for _, e := range b.Errors {
			st.recordError(e, now)
		}
	}
	for _, kv := range bd {
		real := st.group(kv.kind, kv.value, now) // the actual tracked instance, nil if the per-kind cap is full
		target := real                           // where monotonic counters land (routed to other pre-promotion)
		if real == nil {
			target = st.otherGroup(kv.kind)
		} else if !st.top[kv.kind][kv.value] {
			// tracked but not an emitted instance: counters go to other,
			// ranking still sees the views so it can enter the top set
			if pageView {
				real.pvTimes = append(real.pvTimes, now)
			}
			if now.After(real.lastSeen) {
				real.lastSeen = now
			}
			st.recordVitals(b, kv.kind, kv.value, now)
			target = st.otherGroup(kv.kind)
		} else {
			if pageView {
				real.pvTimes = append(real.pvTimes, now)
			}
			if now.After(real.lastSeen) {
				real.lastSeen = now
			}
			st.recordVitals(b, kv.kind, kv.value, now)
		}
		if pageView {
			target.pageviews++
		}
		target.jsErrors += uint64(len(b.Errors))

		// rum-pages reads the real page group directly, not
		// routed through other: it always describes that page's own
		// activity, top-N ranked or not.
		if kv.kind == KindPage && real != nil {
			if n := len(b.Errors); n > 0 {
				real.errWindow = append(real.errWindow, tsCount{now, uint64(n)})
			}
			if frustrations > 0 {
				real.frWindow = append(real.frWindow, tsCount{now, frustrations})
			}
			for _, v := range b.Vitals {
				if v.Element == "" {
					continue
				}
				if len(real.elements) >= maxPageElements {
					real.elements = real.elements[1:]
				}
				real.elements = append(
					real.elements,
					elemSample{
						ts:    now,
						vital: v.Name,
						sel:   v.Element,
						poor:  v.Poor(),
					},
				)
			}
			if b.SessionID != "" {
				if real.sessions == nil {
					real.sessions = map[string]struct{}{}
				}
				if len(real.sessions) < maxPageGroupSessions {
					real.sessions[b.SessionID] = struct{}{}
				}
			}
		}
	}
	st.recordVitals(b, "", "", now)
	if b.Navigation != nil {
		if b.Navigation.HasLoad {
			st.addSample(seriesKey{navLoadName, "", ""}, b.Navigation.LoadMS, now)
		}
		if b.Navigation.HasDCL {
			st.addSample(seriesKey{navDCLName, "", ""}, b.Navigation.DCLMS, now)
		}
	}
	for _, r := range b.Resources {
		st.recordResource(b.PageHost, r, now)
	}

	// Investigate sampling: measure (everything above) always runs;
	// history and events follow the session's decision.
	var investigated bool
	if b.SessionID != "" {
		investigated = st.touchSession(b, pageView, now, a.history)
	} else {
		inv := st.cfg.Investigate
		investigated = inv.rate() >= 1 || inv.alwaysKeep(b)
	}
	// Sessions already emitted self-contained error events with their timeline.
	// Sessionless errors remain investigable without creating a parent session.
	if investigated && b.SessionID == "" && a.history != nil {
		for _, e := range b.Errors {
			if e.Fingerprint == "" {
				continue
			}
			a.history.Event(HistoryEvent{
				Site:        st.cfg.Name,
				TSUnixUS:    now.UnixMicro(),
				Type:        "error",
				Page:        b.PageGroup,
				Browser:     b.Browser,
				Device:      b.Device,
				Country:     b.Country,
				City:        b.City,
				Version:     b.AppVersion,
				UserID:      b.UserID,
				Fingerprint: e.Fingerprint,
				ErrorType:   e.Type,
				Message:     e.Message,
				Text:        truncateRunes(e.Type+": "+e.Message, sessionEventTextMax),
				SampleStack: truncateStack(e.Stack),
			})
		}
	}

	a.appendLive(b, pageView, now)
	return Result{
		Accepted:     true,
		PageView:     pageView,
		Investigated: investigated,
	}
}

type kv struct{ kind, value string }

// breakdowns lists the (kind, value) pairs a beacon contributes to. An
// unknown country is excluded from the country breakdown.
func breakdowns(b *beacon.Beacon) []kv {
	out := make([]kv, 0, 4)
	browser := b.Browser
	if browser == "" {
		browser = unknownValue
	}
	out = append(out, kv{KindBrowser, browser})
	device := b.Device
	if device == "" {
		device = beacon.DeviceDesktop
	}
	out = append(out, kv{KindDevice, device})
	if b.Country != "" {
		out = append(out, kv{KindCountry, b.Country})
	}
	page := b.PageGroup
	if page == "" {
		page = "/"
	}
	out = append(out, kv{KindPage, page})
	if b.AppVersion != "" {
		out = append(out, kv{KindVersion, b.AppVersion})
	}
	return out
}

// group returns the tracked group for (kind, value), creating it unless
// the per-kind tracking cap is reached (nil then).
func (st *siteState) group(kind, value string, now time.Time) *group {
	k := groupKey{kind, value}
	if g, ok := st.groups[k]; ok {
		return g
	}
	if st.groupN[kind] >= maxTrackedGroups {
		return nil
	}
	g := &group{
		lastSeen: now,
	}
	st.groups[k] = g
	st.groupN[kind]++
	return g
}

func (st *siteState) otherGroup(kind string) *group {
	g, ok := st.other[kind]
	if !ok {
		g = &group{}
		st.other[kind] = g
	}
	return g
}
