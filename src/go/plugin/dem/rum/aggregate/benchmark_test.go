// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type discardHistory struct{}

func (discardHistory) Event(aggregate.HistoryEvent) {}

// Fixed experiences with monotonically newer reports measure real update work.
// These fixed receipt times exercise replacement without heap reordering.
// Advancing receipts cost O(log retained) per item; timings are local trends,
// while allocation counts define the cost envelope.
func BenchmarkIngest(b *testing.B) {
	for _, history := range []bool{false, true} {
		name := "measure"
		if history {
			name = "investigate"
		}
		b.Run(name, func(b *testing.B) {
			a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
				Investigate: aggregate.InvestigateCfg{
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
				Received:     time.Now(),
				Vitals: []beacon.Vital{
					{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 1200},
					{ID: "cls", Revision: 1, Name: beacon.CLS, Value: .1},
				},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				input.Vitals[0].Revision = uint64(i + 1)
				input.Vitals[1].Revision = uint64(i + 1)
				a.Ingest(input)
			}
		})
	}
}
func BenchmarkSnapshot(b *testing.B) {
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Investigate: aggregate.InvestigateCfg{
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
				Received:     time.Now(),
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

// Error ingestion adds one immutable metadata object per error to retain detail
// for later session promotion. Work stays bounded by the beacon and live ring;
// it does not traverse persisted history. Timings are local trends, not CI gates.
func BenchmarkIngestErrors(b *testing.B) {
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Investigate: aggregate.InvestigateCfg{
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
		Received:  time.Now(),
		Errors: []beacon.Error{
			{Type: "TypeError", Message: "checkout failed", Stack: "at checkout()", Fingerprint: "checkout"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Ingest(input)
	}
}

// Replayed SDK batches must not allocate work proportional to retained state.
func BenchmarkIngestDuplicate(b *testing.B) {
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	input := &beacon.Beacon{
		Site:         "shop",
		ExperienceID: "document",
		SessionID:    "browser",
		Received:     time.Now(),
		Vitals:       []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 100}},
	}
	a.Ingest(input)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Ingest(input)
	}
}

// A retained population changes memory use, not the per-report traversal cost.
func BenchmarkIngestDuplicateRetained(b *testing.B) {
	for _, size := range []int{1, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
				Name:       "shop",
				PageGroups: 20,
				Countries:  20,
			})
			input := &beacon.Beacon{
				Site:     "shop",
				Received: time.Now(),
				Vitals:   []beacon.Vital{{ID: "lcp", Revision: 1, Name: beacon.LCP, Value: 100}},
			}
			for i := range size {
				input.ExperienceID = strconv.Itoa(i)
				a.Ingest(input)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				a.Ingest(input)
			}
		})
	}
}

// Site-list reads need counts, not facet materialization. Allocation growth
// with retained observations would make polling compete with ingestion.
func BenchmarkActivity(b *testing.B) {
	for _, size := range []int{100, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
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
						Received:     time.Now(),
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

// Cyclic updates exercise receipt-order maintenance at a full retained population.
// Receipt updates require O(log retained) heap work; duplicate checks remain O(1).
func BenchmarkRetainedUpdate(b *testing.B) {
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Name:       "shop",
		PageGroups: 20,
		Countries:  20,
	})
	inputs := make([]beacon.Beacon, 10000)
	for i := range inputs {
		inputs[i] = beacon.Beacon{
			Site:         "shop",
			ExperienceID: strconv.Itoa(i),
			Received:     time.Now(),
			Vitals:       []beacon.Vital{{Name: beacon.LCP, ID: "lcp", Revision: 1, Value: 100}},
		}
		a.Ingest(&inputs[i])
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := &inputs[i%len(inputs)]
		v.Received = time.Now()
		v.Vitals[0].Revision++
		a.Ingest(v)
	}
}

// All derived groups are bounded by canonical observations, not a second
// lossy grouping cap. Measure the upper retained cardinality on read paths.
func BenchmarkHighCardinalityReads(b *testing.B) {
	for _, kind := range []string{"snapshot", "pages"} {
		b.Run(kind, func(b *testing.B) {
			a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
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
						Received:     time.Now(),
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
					a.Pages()
				}
			}
		})
	}
}
