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
// Ingestion is O(batch items), independent of the retained population; timings
// are local trends, while allocation counts define the cost envelope.
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
