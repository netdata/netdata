// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 3 — time-aggregations one by one: every registry time_group
// produces exactly the per-bucket value of its Go oracle
// (fixture/timegroup.go, exact ports of src/web/api/queries/<family>).
//
// Feed contract (verified in layer 0-2): tier0 group>1 buckets feed the
// grouping with the SNRoundTrip'd sample values in timestamp order; gap
// slots are never added; ue=1 points never interpolate. EMPTY flushes
// surface as null values with the EMPTY annotation.
//
// Notable pinned semantics:
//   - ses/des state RUNS ACROSS buckets: an all-gap bucket after data
//     returns the running level, not null;
//   - incremental-sum carries the previous bucket's last value forward as
//     the next bucket's baseline, including from a bucket that held only
//     one sample; an empty bucket keeps the baseline it was given — so an
//     identity (group=1) incremental-sum query answers every bucket but the
//     first (pinned as current contract);
//   - percentile/trimmed-mean are slot-window MEANS (not quantiles) and
//     walk from the top when any bucket value is negative;
//   - median trims by value range, then takes the R-7 quantile.
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

// tgBuckets slices the dimension's samples into consecutive buckets of
// group samples, keeping only numeric (non-gap) values, SNRoundTrip'd —
// exactly what the engine feeds the time grouping.
func tgBuckets(d fixture.Dimension, group int) [][]float64 {
	var out [][]float64
	for start := 0; start < len(d.Points); start += group {
		end := min(start+group, len(d.Points))
		var bucket []float64
		for _, p := range d.Points[start:end] {
			if v, collected := p.CollectedValue(d.ID); collected {
				bucket = append(bucket, fixture.SNRoundTrip(v))
			}
		}
		out = append(out, bucket)
	}
	return out
}

// tgQuery names one time-group request and the oracle that must answer
// it: OracleName/OracleOptions default to the sent pair, and diverge only
// for the registry alias rows (alias == oracle-verified canonical) and
// the silent-fallback pin (an unknown time_group parses to average).
type tgQuery struct {
	Name, Options             string
	OracleName, OracleOptions string
}

// verifyTimeGroup queries ch with the given time_group/options at the
// given bucket size and asserts every bucket against the oracle.
func verifyTimeGroup(t *testing.T, host string, ch fixture.Chart, name, options string, group int) {
	t.Helper()
	verifyTimeGroupAs(t, host, ch, tgQuery{Name: name, Options: options, OracleName: name, OracleOptions: options}, group)
}

func timeGroupColumnAndOracle(
	t *testing.T, host string, ch fixture.Chart, q tgQuery, group int, diagnosticSuffix string,
) ([]canon.Pt, []fixture.TGResult) {
	t.Helper()

	name, options := q.Name, q.Options
	diagnostic := name + optSuffix(options) + diagnosticSuffix
	d := ch.Dimensions[0]
	n := int64(len(d.Points))
	points := n / int64(group)

	params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+n, points)
	params.Set("time_group", name)
	if options != "" {
		params.Set("time_group_options", options)
	}
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatalf("%s: %v", diagnostic, err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatalf("%s: %v", diagnostic, err)
	}
	col := cols[d.ID]
	if int64(len(col)) != points {
		t.Fatalf("%s: got %d buckets, want %d", diagnostic, len(col), points)
	}

	exp := fixture.TGOracle(q.OracleName, q.OracleOptions, tgBuckets(d, group), group, int(points))
	return col, exp
}

func verifyTimeGroupAs(t *testing.T, host string, ch fixture.Chart, q tgQuery, group int) {
	t.Helper()

	name, options := q.Name, q.Options
	col, exp := timeGroupColumnAndOracle(t, host, ch, q, group, "")
	for i, pt := range col {
		want := exp[i]
		bucketT := fixture.T0 + int64((i+1)*group)
		if pt.T != bucketT {
			t.Errorf("%s%s bucket %d: time t0%+d, want t0%+d", name, optSuffix(options), i, pt.T-fixture.T0, bucketT-fixture.T0)
			continue
		}
		switch {
		case want.Empty && pt.Value != nil:
			t.Errorf("%s%s bucket t0%+d: value %v, want null", name, optSuffix(options), pt.T-fixture.T0, *pt.Value)
		case !want.Empty && pt.Value == nil:
			t.Errorf("%s%s bucket t0%+d: null, want %v", name, optSuffix(options), pt.T-fixture.T0, want.Value)
		case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
			t.Errorf("%s%s bucket t0%+d: value %v, want %v", name, optSuffix(options), pt.T-fixture.T0, *pt.Value, want.Value)
		}
	}
}

func verifyTimeGroupEmptyAnnotations(t *testing.T, host string, ch fixture.Chart, name, options string, group int) {
	t.Helper()

	q := tgQuery{Name: name, Options: options, OracleName: name, OracleOptions: options}
	col, exp := timeGroupColumnAndOracle(t, host, ch, q, group, " annotations")
	for i, pt := range col {
		wantT := fixture.T0 + int64((i+1)*group)
		if pt.T != wantT {
			t.Errorf("%s%s annotation bucket %d: time t0%+d, want t0%+d",
				name, optSuffix(options), i, pt.T-fixture.T0, wantT-fixture.T0)
			continue
		}
		if exp[i].Empty && pt.PA&canon.AnnotationEmpty == 0 {
			t.Errorf("%s%s bucket t0%+d: EMPTY annotation missing (pa %d)",
				name, optSuffix(options), pt.T-fixture.T0, pt.PA)
		}
	}
}

func optSuffix(options string) string {
	if options == "" {
		return ""
	}
	return "(" + options + ")"
}

// layer3Canonical is the shared mixed fixture: 60 per-second samples of
// value i, an all-gap decade (i 21..30 — bucket 3 at group 10), an
// anomaly run (i 41..45 — bucket 5 arp 50%), a reset at i 55 (bucket 6).
func layer3Canonical(chartID string) fixture.Chart {
	return fixture.Series(chartID, chartID, fixture.T0, 60, 1, strconv.Itoa, func(i int) string {
		switch {
		case i >= 21 && i <= 30:
			return stream.FlagEmpty
		case i >= 41 && i <= 45:
			return stream.FlagAnomalous
		case i == 55:
			return stream.FlagNotAnomalous + stream.FlagReset
		}
		return stream.FlagNotAnomalous
	})
}

// TestLayer3Families drives every time-grouping family (and the alias/
// variant spread) over the canonical fixture at group 10.
func TestLayer3Families(t *testing.T) {
	registerContract(t, "L3/family-values")
	registerContract(t, "L3/family-annotations")

	ch := layer3Canonical("fixture.l3canon")
	pushLiveBurst(t, "l3-canon", guid(60), ch)
	if _, err := td.WaitRetention("l3-canon", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	groups := []struct {
		name    string
		options string
	}{
		{"average", ""}, {"avg", ""}, {"mean", ""},
		{"sum", ""}, {"min", ""}, {"max", ""},
		{"extremes", ""}, {"latest", ""},
		{"stddev", ""}, {"cv", ""}, {"rsd", ""},
		{"median", ""},
		{"trimmed-median", ""}, {"trimmed-median1", ""}, {"trimmed-median25", ""},
		{"trimmed-median", "10"}, // options override the percent
		{"percentile", ""}, {"percentile25", ""}, {"percentile50", ""}, {"percentile99", ""},
		{"trimmed-mean", ""}, {"trimmed-mean1", ""}, {"trimmed-mean25", ""},
		{"ses", ""}, {"ema", ""}, {"des", ""},
		{"incremental-sum", ""},
		{"countif", ">30"}, {"countif", "<=20"}, {"countif", "!=1"}, {"countif", "=40"},
	}

	t.Run("values", func(t *testing.T) {
		trackContract(t, "L3/family-values")

		for _, tg := range groups {
			t.Run(tg.name+optSuffix(tg.options), func(t *testing.T) {
				verifyTimeGroup(t, "l3-canon", ch, tg.name, tg.options, 10)
			})
		}
	})

	// bucket-level annotations are family-independent: arp 50 on the
	// anomaly-run bucket, RESET on the reset bucket
	t.Run("annotations", func(t *testing.T) {
		trackContract(t, "L3/family-annotations")
		for _, tg := range groups {
			t.Run(tg.name+optSuffix(tg.options), func(t *testing.T) {
				verifyTimeGroupEmptyAnnotations(t, "l3-canon", ch, tg.name, tg.options, 10)
			})
		}

		doc, err := td.DataV3("l3-canon", daemon.DataParams(ch.Context, fixture.T0, fixture.T0+60, 6))
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		col := cols["load"]
		if len(col) != 6 {
			t.Fatalf("got %d buckets, want 6", len(col))
		}
		if col[4].ARP != 50 {
			t.Errorf("anomaly-run bucket arp %v, want 50", col[4].ARP)
		}
		if col[5].PA&canon.AnnotationReset == 0 {
			t.Errorf("reset bucket pa %d, RESET bit missing", col[5].PA)
		}
		if col[2].PA&canon.AnnotationEmpty == 0 {
			t.Errorf("all-gap bucket pa %d, EMPTY bit missing", col[2].PA)
		}
	})
}

// TestLayer3SignSemantics pins the sign-dependent slot walks of
// percentile/trimmed-mean and the extremes champion across all-negative
// and mixed-sign data (per-bucket sign decides the direction).
func TestLayer3SignSemantics(t *testing.T) {
	registerContract(t, "L3/sign-semantics")
	registerContractComponent(t, "L4/minmax-absolute-semantics", "tier0-min")
	registerContractComponent(t, "L4/minmax-absolute-semantics", "tier0-max")

	cases := map[string]struct {
		hostname string
		guid     string
		value    func(i int) string
	}{
		"all-negative": {
			hostname: "l3-neg", guid: guid(61),
			value: func(i int) string { return strconv.Itoa(-i) },
		},
		"mixed-signs": {
			hostname: "l3-mixed", guid: guid(62),
			value: func(i int) string { return strconv.Itoa(i - 30) },
		},
	}

	signGroups := []struct {
		name    string
		options string
	}{
		{"percentile", ""}, {"percentile25", ""},
		{"trimmed-mean", ""}, {"trimmed-mean25", ""},
		{"trimmed-median25", ""},
		{"extremes", ""}, {"median", ""},
	}

	charts := make(map[string]fixture.Chart, len(cases))
	for name, tc := range cases {
		context := "fixture.l3" + tc.hostname[3:]
		ch := fixture.Series(context, context, fixture.T0, 60, 1, tc.value, notAnom)
		pushLiveBurst(t, tc.hostname, tc.guid, ch)
		if _, err := td.WaitRetention(tc.hostname, ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
			t.Fatal(err)
		}
		charts[name] = ch
	}

	t.Run("order-statistic-sign-semantics", func(t *testing.T) {
		trackContract(t, "L3/sign-semantics")
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				for _, tg := range signGroups {
					t.Run(tg.name, func(t *testing.T) {
						verifyTimeGroup(t, tc.hostname, charts[name], tg.name, tg.options, 10)
					})
				}
			})
		}
	})

	// min/max are by ABSOLUTE value (min.h/max.h). Keep their tier-0
	// evidence wholly under the L4 contract instead of the L3 sign verdict.
	for _, group := range []string{"min", "max"} {
		t.Run("absolute-"+group, func(t *testing.T) {
			trackContractComponent(t, "L4/minmax-absolute-semantics", "tier0-"+group)
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) {
					verifyTimeGroup(t, tc.hostname, charts[name], group, "", 10)
				})
			}
		})
	}
}

// TestLayer3SparseBuckets pins single-numeric-value buckets: one value
// per decade, the rest gaps. stddev yields 0.0 (not null); average/min/
// max/median pass the value through; incremental-sum answers from its
// second value onward, measuring each against the one before it — the
// leading single-value bucket seeds the chain and every empty bucket
// preserves the baseline it received (pinned as current contract).
func TestLayer3SparseBuckets(t *testing.T) {
	trackContract(t, "L3/sparse-buckets")

	ch := fixture.Series("fixture.l3sparse", "fixture.l3sparse", fixture.T0, 60, 1, strconv.Itoa, func(i int) string {
		if i%10 == 5 {
			return stream.FlagNotAnomalous
		}
		return stream.FlagEmpty
	})
	pushLiveBurst(t, "l3-sparse", guid(63), ch)
	if _, err := td.WaitRetention("l3-sparse", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	for _, tg := range []string{"average", "min", "max", "median", "stddev", "incremental-sum", "ses", "des", "extremes"} {
		t.Run(tg, func(t *testing.T) {
			verifyTimeGroup(t, "l3-sparse", ch, tg, "", 10)
		})
	}
}

// TestLayer3IdentitySmoothing pins ses/des at group=1 (identity view):
// the smoothing window comes from the requested points (capped 15), and
// incremental-sum at identity answers every bucket but the first.
func TestLayer3IdentitySmoothing(t *testing.T) {
	trackContract(t, "L3/identity-smoothing")

	ch := fixture.Series("fixture.l3ident", "fixture.l3ident", fixture.T0, 60, 1, func(i int) string {
		return strconv.Itoa((i*7)%23 - 5)
	}, notAnom)
	pushLiveBurst(t, "l3-ident", guid(64), ch)
	if _, err := td.WaitRetention("l3-ident", ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	for _, tg := range []string{"ses", "des", "incremental-sum"} {
		t.Run(tg, func(t *testing.T) {
			verifyTimeGroup(t, "l3-ident", ch, tg, "", 1)
		})
	}
}
