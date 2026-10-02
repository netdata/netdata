// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func sampledAgg(rate float64, keepErrors, keepPoor bool) (*Aggregator, *time.Time, *fakeHistorySink) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Key:        "s",
		Name:       "S",
		PageGroups: 20,
		Countries:  20,
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
	b := mk(now, session, "/a")
	b.PageView = true
	return b
}

// Measure is unaffected: every beacon still counts in the charts.
func TestInvestigateSamplingKeepsMeasuringEverything(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, false)
	for i := 0; i < 50; i++ {
		a.Ingest(pageview(*now, fmt.Sprintf("s%d", i)))
	}
	if got := one(a).Counters[CounterPageviews]; got != 50 {
		t.Fatalf("page views counted = %d, want 50", got)
	}
	if len(h.sessions) != 0 || len(h.events) != 0 {
		t.Fatalf("sampled-out sessions must not reach history: %d sessions, %d events", len(h.sessions), len(h.events))
	}
}

func TestInvestigateSamplingIsPerSession(t *testing.T) {
	a, now, h := sampledAgg(0.5, false, false)
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("sess-%d", i)
		b := pageview(*now, id)
		a.Ingest(b)
		if !b.SampledOut != beacon.SessionSampled(id, 0.5) {
			t.Fatalf("%s: SampledOut=%v disagrees with the session decision", id, b.SampledOut)
		}
		b2 := pageview(*now, id)
		a.Ingest(b2)
		if b2.SampledOut != b.SampledOut {
			t.Fatalf("%s: second beacon decided differently", id)
		}
	}
	kept := map[string]bool{}
	for _, s := range h.sessions {
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
	if len(h.sessions) != 0 {
		t.Fatal("not investigated before the error")
	}
	*now = now.Add(time.Second)
	b := mk(*now, "late-error", "/a")
	b.Errors = []beacon.Error{{Type: "TypeError", Message: "boom", Fingerprint: "fp1"}}
	a.Ingest(b)

	if b.SampledOut {
		t.Fatal("the erroring beacon must be investigated")
	}
	var types []string
	for _, e := range h.events {
		types = append(types, e.Type)
	}
	if len(types) != 2 || types[0] != "pageview" || types[1] != "error" {
		t.Fatalf("promoted timeline = %v, want [pageview error]", types)
	}
	if len(h.occs) != 1 || h.occs[0].SessionID != "late-error" {
		t.Fatalf("error occurrences = %+v", h.occs)
	}
}

func TestPoorVitalPromotesSession(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, true)
	a.Ingest(mk(*now, "slow", "/a", v(beacon.LCP, 5200)))
	if len(h.sessions) != 1 {
		t.Fatalf("a poor LCP must keep the session: %d kept", len(h.sessions))
	}
}

// Without always_keep errors, errors of sampled-out sessions are sampled
// out too, so the Errors table matches the Sessions table.
func TestErrorsFollowTheSessionDecisionWhenNotAlwaysKept(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, false)
	b := mk(*now, "quiet", "/a")
	b.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp1"}}
	a.Ingest(b)
	if len(h.occs) != 0 || len(h.groups) != 0 {
		t.Fatalf("errors of a sampled-out session reached history: %+v %+v", h.groups, h.occs)
	}
	if got := one(a).Counters[CounterJSErrors]; got != 1 {
		t.Fatalf("the error must still be measured: %d", got)
	}
}

func TestUnsetRateKeepsEverything(t *testing.T) {
	a, now, h := sampledAgg(0, false, false)
	b := pageview(*now, "s1")
	a.Ingest(b)
	if b.SampledOut || len(h.sessions) != 1 {
		t.Fatal("an unset investigate rate keeps every session")
	}
}

func TestActivityCountsInvestigatedSessions(t *testing.T) {
	a, now, _ := sampledAgg(0.000001, true, false)
	a.Ingest(pageview(*now, "plain"))
	b := mk(*now, "broken", "/a")
	b.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp"}}
	a.Ingest(b)
	act := a.Activity()["s"]
	if act.ActiveSessions != 2 || act.InvestigatedSessions != 1 {
		t.Fatalf("activity = %+v, want 2 active, 1 investigated", act)
	}
}
