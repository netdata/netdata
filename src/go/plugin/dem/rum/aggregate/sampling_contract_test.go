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
		assert.Equal(t, before.Received.UnixMicro(), e.TSUnixUS)
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
				*now = now.Add(beacon.SessionTTL + time.Second)
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
