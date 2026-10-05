// SPDX-License-Identifier: GPL-3.0-or-later

// Error-group state: fingerprint → bounded rolling
// summary, feeding the rum.error_groups chart and the rum-errors
// FUNCTION. Kept in its own file since it is a distinct sub-model from
// the vitals/breakdown series in aggregate.go.
package aggregate

import (
	"container/list"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

const (
	maxErrorGroups   = 200  // per site, LRU by last seen
	errMaxStackBytes = 4096 // defensive second cap; the Faro decoder already bounds this
	errorGroupsTopN  = 10   // rum.error_groups chart instances
)

// errorGroup is one fingerprint's rolling summary.
type errorGroup struct {
	typ, message string
	lastSeen     time.Time
	total        uint64 // monotonic all-time count (chart "errors" incremental dim)
}

type errEntry struct {
	fp string
	g  *errorGroup
}

// recordError updates the error-group LRU for one occurrence.
func (st *siteState) recordError(err beacon.Error, now time.Time) {
	fp := err.Fingerprint
	if fp == "" {
		return // defensive: the collector always computes one
	}
	g := st.touchErrorGroup(fp, now)
	g.total++
	g.lastSeen = now
	if g.typ == "" {
		g.typ = err.Type
		g.message = err.Message
	}
}

func truncateStack(s string) string {
	if len(s) <= errMaxStackBytes {
		return s
	}
	n := errMaxStackBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (st *siteState) touchErrorGroup(fp string, now time.Time) *errorGroup {
	if el, ok := st.errGroupIdx[fp]; ok {
		st.errGroups.MoveToFront(el)
		return el.Value.(*errEntry).g
	}
	for st.errGroups.Len() >= maxErrorGroups {
		st.dropErrorGroup(st.errGroups.Back())
	}
	g := &errorGroup{}
	st.errGroupIdx[fp] = st.errGroups.PushFront(&errEntry{
		fp: fp,
		g:  g,
	})
	return g
}

func (st *siteState) dropErrorGroup(el *list.Element) {
	delete(st.errGroupIdx, el.Value.(*errEntry).fp)
	st.errGroups.Remove(el)
}

// ErrorChartGroup is one rum.error_groups chart instance.
type ErrorChartGroup struct {
	Fingerprint string
	Message     string
	Total       uint64 // monotonic all-time count
}

// rankErrorGroups ranks the LRU by all-time total (top-10),
// folding the rest into a synthetic "other" instance. Unlike the
// generic browser/device/country/page breakdowns, occurrences are never
// routed through "other" at ingest time — every group always keeps its
// own accurate total, so "other" here is a live sum over the current
// non-top members. Trade-off: if a low-ranked group jumps into the top
// set, "other"'s reported total can visibly drop for one collection
// round (looks like a counter reset in the incremental dimension) —
// accepted as a rare, cosmetic artifact on this secondary chart rather
// than replicating the full routing machinery for a bounded 200-entry
// state.
func (st *siteState) rankErrorGroups() []ErrorChartGroup {
	type cand struct {
		fp string
		g  *errorGroup
	}
	cands := make([]cand, 0, st.errGroups.Len())
	for el := st.errGroups.Front(); el != nil; el = el.Next() {
		e := el.Value.(*errEntry)
		cands = append(cands, cand{e.fp, e.g})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].g.total != cands[j].g.total {
			return cands[i].g.total > cands[j].g.total
		}
		return cands[i].fp < cands[j].fp
	})
	n := errorGroupsTopN
	if n > len(cands) {
		n = len(cands)
	}
	out := make([]ErrorChartGroup, 0, n+1)
	for _, c := range cands[:n] {
		out = append(out, ErrorChartGroup{
			Fingerprint: c.fp,
			Message:     c.g.message,
			Total:       c.g.total,
		})
	}

	if rest := cands[n:]; len(rest) > 0 {
		var otherTotal uint64
		for _, c := range rest {
			otherTotal += c.g.total
		}
		out = append(out, ErrorChartGroup{
			Fingerprint: Other,
			Message:     "",
			Total:       otherTotal,
		})
	}
	return out
}
