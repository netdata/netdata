// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Equal(t, now.UnixMicro(), last.ObservedUS)
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

// Session and sessionless errors persist the same complete record: the site's
// configured vnode, the arrival context and the decoder-bounded error detail.
// Only a session timeline defaults an empty page group to "/".
func TestErrorHistoryRecordIsComplete(t *testing.T) {
	message, stack := strings.Repeat("m", 250), strings.Repeat("s", 4096)
	tests := map[string]struct {
		session, page string
	}{
		"session":     {session: "session", page: "/"},
		"sessionless": {session: "", page: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a, _ := newAgg(30 * time.Minute)
			a.SetVnodeGUID(vnodeA)
			sink := &fakeHistorySink{}
			a.SetHistorySink(sink)
			a.Ingest(&beacon.Beacon{
				Site:         "s",
				Received:     t0,
				SessionID:    tc.session,
				VnodeGUID:    "untrusted",
				ExperienceID: "document",
				View:         "checkout",
				ViewID:       "view-1",
				Browser:      "Firefox",
				Device:       "mobile",
				Country:      "GR",
				AppVersion:   "1.2",
				UserID:       "user",
				Errors: []beacon.Error{{
					Fingerprint: "fingerprint",
					Type:        "TypeError",
					Message:     message,
					Stack:       stack,
				}},
			})
			require.Equal(t, []HistoryEvent{{
				VnodeGUID:    vnodeA,
				ExperienceID: "document",
				View:         "checkout",
				ViewID:       "view-1",
				Site:         "s",
				SessionID:    tc.session,
				ObservedUS:   t0.UnixMicro(),
				Type:         "error",
				Page:         tc.page,
				Text:         ("TypeError: " + message)[:sessionEventTextMax],
				Browser:      "Firefox",
				Device:       "mobile",
				Country:      "GR",
				Version:      "1.2",
				UserID:       "user",
				Fingerprint:  "fingerprint",
				ErrorType:    "TypeError",
				Message:      message,
				SampleStack:  stack,
			}}, sink.events)
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

func TestHistoryRecordsAcceptedGoodVitalActivity(t *testing.T) {
	a, now := newAgg(time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(*now, "session", "/entry")
	a.Ingest(b)
	require.Len(t, sink.events, 1)

	*now = now.Add(10 * time.Second)
	b.Received = *now
	b.Events = nil
	b.Vitals = []beacon.Vital{v(beacon.LCP, 100)}
	result := a.Ingest(b)
	require.Len(t, result.Observation.Vitals, 1)
	require.Len(t, sink.events, 2)
	activity := sink.events[1]
	assert.Equal(t, "activity", activity.Type)
	assert.Equal(t, now.UnixMicro(), activity.ObservedUS)
	assert.Equal(t, b.SessionID, activity.SessionID)
	assert.Equal(t, b.ExperienceID, activity.ExperienceID)
	assert.Equal(t, "/entry", activity.Page)

	*now = now.Add(10 * time.Second)
	b.Received = *now
	result = a.Ingest(b)
	assert.Empty(t, result.Observation.Vitals)
	assert.Len(t, sink.events, 2, "replayed vital does not write fresh activity")
	timeline, ok := a.SessionEvents(b.SessionID)
	require.True(t, ok)
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
