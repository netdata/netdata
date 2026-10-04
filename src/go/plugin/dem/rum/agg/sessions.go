// SPDX-License-Identifier: GPL-3.0-or-later

// Session detail state holds per-session summaries and bounded event rings
// for rum-sessions and rum-session-events. The same LRU supplies the
// rum.sessions active-count gauge.
package agg

import (
	"container/list"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

const (
	maxTrackedSessions  = 2000 // per site, LRU by last seen
	sessionEventRingCap = 100
	sessionEventTextMax = 200
)

// sessionEvent is one ring entry (ts, type, page, text).
type sessionEvent struct {
	ts      time.Time
	typ     string // pageview|error|event|view|request
	page    string
	text    string
	traceID string // traced requests
	error   *sessionError
}

// Only error events retain error metadata; ordinary timeline entries keep the
// same small representation regardless of whether history is enabled.
type sessionError struct{ fingerprint, typ, message, stack string }

type sessionDetail struct {
	id                                      string
	started, lastSeen                       time.Time
	pageviews, errors                       uint64
	frustrations                            uint64 // rage, dead and error clicks
	userID                                  string // the site's own user id
	browser, device, country, city, version string
	entryPage, lastPage, lastView           string // lastView is bookkeeping only, not exposed
	events                                  []sessionEvent
	investigated                            bool // kept in full under investigate sampling
}

// touchSession updates (or creates) the session behind b.SessionID and
// appends any events this beacon produced to its ring. Eviction (cap
// maxTrackedSessions) drops the least-recently-seen session, same LRU
// discipline as the error-group state. hist receives self-contained
// events freshly appended this call; nil disables persistence. Only investigated sessions reach
// history; a sampled-out session that hits an always-keep condition
// is promoted and its in-memory timeline so far is written too. It returns
// whether the session is investigated.
func (st *siteState) touchSession(b *beacon.Beacon, now time.Time, hist HistorySink) bool {
	id := b.SessionID
	page := b.PageGroup
	if page == "" {
		page = "/"
	}
	var sd *sessionDetail
	if el, ok := st.sessIdx[id]; ok {
		sd = el.Value.(*sessionDetail)
		st.sess.MoveToFront(el)
	} else {
		for st.sess.Len() >= maxTrackedSessions {
			st.dropSession(st.sess.Back())
		}
		sd = &sessionDetail{
			id:           id,
			started:      now,
			entryPage:    page,
			investigated: beacon.SessionSampled(id, st.cfg.Investigate.rate()),
		}
		st.sessIdx[id] = st.sess.PushFront(sd)
	}
	promoted := !sd.investigated && st.cfg.Investigate.alwaysKeep(b)
	if promoted {
		sd.investigated = true
	}
	sd.lastSeen = now
	if b.Browser != "" {
		sd.browser = b.Browser
	}
	if b.Device != "" {
		sd.device = b.Device
	}
	if b.Country != "" {
		sd.country = b.Country
	}
	if b.City != "" {
		sd.city = b.City
	}
	if b.AppVersion != "" {
		sd.version = b.AppVersion
	}
	if b.UserID != "" {
		sd.userID = b.UserID
	}
	sd.lastPage = page

	var newEvents []sessionEvent
	if b.PageView {
		sd.pageviews++
		newEvents = append(newEvents, sd.pushEvent(sessionEvent{
			ts:   now,
			typ:  "pageview",
			page: page,
		}))
	}
	// A SPA route change with no full page load: inferred from the view
	// name changing between beacons (the base Faro web SDK has no
	// discrete "view changed" event of its own to key off instead). The
	// very first sighting of a view just establishes it, not a change.
	if b.View != "" {
		if sd.lastView != "" && sd.lastView != b.View {
			newEvents = append(newEvents, sd.pushEvent(sessionEvent{
				ts:   now,
				typ:  "view",
				page: page,
				text: b.View,
			}))
		}
		sd.lastView = b.View
	}
	for _, v := range b.Vitals {
		if v.Element != "" && v.Poor() {
			newEvents = append(newEvents, sd.pushEvent(sessionEvent{
				ts:   now,
				typ:  "vital",
				page: page,
				text: truncateRunes(poorVitalText(v), sessionEventTextMax),
			}))
		}
	}
	for _, err := range b.Errors {
		sd.errors++
		newEvents = append(
			newEvents,
			sd.pushEvent(
				sessionEvent{
					ts:   now,
					typ:  "error",
					page: page,
					text: truncateRunes(err.Type+": "+err.Message, sessionEventTextMax),
					error: &sessionError{
						fingerprint: err.Fingerprint,
						typ:         err.Type,
						message:     err.Message,
						stack:       truncateStack(err.Stack),
					},
				},
			),
		)
	}
	for _, ev := range b.Events {
		if ev.Name == beacon.NavigationEvent || ev.Name == beacon.ResourceEvent {
			continue // already charted separately; keep the timeline readable
		}
		if beacon.IsFrustration(ev.Name) {
			sd.frustrations++
			newEvents = append(newEvents, sd.pushEvent(sessionEvent{
				ts:   now,
				typ:  "frustration",
				page: page,
				text: truncateRunes(
					strings.ReplaceAll(ev.Name, "_", " ")+" on "+ev.Attrs["target"],
					sessionEventTextMax,
				),
			}))
			continue
		}
		if ev.TraceID != "" && (ev.Name == beacon.FetchTraceEvent || ev.Name == beacon.XHRTraceEvent) {
			newEvents = append(newEvents, sd.pushEvent(sessionEvent{
				ts:      now,
				typ:     "request",
				page:    page,
				text:    truncateRunes(requestText(ev.Attrs), sessionEventTextMax),
				traceID: ev.TraceID,
			}))
			continue
		}
		newEvents = append(
			newEvents,
			sd.pushEvent(
				sessionEvent{
					ts:   now,
					typ:  "event",
					page: page,
					text: truncateRunes(ev.Name, sessionEventTextMax),
				},
			),
		)
	}

	if promoted {
		newEvents = sd.events
	}
	if hist != nil && sd.investigated {
		if len(newEvents) == 0 {
			newEvents = []sessionEvent{{ts: now, typ: "activity", page: page}}
		}
		for _, e := range newEvents {
			rec := HistoryEvent{
				Site:      st.cfg.Name,
				SessionID: sd.id,
				TSUnixUS:  e.ts.UnixMicro(),
				Type:      e.typ,
				Page:      e.page,
				Text:      e.text,
				TraceID:   e.traceID,
				Browser:   sd.browser,
				Device:    sd.device,
				Country:   sd.country,
				City:      sd.city,
				Version:   sd.version,
				UserID:    sd.userID,
			}
			if e.error != nil {
				rec.Fingerprint = e.error.fingerprint
				rec.ErrorType = e.error.typ
				rec.Message = e.error.message
				rec.SampleStack = e.error.stack
			}
			hist.Event(rec)
		}
	}
	return sd.investigated
}

// pushEvent appends one ring entry (dropping the oldest once the
// in-memory ring cap is hit) and returns it, so callers can also forward
// it to the history writer without duplicating the ring's truncation logic.
func (sd *sessionDetail) pushEvent(e sessionEvent) sessionEvent {
	if len(sd.events) >= sessionEventRingCap {
		sd.events = sd.events[1:]
	}
	sd.events = append(sd.events, e)
	return e
}

// poorVitalText describes a poor vital and its element, e.g. "poor LCP
// 5200 ms on #hero img".
func poorVitalText(v beacon.Vital) string {
	val := strconv.FormatFloat(v.Value, 'f', 0, 64) + " ms"
	if v.Name == beacon.CLS {
		val = strconv.FormatFloat(v.Value, 'f', 2, 64)
	}
	return "poor " + v.Name + " " + val + " on " + v.Element
}

// requestText summarizes a traced request, e.g. "GET api.example.com/orders
// 200 · 7 ms", from the tracing add-on's event attributes.
func requestText(attrs map[string]string) string {
	target := attrs["url.full"]
	if i := strings.Index(target, "://"); i >= 0 {
		target = target[i+3:]
	}
	text := strings.TrimSpace(attrs["http.request.method"] + " " + target)
	if code := attrs["http.response.status_code"]; code != "" {
		text += " " + code
	}
	if ns, err := strconv.ParseFloat(attrs["duration_ns"], 64); err == nil && ns > 0 {
		text += " · " + strconv.FormatFloat(ns/1e6, 'f', 0, 64) + " ms"
	}
	return text
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (st *siteState) dropSession(el *list.Element) {
	delete(st.sessIdx, el.Value.(*sessionDetail).id)
	st.sess.Remove(el)
}

// ---- rum-session-events FUNCTION read ----

// SessionEventInfo is one rum-session-events row.
type SessionEventInfo struct {
	TS      time.Time
	Type    string
	Page    string
	Text    string
	TraceID string
}

// SessionEvents returns one session's event ring (oldest first).
// ok is false when the session id is unknown (caller: 404).
func (a *Aggregator) SessionEvents(sessionID string) ([]SessionEventInfo, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	el, ok := a.site.sessIdx[sessionID]
	if !ok {
		return nil, false
	}
	sd := el.Value.(*sessionDetail)
	out := make([]SessionEventInfo, len(sd.events))
	for i, e := range sd.events {
		out[i] = SessionEventInfo{
			TS:      e.ts,
			Type:    e.typ,
			Page:    e.page,
			Text:    e.text,
			TraceID: e.traceID,
		}
	}
	return out, true
}

func (st *siteState) investigatedSessions() int {
	n := 0
	for el := st.sess.Front(); el != nil; el = el.Next() {
		if el.Value.(*sessionDetail).investigated {
			n++
		}
	}
	return n
}
