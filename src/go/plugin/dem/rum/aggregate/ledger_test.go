// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// A vital's replay evidence decides whether a later report of the same vital is
// a newer revision, a replay, or a different measurement reusing the name.
func TestVitalIdentityAdmission(t *testing.T) {
	tests := map[string]struct {
		renewedAfter  time.Duration // when set, revision 2 is admitted first, this long after the first report
		metricID      string
		revision      uint64
		clockAdvance  time.Duration
		receivedAhead time.Duration
		admitted      bool
		invalid       uint64
	}{
		"newer revision replaces the value": {
			metricID: "lcp-1", revision: 2, admitted: true,
		},
		"replayed revision is ignored": {
			metricID: "lcp-1", revision: 1,
		},
		"another metric identity is invalid": {
			metricID: "lcp-2", revision: 2, invalid: 1,
		},
		"another metric identity stays invalid after the window": {
			metricID: "lcp-2", revision: 2, clockAdvance: 6 * time.Minute, invalid: 1,
		},
		"newer revision is admitted after the window": {
			metricID: "lcp-1", revision: 2, clockAdvance: 6 * time.Minute, admitted: true,
		},
		"a newer revision extends the evidence": {
			renewedAfter: 20 * time.Minute, metricID: "lcp-2", revision: 3, clockAdvance: 20 * time.Minute, invalid: 1,
		},
		"a replay after an extension is still ignored": {
			renewedAfter: 20 * time.Minute, metricID: "lcp-1", revision: 1, clockAdvance: 20 * time.Minute,
		},
		"evidence older than its retention at receipt no longer applies": {
			metricID: "lcp-2", revision: 1, receivedAhead: identityRetention + time.Nanosecond, admitted: true,
		},
		"evidence at exactly its retention still applies": {
			metricID: "lcp-1", revision: 1, receivedAhead: identityRetention,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a, now := newAgg(5 * time.Minute)
			first := mk(*now, "session", "/page", beacon.Vital{ID: "lcp-1", Revision: 1, Name: beacon.LCP, Value: 100})
			require.Len(t, a.Ingest(first).Observation.Vitals, 1)
			if tc.renewedAfter > 0 {
				*now = now.Add(tc.renewedAfter)
				renewal := *first
				renewal.Received = *now
				renewal.Vitals = []beacon.Vital{{ID: "lcp-1", Revision: 2, Name: beacon.LCP, Value: 150}}
				require.Len(t, a.Ingest(&renewal).Observation.Vitals, 1)
			}

			*now = now.Add(tc.clockAdvance)
			next := *first
			next.Received = now.Add(tc.receivedAhead)
			next.Vitals = []beacon.Vital{{ID: tc.metricID, Revision: tc.revision, Name: beacon.LCP, Value: 200}}
			result := a.Ingest(&next)

			assert.Equal(t, tc.admitted, len(result.Observation.Vitals) == 1)
			assert.Equal(t, tc.invalid, a.Snapshot().Counters[CounterInvalidMeasurements])
		})
	}
}

// A replaced value belongs to the window of its latest receipt.
func TestNewerRevisionRenewsWindowMembership(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	first := mk(*now, "session", "/page", beacon.Vital{ID: "lcp-1", Revision: 1, Name: beacon.LCP, Value: 100})
	a.Ingest(first)

	*now = now.Add(4 * time.Minute)
	update := *first
	update.Received = *now
	update.Vitals = []beacon.Vital{{ID: "lcp-1", Revision: 2, Name: beacon.LCP, Value: 200}}
	a.Ingest(&update)

	*now = now.Add(2 * time.Minute)
	assert.Equal(t, VitalStats{N: 1, P50: 200, P75: 200, P95: 200, Good: 1}, a.Snapshot().Vitals[beacon.LCP])
	*now = now.Add(3*time.Minute + time.Nanosecond)
	assert.Equal(t, VitalStats{}, a.Snapshot().Vitals[beacon.LCP])
}

// Everything one report retains refers to one attribute set; a later report
// of the same vital keeps the set of its first admission.
func TestReportItemsShareAttributes(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := mk(*now, "session", "/page", v(beacon.LCP, 100), v(beacon.CLS, 0.1))
	b.Resources = []beacon.Resource{{ID: "resource", Host: "example.org"}}
	a.Ingest(b)

	lcp := observationKey{b.ExperienceID, beacon.LCP}
	cls := observationKey{b.ExperienceID, beacon.CLS}
	shared := a.site.observations[lcp].Value.attrs
	require.NotNil(t, shared)
	assert.Same(t, shared, a.site.observations[cls].Value.attrs)
	assert.Same(t, shared, a.site.vitalIdentities[lcp].Value.attrs)
	assert.Same(t, shared, a.site.vitalIdentities[cls].Value.attrs)
	assert.Same(t, shared, a.site.activity.entries[0].Value.attrs)
	assert.Same(t, shared, a.site.resources.entries[0].Value.attrs)

	*now = now.Add(time.Second)
	update := *b
	update.Received = *now
	update.Browser = "Firefox"
	update.Vitals = []beacon.Vital{{ID: beacon.LCP, Revision: 2, Name: beacon.LCP, Value: 200}}
	a.Ingest(&update)
	assert.Same(t, shared, a.site.observations[lcp].Value.attrs)
	assert.Equal(t, "Chrome", shared.browser)
}

func TestReceiptHeapOrder(t *testing.T) {
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	drain := func(h *receiptHeap[string]) []string {
		var out []string
		for h.Len() > 0 {
			out = append(out, h.pop().Value)
		}
		return out
	}
	tests := map[string]struct {
		change func(h *receiptHeap[string], entries map[string]*receiptEntry[string])
		want   []string
	}{
		"receipt time, then insertion order": {
			change: func(*receiptHeap[string], map[string]*receiptEntry[string]) {},
			want:   []string{"a", "b", "c", "d"},
		},
		"a later receipt moves the entry": {
			change: func(h *receiptHeap[string], e map[string]*receiptEntry[string]) {
				h.renew(e["a"], t0.Add(time.Minute))
			},
			want: []string{"b", "c", "d", "a"},
		},
		"an earlier receipt keeps the entry in place": {
			change: func(h *receiptHeap[string], e map[string]*receiptEntry[string]) {
				h.renew(e["d"], t0.Add(-time.Minute))
			},
			want: []string{"a", "b", "c", "d"},
		},
		"an equal receipt keeps insertion order": {
			change: func(h *receiptHeap[string], e map[string]*receiptEntry[string]) {
				h.renew(e["b"], t0.Add(time.Second))
			},
			want: []string{"a", "b", "c", "d"},
		},
		"removal leaves the rest ordered": {
			change: func(h *receiptHeap[string], e map[string]*receiptEntry[string]) {
				h.remove(e["b"])
			},
			want: []string{"a", "c", "d"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var h receiptHeap[string]
			entries := map[string]*receiptEntry[string]{
				"d": h.add(t0.Add(2*time.Second), "d"),
				"b": h.add(t0.Add(time.Second), "b"),
				"a": h.add(t0, "a"),
				"c": h.add(t0.Add(time.Second), "c"),
			}
			tc.change(&h, entries)
			assert.Equal(t, tc.want, drain(&h))
		})
	}
}
