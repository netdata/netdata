// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func sampledAgg(rate float64, keepErrors, keepPoor bool) (*Aggregator, *time.Time, *fakeHistorySink) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:        "s",
		DisplayName: "S",
		PageGroups:  20,
		Countries:   20,
		Investigate: InvestigateCfg{
			Rate:           rate,
			KeepErrors:     keepErrors,
			KeepPoorVitals: keepPoor,
		},
	})
	h := &fakeHistorySink{}
	a.SetHistorySink(h)
	return a, now, h
}

func pageview(now time.Time, session string) *beacon.Beacon {
	return mk(now, session, "/a")
}

// Measure is unaffected: every beacon still counts in the charts.
func TestInvestigateSamplingKeepsMeasuringEverything(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, false)
	for i := 0; i < 50; i++ {
		a.Ingest(pageview(*now, fmt.Sprintf("s%d", i)))
	}
	if got := a.Snapshot().Counters[CounterPageviews]; got != 50 {
		t.Fatalf("page views counted = %d, want 50", got)
	}
	if len(h.events) != 0 {
		t.Fatalf("sampled-out sessions must not reach history: %+v", h.events)
	}
}

func TestInvestigateSamplingIsPerSession(t *testing.T) {
	a, now, h := sampledAgg(0.5, false, false)
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("sess-%d", i)
		b := pageview(*now, id)
		result := a.Ingest(b)
		if result.Investigated != beacon.SessionSampled(id, 0.5) {
			t.Fatalf("%s: Investigated=%v disagrees with the session decision", id, result.Investigated)
		}
		b2 := pageview(*now, id)
		result2 := a.Ingest(b2)
		if result2.Investigated != result.Investigated {
			t.Fatalf("%s: second beacon decided differently", id)
		}
	}
	kept := map[string]bool{}
	for _, s := range h.events {
		kept[s.SessionID] = true
	}
	if len(kept) < 150 || len(kept) > 250 {
		t.Fatalf("sessions kept at 50%% = %d of 400", len(kept))
	}
}

// A sampled-out session that hits an error is promoted and keeps its
// timeline from the start, so the error is never missing its context.
func TestErrorPromotesSessionWithItsEarlierEvents(t *testing.T) {
	a, now, h := sampledAgg(0.000001, true, false)
	a.Ingest(pageview(*now, "late-error"))
	if len(h.events) != 0 {
		t.Fatal("not investigated before the error")
	}
	*now = now.Add(10 * time.Minute)
	b := mk(*now, "late-error", "/a")
	b.Errors = []beacon.Error{{Type: "TypeError", Message: "boom", Fingerprint: "fp1"}}
	result := a.Ingest(b)

	if !result.Investigated {
		t.Fatal("the erroring beacon must be investigated")
	}
	var types []string
	for _, e := range h.events {
		types = append(types, e.Type)
	}
	if len(types) != 3 || types[0] != "pageview" || types[1] != "pageview" || types[2] != "error" {
		t.Fatalf("promoted timeline = %v, want [pageview pageview error]", types)
	}
	if len(h.events) != 3 || h.events[2].Fingerprint != "fp1" || h.events[2].SessionID != "late-error" {
		t.Fatalf("error occurrence = %+v", h.events)
	}
	if h.events[0].TSUnixUS != now.Add(-10*time.Minute).UnixMicro() {
		t.Fatal("promotion lost original event time")
	}

}

func TestPoorVitalPromotesSession(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, true)
	a.Ingest(mk(*now, "slow", "/a", v(beacon.LCP, 5200)))
	if len(h.events) != 1 {
		t.Fatalf("a poor LCP must keep the session: %d kept", len(h.events))
	}
}

// Without always_keep errors, errors of sampled-out sessions are sampled
// out too, so the error investigation follows session sampling.
func TestErrorsFollowTheSessionDecisionWhenNotAlwaysKept(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, false)
	b := mk(*now, "quiet", "/a")
	b.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp1"}}
	a.Ingest(b)
	if len(h.events) != 0 {
		t.Fatalf("errors of a sampled-out session reached history: %+v", h.events)
	}
	if got := a.Snapshot().Counters[CounterJSErrors]; got != 1 {
		t.Fatalf("the error must still be measured: %d", got)
	}
}

func TestUnsetRateKeepsEverything(t *testing.T) {
	a, now, h := sampledAgg(0, false, false)
	b := pageview(*now, "s1")
	result := a.Ingest(b)
	if !result.Investigated || len(h.events) != 1 {
		t.Fatal("an unset investigate rate keeps every session")
	}
}

func TestActivityCountsInvestigatedSessions(t *testing.T) {
	a, now, _ := sampledAgg(0.000001, true, false)
	a.Ingest(pageview(*now, "plain"))
	b := mk(*now, "broken", "/a")
	b.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp"}}
	a.Ingest(b)
	act := a.Activity()
	if act.ActiveSessions != 2 || act.InvestigatedSessions != 1 {
		t.Fatalf("activity = %+v, want 2 active, 1 investigated", act)
	}
}
