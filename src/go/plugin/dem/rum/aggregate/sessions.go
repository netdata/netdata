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
	vnodeGUID                                 string
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

// touchSession updates (or creates) the session behind b.SessionID and records
// the timeline entries b produced. hist receives self-contained events recorded
// by this call; nil disables persistence. Only investigated sessions reach
// history: a sampled-out session that hits an always-keep condition is promoted,
// and its retained timeline is written before the current entries. It returns
// whether the session is investigated.
func (st *siteState) touchSession(b *beacon.Beacon, pageView bool, now time.Time, hist HistorySink) bool {
	page := b.PageGroup
	if page == "" {
		page = "/"
	}
	sd := st.trackSession(b.SessionID, page, now)
	if !sd.investigated && st.cfg.Investigate.alwaysKeep(b) {
		sd.investigated = true
		// Replay only the preceding context. Current events must never be limited
		// by the ring or inherit the context of this promoting observation.
		if hist != nil {
			for _, event := range sd.events {
				hist.Event(event.history(st.cfg.Name, sd.id))
			}
		}
	}
	if now.After(sd.lastSeen) {
		sd.lastSeen = now
	}
	sd.lastPage = page
	t := timeline{
		site:    st.cfg.Name,
		session: sd,
		hist:    hist,
		context: newEventContext(b),
	}
	if pageView {
		sd.pageviews++
		t.record(sessionEvent{
			ts:       now,
			typ:      "pageview",
			revision: documentRevision(b),
			page:     page,
		})
	}
	t.recordPoorVitals(b.Vitals, page, now)
	for _, err := range b.Errors {
		sd.errors++
		event, detail := newErrorEvent(err, now, page)
		event.error = &detail
		t.record(event)
	}
	t.recordEvents(b.Events, page, now)
	// An investigated report without timeline entries still marks session activity.
	if hist != nil && sd.investigated && !t.recorded &&
		(len(b.Vitals) > 0 || len(b.Logs) > 0 || len(b.Spans) > 0 || b.Navigation != nil || len(b.Resources) > 0) {
		event := sessionEvent{
			ts:      now,
			typ:     "activity",
			page:    page,
			context: t.context,
		}
		hist.Event(event.history(st.cfg.Name, sd.id))
	}
	return sd.investigated
}

// trackSession returns the detail of a current session, or creates one with its
// sampling decision. A session idle longer than sessionTTL starts over; at
// maxTrackedSessions the least recently seen session is evicted.
func (st *siteState) trackSession(id, page string, now time.Time) *sessionDetail {
	if el := st.sessIdx[id]; el != nil {
		sd := el.Value.(*sessionDetail)
		if now.Sub(sd.lastSeen) <= sessionTTL {
			st.sess.MoveToFront(el)
			return sd
		}
		st.dropSession(el)
	}
	for st.sess.Len() >= maxTrackedSessions {
		st.dropSession(st.sess.Back())
	}
	sd := &sessionDetail{
		id:           id,
		started:      now,
		entryPage:    page,
		investigated: SessionSampled(id, st.cfg.Investigate.Rate),
	}
	st.sessIdx[id] = st.sess.PushFront(sd)
	return sd
}

// recordSessionless decides detail for a report without a session. It has no
// timeline: an investigated error goes to history directly, with the page group
// as reported.
func (st *siteState) recordSessionless(b *beacon.Beacon, now time.Time, hist HistorySink) bool {
	investigated := st.cfg.Investigate.Rate >= 1 || st.cfg.Investigate.alwaysKeep(b)
	if investigated && hist != nil {
		context := newEventContext(b)
		for _, e := range b.Errors {
			event, detail := newErrorEvent(e, now, b.PageGroup)
			event.context = context
			rec := event.history(st.cfg.Name, "")
			detail.apply(&rec)
			hist.Event(rec)
		}
	}
	return investigated
}

// timeline appends one report's entries to a session ring and forwards them to
// history while the session is investigated.
type timeline struct {
	site     string
	session  *sessionDetail
	hist     HistorySink
	context  eventContext
	recorded bool
}

// record gives an entry the report's context; a poor vital keeps the context of
// its measurement.
func (t *timeline) record(event sessionEvent) {
	if event.typ != "vital" {
		event.context = t.context
	}
	t.session.pushEvent(event)
	t.recorded = true
	if t.hist != nil && t.session.investigated {
		t.hist.Event(event.history(t.site, t.session.id))
	}
}

// recordPoorVitals records each poor vital with the dimensions frozen when it
// was first measured.
func (t *timeline) recordPoorVitals(vitals []beacon.Vital, page string, now time.Time) {
	for _, v := range vitals {
		if !poorVital(v) {
			continue
		}
		origin := t.context
		originPage := page
		if attrs := v.Origin; attrs != nil {
			origin.browser, origin.device, origin.country, origin.version = attrs.Browser, attrs.Device, attrs.Country, attrs.AppVersion
			originPage = attrs.PageGroup
		}
		t.record(sessionEvent{
			ts:       now,
			typ:      "vital",
			page:     originPage,
			context:  origin,
			metricID: v.ID,
			revision: v.Revision,
			text:     truncateRunes(poorVitalText(v), sessionEventTextMax),
		})
	}
}

// recordEvents records application views, frustration signals, traced requests
// and custom events. Activations, session lifecycle and timing events are
// represented elsewhere or not at all.
func (t *timeline) recordEvents(events []beacon.Event, page string, now time.Time) {
	for _, ev := range events {
		switch {
		case ev.Kind == beacon.EventDocument || ev.Kind == beacon.EventNavigation || ev.Kind == beacon.EventResource ||
			ev.Kind == beacon.EventSession:
			// The page view entry stands for the activation; the rest have none.
		case ev.Kind == beacon.EventView:
			target := ev.Attrs["toView"]
			if target == "" {
				target = page
			}
			t.record(sessionEvent{
				ts:       now,
				typ:      "view",
				revision: ev.Revision,
				page:     target,
				text:     target,
			})
		case beacon.IsFrustration(ev.Name):
			t.session.frustrations++
			t.record(sessionEvent{
				ts:   now,
				typ:  "frustration",
				page: page,
				text: truncateRunes(
					strings.ReplaceAll(ev.Name, "_", " ")+" on "+ev.Attrs["target"],
					sessionEventTextMax,
				),
			})
		case ev.TraceID != "" && ev.Kind == beacon.EventRequest:
			t.record(sessionEvent{
				ts:      now,
				typ:     "request",
				page:    page,
				text:    truncateRunes(requestText(ev.Attrs), sessionEventTextMax),
				traceID: ev.TraceID,
			})
		default:
			t.record(sessionEvent{
				ts:   now,
				typ:  "event",
				page: page,
				text: truncateRunes(ev.Name, sessionEventTextMax),
			})
		}
	}
}

// documentRevision is the revision of the report's document activation, or zero.
func documentRevision(b *beacon.Beacon) uint64 {
	for _, ev := range b.Events {
		if ev.Kind == beacon.EventDocument {
			return ev.Revision
		}
	}
	return 0
}

// newEventContext captures the identity, release and location b arrived with.
func newEventContext(b *beacon.Beacon) eventContext {
	return eventContext{
		vnodeGUID:    b.VnodeGUID,
		experienceID: b.ExperienceID,
		view:         b.View,
		viewID:       b.ViewID,
		browser:      b.Browser,
		device:       b.Device,
		country:      b.Country,
		version:      b.AppVersion,
		userID:       b.UserID,
	}
}

// newErrorEvent returns one JS error entry and the metadata history persists for
// it. Only a ring entry retains the metadata; a sessionless error applies it to
// its record directly and allocates nothing for it.
func newErrorEvent(err beacon.Error, ts time.Time, page string) (sessionEvent, sessionError) {
	event := sessionEvent{
		ts:   ts,
		typ:  "error",
		page: page,
		text: truncateRunes(err.Type+": "+err.Message, sessionEventTextMax),
	}
	detail := sessionError{
		fingerprint: err.Fingerprint,
		typ:         err.Type,
		message:     err.Message,
		stack:       err.Stack, // bounded by the decoder
	}
	return event, detail
}

func (d sessionError) apply(rec *HistoryEvent) {
	rec.Fingerprint, rec.ErrorType = d.fingerprint, d.typ
	rec.Message, rec.SampleStack = d.message, d.stack
}

// history is the only projection of a timeline entry: history persistence and
// the live session-events read both use it.
func (e sessionEvent) history(site, sessionID string) HistoryEvent {
	rec := HistoryEvent{
		VnodeGUID:    e.context.vnodeGUID,
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
		e.error.apply(&rec)
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
		if now.Sub(el.Value.(*sessionDetail).lastSeen) > sessionTTL {
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

// SessionEvents returns one session's event ring (oldest first) projected as
// history records, so live and persisted reads share one projection.
// ok is false when the session id is unknown (caller: 404).
func (a *Aggregator) SessionEvents(sessionID string) ([]HistoryEvent, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.site.evictSessions(a.now())
	el, ok := a.site.sessIdx[sessionID]
	if !ok {
		return nil, false
	}
	sd := el.Value.(*sessionDetail)
	out := make([]HistoryEvent, len(sd.events))
	for i, e := range sd.events {
		out[i] = e.history(a.site.cfg.Name, sd.id)
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
