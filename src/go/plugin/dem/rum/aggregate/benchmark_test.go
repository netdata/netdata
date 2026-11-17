// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type discardHistory struct{}

func (discardHistory) Event(HistoryEvent) {}

// benchClock says whether benchIngest advances the site clock between rounds.
type benchClock bool

const (
	// fixedClock is never moved by benchIngest: retained populations stay as built.
	fixedClock benchClock = false
	// advancingClock moves benchRoundStep per round; reports that take their
	// receipt from it expire with the retention windows, as on a live site.
	advancingClock benchClock = true

	// Ingest benchmarks run in rounds of benchRoundReports reports.
	benchRoundReports = 1000
	benchRoundStep    = time.Minute
	// benchIdentityReports spans the 30-minute identity retention plus a round:
	// under an advancing clock, every identity admitted this many reports earlier
	// has expired, and the retained identity population has reached its size.
	benchIdentityReports = (int(identityRetention/benchRoundStep) + 2) * benchRoundReports
	// benchWarmupReports carries an ingest benchmark past one-time growth: the
	// identity population above, and the live ring's compaction cycle, which
	// repeats from its first compaction at about two ring capacities of rows
	// (every fixture that uses the ring adds at least one row per report).
	benchWarmupReports = max(4*liveRingCap, benchIdentityReports)
)

// benchIngest runs ingest(i) for benchWarmupReports reports before the timer
// and then for b.N timed reports; i counts across both. Allocations therefore
// show the steady-state cost of one report, not growth amortized over b.N: ring
// and slice capacities are reached and retained populations stop growing before
// the timer starts.
//
// The receiver activity log is compacted at most once per minute of site time.
// An advancing clock reaches that compaction every round. A fixed clock never
// does, so the log is emptied in place every round instead, standing in for it.
func benchIngest(b *testing.B, a *Aggregator, now *time.Time, clock benchClock, ingest func(i int)) {
	report := func(i int) {
		if i%benchRoundReports == 0 {
			if clock == advancingClock {
				*now = now.Add(benchRoundStep)
			} else {
				a.site.accepted = a.site.accepted[:0]
			}
		}
		ingest(i)
	}
	for i := range benchWarmupReports {
		report(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		report(benchWarmupReports + i)
	}
}

// Fixed experiences with monotonically newer reports measure real update work.
// The fixed clock and receipt time exercise replacement without heap reordering.
// Advancing receipts cost O(log retained) per item; timings are local trends,
// while allocation counts define the cost envelope.
func BenchmarkIngest(b *testing.B) {
	for _, history := range []bool{false, true} {
		name := "measure"
		if history {
			name = "investigate"
		}
		b.Run(name, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				VnodeGUID: "11111111-1111-4111-8111-111111111111",
				Investigate: InvestigateCfg{
					Rate: 1,
				},
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			if history {
				a.SetHistorySink(discardHistory{})
			}
			input := &beacon.Beacon{
				ExperienceID: "document",
				Site:         "shop",
				SessionID:    "browser",
				Path:         "/products",
				PageGroup:    "/products",
				Browser:      "Chrome",
				Country:      "GR",
				Device:       "desktop",
				Received:     *now,
				Vitals: []beacon.Vital{
					{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 1200},
					{ID: "cls", Revision: 1, Name: beacon.CLS, Value: .1},
				},
			}
			benchIngest(b, a, now, fixedClock, func(i int) {
				input.Vitals[0].Revision = uint64(i + 1)
				input.Vitals[1].Revision = uint64(i + 1)
				a.Ingest(input)
			})
		})
	}
}

// One report touching every ingestion section and timeline kind: a paired
// navigation revision, poor vitals, a new resource with its timing event,
// session lifecycle, frustration, traced request and custom events, and an
// error. "new-document" also admits a new activation each time; "same-document"
// admits its activation again once per identity retention, because duplicates
// do not renew it. Receipts follow an advancing clock, so admitted items expire
// and the retained populations hold a steady size. Timings are local trends;
// allocation counts define the per-report envelope.
func BenchmarkIngestFullReport(b *testing.B) {
	for _, newDocument := range []bool{false, true} {
		name := "same-document"
		if newDocument {
			name = "new-document"
		}
		b.Run(name, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				VnodeGUID: "11111111-1111-4111-8111-111111111111",
				Investigate: InvestigateCfg{
					Rate: 1,
				},
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			a.SetHistorySink(discardHistory{})
			// IDs repeat every benchIdentityReports reports, when the identities
			// admitted under them have expired, so each report still admits a new
			// resource (and activation). Reused keys refill the slots their expired
			// entries left; never-reused keys would make the identity maps grow
			// new tables now and then, and B/op would depend on b.N.
			resources, documents := make([]string, benchIdentityReports), make([]string, benchIdentityReports)
			for i := range benchIdentityReports {
				resources[i], documents[i] = "resource-"+strconv.Itoa(i), "document-"+strconv.Itoa(i)
			}
			input := &beacon.Beacon{
				ExperienceID: "document",
				Site:         "shop",
				SessionID:    "browser",
				PageGroup:    "/products",
				PageHost:     "shop.example.com",
				Browser:      "Chrome",
				Country:      "GR",
				Device:       "desktop",
				Vitals: []beacon.Vital{
					{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 5000, Element: "img.hero"},
					{ID: "cls", Revision: 1, Name: beacon.CLS, Value: .5},
				},
				Navigation: &beacon.Navigation{
					Revision: 1,
					LoadMS:   900,
					HasLoad:  true,
					DCLMS:    20,
					HasDCL:   true,
				},
				Resources: []beacon.Resource{
					{ID: "resource", Host: "cdn.example.com", HasDuration: true, DurationMS: 30, Initiator: "fetch"},
				},
				Events: []beacon.Event{
					{Kind: beacon.EventDocument, ID: "activation", Revision: 1},
					{Kind: beacon.EventSession, ID: "browser", Revision: 1},
					{Kind: beacon.EventNavigation, ID: "navigation", Revision: 1},
					{Kind: beacon.EventResource, ID: "resource", Revision: 1},
					{
						Kind:  beacon.EventCustom,
						Name:  beacon.RageClickEvent,
						Attrs: map[string]string{"target": "button#buy"},
					},
					{
						Kind:    beacon.EventRequest,
						TraceID: "0123456789abcdef0123456789abcdef",
						Attrs: map[string]string{
							"url.full":            "https://api.example.com/cart",
							"http.request.method": "POST",
						},
					},
					{Kind: beacon.EventCustom, Name: "add_to_cart"},
				},
				Errors: []beacon.Error{{Type: "TypeError", Message: "checkout failed", Fingerprint: "6c0f3e2a91d4"}},
			}
			benchIngest(b, a, now, advancingClock, func(i int) {
				revision, id := uint64(i+1), i%benchIdentityReports
				if newDocument {
					input.ExperienceID = documents[id]
				}
				input.Received = *now
				input.Navigation.Revision, input.Events[2].Revision = revision, revision
				input.Vitals[0].Revision, input.Vitals[1].Revision = revision, revision
				input.Resources[0].ID, input.Events[3].ID = resources[id], resources[id]
				a.Ingest(input)
			})
			b.StopTimer()
			reports := uint64(benchWarmupReports + b.N)
			st := &a.site
			if got := st.firstPartyResources + st.thirdPartyResources + st.unknownResources; got != reports {
				b.Fatalf("admitted resources: got %d, want %d", got, reports)
			}
			if got := st.counters[CounterPageviews]; newDocument && got != reports {
				b.Fatalf("admitted activations: got %d, want %d", got, reports)
			}
		})
	}
}

// Error ingestion adds one immutable metadata object per error to retain detail
// for later session promotion. Work stays bounded by the beacon and live ring;
// it does not traverse persisted history. Receipts follow an advancing clock, so
// error activity expires with the window. Timings are
// local trends, not CI gates.
func BenchmarkIngestErrors(b *testing.B) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		VnodeGUID: "11111111-1111-4111-8111-111111111111",
		Investigate: InvestigateCfg{
			Rate: 1,
		},
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	a.SetHistorySink(discardHistory{})
	input := &beacon.Beacon{
		Site:      "shop",
		SessionID: "browser",
		PageGroup: "/checkout",
		Browser:   "Chrome",
		Errors: []beacon.Error{
			{
				Type:        "TypeError",
				Message:     "checkout failed",
				Stack:       "at checkout()",
				Fingerprint: "6c0f3e2a91d4",
			},
		},
	}
	benchIngest(b, a, now, advancingClock, func(int) {
		input.Received = *now
		a.Ingest(input)
	})
}

// A report without a session has no timeline: an investigated error goes
// straight to history without retaining session detail. Receipts advance as in
// BenchmarkIngestErrors.
func BenchmarkIngestSessionlessErrors(b *testing.B) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		VnodeGUID: "11111111-1111-4111-8111-111111111111",
		Investigate: InvestigateCfg{
			Rate: 1,
		},
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	a.SetHistorySink(discardHistory{})
	input := &beacon.Beacon{
		Site:      "shop",
		PageGroup: "/checkout",
		Browser:   "Chrome",
		Errors: []beacon.Error{
			{
				Type:        "TypeError",
				Message:     "checkout failed",
				Stack:       "at checkout()",
				Fingerprint: "6c0f3e2a91d4",
			},
		},
	}
	benchIngest(b, a, now, advancingClock, func(int) {
		input.Received = *now
		a.Ingest(input)
	})
}

// Replayed SDK batches must not allocate work proportional to retained state.
func BenchmarkIngestDuplicate(b *testing.B) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		VnodeGUID:  "11111111-1111-4111-8111-111111111111",
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	input := &beacon.Beacon{
		Site:         "shop",
		ExperienceID: "document",
		SessionID:    "browser",
		Received:     *now,
		Vitals:       []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 100}},
	}
	a.Ingest(input)
	benchIngest(b, a, now, fixedClock, func(int) { a.Ingest(input) })
}

// A retained population changes memory use, not the per-report traversal cost.
func BenchmarkIngestDuplicateRetained(b *testing.B) {
	for _, size := range []int{1, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				VnodeGUID:  "11111111-1111-4111-8111-111111111111",
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			input := &beacon.Beacon{
				Site:     "shop",
				Received: *now,
				Vitals:   []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 100}},
			}
			for i := range size {
				input.ExperienceID = strconv.Itoa(i)
				a.Ingest(input)
			}
			benchIngest(b, a, now, fixedClock, func(int) { a.Ingest(input) })
		})
	}
}

// Cyclic updates exercise receipt-order maintenance at a full retained population.
// Receipt updates require O(log retained) heap work; duplicate checks remain O(1).
// The clock and receipts advance a microsecond per report, so every retained
// item is renewed long before the window could expire it.
func BenchmarkRetainedUpdate(b *testing.B) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		VnodeGUID:  "11111111-1111-4111-8111-111111111111",
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	inputs := make([]beacon.Beacon, 10000)
	for i := range inputs {
		inputs[i] = beacon.Beacon{
			Site:         "shop",
			ExperienceID: strconv.Itoa(i),
			Received:     *now,
			Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 100}},
		}
		a.Ingest(&inputs[i])
	}
	benchIngest(b, a, now, fixedClock, func(i int) {
		*now = now.Add(time.Microsecond)
		v := &inputs[i%len(inputs)]
		v.Received = *now
		v.Vitals[0].Revision++
		a.Ingest(v)
	})
}

// Site-list reads need counts, not facet materialization. Allocation growth
// with retained observations would make polling compete with ingestion.
func BenchmarkActivity(b *testing.B) {
	for _, size := range []int{100, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				VnodeGUID:  "11111111-1111-4111-8111-111111111111",
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			for i := 0; i < size; i++ {
				a.Ingest(
					&beacon.Beacon{
						Site:         "shop",
						ExperienceID: strconv.Itoa(i),
						SessionID:    strconv.Itoa(i),
						PageGroup:    "/" + strconv.Itoa(i),
						Received:     *now,
						Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 100}},
						Events: []beacon.Event{
							{Kind: beacon.EventDocument, ID: "activation", Revision: 1},
						},
					},
				)
			}
			if got := a.Activity().PageviewsWindow; got != size {
				b.Fatalf("retained document activity: got %d, want %d", got, size)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a.Activity()
			}
		})
	}
}

// A rum-live poll reads the rows after its cursor from a full ring, capped at
// the Function's row limit: "first-poll" returns a full batch, "tail" the few
// rows a steady poll sees, "caught-up" none. Timings are local trends;
// allocations and bytes define the per-read envelope.
func BenchmarkLive(b *testing.B) {
	const functionRows = 2000 // the rum-live Function's row cap
	a, now := newAgg(5 * time.Minute)
	for i := range 2 * liveRingCap {
		a.Ingest(&beacon.Beacon{
			ExperienceID: strconv.Itoa(i),
			Site:         "s",
			PageGroup:    "/products",
			Received:     *now,
			Vitals:       []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 100}},
		})
	}
	if got := len(a.live) - a.liveStart; got != liveRingCap {
		b.Fatalf("live ring holds %d rows, want %d", got, liveRingCap)
	}
	head := a.liveSeq
	for _, poll := range []struct {
		name  string
		after uint64
	}{{"first-poll", 0}, {"tail", head - 10}, {"caught-up", head}} {
		b.Run(poll.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				a.Live(poll.after, functionRows, "")
			}
		})
	}
}

func BenchmarkSnapshot(b *testing.B) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		VnodeGUID: "11111111-1111-4111-8111-111111111111",
		Investigate: InvestigateCfg{
			Rate: 1,
		},
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	for i := range 100 {
		a.Ingest(
			&beacon.Beacon{
				ExperienceID: strconv.Itoa(i),
				Site:         "shop",
				SessionID:    "browser",
				Path:         "/products",
				PageGroup:    "/products",
				Browser:      "Chrome",
				Device:       "desktop",
				Country:      "GR",
				Received:     *now,
				Vitals:       []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: float64(i + 1)}},
			},
		)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Snapshot()
	}
}

// Sparse activity and attributed measurements exercise the per-read group
// allocation envelope independently of the dense-window benchmark. Capture is
// O(retained observations + facet memberships); sorting is per distribution.
// Timings are local trends, not CI gates. No cross-read state is retained.
func BenchmarkSnapshotPopulations(b *testing.B) {
	for _, shape := range []string{"empty", "activity", "sparse", "attributed"} {
		b.Run(shape, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				Name:       "s",
				PageGroups: 20,
				Countries:  20,
			})
			if shape != "empty" {
				for i := range 10000 {
					input := mk(*now, fmt.Sprint(i), "/page")
					input.PageGroup = fmt.Sprintf("/page_%d", i%1000)
					input.Browser = fmt.Sprintf("browser_%d", i%7)
					input.Country = fmt.Sprintf("country_%d", i%31)
					input.AppVersion = fmt.Sprintf("version_%d", i%37)
					if shape == "attributed" || shape == "sparse" && i%10 == 0 {
						for j, name := range beacon.Vitals {
							if shape == "sparse" && j != 0 {
								break
							}
							value := float64((i*37 + j*23) % 10000)
							if name == beacon.CLS {
								value /= 10000
							}
							metric := v(name, value)
							metric.Element = fmt.Sprintf("#element_%d", i%11)
							input.Vitals = append(input.Vitals, metric)
						}
					}
					a.Ingest(input)
				}
			}
			for _, kind := range []string{"snapshot", "pages"} {
				b.Run(kind, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if kind == "snapshot" {
							a.Snapshot()
						} else {
							a.Pages("")
						}
					}
				})
			}
		})
	}
}

// Derived groups follow retained observations. This fixture measures 10,000
// distinct page/version groups independently of the chart top-N.
func BenchmarkHighCardinalityReads(b *testing.B) {
	for _, kind := range []string{"snapshot", "pages"} {
		b.Run(kind, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				VnodeGUID:  "11111111-1111-4111-8111-111111111111",
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			for i := 0; i < 10000; i++ {
				id := strconv.Itoa(i)
				a.Ingest(
					&beacon.Beacon{
						Site:         "shop",
						ExperienceID: id,
						SessionID:    id,
						PageGroup:    "/" + id,
						AppVersion:   id,
						Received:     *now,
						Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 100}},
					},
				)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if kind == "snapshot" {
					a.Snapshot()
				} else {
					a.Pages("")
				}
			}
		})
	}
}

// A hot site at 500 new documents/s retains 150,000 documents in five minutes.
// Three vitals and a resource per document expose reduction/sorting costs even
// with only 20 pages and three versions. Timings are local trends, not an SLO.
func BenchmarkDenseWindowReads(b *testing.B) {
	for _, kind := range []string{"snapshot", "pages"} {
		b.Run(kind, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			for i := range 150000 {
				a.Ingest(&beacon.Beacon{
					Site:         "shop",
					ExperienceID: strconv.Itoa(i),
					SessionID:    strconv.Itoa(i),
					PageGroup:    "/page_" + strconv.Itoa(i%20),
					PageHost:     "example.org",
					AppVersion:   strconv.Itoa(i % 3),
					Browser:      "Chrome",
					Device:       "desktop",
					Country:      "GR",
					Received:     *now,
					Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "activation", Revision: 1}},
					Vitals: []beacon.Vital{
						{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: float64((i * 37) % 10000)},
						{ID: "inp", Revision: 1, Name: beacon.INP, Value: float64((i * 23) % 1000)},
						{ID: "cls", Revision: 1, Name: beacon.CLS, Value: float64(i%100) / 100},
					},
					Resources: []beacon.Resource{
						{
							ID:          "resource",
							Host:        "example.org",
							Initiator:   "fetch",
							HasDuration: true,
							DurationMS:  float64(i % 1000),
						},
					},
				})
			}
			if got := a.Activity().PageviewsWindow; got != 150000 {
				b.Fatalf("retained document activity: got %d, want 150000", got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if kind == "snapshot" {
					a.Snapshot()
				} else {
					a.Pages("")
				}
			}
		})
	}
}
