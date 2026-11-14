// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/require"
)

func TestExactPopulationsBeyondFormerCapacity(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{Name: "s", PageGroups: 20, Countries: 20})
	const documents = 10001
	for i := range documents {
		b := mk(*now, fmt.Sprint(i), "/entry", v(beacon.LCP, 100), v(beacon.INP, 10), v(beacon.CLS, 0.1))
		b.PageHost = "example.org"
		b.Resources = []beacon.Resource{{ID: "resource", Host: "example.org", Initiator: "fetch", HasDuration: true, DurationMS: 5}}
		a.Ingest(b)
		a.Ingest(b) // Every population must retain replay evidence, too.
	}
	s := a.Snapshot()
	require.EqualValues(t, documents, s.PageviewsWindow)
	require.Equal(t, documents, s.ObservedSessions)
	for _, name := range []string{beacon.LCP, beacon.INP, beacon.CLS} {
		require.Equal(t, documents, s.Vitals[name].N, name)
		require.Equal(t, documents, s.Vitals[name].Good, name)
	}
	require.Equal(t, 100., s.Vitals[beacon.LCP].P75)
	require.Equal(t, documents, s.API.N)
	require.Equal(t, 5., s.API.P75)
	pages := a.Pages("")
	require.Len(t, pages, 1)
	require.Equal(t, documents, pages[0].Sessions)
	require.Equal(t, documents, pages[0].PageviewsWindow)
}

func TestIngestExpiresCanonicalStateWithoutReads(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	first := mk(*now, "old", "/old", v(beacon.LCP, 100))
	first.Resources = []beacon.Resource{{ID: "resource", Host: "example.org"}}
	a.Ingest(first)
	*now = now.Add(5*time.Minute + time.Nanosecond)
	a.Ingest(mk(*now, "current", "/current", v(beacon.LCP, 200)))
	// Inspect retained ownership before any reader can trigger cleanup.
	require.Len(t, a.site.observations, 1)
	require.Equal(t, 1, a.site.activity.Len())
	require.Zero(t, a.site.resources.Len())
	require.Len(t, a.site.sessions, 1)
	require.Len(t, a.site.pageSessions, 1)
	identities := func() int { return len(a.site.identities) + len(a.site.vitalIdentities) }
	require.Equal(t, 4, identities(), "identity evidence outlives measurement windows")
	require.Len(t, a.site.resourceIdentities, 1)
	*now = now.Add(30 * time.Minute)
	a.Ingest(mk(*now, "last", "/last"))
	require.Equal(t, 3, identities(), "the exact 30-minute boundary remains included")
	*now = now.Add(time.Nanosecond)
	a.Ingest(&beacon.Beacon{Site: "s", Received: *now})
	require.Equal(t, 1, identities())
	require.Empty(t, a.site.resourceIdentities)
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
