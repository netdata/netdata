// SPDX-License-Identifier: GPL-3.0-or-later

// Ingestion rate semantics (layer-1 extension): the db stores PER-SECOND
// rates regardless of the chart's update_every. A v1 (non-interpolated)
// child streams RAW collected counters and the PARENT runs the full
// rrdset_done math — so a counter advancing by K per update_every=UE
// seconds must be stored as K*(mul/div)/UE per second.
//
// v1 samples carry no absolute timestamps and the engine clamps any
// sample whose chart clock would run ahead of the wall clock
// (rrdset-collection.c: "COLLECTION TIME IN FUTURE" → duration 0), so
// these fixtures are PACED IN REAL TIME with explicit BEGIN microseconds
// — the only corpus fixtures not on the fixed 2023 epoch. The assertion
// window is discovered from the db retention.
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestIncrementalRates(t *testing.T) {
	cases := map[string]struct {
		ue       int
		algo     string
		mul, div int
		step     int64 // counter increment per interval (incremental) or the constant reading (absolute)
		want     float64
		samples  int
	}{
		// counter +100 per 5s interval → 20/s in the db
		"incremental-ue5": {ue: 5, algo: "incremental", mul: 1, div: 1, step: 100, want: 20, samples: 8},
		// per-second control: +7 per 1s interval → 7/s
		"incremental-ue1": {ue: 1, algo: "incremental", mul: 1, div: 1, step: 7, want: 7, samples: 10},
		// scaling: +100 per 2s with mul=8,div=10 → 100*0.8/2 = 40/s
		"incremental-scaled-ue2": {ue: 2, algo: "incremental", mul: 8, div: 10, step: 100, want: 40, samples: 8},
		// absolute control at ue=2: the value is NOT divided by the
		// interval — gauges are level readings, not rates
		"absolute-ue2": {ue: 2, algo: "absolute", mul: 1, div: 1, step: 100, want: 100, samples: 9},
	}

	i := 0
	for name, tc := range cases {
		i++
		hostname := "rates-" + name
		context := "fixture.rates_" + name
		g := guid(100 + i)

		t.Run(name, func(t *testing.T) {
			t.Parallel() // the cases pace in real time — overlap them

			conn := connect(t, hostname, g, stream.CapsLiveV1)
			conn.DefineChart(stream.Chart{
				ID: context, Title: "rates", Units: "units/s", Family: "fixture",
				Context: context, UpdateEvery: tc.ue,
			})
			conn.Dimension("rate", tc.algo, tc.mul, tc.div)

			counter := int64(1000)
			for s := range tc.samples {
				usec := int64(tc.ue) * 1_000_000
				if s == 0 {
					usec = 0 // first sample: the parent clocks it "now"
				} else {
					time.Sleep(time.Duration(tc.ue) * time.Second)
				}
				conn.Begin(context, usec)
				if tc.algo == "incremental" {
					counter += tc.step
					conn.Set("rate", strconv.FormatInt(counter, 10))
				} else {
					conn.Set("rate", strconv.FormatInt(tc.step, 10))
				}
				conn.End()
				if err := conn.Flush(); err != nil {
					t.Fatal(err)
				}
			}

			// discover the landing window (wall-clocked, not the epoch)
			deadline := time.Now().Add(15 * time.Second)
			var ret daemon.Retention
			for {
				doc, err := td.DataV3(hostname, daemon.DataParams(context, 0, 0, 2))
				if err == nil {
					if r, ok := daemon.QueryRetention(doc); ok && r.LastEntry > r.FirstEntry {
						ret = r
						if int(r.LastEntry-r.FirstEntry)/tc.ue >= tc.samples-2 {
							break
						}
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("rate samples did not settle (last retention [%d,%d])", ret.FirstEntry, ret.LastEntry)
				}
				time.Sleep(200 * time.Millisecond)
			}

			points := (ret.LastEntry - ret.FirstEntry) / int64(tc.ue)
			doc, err := td.DataV3(hostname, daemon.DataParams(context, ret.FirstEntry, ret.LastEntry, points))
			if err != nil {
				t.Fatal(err)
			}
			cols, err := canon.Columns(doc)
			if err != nil {
				t.Fatal(err)
			}
			col := cols["rate"]
			if len(col) == 0 {
				t.Fatal("no rows")
			}

			// the first stored sample of an incremental dimension has no
			// prior counter, and the first stored interval carries the
			// unaligned-start interpolation spill — assert from the third row
			steady := 0
			for idx, pt := range col {
				if idx < 2 || idx == len(col)-1 || pt.Value == nil {
					continue
				}
				if !tierValueMatch(*pt.Value, tc.want, 1e-9) {
					t.Errorf("row t=%d: value %v, want %v per second", pt.T, *pt.Value, tc.want)
				} else {
					steady++
				}
			}
			if steady < 2 {
				t.Errorf("only %d steady points matched (of %d rows) — rate conversion unstable?", steady, len(col))
			}
		})
	}
}

// viewUnits extracts view.units from a json2 reply (string or array form).
func viewUnits(doc map[string]any) string {
	view, _ := doc["view"].(map[string]any)
	switch u := view["units"].(type) {
	case string:
		return u
	case []any:
		if len(u) > 0 {
			if s, ok := u[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// TestSumOverTimeVolume pins the two modes of sum-over-time
// (query-execute.c query_point_grouping_value): on RATE-stored metrics
// (incremental algorithm) every point is multiplied by its duration, so
// the sum is the VOLUME (value x seconds) at any update_every; on
// non-rate metrics the plain values are summed. Reuses the wall-clocked
// rates fixtures.
//
// CASE-020 (red): the volume mode does not adjust the units — a rate in
// "units/s" summed over time is a volume in "units", but the response
// still reports "units/s".
func TestSumOverTimeVolume(t *testing.T) {
	sumQuery := func(t *testing.T, hostname, context string, ue int, bucket int64) (map[string][]canon.Pt, map[string]any) {
		t.Helper()
		doc, err := td.DataV3(hostname, daemon.DataParams(context, 0, 0, 2))
		if err != nil {
			t.Skip("rates fixture not available (TestIncrementalRates failed?)")
		}
		ret, ok := daemon.QueryRetention(doc)
		if !ok || ret.LastEntry <= ret.FirstEntry {
			t.Skip("rates fixture not available (TestIncrementalRates failed?)")
		}
		// align the window to whole buckets
		span := (ret.LastEntry - ret.FirstEntry) / bucket * bucket
		params := daemon.DataParams(context, ret.LastEntry-span, ret.LastEntry, span/bucket)
		params.Set("time_group", "sum")
		doc, err = td.DataV3(hostname, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		return cols, doc
	}

	t.Run("rate-volume", func(t *testing.T) {
		// 20 units/s stored per 5s point → 100 units of volume per point;
		// a 2-point bucket accumulates 200
		for _, bucketPoints := range []int64{1, 2} {
			cols, _ := sumQuery(t, "rates-incremental-ue5", "fixture.rates_incremental-ue5", 5, bucketPoints*5)
			col := cols["rate"]
			if len(col) < 3 {
				t.Fatalf("bucket=%d: only %d rows", bucketPoints, len(col))
			}
			want := float64(20 * 5 * bucketPoints)
			steady := 0
			for idx, pt := range col {
				if idx == 0 || idx == len(col)-1 || pt.Value == nil {
					continue
				}
				if !tierValueMatch(*pt.Value, want, 1e-9) {
					t.Errorf("bucket=%dpt row t=%d: value %v, want volume %v", bucketPoints, pt.T, *pt.Value, want)
				} else {
					steady++
				}
			}
			if steady == 0 {
				t.Errorf("bucket=%dpt: no steady volume points matched", bucketPoints)
			}
		}
	})

	t.Run("gauge-plain-sum", func(t *testing.T) {
		// absolute (non-rate) metrics sum plainly: 100 per point, 200 for
		// a 2-point bucket — no duration multiplication
		cols, _ := sumQuery(t, "rates-absolute-ue2", "fixture.rates_absolute-ue2", 2, 2*2)
		col := cols["rate"]
		steady := 0
		for idx, pt := range col {
			if idx == 0 || idx == len(col)-1 || pt.Value == nil {
				continue
			}
			if !tierValueMatch(*pt.Value, 200, 1e-9) {
				t.Errorf("row t=%d: value %v, want plain sum 200", pt.T, *pt.Value)
			} else {
				steady++
			}
		}
		if steady == 0 {
			t.Errorf("no steady plain-sum points matched")
		}
	})

	t.Run("units", func(t *testing.T) {
		_, doc := sumQuery(t, "rates-incremental-ue5", "fixture.rates_incremental-ue5", 5, 5)
		units := viewUnits(doc)
		t.Logf("sum-over-time of a units/s rate reports units %q", units)
		expectAgentStatus(t, "CASE-020/sum-over-time-units", units == "units")
	})
}
