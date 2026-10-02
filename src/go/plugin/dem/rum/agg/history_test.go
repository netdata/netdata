// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// fakeHistorySink records every call for assertions, standing in for the
// real writer (plugin/dem/rum/history) without pulling in SQLite/channels.
type fakeHistorySink struct {
	sessions []HistorySession
	events   []HistorySessionEvent
	groups   []HistoryErrorGroup
	occs     []HistoryErrorOccurrence
}

func (f *fakeHistorySink) Session(rec HistorySession)                 { f.sessions = append(f.sessions, rec) }
func (f *fakeHistorySink) SessionEvent(rec HistorySessionEvent)       { f.events = append(f.events, rec) }
func (f *fakeHistorySink) ErrorGroup(rec HistoryErrorGroup)           { f.groups = append(f.groups, rec) }
func (f *fakeHistorySink) ErrorOccurrence(rec HistoryErrorOccurrence) { f.occs = append(f.occs, rec) }

func TestHistorySinkReceivesSessionAndEvents(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)

	b1 := mk(*now, "sess1", "/a")
	b1.City = "Athens"
	a.Ingest(b1)
	*now = now.Add(5 * time.Second)
	b2 := mk(*now, "sess1", "/b")
	a.Ingest(b2)

	if len(sink.sessions) != 2 {
		t.Fatalf("expected a Session call per beacon, got %d", len(sink.sessions))
	}
	last := sink.sessions[len(sink.sessions)-1]
	if last.SessionID != "sess1" || sink.sessions[0].Pageviews+last.Pageviews != 2 || last.LastPage != "/b" ||
		last.EntryPage != "/a" {
		t.Fatalf("session record = %+v", last)
	}
	if last.City != "Athens" {
		t.Fatalf("city must stick from the first beacon that carried it: %+v", last)
	}
	if len(sink.events) != 2 {
		t.Fatalf("expected one pageview event per beacon, got %d: %+v", len(sink.events), sink.events)
	}
	if sink.events[0].Type != "pageview" || sink.events[0].Page != "/a" {
		t.Fatalf("event[0] = %+v", sink.events[0])
	}
}

func TestHistorySinkReceivesErrorGroupAndOccurrence(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)

	b := errBeacon(
		"sess1",
		"/a",
		"Chrome",
		beacon.Error{
			Type:        "TypeError",
			Message:     "boom",
			Stack:       "at f()",
			Fingerprint: "fp1",
		},
	)
	a.Ingest(b)
	_ = now

	if len(sink.groups) != 1 || sink.groups[0].Fingerprint != "fp1" || sink.groups[0].Message != "boom" {
		t.Fatalf("error group record = %+v", sink.groups)
	}
	if len(sink.occs) != 1 || sink.occs[0].Fingerprint != "fp1" || sink.occs[0].SessionID != "sess1" ||
		sink.occs[0].Page != "/a" ||
		sink.occs[0].Browser != "Chrome" {
		t.Fatalf("error occurrence record = %+v", sink.occs)
	}
}

func TestHistorySinkSkipsErrorsWithoutFingerprint(t *testing.T) {
	a, _ := newAgg(30 * time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)

	a.Ingest(errBeacon("sess1", "/a", "Chrome", beacon.Error{
		Type:    "E",
		Message: "m",
	})) // no fingerprint

	if len(sink.groups) != 0 || len(sink.occs) != 0 {
		t.Fatalf("errors without a fingerprint must not reach history: groups=%+v occs=%+v", sink.groups, sink.occs)
	}
}

func TestNilHistorySinkIsSafe(t *testing.T) {
	a, _ := newAgg(30 * time.Minute) // SetHistorySink never called
	a.Ingest(mk(t0, "sess1", "/a"))
	a.Ingest(errBeacon("sess1", "/a", "Chrome", beacon.Error{
		Type:        "E",
		Message:     "m",
		Fingerprint: "fp1",
	}))
	// No panic, no special-casing needed by the caller.
}
