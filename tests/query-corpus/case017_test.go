// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-017 — tier boundary absorption: a tier>0 query whose `after` equals a
// stored tier point's end absorbs that point into the FIRST bucket, leaking
// data that lies entirely outside (after, before] into the result.
//
// Root cause (verified at a7cc6d573c):
//   - the first plan of a tier>0 read expands its storage scan BACKWARDS by
//     POINTS_TO_EXPAND_QUERY points to seed interpolation
//     (query-plan.c:219 — tier 0 gets no expansion, which is why tier0 is
//     clean);
//   - the group-assignment boundary check is INCLUSIVE at the bucket start
//     (query-execute.c:295, end_time_s >= now_start_time), so the expanded
//     read's point ending exactly at `after` is added to the first group
//     instead of only seeding interpolation.
//
// Affects forced-tier queries AND auto-tier queries that land on tier>=1
// (zoomed-out views), whenever `after` is aligned to the tier grid — the
// first bucket then includes up to one full tier window of pre-window data.
package corpus

import (
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
)

func TestCase017TierBoundaryAbsorption(t *testing.T) {
	// 400 per-second samples of value i%10 from T0+1: tier1 windows end at
	// T0+40 (partial) then every 60s; data extends two full windows past the
	// last queried end (tier write-delay settle rule)
	ch := fixture.Series("fixture.c017", "fixture.c017", fixture.T0, 400, 1, modVal, notAnom)
	pushLiveBurst(t, "c017", guid(50), ch)
	if _, err := td.WaitRetention("c017", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// tier0 control: a stored sample at exactly `after` stays excluded —
	// bucket (T0+10, T0+11] must read sample T0+11 alone
	doc, err := td.DataV3("c017", daemon.DataParams(ch.Context, fixture.T0+10, fixture.T0+15, 5))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if col := cols["load"]; len(col) != 5 || col[0].Value == nil || *col[0].Value != 1 {
		t.Errorf("tier0 control: first bucket %+v, want value 1 (sample at `after` excluded) — tier0 gained the absorption?", col[0])
	}

	// tier1: after = T0+100 coincides with a stored tier1 window end.
	// Clean (after, before] first bucket (T0+160) = window(T0+160) alone;
	// the bug merges window(T0+100) — 60 pre-window samples — into it.
	windows := ch.Dimensions[0].TierWindows(tier1Gran)
	w100, w160 := windows[fixture.T0+100], windows[fixture.T0+160]

	doc, err = td.DataV3("c017", daemon.DataParamsTier(ch.Context, 1, fixture.T0+100, fixture.T0+280, 3, "sum"))
	if err != nil {
		t.Fatal(err)
	}
	cols, err = canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	if len(col) != 3 || col[0].T != fixture.T0+160 || col[0].Value == nil {
		t.Fatalf("tier1 probe returned unexpected shape: %+v", col)
	}

	got := *col[0].Value
	clean := w160.Sum
	absorbed := w100.Sum + w160.Sum
	switch {
	case tierValueMatch(got, clean, 0):
		expectAgentStatus(t, "CASE-017/tier-boundary-absorption", true)
	case tierValueMatch(got, absorbed, 0):
		t.Logf("first bucket sum %v = window(T0+100) %v + window(T0+160) %v — pre-window data absorbed",
			got, w100.Sum, w160.Sum)
		expectAgentStatus(t, "CASE-017/tier-boundary-absorption", false)
	default:
		t.Fatalf("first bucket sum %v matches neither clean %v nor absorbed %v — new behavior, investigate",
			got, clean, absorbed)
	}
}
