// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
)

func errBeacon(session, page, browser string, errs ...beacon.Error) *beacon.Beacon {
	return &beacon.Beacon{
		Site:      "s",
		SessionID: session,
		Path:      page,
		PageGroup: page,
		Browser:   browser,
		Device:    "desktop",
		Errors:    errs,
	}
}

func TestErrorGroupRankingAndOtherFold(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	// 12 distinct fingerprints, decreasing frequency so ranking is deterministic.
	for i := 0; i < 12; i++ {
		fp := fmt.Sprintf("fp%02d", i)
		n := 12 - i // fp00 occurs 12 times, fp11 once
		for j := 0; j < n; j++ {
			b := errBeacon(fmt.Sprintf("sess-%d-%d", i, j), "/a", "Chrome",
				beacon.Error{
					Type:        "TypeError",
					Message:     "boom",
					Fingerprint: fp,
				})
			a.Ingest(b)
		}
	}
	*now = now.Add(10 * time.Second)
	snap := one(a)
	if len(snap.ErrorGroups) != errorGroupsTopN+1 { // top 10 + other
		t.Fatalf("ErrorGroups = %+v, want %d entries", snap.ErrorGroups, errorGroupsTopN+1)
	}
	for i := 0; i < errorGroupsTopN; i++ {
		wantFP := fmt.Sprintf("fp%02d", i)
		if snap.ErrorGroups[i].Fingerprint != wantFP || snap.ErrorGroups[i].Total != uint64(12-i) {
			t.Fatalf("rank[%d] = %+v, want fp=%s total=%d", i, snap.ErrorGroups[i], wantFP, 12-i)
		}
	}
	last := snap.ErrorGroups[errorGroupsTopN]
	if last.Fingerprint != Other || last.Total != 3 { // fp10(2)+fp11(1) folded
		t.Fatalf("other = %+v", last)
	}
}

func TestErrorGroupRankChangePreservesCounts(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < 10; i++ {
		fp := fmt.Sprintf("fp%02d", i)
		b := errBeacon("s1", "/a", "Chrome", beacon.Error{
			Type:        "E",
			Message:     "m",
			Fingerprint: fp,
		})
		a.Ingest(b)
	}
	*now = now.Add(10 * time.Second)
	one(a) // first ranking = {fp00..fp09}

	// A new, much more frequent fingerprint displaces fp09 (the lowest-ranked, ties by string).
	for i := 0; i < 5; i++ {
		a.Ingest(errBeacon("s2", "/a", "Chrome", beacon.Error{
			Type:        "E",
			Message:     "m",
			Fingerprint: "zzz",
		}))
	}
	*now = now.Add(10 * time.Second)
	snap := one(a)
	want := []ErrorChartGroup{{Fingerprint: "zzz", Message: "m", Total: 5}}
	for i := 0; i < 9; i++ {
		want = append(want, ErrorChartGroup{
			Fingerprint: fmt.Sprintf("fp%02d", i),
			Message:     "m",
			Total:       1,
		})
	}
	want = append(want, ErrorChartGroup{
		Fingerprint: Other,
		Total:       1,
	})
	assert.Equal(t, want, snap.ErrorGroups)
	// A displaced group can become active again with its accumulated count.
	for i := 0; i < 6; i++ {
		a.Ingest(errBeacon("s3", "/a", "Chrome", beacon.Error{
			Type:        "E",
			Message:     "m",
			Fingerprint: "fp09",
		}))
	}
	want = []ErrorChartGroup{
		{Fingerprint: "fp09", Message: "m", Total: 7},
		{Fingerprint: "zzz", Message: "m", Total: 5},
	}
	for i := 0; i < 8; i++ {
		want = append(want, ErrorChartGroup{
			Fingerprint: fmt.Sprintf("fp%02d", i),
			Message:     "m",
			Total:       1,
		})
	}
	want = append(want, ErrorChartGroup{
		Fingerprint: Other,
		Total:       1,
	})
	assert.Equal(t, want, one(a).ErrorGroups)
}

func TestErrorGroupLRUBounds(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < maxErrorGroups+50; i++ {
		fp := fmt.Sprintf("fp%04d", i)
		a.Ingest(errBeacon("s", "/a", "Chrome", beacon.Error{
			Type:        "E",
			Message:     "m",
			Fingerprint: fp,
		}))
	}
	got := trackedErrorFingerprints(a, "s")
	if len(got) != maxErrorGroups {
		t.Fatalf("tracked groups = %d, want %d", len(got), maxErrorGroups)
	}
	// The earliest fingerprints must have been evicted (LRU by last seen).
	for _, fp := range got {
		if fp == "fp0000" {
			t.Fatal("oldest fingerprint should have been evicted")
		}
	}
	_ = now
}

func trackedErrorFingerprints(a *Aggregator, site string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for el := a.sites[site].errGroups.Front(); el != nil; el = el.Next() {
		out = append(out, el.Value.(*errEntry).fp)
	}
	return out
}

func TestTruncateStackKeepsValidUTF8(t *testing.T) {
	// "é" is 2 bytes; an odd prefix length forces the cut into a rune.
	s := "x" + strings.Repeat("é", errMaxStackBytes)
	got := truncateStack(s)
	if len(got) > errMaxStackBytes || !utf8.ValidString(got) {
		t.Fatalf("len=%d valid=%v", len(got), utf8.ValidString(got))
	}
}
