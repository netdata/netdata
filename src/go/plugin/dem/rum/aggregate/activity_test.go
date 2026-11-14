// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityRetainsReceiptEvidenceAfterQuietWindow(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	require.True(t, a.Activity().LastBeaconAt.IsZero())
	require.Equal(t, -1, a.Activity().LastBeaconAgeS)
	a.Ingest(mk(*now, "session", "/"))
	accepted := *now
	*now = now.Add(2 * time.Hour)
	activity := a.Activity()
	require.Equal(t, accepted, activity.LastBeaconAt)
	require.Equal(t, 7200, activity.LastBeaconAgeS)
	require.Zero(t, activity.BeaconsPerMin)
	replacement, _ := newAgg(5 * time.Minute)
	require.True(t, replacement.Activity().LastBeaconAt.IsZero())
}

func TestActivitySummaryMatchesSnapshotThroughExpiry(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := 0; i < 10000+1; i++ {
		b := mk(*now, fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.ViewID = "view"
		b.View = "view"
		b.Events = append(b.Events, beacon.Event{
			Kind:     beacon.EventView,
			ID:       "view",
			Revision: 1,
		})
		b.Errors = []beacon.Error{{Fingerprint: "error"}}
		a.Ingest(b)
	}
	for _, advance := range []time.Duration{0, time.Minute + time.Second} {
		*now = now.Add(advance)
		s := a.Snapshot()
		activity := a.Activity()
		assert.EqualValues(t, s.PageviewsWindow, activity.PageviewsWindow)
		assert.Equal(t, s.ApplicationViewsWindow, activity.ApplicationViewsWindow)
		assert.Equal(t, s.JSErrorsWindow, activity.JSErrorsWindow)
		assert.Equal(t, s.ObservedSessions, activity.ObservedSessions)
	}
}

func TestActivityCountsInvestigatedSessions(t *testing.T) {
	a, now, _ := sampledAgg(0.000001, true, false)
	a.Ingest(pageview(*now, "plain"))
	b := mk(*now, "broken", "/a")
	b.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp"}}
	a.Ingest(b)
	act := a.Activity()
	if act.ObservedSessions != 2 || act.InvestigatedSessions != 1 {
		t.Fatalf("activity = %+v, want 2 active, 1 investigated", act)
	}
}

func TestReceiverActivityExpiresWithoutPublication(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		a, now := newAgg(5 * time.Minute)
		for second := range 181 {
			*now = t0.Add(time.Duration(second) * time.Second)
			if rejected {
				a.Reject(beacon.RejectRate)
				a.Reject(beacon.RejectBot)
			} else {
				a.Ingest(&beacon.Beacon{Site: "s", Received: *now})
			}
			// The one-minute cleanup cadence retains at most two activity windows.
			require.LessOrEqual(t, len(a.site.accepted), 121)
			require.LessOrEqual(t, len(a.site.rejected), 121)
			require.LessOrEqual(t, len(a.site.bots), 121)
		}
		activity := a.Activity()
		if rejected {
			require.Equal(t, 61, activity.RejectedPerMin)
			require.Equal(t, 61, activity.BotsPerMin)
		} else {
			require.Equal(t, 61, activity.BeaconsPerMin)
		}
	}
}

func TestBotsFilteredPerMinute(t *testing.T) {
	a, _ := newAgg(5 * time.Minute)
	a.Reject(beacon.RejectBot)
	a.Reject(beacon.RejectBot)
	if got := a.Activity().BotsPerMin; got != 2 {
		t.Fatalf("bots per min = %d", got)
	}
}
