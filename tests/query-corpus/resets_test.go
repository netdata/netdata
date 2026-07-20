// SPDX-License-Identifier: GPL-3.0-or-later

// S4 — counter resets and overflows through the parent's rrdset_done
// arithmetic (src/database/rrdset-collection.c), driven over the REAL
// v1 streaming protocol (BEGIN/SET/END, real-time paced like
// rates_test.go):
//
//   - implausible backward step: last > new with a wrapped delta above
//     10% of the cap → the sample contributes a ZERO increment and the
//     stored point carries SN_FLAG_RESET;
//   - plausible 32-bit wrap: wrapped delta below 10% of the cap → the
//     reconstructed delta is stored — with the engine's cap of
//     0xFFFFFFFF the reconstruction is (cap - last + new), ONE LESS
//     than the true modulo-2^32 delta (pinned quirk: a wrap of +100
//     stores 99); the point still carries SN_FLAG_RESET;
//   - percentage-of-incremental-row: the pre-pass absorbs a backward
//     step (last := collected) so the resetting dimension contributes
//     0% that sample while the surviving dimensions split 100%;
//   - long collection gap: a pause beyond the lost-iterations
//     threshold RESETS the collection (rrdset_collection_reset) — the
//     across-gap counter delta is NEVER spread as a rate spike, the
//     gap rows read null, and no RESET flag is stamped (the restart is
//     a clean first-sample).
//
// Excluded, recorded in the SOW: the 64-bit cap branch (requires a
// collected history the signed text protocol cannot express
// coherently) and the float reset path (parent-side float collection
// is unreachable through streaming: v1 SET is integer, v2 ships final
// values).
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// resetsSettle polls until the paced fixture's retention covers at
// least want samples, returning the landing window.
func resetsSettle(t *testing.T, hostname, context string, want int) daemon.Retention {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var ret daemon.Retention
	for {
		doc, err := td.DataV3(hostname, daemon.DataParams(context, 0, 0, 2))
		if err == nil {
			if r, ok := daemon.QueryRetention(doc); ok && r.LastEntry > r.FirstEntry {
				ret = r
				if int(r.LastEntry-r.FirstEntry) >= want {
					return ret
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("samples did not settle (last retention [%d,%d])", ret.FirstEntry, ret.LastEntry)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func resetsFetch(t *testing.T, hostname, context string, ret daemon.Retention) map[string][]canon.Pt {
	t.Helper()
	points := ret.LastEntry - ret.FirstEntry
	doc, err := td.DataV3(hostname, daemon.DataParams(context, ret.FirstEntry, ret.LastEntry, points))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	return cols
}

// assertSteadyWithBlend: interior rows (idx>=2, not last) must equal
// steady, except the rows touched by the special sample. Samples land
// at wall-clock sub-second offsets, so rrdset_done's interpolation
// BLENDS adjacent sample rates into the stored seconds: the special
// sample's contribution spreads over at most two ADJACENT rows, each
// between special and steady, conserving the rate mass exactly —
// the deviation from steady summed over the affected rows equals
// special - steady. wantResets rows must carry the RESET annotation
// (the resetting sample's first stored row).
func assertSteadyWithBlend(t *testing.T, col []canon.Pt, steady, special float64, wantResets int) {
	t.Helper()
	resets := 0
	var deviant []int
	var devSum float64
	lo := min(special, steady) - 1e-4
	hi := max(special, steady) + 1e-4
	for idx, pt := range col {
		if idx < 2 || idx == len(col)-1 || pt.Value == nil {
			continue
		}
		if pt.PA&canon.AnnotationReset != 0 {
			resets++
		}
		if !tierValueMatch(*pt.Value, steady, 1e-9) {
			deviant = append(deviant, idx)
			devSum += *pt.Value - steady
			if *pt.Value < lo || *pt.Value > hi {
				t.Errorf("row t=%d: %s outside the blend range [%v, %v]", pt.T, fmtPt(pt), lo, hi)
			}
		}
	}
	if resets != wantResets {
		t.Errorf("saw %d RESET rows, want %d", resets, wantResets)
	}
	switch {
	case len(deviant) == 0:
		t.Errorf("no rows deviate from steady %v — the special sample %v left no trace", steady, special)
	case len(deviant) > 2:
		t.Errorf("%d rows deviate from steady, want at most 2 (one blended pair)", len(deviant))
	case len(deviant) == 2 && deviant[1] != deviant[0]+1:
		t.Errorf("deviant rows %v are not adjacent", deviant)
	}
	if len(deviant) > 0 && !tierValueMatch(devSum, special-steady, 1e-4) {
		t.Errorf("blend mass %v, want %v (special %v - steady %v)", devSum, special-steady, special, steady)
	}
}

func TestCounterResets(t *testing.T) {
	type sample struct {
		values map[string]string
		pause  int // seconds to sleep before this sample (0 = ue)
	}
	cases := map[string]struct {
		guidIdx int
		ue      int
		dims    map[string]string // id → algorithm
		samples []sample
		verify  func(t *testing.T, cols map[string][]canon.Pt)
	}{
		// counter drops 1028 → 100: wrapped delta ~2^32 is implausible
		// (>10% of the 32-bit cap) → zero increment + RESET
		"implausible-backward-step": {
			guidIdx: 141, ue: 1,
			dims: map[string]string{"c": "incremental"},
			samples: func() []sample {
				vals := []int64{1000, 1007, 1014, 1021, 1028, 100, 107, 114, 121, 128}
				out := make([]sample, len(vals))
				for i, v := range vals {
					out[i] = sample{values: map[string]string{"c": strconv.FormatInt(v, 10)}}
				}
				return out
			}(),
			verify: func(t *testing.T, cols map[string][]canon.Pt) {
				assertSteadyWithBlend(t, cols["c"], 7, 0, 1)
			},
		},
		// counter wraps 4294967200 → 4 over the 32-bit boundary: the
		// wrapped delta 99 is plausible (<10% of cap) and stored — one
		// LESS than the true +100 (cap 0xFFFFFFFF, pinned quirk)
		"plausible-32bit-wrap": {
			guidIdx: 142, ue: 1,
			dims: map[string]string{"c": "incremental"},
			samples: func() []sample {
				vals := []int64{4294967000, 4294967100, 4294967200, 4, 104, 204, 304, 404}
				out := make([]sample, len(vals))
				for i, v := range vals {
					out[i] = sample{values: map[string]string{"c": strconv.FormatInt(v, 10)}}
				}
				return out
			}(),
			verify: func(t *testing.T, cols map[string][]canon.Pt) {
				assertSteadyWithBlend(t, cols["c"], 100, 99, 1)
			},
		},
		// two percentage-of-incremental-row dims (+30/+70 per second =
		// 30%/70%): dim a steps backwards once — the pre-pass absorbs
		// it (a contributes 0% and carries RESET, b takes 100%)
		"pcent-over-diff-reset": {
			guidIdx: 143, ue: 1,
			dims: map[string]string{
				"a": "percentage-of-incremental-row",
				"b": "percentage-of-incremental-row",
			},
			samples: func() []sample {
				a := []int64{1000, 1030, 1060, 1090, 1120, 10, 40, 70, 100, 130}
				b := []int64{2000, 2070, 2140, 2210, 2280, 2350, 2420, 2490, 2560, 2630}
				out := make([]sample, len(a))
				for i := range a {
					out[i] = sample{values: map[string]string{
						"a": strconv.FormatInt(a[i], 10),
						"b": strconv.FormatInt(b[i], 10),
					}}
				}
				return out
			}(),
			verify: func(t *testing.T, cols map[string][]canon.Pt) {
				assertSteadyWithBlend(t, cols["a"], 30, 0, 1)
				// b takes 100% on the reset sample but does NOT reset
				assertSteadyWithBlend(t, cols["b"], 70, 100, 0)
			},
		},
		// a 9s silence (> the 5-iteration gap threshold at ue=1) resets
		// the collection: gap rows are null, the across-gap counter
		// delta is NEVER spread as a spike, and nothing carries RESET
		"gap-collection-reset": {
			guidIdx: 144, ue: 1,
			dims: map[string]string{"c": "incremental"},
			samples: func() []sample {
				pre := []int64{1000, 1007, 1014, 1021, 1028}
				post := []int64{1035, 1042, 1049, 1056, 1063, 1070}
				var out []sample
				for _, v := range pre {
					out = append(out, sample{values: map[string]string{"c": strconv.FormatInt(v, 10)}})
				}
				for i, v := range post {
					s := sample{values: map[string]string{"c": strconv.FormatInt(v, 10)}}
					if i == 0 {
						s.pause = 9
					}
					out = append(out, s)
				}
				return out
			}(),
			verify: func(t *testing.T, cols map[string][]canon.Pt) {
				col := cols["c"]
				nulls, sevens, other := 0, 0, 0
				for idx, pt := range col {
					if idx < 2 || idx == len(col)-1 {
						continue
					}
					if pt.Value == nil {
						nulls++
						continue
					}
					if pt.PA&canon.AnnotationReset != 0 {
						t.Errorf("row t=%d carries RESET — a clean collection restart must not", pt.T)
					}
					if *pt.Value > 7+1e-6 {
						t.Errorf("row t=%d: %s — the across-gap delta leaked as a rate spike", pt.T, fmtPt(pt))
					}
					if tierValueMatch(*pt.Value, 7, 1e-9) {
						sevens++
					} else {
						other++
					}
				}
				if nulls < 3 {
					t.Errorf("saw %d null gap rows, want >= 3", nulls)
				}
				if sevens < 4 {
					t.Errorf("saw %d steady rows, want >= 4", sevens)
				}
				// up to three edge rows interpolate between 0 and 7:
				// the partial row before the silence plus the restart
				// blend pair — the contract pinned here is no spike,
				// no reset, gap nulls, steady rates on both sides
				if other > 3 {
					t.Errorf("saw %d non-steady non-null rows, want <= 3 gap/restart edges", other)
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel() // paced in real time — overlap the cases

			hostname := "resets-" + name
			context := "fixture.resets_" + name

			conn := connect(t, hostname, guid(tc.guidIdx), stream.CapsLiveV1)
			conn.DefineChart(stream.Chart{
				ID: context, Title: "resets", Units: "units/s", Family: "fixture",
				Context: context, UpdateEvery: tc.ue,
			})
			for id, algo := range tc.dims {
				conn.Dimension(id, algo, 1, 1)
			}

			for s, smp := range tc.samples {
				usec := int64(tc.ue) * 1_000_000
				switch {
				case s == 0:
					usec = 0 // first sample: the parent clocks it "now"
				case smp.pause > 0:
					time.Sleep(time.Duration(smp.pause) * time.Second)
					usec = int64(smp.pause) * 1_000_000
				default:
					time.Sleep(time.Duration(tc.ue) * time.Second)
				}
				conn.Begin(context, usec)
				for id, v := range smp.values {
					conn.Set(id, v)
				}
				conn.End()
				if err := conn.Flush(); err != nil {
					t.Fatal(err)
				}
			}

			ret := resetsSettle(t, hostname, context, len(tc.samples)-2)
			tc.verify(t, resetsFetch(t, hostname, context, ret))
		})
	}
}
