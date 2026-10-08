// SPDX-License-Identifier: GPL-3.0-or-later

// Session detail state holds per-session summaries and bounded event rings
// for rum-sessions and rum-session-events, independent of window session measurement.
package aggregate

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
	metricID string
	revision uint64
	ts       time.Time
	typ      string // pageview|error|event|view|request
	page     string
	text     string
	traceID  string // traced requests
	error    *sessionError
	context  eventContext
}

// eventContext is captured when an observation arrives. Replayed context must
// not acquire the identity, release or location of a later promoting event.
type eventContext struct {
	browser, device, country, version, userID string
	experienceID, view, viewID                string
}

// Only error events retain error metadata; ordinary timeline entries keep the
// same small representation regardless of whether history is enabled.
type sessionError struct{ fingerprint, typ, message, stack string }

type sessionDetail struct {
	id                  string
	started, lastSeen   time.Time
	pageviews, errors   uint64
	frustrations        uint64 // rage, dead and error clicks
	entryPage, lastPage string
	events              []sessionEvent
	investigated        bool // currently selected for retained/exported detail
}

// touchSession updates (or creates) the session behind b.SessionID and
// appends any events this beacon produced to its ring. Eviction (cap
// maxTrackedSessions) drops the least-recently-seen session, same LRU
// discipline as the error-group state. hist receives self-contained
// events freshly appended this call; nil disables persistence. Only investigated sessions reach
// history; a sampled-out session that hits an always-keep condition
// is promoted and its in-memory timeline so far is written too. It returns
// whether the session is investigated.
func (st *siteState) touchSession(b *beacon.Beacon, pageView bool, now time.Time, hist HistorySink) bool {
	id := b.SessionID
	page := b.PageGroup
	if page == "" {
		page = "/"
	}
	var sd *sessionDetail
	if el := st.sessIdx[id]; el != nil && now.Sub(el.Value.(*sessionDetail).lastSeen) > beacon.SessionTTL {
		st.dropSession(el)
	}
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
			investigated: beacon.SessionSampled(id, st.cfg.Investigate.Rate),
		}
		st.sessIdx[id] = st.sess.PushFront(sd)
	}
	promoted := !sd.investigated && st.cfg.Investigate.alwaysKeep(b)
	if promoted {
		sd.investigated = true
		// Replay only the preceding context. Current events must never be limited
		// by the ring or inherit the context of this promoting observation.
		if hist != nil {
			for _, event := range sd.events {
				hist.Event(event.history(st.cfg.Name, id))
			}
		}
	}
	if now.After(sd.lastSeen) {
		sd.lastSeen = now
	}
	sd.lastPage = page
	context := eventContext{
		experienceID: b.ExperienceID,
		view:         b.View,
		viewID:       b.ViewID,
		browser:      b.Browser,
		device:       b.Device,
		country:      b.Country,
		version:      b.AppVersion,
		userID:       b.UserID,
	}
	recorded := false
	record := func(event sessionEvent) {
		if event.typ != "vital" {
			event.context = context
		}
		sd.pushEvent(event)
		recorded = true
		if hist != nil && sd.investigated {
			hist.Event(event.history(st.cfg.Name, id))
		}
	}
	if pageView {
		var revision uint64
		for _, ev := range b.Events {
			if ev.Kind == beacon.EventDocument {
				revision = ev.Revision
				break
			}
		}
		sd.pageviews++
		record(sessionEvent{
			ts:       now,
			typ:      "pageview",
			revision: revision,
			page:     page,
		})
	}

	for _, v := range b.Vitals {
		if v.Poor() {
			origin := context
			originPage := page
			if attrs := v.Origin; attrs != nil {
				origin.browser, origin.device, origin.country, origin.version = attrs.Browser, attrs.Device, attrs.Country, attrs.AppVersion
				originPage = attrs.PageGroup
			}
			record(
				sessionEvent{
					ts:       now,
					typ:      "vital",
					page:     originPage,
					context:  origin,
					metricID: v.ID,
					revision: v.Revision,
					text:     truncateRunes(poorVitalText(v), sessionEventTextMax),
				},
			)
		}
	}
	for _, err := range b.Errors {
		sd.errors++
		record(sessionEvent{
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
		})
	}
	for _, ev := range b.Events {
		if ev.Kind == beacon.EventDocument || ev.Kind == beacon.EventNavigation || ev.Kind == beacon.EventResource ||
			ev.Kind == beacon.EventSession {
			continue
		}
		if ev.Kind == beacon.EventView {
			target := ev.Attrs["toView"]
			if target == "" {
				target = page
			}
			record(sessionEvent{
				ts:       now,
				typ:      "view",
				revision: ev.Revision,
				page:     target,
				text:     target,
			})
			continue
		}
		if beacon.IsFrustration(ev.Name) {
			sd.frustrations++
			record(sessionEvent{
				ts:   now,
				typ:  "frustration",
				page: page,
				text: truncateRunes(
					strings.ReplaceAll(ev.Name, "_", " ")+" on "+ev.Attrs["target"],
					sessionEventTextMax,
				),
			})
			continue
		}
		if ev.TraceID != "" && ev.Kind == beacon.EventRequest {
			record(
				sessionEvent{
					ts:      now,
					typ:     "request",
					page:    page,
					text:    truncateRunes(requestText(ev.Attrs), sessionEventTextMax),
					traceID: ev.TraceID,
				},
			)
			continue
		}
		record(sessionEvent{
			ts:   now,
			typ:  "event",
			page: page,
			text: truncateRunes(ev.Name, sessionEventTextMax),
		})
	}
	if hist != nil && sd.investigated && !recorded &&
		(len(b.Vitals) > 0 || len(b.Logs) > 0 || len(b.Spans) > 0 || b.Navigation != nil || len(b.Resources) > 0) {
		event := sessionEvent{
			ts:      now,
			typ:     "activity",
			page:    page,
			context: context,
		}
		hist.Event(event.history(st.cfg.Name, id))
	}
	return sd.investigated
}

func (e sessionEvent) history(site, sessionID string) HistoryEvent {
	rec := HistoryEvent{
		ExperienceID: e.context.experienceID,
		View:         e.context.view,
		ViewID:       e.context.viewID,
		MetricID:     e.metricID,
		Revision:     e.revision,
		Site:         site,
		SessionID:    sessionID,
		ObservedUS:   e.ts.UnixMicro(),
		Type:         e.typ,
		Page:         e.page,
		Text:         e.text,
		TraceID:      e.traceID,
		Browser:      e.context.browser,
		Device:       e.context.device,
		Country:      e.context.country,
		Version:      e.context.version,
		UserID:       e.context.userID,
	}
	if e.error != nil {
		rec.Fingerprint, rec.ErrorType = e.error.fingerprint, e.error.typ
		rec.Message, rec.SampleStack = e.error.message, e.error.stack
	}
	return rec
}

// pushEvent retains bounded context independently of current-event delivery.
func (sd *sessionDetail) pushEvent(e sessionEvent) {
	if len(sd.events) >= sessionEventRingCap {
		sd.events = sd.events[1:]
	}
	sd.events = append(sd.events, e)
}

// poorVitalText describes a poor vital and its element, e.g. "poor LCP
// 5200 ms on #hero img".
func poorVitalText(v beacon.Vital) string {
	val := strconv.FormatFloat(v.Value, 'f', 0, 64) + " ms"
	if v.Name == beacon.CLS {
		val = strconv.FormatFloat(v.Value, 'f', 2, 64)
	}
	text := "poor " + v.Name + " " + val
	if v.Element != "" {
		text += " on " + v.Element
	}
	return text
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

// LRU order follows ingestion, which can differ from receipt time.
func (st *siteState) evictSessions(now time.Time) {
	for el := st.sess.Back(); el != nil; {
		previous := el.Prev()
		if now.Sub(el.Value.(*sessionDetail).lastSeen) > beacon.SessionTTL {
			st.dropSession(el)
		}
		el = previous
	}
}

func (st *siteState) dropSession(el *list.Element) {
	delete(st.sessIdx, el.Value.(*sessionDetail).id)
	st.sess.Remove(el)
}

// ---- rum-session-events FUNCTION read ----

// SessionEventInfo is one rum-session-events row.
type SessionEventInfo struct {
	ExperienceID, View, ViewID, MetricID string
	Revision                             uint64
	TS                                   time.Time
	Type                                 string
	Page                                 string
	Text                                 string
	TraceID                              string
	UserID                               string
}

// SessionEvents returns one session's event ring (oldest first).
// ok is false when the session id is unknown (caller: 404).
func (a *Aggregator) SessionEvents(sessionID string) ([]SessionEventInfo, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.site.evictSessions(a.now())
	el, ok := a.site.sessIdx[sessionID]
	if !ok {
		return nil, false
	}
	sd := el.Value.(*sessionDetail)
	out := make([]SessionEventInfo, len(sd.events))
	for i, e := range sd.events {
		out[i] = SessionEventInfo{
			ExperienceID: e.context.experienceID,
			View:         e.context.view,
			ViewID:       e.context.viewID,
			MetricID:     e.metricID,
			Revision:     e.revision,
			TS:           e.ts,
			Type:         e.typ,
			Page:         e.page,
			Text:         e.text,
			TraceID:      e.traceID,
			UserID:       e.context.userID,
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
