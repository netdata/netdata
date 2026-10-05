// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityExpiresSessionsWithoutChartSnapshot(t *testing.T) {
	for name, sameSession := range map[string]bool{"distinct sessions": false, "delayed refresh": true} {
		t.Run(name, func(t *testing.T) {
			for _, read := range []string{"activity", "snapshot"} {
				t.Run(read, func(t *testing.T) {
					a, now := newAgg(time.Minute)
					a.Ingest(mk(t0.Add(10*time.Second), "fresh", "/"))
					id := "old"
					if sameSession {
						id = "fresh"
					}
					a.Ingest(mk(t0, id, "/"))
					*now = t0.Add(beacon.SessionTTL + 5*time.Second)
					if read == "activity" {
						act := a.Activity()
						assert.Equal(t, 1, act.ActiveSessions)
						assert.Equal(t, 1, act.InvestigatedSessions)
					} else {
						assert.Equal(t, 1, a.Snapshot().ActiveSessions)
					}
					*now = t0.Add(beacon.SessionTTL + 10*time.Second)
					assert.Equal(t, 1, a.Activity().ActiveSessions) // inclusive TTL
					*now = now.Add(time.Nanosecond)
					assert.Zero(t, a.Activity().ActiveSessions)
				})
			}
		})
	}
}

func TestWindowEvictionWithOutOfOrderReceipts(t *testing.T) {
	a, now := newAgg(time.Minute)
	for _, sample := range []struct {
		at    time.Time
		value float64
	}{{t0.Add(10 * time.Second), 1000}, {t0, 9000}, {t0.Add(5 * time.Second), 2000}} {
		b := mk(sample.at, "", "/checkout", v(beacon.LCP, sample.value))
		b.PageHost = "shop.example"
		b.Navigation = &beacon.Navigation{
			HasLoad: true,
			LoadMS:  sample.value,
		}
		b.Resources = []beacon.Resource{{Host: "shop.example", Initiator: "fetch", DurationMS: sample.value}}
		b.Errors = []beacon.Error{{Fingerprint: "error", Type: "Error"}}
		b.Events = []beacon.Event{{Name: beacon.RageClickEvent}}
		a.Ingest(b)
	}
	*now = t0.Add(65 * time.Second)
	assert.Equal(
		t,
		SiteActivity{
			BeaconsPerMin:   2,
			LastBeaconAgeS:  55,
			PageviewsWindow: 2,
			JSErrorsWindow:  2,
		},
		a.Activity(),
	)
	snapshot := a.Snapshot()
	assert.Equal(t, uint64(2), snapshot.PageviewsWindow)
	assert.Equal(t, uint64(2), snapshot.JSErrorsWindow)
	assert.Equal(t, 2, snapshot.Vitals[beacon.LCP].N)
	assert.Equal(t, float64(2000), snapshot.Vitals[beacon.LCP].P75)
	assert.Equal(t, 2, snapshot.Load.N)
	assert.Equal(t, float64(2000), snapshot.Load.P75)
	assert.Equal(t, 2, snapshot.API.N)
	assert.Equal(t, float64(2000), snapshot.API.P75)
	require.Len(t, snapshot.ResourceHosts, 1)
	assert.Equal(t, float64(2000), snapshot.ResourceHosts[0].P75Duration)
	pages := a.Pages()
	require.Len(t, pages, 1)
	assert.Equal(t, 2, pages[0].PageviewsWindow)
	assert.Equal(t, 2, pages[0].ErrorsWindow)
	assert.Equal(t, 2, pages[0].FrustrationWindow)
}

func TestDedupKeepsNewestReceipt(t *testing.T) {
	a, now := newAgg(time.Minute)
	for _, tc := range []struct {
		seconds  int
		pageview bool
	}{{12, true}, {0, false}, {13, false}, {23, false}, {34, true}} {
		*now = t0.Add(time.Duration(tc.seconds) * time.Second)
		assert.Equal(t, tc.pageview, a.Ingest(mk(*now, "session", "/page")).PageView)
	}
}

func TestPageviewIdentityIsUnambiguous(t *testing.T) {
	a, now := newAgg(time.Minute)
	for _, pair := range [][2]string{{"a|b", "c"}, {"a", "b|c"}} {
		b := mk(*now, pair[0], "/")
		b.PageID = pair[1]
		assert.True(t, a.Ingest(b).PageView)
		assert.False(t, a.Ingest(b).PageView)
	}
	assert.Equal(t, uint64(2), a.Snapshot().Counters[CounterPageviews])
}

func TestLiveSkipsExpiredRowsBehindFreshRows(t *testing.T) {
	a, now := newAgg(time.Minute)
	for _, seconds := range []int{10, 0, 15, 5} {
		a.Ingest(mk(t0.Add(time.Duration(seconds)*time.Second), "", "/"))
	}
	*now = t0.Add(65 * time.Second)
	var after uint64
	for _, seq := range []uint64{1, 3, 4} {
		rows, next := a.Live(after, 1)
		require.Len(t, rows, 1)
		assert.Equal(t, seq, rows[0].Seq)
		assert.False(t, rows[0].TS.Before(now.Add(-time.Minute)))
		assert.Equal(t, seq, next)
		after = next
	}
	rows, next := a.Live(after, 1)
	assert.Empty(t, rows)
	assert.Equal(t, after, next)
}
