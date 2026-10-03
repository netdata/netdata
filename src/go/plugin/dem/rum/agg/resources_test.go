// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
)

func resBeacon(pageHost string, resources ...beacon.Resource) *beacon.Beacon {
	return &beacon.Beacon{
		Site:      "s",
		Path:      "/a",
		PageGroup: "/a",
		PageHost:  pageHost,
		Browser:   "Chrome",
		Device:    "desktop",
		Resources: resources,
	}
}

func TestIsFirstParty(t *testing.T) {
	tests := map[string]struct {
		pageHost, resHost string
		want              bool
	}{
		"exact match":                       {"www.netdata.cloud", "www.netdata.cloud", true},
		"same registrable domain":           {"www.netdata.cloud", "app.netdata.cloud", true},
		"third party":                       {"www.netdata.cloud", "forms.hsforms.com", false},
		"empty page host":                   {"", "cdn.example.com", false},
		"empty resource host":               {"example.com", "", false},
		"unknown resource host":             {"example.com", unknownValue, false},
		"bare IP falls back to exact match": {"127.0.0.1", "127.0.0.1", true},
		"bare IP mismatch":                  {"127.0.0.1", "10.0.0.1", false},
		"resource host with port":           {"www.shop.test", "www.shop.test:8099", true},
		"same site, other port":             {"www.shop.test", "api.shop.test:8098", true},
		"third party with port":             {"www.shop.test", "api.other.test:8097", false},
		"bare IP with port":                 {"127.0.0.1", "127.0.0.1:8080", true},
		"bracketed IPv6 with port":          {"::1", "[::1]:8080", true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isFirstParty(tc.pageHost, tc.resHost); got != tc.want {
				t.Fatalf("isFirstParty(%q,%q) = %v want %v", tc.pageHost, tc.resHost, got, tc.want)
			}
		})
	}
}

func TestResourcesNotSeenBeforeAnyEvent(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if snap.ResourcesSeen || len(snap.ResourceHosts) != 0 {
		t.Fatalf("expected no resource state before any resource event: %+v", snap)
	}
}

func TestResourceFirstThirdPartyCounters(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := resBeacon("shop.example.com",
		beacon.Resource{
			Host:       "shop.example.com",
			DurationMS: 50,
			Initiator:  "script",
		},
		beacon.Resource{
			Host:       "cdn.other.com",
			DurationMS: 80,
			Initiator:  "img",
		},
	)
	a.Ingest(b)
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if !snap.ResourcesSeen || snap.FirstPartyResources != 1 || snap.ThirdPartyResources != 1 {
		t.Fatalf(
			"counters: seen=%v first=%d third=%d",
			snap.ResourcesSeen,
			snap.FirstPartyResources,
			snap.ThirdPartyResources,
		)
	}
}

func TestResourceHostRankingTop10(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < 12; i++ {
		host := fmt.Sprintf("host%02d.example.com", i)
		n := 12 - i
		for j := 0; j < n; j++ {
			a.Ingest(
				resBeacon(
					"page.example.com",
					beacon.Resource{
						Host:       host,
						DurationMS: float64(100 + i),
						Initiator:  "script",
					},
				),
			)
		}
	}
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if len(snap.ResourceHosts) != resourceHostTopN {
		t.Fatalf("ResourceHosts = %+v want %d entries (no other fold)", snap.ResourceHosts, resourceHostTopN)
	}
	for i := 0; i < resourceHostTopN; i++ {
		want := fmt.Sprintf("host%02d.example.com", i)
		if snap.ResourceHosts[i].Host != want || snap.ResourceHosts[i].Count != uint64(12-i) {
			t.Fatalf("rank[%d] = %+v want host=%s count=%d", i, snap.ResourceHosts[i], want, 12-i)
		}
		if snap.ResourceHosts[i].P75Duration != float64(100+i) {
			t.Fatalf("rank[%d] p75 = %v want %v", i, snap.ResourceHosts[i].P75Duration, 100+i)
		}
	}
}

func TestResourceHostRankChangePreservesCounts(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < 10; i++ {
		host := fmt.Sprintf("h%02d.example.com", i)
		a.Ingest(resBeacon("page.example.com", beacon.Resource{
			Host:       host,
			DurationMS: 50,
		}))
	}
	*now = now.Add(10 * time.Second)
	one(a) // first ranking = {h00..h09}

	for i := 0; i < 5; i++ {
		a.Ingest(resBeacon("page.example.com", beacon.Resource{
			Host:       "zzz.example.com",
			DurationMS: 50,
		}))
	}
	*now = now.Add(10 * time.Second)
	snap := one(a)
	want := []ResourceHostGroup{{Host: "zzz.example.com", Count: 5, HasDuration: true, P75Duration: 50}}
	for i := 0; i < 9; i++ {
		want = append(
			want,
			ResourceHostGroup{
				Host:        fmt.Sprintf("h%02d.example.com", i),
				Count:       1,
				HasDuration: true,
				P75Duration: 50,
			},
		)
	}
	assert.Equal(t, want, snap.ResourceHosts)
	for i := 0; i < 6; i++ {
		a.Ingest(resBeacon("page.example.com", beacon.Resource{
			Host:       "h09.example.com",
			DurationMS: 50,
		}))
	}
	want = []ResourceHostGroup{
		{Host: "h09.example.com", Count: 7, HasDuration: true, P75Duration: 50},
		{Host: "zzz.example.com", Count: 5, HasDuration: true, P75Duration: 50},
	}
	for i := 0; i < 8; i++ {
		want = append(
			want,
			ResourceHostGroup{
				Host:        fmt.Sprintf("h%02d.example.com", i),
				Count:       1,
				HasDuration: true,
				P75Duration: 50,
			},
		)
	}
	assert.Equal(t, want, one(a).ResourceHosts)
}

func TestResourceHostsBounded(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < maxTrackedGroups+50; i++ {
		host := fmt.Sprintf("host-%d.example.com", i)
		a.Ingest(resBeacon("page.example.com", beacon.Resource{
			Host:       host,
			DurationMS: 10,
		}))
	}
	*now = now.Add(10 * time.Second)
	snap := one(a)
	// Only top-10 are charted, but the site-level totals must count every
	// resource regardless of per-host tracking bound.
	if snap.FirstPartyResources+snap.ThirdPartyResources != uint64(maxTrackedGroups+50) {
		t.Fatalf(
			"totals = first=%d third=%d want sum %d",
			snap.FirstPartyResources,
			snap.ThirdPartyResources,
			maxTrackedGroups+50,
		)
	}
}

func TestResourceNoDurationSampleP75Zero(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(resBeacon("page.example.com", beacon.Resource{
		Host:       "h.example.com",
		DurationMS: 0,
	}))
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if len(snap.ResourceHosts) != 1 || snap.ResourceHosts[0].P75Duration != 0 {
		t.Fatalf("expected a zero p75 with no duration sample: %+v", snap.ResourceHosts)
	}
}

func TestResourceHostDurationSeriesIsCapped(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	for i := 0; i < maxSamplesPerSeries+50; i++ {
		a.Ingest(
			resBeacon("shop.example.com", beacon.Resource{
				Host:       "cdn.other.com",
				DurationMS: 10,
				Initiator:  "img",
			}),
		)
	}
	*now = now.Add(10 * time.Second)
	a.mu.Lock()
	g := a.sites["s"].resHosts["cdn.other.com"]
	n, dropped := len(g.dur.vals), a.sites["s"].counters[CounterSamplesDropped]
	a.mu.Unlock()
	if n != maxSamplesPerSeries || dropped != 50 {
		t.Fatalf("series len = %d (want %d), dropped = %d (want 50)", n, maxSamplesPerSeries, dropped)
	}
}

func TestAPIResponseTime(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(resBeacon(
		"shop.example.com",
		beacon.Resource{
			Host:       "api.example.com",
			DurationMS: 100,
			Initiator:  "fetch",
		},
		beacon.Resource{
			Host:       "shop.example.com",
			DurationMS: 300,
			Initiator:  "xmlhttprequest",
		},
		beacon.Resource{
			Host:       "shop.example.com",
			DurationMS: 900,
			Initiator:  "script",
		}, // not an API call
		beacon.Resource{
			Host:       "forms.hsforms.com",
			DurationMS: 900,
			Initiator:  "xmlhttprequest",
		}, // third party
		beacon.Resource{
			Host:       "rum.example.com",
			DurationMS: 900,
			Initiator:  "fetch",
			Self:       true,
		}, // the snippet's own beacon
		beacon.Resource{
			Host:       "shop.example.com",
			DurationMS: 0,
			Initiator:  "fetch",
		}, // no timing
	))
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if snap.API.N != 2 || snap.API.P50 != 100 || snap.API.P95 != 300 {
		t.Fatalf("API stats = %+v, want N=2 p50=100 p95=300", snap.API)
	}
}

func TestAPIResponseTimeWindowed(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(
		resBeacon("shop.example.com", beacon.Resource{
			Host:       "shop.example.com",
			DurationMS: 120,
			Initiator:  "fetch",
		}),
	)
	*now = now.Add(6 * time.Minute)
	if snap := one(a); snap.API.N != 0 {
		t.Fatalf("API sample should leave the window: %+v", snap.API)
	}
}
