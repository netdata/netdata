// SPDX-License-Identifier: GPL-3.0-or-later

// S5 — the update_every sweep: the corpus otherwise runs at ue {1,2,5};
// this file drives ue {10, 30, 60, 600, 3600} through the backdated v2
// protocol and pins, per ue:
//   - tier0 ingestion identity (value/ARP/annotations per point);
//   - tier1 rollup windows on the scaled grid (granularity = ue x 60,
//     absolute wall-clock alignment) including partial counts, a
//     stored-empty window family, and fractional anomaly rates;
//   - time-grouping bucket arithmetic on the ue grid (bucket ends at
//     multiples of group x ue from T0).
//
// The paced v1 rate contract at large ue is wall-time bound: one ue=10
// case lives in rates_test-style pacing here; larger ue rate pacing is
// infeasible in test wall-time (recorded in the SOW; the per-second
// normalization K*(mul/div)/UE is pinned at ue {1,2,5,10}).
package corpus

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestUpdateEverySweep(t *testing.T) {
	const n = 400
	for idx, ue := range []int{10, 30, 60, 600, 3600} {
		host := fmt.Sprintf("ue-%d", ue)
		ctx := fmt.Sprintf("fixture.ue%d", ue)
		// sample ends sit on the absolute ue grid: storage keeps off-grid
		// times as pushed but the VIEW re-grids to absolute ue multiples
		// (TestOffGridTimestamps pins both halves; off-grid straddle
		// semantics are layer-9 seeds)
		base := fixture.T0 - fixture.T0%int64(ue)
		ch := fixture.Series(ctx, ctx, base, n, ue, func(i int) string {
			return strconv.Itoa(i % 1000)
		}, func(i int) string {
			switch {
			case i >= 90 && i <= 170:
				return stream.FlagEmpty // spans window boundaries → partial + stored-empty
			case i >= 200 && i <= 225:
				return stream.FlagAnomalous // fractional window anomaly rates
			}
			return stream.FlagNotAnomalous
		})
		// values up to 399 quantize with ~1e-15 SN artifacts that the
		// 7-fractional-digit JSON print truncates — allow print noise
		ch.ValueTolerance = 1e-8

		t.Run(fmt.Sprintf("ue-%d", ue), func(t *testing.T) {
			pushLiveBurst(t, host, guid(150+idx), ch)
			if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
				t.Fatal(err)
			}

			t.Run("tier0-identity", func(t *testing.T) {
				settleAndVerify(t, host, ch)
			})

			t.Run("tier1-windows", func(t *testing.T) {
				gran := int64(ue) * tier1Gran
				first := base + int64(ue)
				firstEnd := first + (gran-first%gran)%gran
				// n samples cover ~6.6 windows; assert 4 and leave the
				// tail margin (layer-2 discipline)
				verifyTierWindows(t, host, ch, 1, gran, firstEnd, firstEnd+3*gran)
			})

			// two grid modes, both with boundaries coinciding with the
			// (aligned) sample ends so no boundary interpolation fires:
			//   default — bucket ends snap to ABSOLUTE multiples of
			//   group x ue; `after` rounds UP to that grid, trimming
			//   the leading samples;
			//   unaligned — the grid anchors at `after` itself.
			// A grid whose boundaries CUT sample intervals triggers
			// virtual-point boundary interpolation — layer-9 seed,
			// evidence recorded in the SOW.
			assertBuckets := func(t *testing.T, unaligned bool) {
				const group = 10
				bucketSpan := int64(group * ue)
				anchor := base
				d := ch.Dimensions[0]
				if !unaligned {
					anchor = base + (bucketSpan-base%bucketSpan)%bucketSpan
					skip := int((anchor - base) / int64(ue))
					d = fixture.Dimension{ID: d.ID, Points: d.Points[skip:]}
				}
				buckets := tgBuckets(d, group)
				span := int64(n * ue)
				for _, tg := range []string{"average", "sum", "stddev"} {
					params := daemon.DataParams(ctx, base, base+span, int64(n/group))
					params.Set("time_group", tg)
					if unaligned {
						params.Set("options", "jsonwrap|unaligned")
					}
					doc, err := td.DataV3(host, params)
					if err != nil {
						t.Fatal(err)
					}
					cols, err := canon.Columns(doc)
					if err != nil {
						t.Fatal(err)
					}
					col := cols[d.ID]
					if len(col) != len(buckets) {
						t.Fatalf("%s: got %d buckets, want %d", tg, len(col), len(buckets))
					}
					exp := fixture.TGOracle(tg, "", buckets, group, len(buckets))
					for i, pt := range col {
						want := exp[i]
						bucketT := anchor + int64(i+1)*bucketSpan
						if pt.T != bucketT {
							t.Errorf("%s bucket %d: time t0%+d, want t0%+d", tg, i, pt.T-fixture.T0, bucketT-fixture.T0)
							continue
						}
						switch {
						case want.Empty && pt.Value != nil:
							t.Errorf("%s bucket t0%+d: value %v, want null", tg, pt.T-fixture.T0, *pt.Value)
						case !want.Empty && pt.Value == nil:
							t.Errorf("%s bucket t0%+d: null, want %v", tg, pt.T-fixture.T0, want.Value)
						case !want.Empty && !tierValueMatch(*pt.Value, want.Value, 1e-9):
							t.Errorf("%s bucket t0%+d: value %v, want %v", tg, pt.T-fixture.T0, *pt.Value, want.Value)
						}
					}
				}
			}
			t.Run("time-group-aligned", func(t *testing.T) { assertBuckets(t, false) })
			t.Run("time-group-unaligned", func(t *testing.T) { assertBuckets(t, true) })
		})
	}
}

// TestIncrementalRateUE10 extends the paced v1 rate contract to ue=10:
// a counter advancing +50 per 10s interval stores 5/s.
func TestIncrementalRateUE10(t *testing.T) {
	t.Parallel() // paced in real time — overlap the other paced cases

	const (
		hostname = "rates-ue10"
		context  = "fixture.rates_ue10"
		ue       = 10
		samples  = 6
	)
	conn := connect(t, hostname, guid(156), stream.CapsLiveV1)
	conn.DefineChart(stream.Chart{
		ID: context, Title: "rates", Units: "units/s", Family: "fixture",
		Context: context, UpdateEvery: ue,
	})
	conn.Dimension("rate", "incremental", 1, 1)

	counter := int64(1000)
	for s := range samples {
		usec := int64(ue) * 1_000_000
		if s == 0 {
			usec = 0
		} else {
			time.Sleep(ue * time.Second)
		}
		counter += 50
		conn.Begin(context, usec)
		conn.Set("rate", strconv.FormatInt(counter, 10))
		conn.End()
		if err := conn.Flush(); err != nil {
			t.Fatal(err)
		}
	}

	ret := resetsSettle(t, hostname, context, (samples-2)*ue)
	points := (ret.LastEntry - ret.FirstEntry) / int64(ue)
	doc, err := td.DataV3(hostname, daemon.DataParams(context, ret.FirstEntry, ret.LastEntry, points))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["rate"]
	steady := 0
	for idx, pt := range col {
		if idx < 2 || idx == len(col)-1 || pt.Value == nil {
			continue
		}
		if !tierValueMatch(*pt.Value, 5, 1e-9) {
			t.Errorf("row t=%d: value %v, want 5 per second", pt.T, *pt.Value)
		} else {
			steady++
		}
	}
	// 6 paced samples yield ~4 native rows and a single interior row —
	// one exact match already discriminates the /ue divisor (a wrong
	// divisor reads 50 or 4.55, never 5)
	if steady < 1 {
		t.Errorf("no steady points matched (of %d rows)", len(col))
	}
}

// TestOffGridTimestamps pins BOTH halves of the off-grid contract for
// samples pushed off the absolute update_every grid (pushed T0+30i with
// T0%30=20):
//   - STORAGE keeps the pushed timestamps exactly — retention reads
//     [T0+30, T0+300], no ingestion-side snapping;
//   - the VIEW re-grids to absolute ue multiples: each grid slot serves
//     the sample it covers, re-timed to the slot end (T0+30i+10).
// Invisible whenever the epoch is ue-aligned — the authority for why
// sweep fixtures pre-align their timestamps so oracles map 1:1.
func TestOffGridTimestamps(t *testing.T) {
	const ue = 30
	const n = 10
	ctx := "fixture.uesnap"
	if fixture.T0%ue == 0 {
		t.Fatal("fixture epoch became ue-aligned — pick another ue")
	}
	ch := fixture.Series(ctx, ctx, fixture.T0, n, ue, strconv.Itoa, notAnom)
	pushLiveBurst(t, "ue-snap", guid(157), ch)

	ret, err := td.WaitRetention("ue-snap", ctx, ch.FirstT(), ch.LastT(), 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ret.FirstEntry != ch.FirstT() || ret.LastEntry != ch.LastT() {
		t.Errorf("retention [%d,%d], want the pushed off-grid [%d,%d] — storage must not re-time samples",
			ret.FirstEntry, ret.LastEntry, ch.FirstT(), ch.LastT())
	}

	snap := int64(ue - fixture.T0%ue) // +10 for T0%30=20
	gridFirst := ch.FirstT() + snap
	doc, err := td.DataV3("ue-snap", daemon.DataParams(ctx, gridFirst-ue, gridFirst+int64((n-1)*ue), n))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	col := cols["load"]
	if len(col) != n {
		t.Fatalf("got %d rows, want %d", len(col), n)
	}
	for i, pt := range col {
		wantT := gridFirst + int64(i*ue)
		if pt.T != wantT {
			t.Errorf("row %d: time t0%+d, want the grid slot t0%+d", i, pt.T-fixture.T0, wantT-fixture.T0)
		}
		if i == len(col)-1 {
			// the final grid slot has no following sample to
			// interpolate against — the engine serves null
			continue
		}
		// grid slots cutting sample intervals serve INTERPOLATED
		// values (observed: i + 1/3 at the 10/30 phase) — the exact
		// virtual-point oracle is layer-9 work (SOW seed); pinned here
		// as the envelope between the two neighboring samples
		if pt.Value == nil || *pt.Value < float64(i+1)-1e-6 || *pt.Value > float64(i+2)+1e-6 {
			t.Errorf("row %d: %s, want within [%d,%d] (boundary interpolation)", i, fmtPt(pt), i+1, i+2)
		}
	}
}
