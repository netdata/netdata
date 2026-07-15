// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 4 — tier edges, parts (a) auto-tier selection and (b) the
// time-aggregation family matrix over tier data.
//
// On tier>=1 every family except min/max/sum consumes the per-tier-point
// AVERAGE (tier_query_fetch registry map) — so time_group=average over
// rollup tiers is an average of window averages (pinned quantitatively
// here with unequal window counts), min/max/sum stay exact.
//
// Part (c) — plan switching across tiers with different retention — runs
// on a dedicated small-quota daemon and is implemented separately.
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// perTierPoints extracts db.per_tier[].points from a json2 reply.
func perTierPoints(doc map[string]any) []int64 {
	db, _ := doc["db"].(map[string]any)
	tiersAny, _ := db["per_tier"].([]any)
	out := make([]int64, 0, len(tiersAny))
	for _, ta := range tiersAny {
		tier, _ := ta.(map[string]any)
		points, _ := tier["points"].(float64)
		out = append(out, int64(points))
	}
	return out
}

// tierFetchBuckets slices the dimension's stored tier windows into view
// buckets of bucketSpan seconds starting after `after`, yielding the
// fetched value sequence per bucket (empty/never-stored windows skipped)
// plus the per-bucket anomaly totals.
func tierFetchBuckets(d fixture.Dimension, name string, granularity, after, bucketSpan int64, buckets int) ([][]float64, []struct{ AC, Count int }) {
	windows := d.TierWindows(granularity)
	vals := make([][]float64, buckets)
	stats := make([]struct{ AC, Count int }, buckets)
	for k := 0; k < buckets; k++ {
		lo := after + int64(k)*bucketSpan
		hi := lo + bucketSpan
		for end := lo + granularity; end <= hi; end += granularity {
			tp, ok := windows[end]
			if !ok || tp.Empty {
				continue
			}
			vals[k] = append(vals[k], fixture.TierFetchValue(name, tp))
			stats[k].AC += tp.AnomalyCount
			stats[k].Count += tp.Count
		}
	}
	return vals, stats
}

// TestLayer4FamilyTierMatrix drives the grouping families over FORCED
// tier1 data with 6 tier windows per view bucket — partial windows (the
// unaligned head, a gap run) and anomaly runs included, so families see
// unequal per-window counts.
func TestLayer4FamilyTierMatrix(t *testing.T) {
	ch := fixture.Series("fixture.l4matrix", "fixture.l4matrix", fixture.T0, 2400, 1, func(i int) string {
		return strconv.FormatFloat(float64(i%13-6)+float64(i%7)/10, 'f', 1, 64)
	}, func(i int) string {
		switch {
		case i >= 381 && i <= 480:
			return stream.FlagEmpty // window T0+460 all-gap; 400/520 partial
		case i >= 601 && i <= 690:
			return stream.FlagAnomalous // fractional window anomaly rates
		}
		return stream.FlagNotAnomalous
	})
	ch.ValueTolerance = 1e-9

	pushLiveBurst(t, "l4-matrix", guid(70), ch)
	if _, err := td.WaitRetention("l4-matrix", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	// aligned=true rounds `before` UP to a multiple of group×granularity,
	// so the window must be 360-aligned in absolute terms: T0 % 360 = 80,
	// hence bucket ends at T0+280+360k. after also predates the data
	// (clean start).
	const (
		after      = fixture.T0 - 80
		bucketSpan = 6 * tier1Gran
		buckets    = 6
	)

	groups := []struct {
		name    string
		options string
	}{
		{"average", ""}, {"sum", ""}, {"min", ""}, {"max", ""},
		{"extremes", ""}, {"stddev", ""}, {"cv", ""},
		{"median", ""}, {"trimmed-median25", ""},
		{"percentile", ""}, {"percentile25", ""},
		{"trimmed-mean", ""}, {"trimmed-mean25", ""},
		{"countif", ">0"},
		{"ses", ""}, {"des", ""}, {"incremental-sum", ""},
	}

	d := ch.Dimensions[0]
	for _, tg := range groups {
		t.Run(tg.name+optSuffix(tg.options), func(t *testing.T) {
			params := daemon.DataParamsTier(ch.Context, 1, after, after+buckets*bucketSpan, buckets, tg.name)
			if tg.options != "" {
				params.Set("time_group_options", tg.options)
			}
			doc, err := td.DataV3("l4-matrix", params)
			if err != nil {
				t.Fatal(err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			col := cols[d.ID]
			if len(col) != buckets {
				t.Fatalf("got %d buckets, want %d", len(col), buckets)
			}

			// view group = bucketSpan query-granularity units (virtual
			// points, qg=1) — drives only the ses/des window (capped 15)
			vals, stats := tierFetchBuckets(d, tg.name, tier1Gran, after, bucketSpan, buckets)
			exp := fixture.TGOracle(tg.name, tg.options, vals, bucketSpan, buckets)

			for i, pt := range col {
				want := exp[i]
				wantT := int64(after) + int64(i+1)*bucketSpan
				if pt.T != wantT {
					t.Errorf("bucket %d: time t0%+d, want t0%+d", i, pt.T-fixture.T0, wantT-fixture.T0)
					continue
				}
				switch {
				case want.Empty && pt.Value != nil:
					t.Errorf("bucket t0%+d: value %v, want null", pt.T-fixture.T0, *pt.Value)
				case !want.Empty && pt.Value == nil:
					t.Errorf("bucket t0%+d: null, want %v", pt.T-fixture.T0, want.Value)
				case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
					t.Errorf("bucket t0%+d: value %v, want %v", pt.T-fixture.T0, *pt.Value, want.Value)
				}
				if st := stats[i]; st.Count > 0 {
					expARP := 100 * float64(st.AC) / float64(st.Count)
					if !tierValueMatch(pt.ARP, expARP, 0) {
						t.Errorf("bucket t0%+d: arp %v, want %v (%d/%d)", pt.T-fixture.T0, pt.ARP, expARP, st.AC, st.Count)
					}
				}
			}
		})
	}
}

// TestLayer4AutoTierSelection pins the automatic tier choice: with no
// tier parameter, the planner serves coarse windows from the highest
// fitting tier — and the values equal that tier's oracle. Reuses the
// layer-2 tier2 fixture (17200 replicated samples on host l2-tier2).
func TestLayer4AutoTierSelection(t *testing.T) {
	const host, context = "l2-tier2", "fixture.l2tier2"
	value := func(i int) string { return strconv.Itoa(i % 1000) }
	flags := func(i int) string {
		if i >= 6500 && i <= 10000 {
			return stream.FlagEmpty
		}
		return stream.FlagNotAnomalous
	}
	ch := fixture.Series(context, context, fixture.T0, 17200, 1, value, flags)
	d := ch.Dimensions[0]

	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Skip("layer-2 tier2 fixture not available (TestLayer2Tier2 failed?)")
	}

	// Selection rule (query-plan.c query_metric_best_tier_for_timeframe):
	// among tiers whose point density over the window is ACCEPTABLE
	// (>= wanted points, floored at QUERY_PLAN_MIN_POINTS=10), the
	// COARSEST acceptable tier wins (smallest weight). With full coverage
	// on all tiers, tier2 (3600s windows) is acceptable only for windows
	// >= ~10h — beyond this fixture — so even 3600s buckets are served
	// from tier1. Auto-selection OF tier2 needs coverage differences
	// (layer 4 part c) or a multi-day fixture.
	cases := map[string]struct {
		tier                  int
		after, before, points int64
	}{
		// 3600s buckets: tier2 grid-aligned, still served from tier1
		// (coarsest ACCEPTABLE: tier1 density 180 >= 10, tier2 3 < 10)
		"coarse-buckets-from-tier1": {tier: 1, after: fixture.T0 - 800, before: fixture.T0 + 10000, points: 3},
		// 60s buckets: tier1 exactly acceptable
		"tier1": {tier: 1, after: fixture.T0 - 20, before: fixture.T0 + 3580, points: 60},
		// per-second identity: only tier0 delivers the density
		"tier0": {tier: 0, after: fixture.T0 + 100, before: fixture.T0 + 160, points: 60},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doc, err := td.DataV3(host, daemon.DataParams(context, tc.after, tc.before, tc.points))
			if err != nil {
				t.Fatal(err)
			}

			perTier := perTierPoints(doc)
			for tier, points := range perTier {
				switch {
				case tier == tc.tier && points == 0:
					t.Errorf("expected tier %d to serve the query, but it read 0 points (per_tier %v)", tc.tier, perTier)
				case tier != tc.tier && points != 0:
					t.Errorf("tier %d read %d points, expected only tier %d to serve (per_tier %v)", tier, points, tc.tier, perTier)
				}
			}

			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			col := cols[d.ID]
			if int64(len(col)) != tc.points {
				t.Fatalf("got %d buckets, want %d", len(col), tc.points)
			}

			span := (tc.before - tc.after) / tc.points
			var exp []fixture.TGResult
			if tc.tier == 0 {
				for _, p := range d.Points {
					if p.T > tc.after && p.T <= tc.before {
						v, _ := strconv.ParseFloat(p.Collected, 64)
						exp = append(exp, fixture.TGResult{Value: fixture.SNRoundTrip(v)})
					}
				}
			} else {
				vals, _ := tierFetchBuckets(d, "average", tier1Gran, tc.after, span, int(tc.points))
				exp = fixture.TGOracle("average", "", vals, int(span), int(tc.points))
			}

			for i, pt := range col {
				want := exp[i]
				switch {
				case want.Empty && pt.Value != nil:
					t.Errorf("bucket t0%+d: value %v, want null", pt.T-fixture.T0, *pt.Value)
				case !want.Empty && pt.Value == nil:
					t.Errorf("bucket t0%+d: null, want %v", pt.T-fixture.T0, want.Value)
				case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
					t.Errorf("bucket t0%+d: value %v, want %v", pt.T-fixture.T0, *pt.Value, want.Value)
				}
			}
		})
	}
}
