// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHistorySink struct{ events []HistoryEvent }

func (f *fakeHistorySink) Event(rec HistoryEvent) { f.events = append(f.events, rec) }

func TestHistoryEventsAreSelfContained(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(*now, "sess1", "/a")

	b.UserID = "customer"
	a.Ingest(b)
	*now = now.Add(5 * time.Second)
	a.Ingest(mk(*now, "sess1", "/b"))
	require.Len(t, sink.events, 2)
	assert.Equal(t, "pageview", sink.events[0].Type)
	assert.Equal(t, "/a", sink.events[0].Page)
	last := sink.events[1]
	assert.Equal(t, "sess1", last.SessionID)
	assert.Equal(t, "/b", last.Page)
	assert.Equal(t, "customer", sink.events[0].UserID)
	assert.Empty(t, last.UserID)
	assert.Equal(t, now.UnixMicro(), last.TSUnixUS)
}
func TestErrorHistoryNeedsNoParentRecord(t *testing.T) {
	for _, session := range []string{"sess1", ""} {
		t.Run(session, func(t *testing.T) {
			a, _ := newAgg(30 * time.Minute)
			sink := &fakeHistorySink{}
			a.SetHistorySink(sink)
			a.Ingest(
				errBeacon(
					session,
					"/a",
					"Chrome",
					beacon.Error{
						Type:        "TypeError",
						Message:     "boom",
						Stack:       "at f()",
						Fingerprint: "fp1",
					},
				),
			)
			var errors []HistoryEvent
			for _, event := range sink.events {
				if event.Type == "error" {
					errors = append(errors, event)
				}
			}
			require.Len(t, errors, 1)
			event := errors[0]
			assert.Equal(t, "error", event.Type)
			assert.Equal(t, "TypeError", event.ErrorType)
			assert.Equal(t, "boom", event.Message)
			assert.Equal(t, "fp1", event.Fingerprint)
			assert.Equal(t, "at f()", event.SampleStack)
			assert.Equal(t, session, event.SessionID)
			assert.Equal(t, "/a", event.Page)
			assert.Equal(t, "Chrome", event.Browser)
		})
	}
}
func TestHistoryRecordsActivityWithoutTimelineEvents(t *testing.T) {
	a, now := newAgg(time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(*now, "session", "/a")
	b.View = ""
	a.Ingest(b)
	*now = now.Add(time.Second)
	b.Received = *now
	b.Logs = []beacon.Log{{Message: "console observation"}}
	a.Ingest(b)
	require.Len(t, sink.events, 2)
	assert.Equal(t, "activity", sink.events[1].Type)
	timeline, _ := a.SessionEvents("session")
	require.Len(t, timeline, 1)
	assert.Equal(t, "pageview", timeline[0].Type)
}
func TestNilHistorySinkIsSafe(t *testing.T) {
	a, _ := newAgg(30 * time.Minute)
	a.Ingest(mk(t0, "sess1", "/a"))
	a.Ingest(errBeacon("sess1", "/a", "Chrome", beacon.Error{
		Type:        "E",
		Message:     "m",
		Fingerprint: "fp1",
	}))
}
