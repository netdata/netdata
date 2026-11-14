// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func decodeSamplingBeacon(t *testing.T, now time.Time, payload map[string]any) *beacon.Beacon {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	b, err := faro.Decode(raw, faro.Options{
		Site: "s",
		Now:  now,
	})
	require.NoError(t, err)
	return b
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

func TestFullRateKeepsEverything(t *testing.T) {
	a, now, h := sampledAgg(1, false, false)
	b := pageview(*now, "s1")
	result := a.Ingest(b)
	if !result.Investigated || len(h.events) != 1 {
		t.Fatal("a full investigate rate selects every session")
	}
}

func TestZeroDetailSelectionWithOverrides(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprint(keep), func(t *testing.T) {
			a, now, h := sampledAgg(0, keep, false)
			assert.False(t, a.Ingest(mk(*now, "s1", "/before")).Investigated)
			assert.Empty(t, h.events)
			b := mk(*now, "s1", "/failure")
			b.Errors = []beacon.Error{{Type: "Error", Message: "failed", Fingerprint: "f"}}
			assert.Equal(t, keep, a.Ingest(b).Investigated)
			assert.EqualValues(t, 2, a.Snapshot().Counters[CounterPageviews])
			assert.EqualValues(t, 1, a.Snapshot().Counters[CounterJSErrors])
			if keep {
				require.Len(t, h.events, 3)
			} else {
				assert.Empty(t, h.events)
			}
		})
	}
}

func TestSessionlessSamplingKeepsMeasurementsWithoutInventingSessions(t *testing.T) {
	for _, rate := range []float64{0, 0.5, 1} {
		for _, keep := range []bool{false, true} {
			t.Run(fmt.Sprintf("rate=%v/overrides=%v", rate, keep), func(t *testing.T) {
				a, now, h := sampledAgg(rate, keep, keep)
				assert.Equal(t, rate == 1, a.Ingest(mk(*now, "", "/plain")).Investigated)
				b := mk(*now, "", "/error")
				b.Errors = []beacon.Error{{Type: "Error", Message: "failed", Fingerprint: "failure"}}
				assert.Equal(t, rate == 1 || keep, a.Ingest(b).Investigated)
				assert.Equal(t, rate == 1 || keep, a.Ingest(mk(*now, "", "/slow", v(beacon.LCP, 5200))).Investigated)
				assert.EqualValues(t, 3, a.Snapshot().Counters[CounterPageviews])
				assert.Zero(t, a.Activity().ObservedSessions)
				if rate == 1 || keep {
					require.Len(t, h.events, 1, "only the standalone error has a native history surface")
					assert.Empty(t, h.events[0].SessionID)
					assert.Equal(t, "error", h.events[0].Type)
				} else {
					assert.Empty(t, h.events)
				}
			})
		}
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

func TestPromotionPreservesEntireTriggeringBeacon(t *testing.T) {
	a, now, history := sampledAgg(0.000001, true, false)
	a.Ingest(mk(*now, "late-error", "/before"))
	require.Empty(t, history.events)
	events := make([]map[string]string, 101)
	for i := range events {
		events[i] = map[string]string{"name": fmt.Sprintf("action_%03d", i)}
	}
	b := decodeSamplingBeacon(t, *now, map[string]any{
		"meta": map[string]any{
			"session": map[string]string{"id": "late-error"},
			"page":    map[string]string{"url": "https://shop.example.org/checkout"},
		},
		"exceptions": []map[string]string{{"type": "TypeError", "value": "checkout failed"}}, "events": events,
	})
	require.True(t, a.Ingest(b).Investigated)
	var errors, actions int
	for _, event := range history.events {
		if event.Type == "error" {
			errors++
			assert.Equal(t, "checkout failed", event.Message)
		}
		if event.Type == "event" {
			actions++
		}
	}
	assert.Equal(t, 1, errors)
	assert.Equal(t, 101, actions)
	assert.Equal(t, "/before", history.events[0].Page)
	size := len(history.events)
	*now = now.Add(time.Second)
	a.Ingest(mk(*now, "late-error", "/after"))
	assert.Len(t, history.events, size+1, "prior context must not be replayed twice")
	live, ok := a.SessionEvents("late-error")
	require.True(t, ok)
	assert.Len(t, live, sessionEventRingCap, "the live context bound stays unchanged")
}

func TestPromotionRetainsUnattributedPoorVital(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value float64
	}{
		{"lcp", 5200}, {"fcp", 3500}, {"ttfb", 2000}, {"inp", 600}, {"cls", 0.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, now, h := sampledAgg(0.000001, false, true)
			b := decodeSamplingBeacon(t, *now, map[string]any{
				"meta": map[string]any{
					"session": map[string]string{"id": "slow"},
					"page":    map[string]string{"id": "activation", "url": "https://shop.example.org/"},
				},
				"measurements": []map[string]any{
					{
						"type":    "web-vitals",
						"values":  map[string]float64{tc.name: tc.value},
						"context": map[string]string{"id": "metric", "observation_sequence": "1"},
					},
				},
			})
			require.True(t, a.Ingest(b).Investigated)
			var vitals []HistoryEvent
			for _, e := range h.events {
				if e.Type == "vital" {
					vitals = append(vitals, e)
				}
			}
			require.Len(t, vitals, 1)
			assert.Contains(t, vitals[0].Text, "poor ")
			assert.NotContains(t, vitals[0].Text, " on ")
		})
	}
}

func TestPromotionPreservesOriginalContext(t *testing.T) {
	a, now, h := sampledAgg(0.000001, true, false)
	before := mk(*now, "late-error", "/before")
	before.UserID = "identity-before"
	before.AppVersion = "before"
	before.Events = append(before.Events, beacon.Event{
		Name: "prior_action",
	})
	a.Ingest(before)
	require.Empty(t, h.events)
	*now = now.Add(time.Second)
	after := mk(*now, "late-error", "/after")
	after.UserID = "identity-after"
	after.AppVersion = "after"
	after.Browser = "Firefox"
	after.Device = "mobile"
	after.Country = "DE"
	after.Errors = []beacon.Error{{Type: "Error", Message: "failure", Fingerprint: "failure"}}
	require.True(t, a.Ingest(after).Investigated)
	require.Len(t, h.events, 4)
	for _, e := range h.events[:2] {
		assert.Equal(t, "identity-before", e.UserID)
		assert.Equal(t, "before", e.Version)
		assert.Equal(t, "GR", e.Country)
		assert.Equal(t, "Chrome", e.Browser)
		assert.Equal(t, "desktop", e.Device)
		assert.Equal(t, before.Received.UnixMicro(), e.ObservedUS)
	}
	for _, e := range h.events[2:] {
		assert.Equal(t, "identity-after", e.UserID)
		assert.Equal(t, "after", e.Version)
	}
	*now = now.Add(time.Second)
	anonymous := mk(*now, "late-error", "/anonymous")
	a.Ingest(anonymous)
	assert.Empty(t, h.events[len(h.events)-1].UserID, "an anonymous observation must not inherit a prior identity")
	assert.Empty(t, h.events[len(h.events)-1].Version)
}

func TestPromotionReplaysOnlyAvailablePriorContext(t *testing.T) {
	a, now, h := sampledAgg(0, true, false)
	for i := range 125 {
		a.Ingest(mk(*now, "s", fmt.Sprintf("/before/page%d", i)))
	}
	require.Empty(t, h.events)
	b := mk(*now, "s", "/error")
	b.Errors = []beacon.Error{{Type: "Error", Message: "failed", Fingerprint: "failure"}}
	a.Ingest(b)
	require.Len(t, h.events, 102)
	assert.Equal(t, "/before/page25", h.events[0].Page)
	assert.Equal(t, "/before/page124", h.events[99].Page)
	assert.Equal(t, "error", h.events[101].Type)
}

func TestPromotionLastsOnlyWhileSessionIsTracked(t *testing.T) {
	for _, reset := range []string{"expiry", "eviction", "restart"} {
		t.Run(reset, func(t *testing.T) {
			a, now, h := sampledAgg(0, true, false)
			b := mk(*now, "promoted", "/error")
			b.Errors = []beacon.Error{{Type: "Error", Message: "failed", Fingerprint: "failure"}}
			require.True(t, a.Ingest(b).Investigated)
			require.True(t, a.Ingest(mk(*now, "promoted", "/after")).Investigated)
			switch reset {
			case "expiry":
				*now = now.Add(sessionTTL + time.Second)
				a.Activity()
			case "eviction":
				for i := range maxTrackedSessions {
					a.Ingest(mk(*now, fmt.Sprintf("other-%d", i), "/other"))
				}
			case "restart":
				a, now, h = sampledAgg(0, true, false)
			}
			saved := len(h.events)
			assert.False(t, a.Ingest(mk(*now, "promoted", "/return")).Investigated)
			assert.Len(t, h.events, saved, "lost promotion state must not promise continued history")
		})
	}
}

func TestInactiveDetailPromotionExpiresOnIngestion(t *testing.T) {
	a, now, _ := sampledAgg(0, true, false)
	b := mk(*now, "s", "/")
	b.Errors = []beacon.Error{{Message: "failure"}}
	require.True(t, a.Ingest(b).Investigated)
	*now = now.Add(31 * time.Minute)
	assert.False(t, a.Ingest(mk(*now, "s", "/new")).Investigated)
}
