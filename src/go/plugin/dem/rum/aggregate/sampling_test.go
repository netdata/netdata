// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
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
		if result.Investigated != SessionSampled(id, 0.5) {
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
// available preceding timeline context as well as the current error.
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
	if h.events[0].ObservedUS != now.Add(-10*time.Minute).UnixMicro() {
		t.Fatal("promotion lost Agent receipt time")
	}

}

func TestPoorVitalPromotesSession(t *testing.T) {
	a, now, h := sampledAgg(0.000001, false, true)
	a.Ingest(mk(*now, "slow", "/a", v(beacon.LCP, 5200)))
	if len(h.events) != 2 || h.events[1].Type != "vital" {
		t.Fatalf("a poor LCP must preserve the pageview and triggering vital: %+v", h.events)
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

func TestFullRateKeepsEverything(t *testing.T) {
	a, now, h := sampledAgg(1, false, false)
	b := pageview(*now, "s1")
	result := a.Ingest(b)
	if !result.Investigated || len(h.events) != 1 {
		t.Fatal("a full investigate rate selects every session")
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

func TestSessionSampledIsStablePerSessionAndProportional(t *testing.T) {
	if !SessionSampled("any", 1) || SessionSampled("any", 0) {
		t.Fatal("rate 1 keeps every session, rate 0 none")
	}
	first := SessionSampled("s-42", 0.3)
	second := SessionSampled("s-42", 0.3)
	if first != second {
		t.Fatal("the decision must be the same every time for one session")
	}
	kept := 0
	for i := 0; i < 20000; i++ {
		if SessionSampled(fmt.Sprintf("session-%d", i), 0.1) {
			kept++
		}
	}
	if kept < 1700 || kept > 2300 {
		t.Fatalf("10%% of 20000 sessions kept = %d", kept)
	}
}

func TestHasPoorVital(t *testing.T) {
	for _, tc := range []struct {
		vitals []beacon.Vital
		want   bool
	}{
		{nil, false},
		{[]beacon.Vital{{Name: beacon.LCP, Value: 2400}, {Name: beacon.CLS, Value: 0.05}}, false},
		{[]beacon.Vital{{Name: beacon.LCP, Value: 4001}}, true},
		{[]beacon.Vital{{Name: beacon.INP, Value: 501}}, true},
		{[]beacon.Vital{{Name: beacon.CLS, Value: 0.26}}, true},
		{[]beacon.Vital{{Name: beacon.FCP, Value: 3001}}, true},
		{[]beacon.Vital{{Name: beacon.TTFB, Value: 1801}}, true},
		{[]beacon.Vital{{Name: beacon.LCP, Value: 4000}}, false},
		{[]beacon.Vital{{Name: "FID", Value: 1e9}}, false},
	} {
		if got := hasPoorVital(&beacon.Beacon{
			Vitals: tc.vitals,
		}); got != tc.want {
			t.Errorf("%+v: got %v", tc.vitals, got)
		}
	}
}

// A vital promotes an otherwise unsampled session exactly when the charts
// rate it poor, at and just above each Core Web Vitals poor limit.
func TestPoorVitalPromotionMatchesChartRating(t *testing.T) {
	limits := map[string]float64{beacon.LCP: 4000, beacon.FCP: 3000, beacon.TTFB: 1800, beacon.INP: 500, beacon.CLS: 0.25}
	for _, name := range beacon.Vitals {
		for _, value := range []float64{limits[name], limits[name] * 1.001} {
			a, now, _ := sampledAgg(0, false, true)
			promoted := a.Ingest(mk(*now, "session", "/", v(name, value))).Investigated
			chartPoor := a.Snapshot().Vitals[name].Poor == 1
			assert.Equal(t, value > limits[name], promoted, "%s=%v promotion", name, value)
			assert.Equal(t, promoted, chartPoor, "%s=%v promotion vs chart rating", name, value)
		}
	}
}
