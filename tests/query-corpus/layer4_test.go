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

// tierFetchBuckets slices the dimension's stored tier windows into view
// buckets of bucketSpan seconds starting after `after`, yielding the
// fetched value sequence per bucket (empty/never-stored windows skipped)
// plus the per-bucket anomaly and stored-gap totals.
type tierBucketStats struct {
	AC, Count, GapCount int
}

func tierFetchBuckets(d fixture.Dimension, name string, granularity, updateEvery, after, bucketSpan int64, buckets int) ([][]float64, []tierBucketStats) {
	windows := d.TierWindows(granularity, updateEvery)
	vals := make([][]float64, buckets)
	stats := make([]tierBucketStats, buckets)
	for k := 0; k < buckets; k++ {
		lo := after + int64(k)*bucketSpan
		hi := lo + bucketSpan
		for end := lo + granularity; end <= hi; end += granularity {
			tp, ok := windows[end]
			if !ok {
				continue
			}
			stats[k].GapCount += tp.GapCount
			if tp.Empty {
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
	contracts := map[string]bool{
		"L4/family-tier-source":        true,
		"L4/family-tier-grid":          true,
		"L4/family-tier-values":        true,
		"L4/family-tier-anomaly-rates": true,
		"L4/family-tier-annotations":   true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

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
	fail := func(contract, format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		contracts[contract] = false
	}

	d := ch.Dimensions[0]
	for _, tg := range groups {
		completed := false
		t.Run(tg.name+optSuffix(tg.options), func(t *testing.T) {
			minmaxComponent := tg.name == "min" || tg.name == "max"
			minmaxOK := true
			if tg.name == "min" || tg.name == "max" {
				trackContractComponent(t, "L4/minmax-absolute-semantics", "tier1-"+tg.name)
			}
			mark := func(contract, format string, args ...any) {
				t.Helper()
				fail(contract, tg.name+optSuffix(tg.options)+": "+format, args...)
				if minmaxComponent && (contract == "L4/family-tier-source" ||
					contract == "L4/family-tier-grid" || contract == "L4/family-tier-values") {
					minmaxOK = false
				}
			}
			params := daemon.DataParamsTier(ch.Context, 1, after, after+buckets*bucketSpan, buckets, tg.name)
			if tg.options != "" {
				params.Set("time_group_options", tg.options)
			}
			doc, err := td.DataV3("l4-matrix", params)
			if err != nil {
				t.Fatal(err)
			}
			if !assertSelectedTier(t, doc, 1) {
				mark("L4/family-tier-source", "forced tier-1 query was not served exclusively from tier 1")
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !assertExactView(t, doc, after, after+buckets*bucketSpan, bucketSpan) {
				mark("L4/family-tier-grid", "response view is not the exact requested grid")
				contracts["L4/family-tier-values"] = false
				contracts["L4/family-tier-anomaly-rates"] = false
				contracts["L4/family-tier-annotations"] = false
			}
			if !assertOnlyColumn(t, cols, d.ID) {
				mark("L4/family-tier-source", "response contains the wrong source columns")
			}
			col := cols[d.ID]
			if len(col) != buckets {
				mark("L4/family-tier-grid", "got %d buckets, want %d", len(col), buckets)
				contracts["L4/family-tier-values"] = false
				contracts["L4/family-tier-anomaly-rates"] = false
				contracts["L4/family-tier-annotations"] = false
			}

			// view group = bucketSpan query-granularity units (virtual
			// points, qg=1) — drives only the ses/des window (capped 15)
			vals, stats := tierFetchBuckets(d, tg.name, tier1Gran, int64(ch.UpdateEvery), after, bucketSpan, buckets)
			exp := fixture.TGOracle(tg.name, tg.options, vals, bucketSpan, buckets)

			for i, pt := range col {
				if i >= len(exp) || i >= len(stats) {
					break
				}
				want := exp[i]
				wantT := int64(after) + int64(i+1)*bucketSpan
				if pt.T != wantT {
					mark("L4/family-tier-grid", "bucket %d time t0%+d, want t0%+d", i, pt.T-fixture.T0, wantT-fixture.T0)
					contracts["L4/family-tier-values"] = false
					contracts["L4/family-tier-anomaly-rates"] = false
					contracts["L4/family-tier-annotations"] = false
					continue
				}
				switch {
				case want.Empty && pt.Value != nil:
					mark("L4/family-tier-values", "bucket t0%+d value %v, want null", pt.T-fixture.T0, *pt.Value)
				case !want.Empty && pt.Value == nil:
					mark("L4/family-tier-values", "bucket t0%+d null, want %v", pt.T-fixture.T0, want.Value)
				case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
					mark("L4/family-tier-values", "bucket t0%+d value %v, want %v", pt.T-fixture.T0, *pt.Value, want.Value)
				}
				if st := stats[i]; st.Count > 0 {
					expARP := 100 * float64(st.AC) / float64(st.Count)
					if !tierValueMatch(pt.ARP, expARP, 0) {
						mark("L4/family-tier-anomaly-rates", "bucket t0%+d arp %v, want %v (%d/%d)",
							pt.T-fixture.T0, pt.ARP, expARP, st.AC, st.Count)
					}
				} else if pt.ARP != 0 {
					mark("L4/family-tier-anomaly-rates", "bucket t0%+d arp %v, want 0 with no contributors",
						pt.T-fixture.T0, pt.ARP)
				}
				wantPA := int64(0)
				if want.Empty {
					wantPA = canon.AnnotationEmpty
				} else if stats[i].GapCount > 0 {
					wantPA = canon.AnnotationPartial
				}
				if pt.PA != wantPA {
					mark("L4/family-tier-annotations", "bucket t0%+d pa %d, want %d", pt.T-fixture.T0, pt.PA, wantPA)
				}
			}
			if !minmaxOK {
				t.Errorf("BROKEN L4/minmax-absolute-semantics (tier1-%s)", tg.name)
			}
			completed = true
		})
		if !completed {
			for contract := range contracts {
				contracts[contract] = false
			}
		}
	}

	for _, contract := range []string{
		"L4/family-tier-source", "L4/family-tier-grid", "L4/family-tier-values",
		"L4/family-tier-anomaly-rates", "L4/family-tier-annotations",
	} {
		assertContract(t, contract, contracts[contract])
	}
}

// TestLayer4AutoTierSelection pins the automatic tier choice: with no
// tier parameter, the planner serves coarse windows from the highest
// fitting tier — and the values equal that tier's oracle. Reuses the
// layer-2 tier2 fixture (17200 replicated samples on host l2-tier2).
func TestLayer4AutoTierSelection(t *testing.T) {
	contracts := map[string]bool{
		"L4/auto-tier-choice":        true,
		"L4/auto-tier-grid":          true,
		"L4/auto-tier-values":        true,
		"L4/auto-tier-anomaly-rates": true,
		"L4/auto-tier-annotations":   true,
	}
	for contract := range contracts {
		registerContract(t, contract)
	}

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
	fail := func(contract, format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		contracts[contract] = false
	}

	for name, tc := range cases {
		doc, err := td.DataV3(host, daemon.DataParams(context, tc.after, tc.before, tc.points))
		if err != nil {
			t.Fatal(err)
		}

		if !assertSelectedTier(t, doc, tc.tier) {
			fail("L4/auto-tier-choice", "%s: expected only tier %d to serve the automatic-tier query", name, tc.tier)
		}

		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		span := (tc.before - tc.after) / tc.points
		if !assertExactView(t, doc, tc.after, tc.before, span) {
			fail("L4/auto-tier-grid", "%s: response view is not the exact requested grid", name)
			contracts["L4/auto-tier-values"] = false
			contracts["L4/auto-tier-anomaly-rates"] = false
			contracts["L4/auto-tier-annotations"] = false
		}
		if !assertOnlyColumn(t, cols, d.ID) {
			fail("L4/auto-tier-grid", "%s: response contains the wrong columns", name)
		}
		col := cols[d.ID]
		if int64(len(col)) != tc.points {
			fail("L4/auto-tier-grid", "%s: got %d buckets, want %d", name, len(col), tc.points)
			contracts["L4/auto-tier-values"] = false
			contracts["L4/auto-tier-anomaly-rates"] = false
			contracts["L4/auto-tier-annotations"] = false
		}

		var (
			exp   []fixture.TGResult
			stats []tierBucketStats
		)
		if tc.tier == 0 {
			vals := make([][]float64, tc.points)
			stats = make([]tierBucketStats, tc.points)
			for _, p := range d.Points {
				if p.T > tc.after && p.T <= tc.before {
					bucket := (p.T - tc.after - 1) / span
					if v, collected := p.CollectedValue(d.ID); collected {
						vals[bucket] = append(vals[bucket], fixture.SNRoundTrip(v))
						stats[bucket].Count++
						if p.Flags == stream.FlagAnomalous {
							stats[bucket].AC++
						}
					}
				}
			}
			exp = fixture.TGOracle("average", "", vals, int(span), int(tc.points))
		} else {
			var vals [][]float64
			vals, stats = tierFetchBuckets(d, "average", tier1Gran, int64(ch.UpdateEvery), tc.after, span, int(tc.points))
			exp = fixture.TGOracle("average", "", vals, int(span), int(tc.points))
		}

		for i, pt := range col {
			if i >= len(exp) || i >= len(stats) {
				break
			}
			want := exp[i]
			wantT := tc.after + int64(i+1)*span
			if pt.T != wantT {
				fail("L4/auto-tier-grid", "%s: bucket %d time %d, want %d", name, i, pt.T, wantT)
				contracts["L4/auto-tier-values"] = false
				contracts["L4/auto-tier-anomaly-rates"] = false
				contracts["L4/auto-tier-annotations"] = false
				continue
			}
			switch {
			case want.Empty && pt.Value != nil:
				fail("L4/auto-tier-values", "%s: bucket t0%+d value %v, want null", name, pt.T-fixture.T0, *pt.Value)
			case !want.Empty && pt.Value == nil:
				fail("L4/auto-tier-values", "%s: bucket t0%+d null, want %v", name, pt.T-fixture.T0, want.Value)
			case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
				fail("L4/auto-tier-values", "%s: bucket t0%+d value %v, want %v", name, pt.T-fixture.T0, *pt.Value, want.Value)
			}
			wantARP := 0.0
			if stats[i].Count > 0 {
				wantARP = 100 * float64(stats[i].AC) / float64(stats[i].Count)
			}
			if !tierValueMatch(pt.ARP, wantARP, 0) {
				fail("L4/auto-tier-anomaly-rates", "%s: bucket t0%+d arp %v, want %v (%d/%d)",
					name, pt.T-fixture.T0, pt.ARP, wantARP, stats[i].AC, stats[i].Count)
			}
			wantPA := int64(0)
			if want.Empty {
				wantPA = canon.AnnotationEmpty
			} else if stats[i].GapCount > 0 {
				wantPA = canon.AnnotationPartial
			}
			if pt.PA != wantPA {
				fail("L4/auto-tier-annotations", "%s: bucket t0%+d pa %d, want %d", name, pt.T-fixture.T0, pt.PA, wantPA)
			}
		}
	}

	for _, contract := range []string{
		"L4/auto-tier-choice", "L4/auto-tier-grid", "L4/auto-tier-values",
		"L4/auto-tier-anomaly-rates", "L4/auto-tier-annotations",
	} {
		assertContract(t, contract, contracts[contract])
	}
}
