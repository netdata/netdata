// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func TestSessionDetailAccumulates(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	b1 := mk(*now, "sess1", "/a")
	b1.AppVersion = "1.0.0"
	a.Ingest(b1)
	*now = now.Add(5 * time.Second)
	b2 := mk(*now, "sess1", "/b")
	b2.Errors = []beacon.Error{{Type: "TypeError", Message: "boom", Fingerprint: "fp1"}}
	a.Ingest(b2)
	*now = now.Add(5 * time.Second)

	got := trackedSessions(a)
	if len(got) != 1 {
		t.Fatalf("sessions = %+v", got)
	}
	sd := got[0]
	if sd.id != "sess1" || sd.pageviews != 2 || sd.errors != 1 {
		t.Fatalf("session = %+v", sd)
	}
	if sd.entryPage != "/a" || sd.lastPage != "/b" {
		t.Fatalf("entry/last page = %q/%q", sd.entryPage, sd.lastPage)
	}
	if now.Sub(sd.started) != 10*time.Second || sd.lastSeen.Sub(sd.started) != 5*time.Second {
		t.Fatalf("times: started=%v lastSeen=%v", sd.started, sd.lastSeen)
	}
}

func trackedSessions(a *Aggregator) []*sessionDetail {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []*sessionDetail
	for el := a.site.sess.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*sessionDetail))
	}
	return out
}

func TestSessionEventsRing(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	a.Ingest(mk(*now, "sess1", "/a"))
	*now = now.Add(time.Second)
	b := mk(*now, "sess1", "/b")
	b.Errors = []beacon.Error{{Type: "E", Message: "boom"}}
	a.Ingest(b)
	*now = now.Add(time.Second)
	b2 := mk(*now, "sess1", "/b")
	b2.Events = []beacon.Event{{Name: "checkout"}}
	a.Ingest(b2)

	events, ok := a.SessionEvents("sess1")
	if !ok {
		t.Fatal("session must be found")
	}
	wantTypes := []string{"pageview", "pageview", "error", "event"}
	if len(events) != len(wantTypes) {
		t.Fatalf("events = %+v", events)
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Fatalf("event[%d].Type = %q want %q (%+v)", i, events[i].Type, want, events)
		}
	}
	if events[2].Text != "E: boom" {
		t.Fatalf("error text = %q", events[2].Text)
	}
	if events[3].Text != "checkout" {
		t.Fatalf("event text = %q", events[3].Text)
	}
}

func TestSessionEventsFiltersNavigationAndResource(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	b := mk(*now, "sess1", "/a")
	b.Events = append(b.Events, beacon.Event{
		Kind: beacon.EventNavigation,
	}, beacon.Event{
		Kind: beacon.EventResource,
	}, beacon.Event{
		Name: "custom",
	})
	a.Ingest(b)
	*now = now.Add(time.Second)

	events, ok := a.SessionEvents("sess1")
	if !ok {
		t.Fatal("session must be found")
	}
	// pageview + the one custom event only.
	if len(events) != 2 || events[1].Text != "custom" {
		t.Fatalf("events = %+v", events)
	}
}

func TestSessionViewChangeEmitsViewEvent(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	b1 := mk(*now, "sess1", "/")
	b1.View = "home"
	a.Ingest(b1)
	*now = now.Add(time.Second)
	b2 := mk(*now, "sess1", "/")
	b2.View = "checkout"
	b2.ViewID = "checkout-occurrence"
	b2.Events = []beacon.Event{
		{Kind: beacon.EventView, ID: "checkout-event", Revision: 1, Attrs: map[string]string{"toView": "checkout"}},
	}
	a.Ingest(b2)

	events, ok := a.SessionEvents("sess1")
	if !ok {
		t.Fatal("session must be found")
	}
	var sawView bool
	for _, e := range events {
		if e.Type == "view" {
			sawView = true
			if e.Text != "checkout" {
				t.Fatalf("view event text = %q", e.Text)
			}
		}
	}
	if !sawView {
		t.Fatalf("expected a view event on view change: %+v", events)
	}
}

func TestSessionEventsUnknownID(t *testing.T) {
	a, _ := newAgg(30 * time.Minute)
	if _, ok := a.SessionEvents("nope"); ok {
		t.Fatal("unknown session must report not found")
	}
}

func TestSessionEventRingBounded(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	for i := 0; i < sessionEventRingCap+20; i++ {
		b := mk(*now, "sess1", fmt.Sprintf("/p%d", i))
		a.Ingest(b)
		*now = now.Add(time.Second)
	}
	events, ok := a.SessionEvents("sess1")
	if !ok || len(events) != sessionEventRingCap {
		t.Fatalf("events len = %d want %d (ok=%v)", len(events), sessionEventRingCap, ok)
	}
}

func TestSessionLRUEvictsOldest(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	for i := 0; i < maxTrackedSessions+50; i++ {
		a.Ingest(mk(*now, fmt.Sprintf("sess-%d", i), "/a"))
	}
	got := trackedSessions(a)
	if len(got) != maxTrackedSessions {
		t.Fatalf("tracked sessions = %d want %d", len(got), maxTrackedSessions)
	}
	for _, sd := range got {
		if sd.id == "sess-0" {
			t.Fatal("oldest session should have been evicted")
		}
	}
	_ = now
}

func TestSessionsStayWithTheirOwner(t *testing.T) {
	for _, site := range []string{"a", "b"} {
		a, now := newAgg(30*time.Minute, SiteCfg{
			Name:       site,
			PageGroups: 20,
			Countries:  20,
		})
		for _, incoming := range []string{"a", "b"} {
			a.Ingest(&beacon.Beacon{
				ExperienceID: incoming,
				Events:       []beacon.Event{{ID: incoming, Revision: 1, Kind: beacon.EventDocument}},
				Site:         incoming,
				Received:     *now,
				SessionID:    "shared",
				PageGroup:    "/" + incoming,
			})
		}
		events, ok := a.SessionEvents("shared")
		if !ok || len(events) != 1 || events[0].Page != "/"+site {
			t.Fatalf("site %s events = %+v, found=%v", site, events, ok)
		}
	}
}

// The observation window controls session measurement independently of details.
func TestObservedSessionsAgeByWindow(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	a.Ingest(mk(*now, "s2", "/a"))
	*now = now.Add(20 * time.Minute)
	a.Ingest(mk(*now, "s1", "/b"))
	*now = t0.Add(31 * time.Minute) // s2 idle 31m, s1 idle 11m
	if got := a.Snapshot().ObservedSessions; got != 1 {
		t.Fatalf("active=%d want 1", got)
	}
}

// A traced request lands in the timeline with its trace id.
func TestTracedRequestInSessionTimeline(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := mk(*now, "s1", "/checkout")
	b.Events = []beacon.Event{{
		Kind: beacon.EventRequest, Name: "faro.tracing.fetch", TraceID: "360b7dd27710ac98272283e6f1f29102",
		Attrs: map[string]string{
			"http.request.method":       "POST",
			"url.full":                  "https://api.example.com/orders",
			"http.response.status_code": "201",
			"duration_ns":               "12400000",
		},
	}}
	a.Ingest(b)
	events, ok := a.SessionEvents("s1")
	if !ok {
		t.Fatal("session missing")
	}
	var req *SessionEventInfo
	for i := range events {
		if events[i].Type == "request" {
			req = &events[i]
		}
	}
	if req == nil || req.TraceID != "360b7dd27710ac98272283e6f1f29102" ||
		req.Text != "POST api.example.com/orders 201 · 12 ms" {
		t.Fatalf("request event = %+v", req)
	}
}

func TestBotsFilteredPerMinute(t *testing.T) {
	a, _ := newAgg(5 * time.Minute)
	a.Reject("s", beacon.RejectBot)
	a.Reject("s", beacon.RejectBot)
	if got := a.Activity().BotsPerMin; got != 2 {
		t.Fatalf("bots per min = %d", got)
	}
}

// A poor vital names its element on the timeline and in Pages;
// frustration signals count per session, page and site; the user id is kept.
func TestElementsFrustrationAndUser(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	h := &fakeHistorySink{}
	a.SetHistorySink(h)
	b := mk(*now, "s1", "/checkout")

	b.UserID = "u_42"
	b.Vitals = []beacon.Vital{
		{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 5200, Element: "#hero img"},
	}
	b.Events = []beacon.Event{{Name: beacon.RageClickEvent, Attrs: map[string]string{"target": "button#buy"}}}
	a.Ingest(b)
	a.Snapshot()

	events, _ := a.SessionEvents("s1")
	var texts []string
	for _, e := range events {
		if e.Type == "vital" || e.Type == "frustration" {
			texts = append(texts, e.Text)
		}
	}
	if len(texts) != 2 || texts[0] != "poor LCP 5200 ms on #hero img" || texts[1] != "rage click on button#buy" {
		t.Fatalf("timeline = %v", texts)
	}
	pages := a.Pages()
	if len(pages) != 1 || pages[0].LCPElement != "#hero img" || pages[0].FrustrationWindow != 1 {
		t.Fatalf("pages = %+v", pages)
	}
	if got := a.Snapshot().Counters[CounterRageClicks]; got != 1 {
		t.Fatalf("rage clicks = %d", got)
	}
	last := h.events[len(h.events)-1]
	if last.UserID != "u_42" || last.Type != "frustration" {
		t.Fatalf("history event = %+v", last)
	}
}
